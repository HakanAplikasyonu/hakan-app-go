package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// CodeNotInPlan is the warning the gate raises (docs/02 §2).
const CodeNotInPlan = "ACCOUNT_NOT_IN_TARGET_PLAN"

// Producer is the warning producer name of the gate.
const Producer = "control:target_plan"

const maxListedCodes = 50

// Gate is the pre-export check: the accounting export (docs/02 §2) calls
// Evaluate with the account codes of one journal entry (or one batch) before
// handing it to a driver. Whether a missing code blocks the export is the
// office / engagement setting of ACCOUNT_NOT_IN_TARGET_PLAN
// (warning_rule_configs): severity blocker (default) blocks, warning only
// warns, disabled does neither.
type Gate struct {
	svc    *Service
	engine *engine.Engine
	config engine.ConfigResolver
}

// NewGate builds the gate.
func NewGate(svc *Service, eng *engine.Engine, cfg engine.ConfigResolver) *Gate {
	return &Gate{svc: svc, engine: eng, config: cfg}
}

// Verdict is the gate's answer.
type Verdict struct {
	CheckResult
	// Enabled / Severity are the effective setting of the warning code.
	Enabled  bool             `json:"enabled"`
	Severity catalog.Severity `json:"severity"`
	// Blocked: the entry must not go to the export.
	Blocked bool `json:"blocked"`
}

// Evaluate checks codes against the engagement's active plan for target and
// records the result as a warning on ent (raised, refreshed or
// auto-resolved). With no plan loaded nothing is reported or blocked.
func (g *Gate) Evaluate(ctx context.Context, s engine.Subject, ent engine.Entity, tgt string, codes []string) (Verdict, error) {
	var v Verdict
	res, err := g.svc.Check(ctx, Subject{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID}, tgt, codes)
	if err != nil {
		return v, err
	}
	v.CheckResult = res
	cfg, err := g.config.Resolve(ctx, s, []string{CodeNotInPlan})
	if err != nil {
		return v, fmt.Errorf("resolve warning config: %w", err)
	}
	eff := cfg[CodeNotInPlan]
	v.Enabled, v.Severity = eff.Enabled, eff.Severity

	var findings []engine.Finding
	// A group account (one with sub-accounts) cannot take a posting in Luca
	// ("ana hesaba fiş kesilemez"): it is reported like a missing code (#319).
	bad := len(res.Missing) + len(res.Invalid) + len(res.NonLeaf)
	if res.PlanLoaded && bad > 0 {
		params := map[string]any{
			"missing":       head(res.Missing),
			"invalid":       head(res.Invalid),
			"non_leaf":      head(res.NonLeaf),
			"missing_count": bad,
			"target":        orDefault(tgt),
		}
		if res.Import != nil {
			params["plan_import"] = res.Import.UUID.String()
		}
		findings = append(findings, engine.Finding{Code: CodeNotInPlan, Params: params})
		v.Blocked = eff.Enabled && eff.Severity == catalog.SeverityBlocker
	}
	if _, err := g.engine.Apply(ctx, s, Producer, ent, []string{CodeNotInPlan}, findings); err != nil {
		return v, err
	}
	return v, nil
}

// ExportGate adapts the gate to journals.PlanGate (#319): before a batch
// goes to a driver, its accounts are checked against the engagement's active
// plan of the driver's target program. Codes are normalized first
// ("120 01 006" → "120.01.006", and the line is rewritten to the plan's form
// when that is found); missing, invalid and group (non-leaf) accounts become
// ACCOUNT_NOT_IN_TARGET_PLAN issues, one per line, and the result is recorded
// as a warning on each period of the batch. An issue blocks the export when
// the export setting blocks unknown accounts (block) and the warning code's
// setting is an enabled blocker; a disabled code reports nothing. With no
// plan loaded nothing is checked.
//
//	journals.New(pool, store).WithPlanGate(accountplan.NewGate(...).ExportGate())
func (g *Gate) ExportGate() func(ctx context.Context, scope engagementctx.Scope, driver string, entries []accountingexport.Entry, block bool) ([]accountingexport.Issue, error) {
	return func(ctx context.Context, scope engagementctx.Scope, driver string, entries []accountingexport.Entry, block bool) ([]accountingexport.Issue, error) {
		if !targetRe.MatchString(driver) {
			return nil, nil
		}
		byPeriod := map[string][]string{}
		for _, e := range entries {
			p := e.Date.Format("2006-01")
			for _, l := range e.Lines {
				byPeriod[p] = append(byPeriod[p], l.Account)
			}
		}
		periods := make([]string, 0, len(byPeriod))
		for p := range byPeriod {
			periods = append(periods, p)
		}
		sort.Strings(periods)
		subj := engine.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
		missing, nonLeaf := map[string]bool{}, map[string]bool{}
		var verdict Verdict
		for _, p := range periods {
			v, err := g.Evaluate(ctx, subj, engine.Entity{Type: catalog.EntityPeriod, Period: p}, driver, byPeriod[p])
			if err != nil {
				return nil, err
			}
			if !v.PlanLoaded {
				return nil, nil
			}
			verdict = v
			for _, c := range append(v.Missing, v.Invalid...) {
				missing[c] = true
			}
			for _, c := range v.NonLeaf {
				nonLeaf[c] = true
			}
		}
		if !verdict.Enabled {
			return nil, nil
		}
		sev := accountingexport.SeverityWarning
		if block && verdict.Blocked {
			sev = accountingexport.SeverityBlocker
		}
		var issues []accountingexport.Issue
		for _, e := range entries {
			for i := range e.Lines {
				raw := e.Lines[i].Account
				n := NormalizeCode(raw)
				switch {
				case missing[raw] || missing[n]:
					issues = append(issues, accountingexport.Issue{Code: CodeNotInPlan, Severity: sev, EntryNo: e.No, Line: i + 1,
						Params: map[string]any{"account": raw}})
				case nonLeaf[n]:
					issues = append(issues, accountingexport.Issue{Code: CodeNotInPlan, Severity: sev, EntryNo: e.No, Line: i + 1,
						Params: map[string]any{"account": raw, "reason": "group_account"}})
				case n != raw:
					// The plan has the normalized form: write that one.
					e.Lines[i].Account = n
				}
			}
		}
		return issues, nil
	}
}

func head(s []string) []string {
	if len(s) > maxListedCodes {
		return s[:maxListedCodes]
	}
	return s
}

func orDefault(t string) string {
	if t == "" {
		return DefaultTarget
	}
	return t
}
