package usecase

import (
	"context"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
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
	bad := len(res.Missing) + len(res.Invalid)
	if res.PlanLoaded && bad > 0 {
		params := map[string]any{
			"missing":       head(res.Missing),
			"invalid":       head(res.Invalid),
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
