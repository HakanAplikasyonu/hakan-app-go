package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Suggestion kind and fields used in the review queue.
const (
	Kind            = "account_code"
	FieldAccount    = "account_code"
	FieldVATAccount = "vat_account_code"
	TargetItem      = "invoice_item"
	TargetCustomer  = "customer"
	// MinCandidateDecisions: "aynı düzeltme 3 kez → kural yapılsın mı?".
	MinCandidateDecisions = 3
)

// Submitter is the review queue (#14).
type Submitter interface {
	Submit(ctx context.Context, sub reviewusecase.Subject, in reviewusecase.Input) (reviewusecase.Suggestion, error)
}

// Service runs rules and suggestions.
type Service struct {
	store  Store
	review Submitter
}

// New builds the service; review may be nil when only rules are managed.
func New(store Store, review Submitter) *Service { return &Service{store: store, review: review} }

var (
	confManual    = decimal.RequireFromString("0.9600")
	confCandidate = decimal.RequireFromString("0.9600")
	confHistory1  = decimal.RequireFromString("0.7000")
	confHistoryN  = decimal.RequireFromString("0.8500")
	confCariNew   = decimal.RequireFromString("0.7000")
	confCariFound = decimal.RequireFromString("0.9000")
	// The accountant confirmed this VKN → cari before (#319): applied
	// without asking (above the review queue's auto-accept threshold).
	confCariMemory = decimal.RequireFromString("0.9700")
	one            = decimal.NewFromInt(1)
)

func refFor(id int64, version int) string { return fmt.Sprintf("account_rule:%d@v%d", id, version) }

func validateRule(in *RuleInput, origin string) error {
	bad := func(m string) error { return fmt.Errorf("%w: %s", ErrInvalid, m) }
	if in.Role == "" {
		in.Role = engine.RoleLine
	}
	in.Name = strings.TrimSpace(in.Name)
	in.AccountCode = strings.TrimSpace(in.AccountCode)
	switch {
	case !in.Role.Valid():
		return bad("role must be line, vat or counterparty")
	case !engine.ValidCode(in.AccountCode):
		return bad("account_code must be a dotted account code (e.g. 770.01)")
	case len(in.Name) > 200:
		return bad("name is too long")
	}
	if in.Priority == 0 {
		in.Priority = 100
	}
	if in.Priority < 1 || in.Priority > 999 {
		return bad("priority must be 1..999 (1000 is the platform defaults)")
	}
	if in.Confidence == nil {
		c := confManual
		if origin == engine.OriginCandidate {
			c = confCandidate
		}
		in.Confidence = &c
	}
	if in.Confidence.IsNegative() || in.Confidence.GreaterThan(one) {
		return bad("confidence must be between 0 and 1")
	}
	r := in.Confidence.Round(4)
	in.Confidence = &r
	if err := in.Conditions.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

// Rules lists one level's rules (eng 0 = office template).
func (s *Service) Rules(ctx context.Context, org, eng int64, includeRetired bool) ([]Rule, error) {
	return s.store.ListRules(ctx, org, eng, includeRetired)
}

// Versions lists every version of a rule, newest first.
func (s *Service) Versions(ctx context.Context, org, eng int64, key uuid.UUID) ([]Rule, error) {
	v, err := s.store.Versions(ctx, org, eng, key)
	if err == nil && len(v) == 0 {
		return nil, ErrNotFound
	}
	return v, err
}

// CreateRule adds a rule at one level.
func (s *Service) CreateRule(ctx context.Context, org, eng int64, in RuleInput, userID int64) (Rule, error) {
	if err := validateRule(&in, engine.OriginManual); err != nil {
		return Rule{}, err
	}
	return s.store.CreateRule(ctx, org, eng, in, engine.OriginManual, userID)
}

// UpdateRule stores a new version; the previous version is kept unchanged
// (retired) and so are the suggestions and assignments that reference it.
func (s *Service) UpdateRule(ctx context.Context, org, eng int64, key uuid.UUID, in RuleInput, userID int64) (Rule, error) {
	if err := validateRule(&in, engine.OriginManual); err != nil {
		return Rule{}, err
	}
	return s.store.NewVersion(ctx, org, eng, key, in, userID)
}

// RetireRule deactivates a rule.
func (s *Service) RetireRule(ctx context.Context, org, eng int64, key uuid.UUID) error {
	return s.store.Retire(ctx, org, eng, key)
}

// ApplyTemplate copies the office template rules the engagement does not
// have yet (new engagements get them on creation).
func (s *Service) ApplyTemplate(ctx context.Context, org, eng int64) (int, error) {
	return s.store.CopyTemplate(ctx, org, eng)
}

// SuggestionView is one submitted suggestion.
type SuggestionView struct {
	UUID        uuid.UUID       `json:"uuid"`
	TargetType  string          `json:"target_type"`
	TargetField string          `json:"target_field"`
	LineNo      int             `json:"line_no,omitempty"`
	AccountCode string          `json:"account_code"`
	Confidence  decimal.Decimal `json:"confidence"`
	Status      string          `json:"status"`
	Rationale   string          `json:"rationale"`
	RuleRef     string          `json:"rule_ref"`
	// Counterparty (cari) suggestions only (#310): the proposed account must
	// be opened in the accounting program (not in the imported plan yet), and
	// the name it is matched or opened under.
	OpenNew     *bool  `json:"open_new,omitempty"`
	AccountName string `json:"account_name,omitempty"`
}

// SuggestInvoice proposes account codes for the invoice's lines (line and
// VAT account) and its counterparty, and submits them to the review queue.
// Fields that already carry an applied code are skipped.
func (s *Service) SuggestInvoice(ctx context.Context, sub Subject, invoiceUUID uuid.UUID) ([]SuggestionView, error) {
	if s.review == nil {
		return nil, fmt.Errorf("accountrules: review queue not configured")
	}
	inv, err := s.store.Invoice(ctx, sub.TaxpayerID, invoiceUUID)
	if err != nil {
		return nil, err
	}
	rules, err := s.activeRules(ctx, sub)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(inv.Lines))
	for _, l := range inv.Lines {
		ids = append(ids, l.ID)
	}
	done, err := s.store.Assigned(ctx, sub, TargetItem, ids)
	if err != nil {
		return nil, err
	}
	cpKey := counterpartyKey(inv)
	var history []HistoryRow
	if inv.TaxNumber != "" || inv.IdentityNumber != "" {
		if history, err = s.store.History(ctx, sub, FieldAccount, inv.Direction, inv.TaxNumber, inv.IdentityNumber, inv.ID); err != nil {
			return nil, err
		}
	}
	var out []SuggestionView
	plans := map[string][]engine.PlanAccount{}
	for _, l := range inv.Lines {
		x := engine.Context{
			Direction: inv.Direction, InvoiceType: inv.Type, CounterpartyTaxID: firstNonEmpty(inv.TaxNumber, inv.IdentityNumber),
			CounterpartyName: inv.CustomerName, ItemName: l.Name, LineType: l.LineType, TaxRate: l.TaxRate,
		}
		if done[l.ID][FieldAccount] == "" {
			a := lineAssignment(rules, x, history)
			v, err := s.submit(ctx, sub, inv, &l, TargetItem, l.ID, FieldAccount, a, cpKey, nil)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if l.TaxRate.IsPositive() && done[l.ID][FieldVATAccount] == "" {
			if a, ok := engine.Pick(rules, engine.RoleVAT, x); ok {
				if a, err = s.vatSubAccount(ctx, sub, plans, a, l.TaxRate); err != nil {
					return nil, err
				}
				v, err := s.submit(ctx, sub, inv, &l, TargetItem, l.ID, FieldVATAccount, a, cpKey, nil)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		}
	}
	if inv.CustomerID != nil {
		cdone, err := s.store.Assigned(ctx, sub, TargetCustomer, []int64{*inv.CustomerID})
		if err != nil {
			return nil, err
		}
		if cdone[*inv.CustomerID][FieldAccount] == "" {
			a, extra, err := s.counterpartyAssignment(ctx, sub, rules, inv)
			switch {
			case errors.Is(err, errNoCounterpartyMain):
				// No cari suggestion for this invoice.
			case err != nil:
				return nil, err
			default:
				v, err := s.submit(ctx, sub, inv, nil, TargetCustomer, *inv.CustomerID, FieldAccount, a, cpKey, extra)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		}
	}
	if out == nil {
		out = []SuggestionView{}
	}
	return out, nil
}

// AutoResult is what AutoSuggest did.
type AutoResult struct {
	Invoices    int `json:"invoices"`
	Suggestions int `json:"suggestions"`
	// Applied: suggestions above the review queue's auto-accept threshold
	// (a rule the accountant made, a remembered cari): applied without
	// entering the queue.
	Applied int `json:"applied"`
	// Skipped: the engagement has no plan and no rule yet.
	Skipped bool `json:"skipped"`
}

// AutoSuggest proposes account codes for newly stored invoices (#319): the
// UBL upload job calls it through the queue once the invoices are written.
// It runs only for engagements with an imported plan or an active rule;
// suggestions confident enough (a promoted rule, a remembered cari) are
// applied by the review queue right away, the rest wait there. An invoice
// that is gone (deleted, other taxpayer) is skipped. "Hesap kodu öner"
// (SuggestInvoice) stays for a manual re-run.
func (s *Service) AutoSuggest(ctx context.Context, sub Subject, invoices []uuid.UUID) (AutoResult, error) {
	var res AutoResult
	ok, err := s.store.HasMemory(ctx, sub)
	if err != nil {
		return res, err
	}
	if !ok {
		res.Skipped = true
		return res, nil
	}
	for _, id := range invoices {
		views, err := s.SuggestInvoice(ctx, sub, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return res, fmt.Errorf("suggest invoice %s: %w", id, err)
		}
		res.Invoices++
		res.Suggestions += len(views)
		for _, v := range views {
			if v.Status == string(reviewusecase.StatusAutoAccepted) {
				res.Applied++
			}
		}
	}
	return res, nil
}

// lineAssignment: engagement rule > counterparty history > platform default.
func lineAssignment(rules []engine.Rule, x engine.Context, history []HistoryRow) engine.Assignment {
	if a, ok := engine.Pick(rules, engine.RoleLine, x); ok && a.RuleID != 0 {
		return a
	}
	if len(history) > 0 {
		h := history[0]
		conf := confHistory1
		if h.Uses >= 2 {
			conf = confHistoryN
		}
		return engine.Assignment{
			Role: engine.RoleLine, AccountCode: h.AccountCode, Confidence: conf, RuleRef: "history:counterparty",
			Rationale: fmt.Sprintf("Bu karşı tarafın önceki %d satırı %s hesabına işlendi (son dönem %s)", h.Uses, h.AccountCode, h.LastPeriod),
		}
	}
	a, _ := engine.Pick(rules, engine.RoleLine, x)
	return a
}

// vatSubAccount moves a VAT account to the plan's sub-account for the
// line's rate (#319): "191" → "191.01 İNDİRİLECEK KDV %20". With no plan,
// no sub-accounts or no unique rate match the code stays as picked.
func (s *Service) vatSubAccount(ctx context.Context, sub Subject, plans map[string][]engine.PlanAccount, a engine.Assignment, rate decimal.Decimal) (engine.Assignment, error) {
	if len(a.AccountCode) < 3 {
		return a, nil
	}
	main := a.AccountCode[:3]
	plan, ok := plans[main]
	if !ok {
		var err error
		if plan, err = s.store.PlanUnder(ctx, sub, main); err != nil {
			return a, err
		}
		plans[main] = plan
	}
	if p, ok := engine.VATSubAccount(a.AccountCode, rate, plan); ok {
		a.Rationale += fmt.Sprintf("; hesap planında %%%s oranlı alt hesap: %s %s", rate.String(), p.Code, p.Name)
		a.AccountCode = p.Code
	}
	return a, nil
}

var errNoCounterpartyMain = errors.New("accountrules: no cari main account for this invoice direction")

func (s *Service) counterpartyAssignment(ctx context.Context, sub Subject, rules []engine.Rule, inv Invoice) (engine.Assignment, map[string]any, error) {
	x := engine.Context{Direction: inv.Direction, InvoiceType: inv.Type,
		CounterpartyTaxID: firstNonEmpty(inv.TaxNumber, inv.IdentityNumber), CounterpartyName: inv.CustomerName}
	a, _ := engine.Pick(rules, engine.RoleCounterparty, x)
	// Order (#319): the engagement's own cari rule > the cari the accountant
	// confirmed for this VKN > an office template rule > the plan account
	// with the same name > a new account.
	if a.RuleID != 0 && a.Tier == 0 {
		return a, nil, nil
	}
	main := mainOf(rules, x)
	if main == "" {
		// No default for this direction (neither incoming nor outgoing):
		// proposing ".01.001" would be nonsense; leave the cari to the office.
		return a, nil, errNoCounterpartyMain
	}
	if taxID := x.CounterpartyTaxID; taxID != "" {
		code, name, ok, err := s.store.CounterpartyAccount(ctx, sub, taxID, main)
		if err != nil {
			return a, nil, err
		}
		if ok {
			return engine.Assignment{Role: engine.RoleCounterparty, AccountCode: code, Confidence: confCariMemory,
					RuleRef:   "memory:counterparty_tax_id",
					Rationale: fmt.Sprintf("Bu VKN (%s) için daha önce onaylanan cari hesap: %s %s", taxID, code, name)},
				map[string]any{"open_new": false, "account_name": name}, nil
		}
	}
	if a.RuleID != 0 {
		return a, nil, nil
	}
	plan, err := s.store.PlanUnder(ctx, sub, main)
	if err != nil {
		return a, nil, err
	}
	p := engine.ProposeCounterparty(main, inv.CustomerName, plan)
	extra := map[string]any{"open_new": !p.Existing, "account_name": p.Name}
	if p.Existing {
		return engine.Assignment{Role: engine.RoleCounterparty, AccountCode: p.AccountCode, Confidence: confCariFound,
			RuleRef:   "global:counterparty_plan_match",
			Rationale: fmt.Sprintf("Hesap planında %q adlı cari hesap var: %s", p.Name, p.AccountCode)}, extra, nil
	}
	why := fmt.Sprintf("Cari hesap açma önerisi: %q için %s altında %s", inv.CustomerName, main, p.AccountCode)
	if len(plan) == 0 {
		why += " (hedef hesap planı yüklenmemiş ya da " + main + " altında hesap yok)"
	}
	return engine.Assignment{Role: engine.RoleCounterparty, AccountCode: p.AccountCode, Confidence: confCariNew,
		RuleRef: "global:counterparty_open", Rationale: why}, extra, nil
}

// mainOf is the cari main account (120 / 320) of the platform default for
// the invoice direction.
func mainOf(rules []engine.Rule, x engine.Context) string {
	var defaults []engine.Rule
	for _, r := range rules {
		if r.ID == 0 {
			defaults = append(defaults, r)
		}
	}
	a, _ := engine.Pick(defaults, engine.RoleCounterparty, x)
	if len(a.AccountCode) > 3 {
		return a.AccountCode[:3]
	}
	return a.AccountCode
}

func (s *Service) submit(ctx context.Context, sub Subject, inv Invoice, l *InvoiceLine, targetType string, targetID int64,
	field string, a engine.Assignment, cpKey string, extra map[string]any) (SuggestionView, error) {
	value := map[string]any{"account_code": a.AccountCode}
	for k, v := range extra {
		value[k] = v
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return SuggestionView{}, err
	}
	inputs := map[string]any{
		"invoice_uuid": inv.UUID.String(), "direction": inv.Direction, "invoice_type": inv.Type,
		"counterparty_key": cpKey, "counterparty_name": inv.CustomerName, "matched": a.Matched,
	}
	if t := firstNonEmpty(inv.TaxNumber, inv.IdentityNumber); t != "" {
		inputs["counterparty_tax_id"] = t
	}
	if inv.Number != "" {
		inputs["invoice_number"] = inv.Number
	}
	if inv.IssueDate != "" {
		inputs["issue_date"] = inv.IssueDate
	}
	if !inv.Total.IsZero() {
		inputs["invoice_total"] = inv.Total.StringFixed(2)
	}
	if a.RuleID != 0 {
		inputs["rule_id"] = a.RuleID
	}
	if l != nil {
		inputs["item_name"], inputs["line_type"], inputs["tax_rate"], inputs["line_no"] = l.Name, l.LineType, l.TaxRate.String(), l.LineNo
		if !l.Amount.IsZero() {
			inputs["line_amount"] = l.Amount.StringFixed(2)
		}
	}
	tid := targetID
	sug, err := s.review.Submit(ctx, reviewusecase.Subject{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID},
		reviewusecase.Input{
			Kind: Kind, TargetType: targetType, TargetID: &tid, TargetField: field, Period: inv.Period,
			Value: raw, Confidence: a.Confidence, Rationale: a.Rationale,
			Provenance: reviewusecase.Provenance{Origin: reviewusecase.OriginRule, RuleRef: a.RuleRef, Inputs: inputs},
		})
	if err != nil {
		return SuggestionView{}, err
	}
	v := SuggestionView{UUID: sug.UUID, TargetType: targetType, TargetField: field, AccountCode: a.AccountCode,
		Confidence: sug.Input.Confidence, Status: string(sug.Status), Rationale: a.Rationale, RuleRef: a.RuleRef}
	if l != nil {
		v.LineNo = l.LineNo
	}
	if open, ok := extra["open_new"].(bool); ok {
		v.OpenNew = &open
	}
	if name, ok := extra["account_name"].(string); ok {
		v.AccountName = name
	}
	return v, nil
}

func (s *Service) activeRules(ctx context.Context, sub Subject) ([]engine.Rule, error) {
	rows, err := s.store.ListRules(ctx, sub.OrganizationID, sub.EngagementID, false)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Rule, 0, len(rows)+len(engine.Defaults))
	for _, r := range rows {
		out = append(out, engine.Rule{ID: r.ID, Key: r.Key, Version: r.Version, Role: r.Role, Priority: r.Priority,
			Name: r.Name, Conditions: r.Conditions, AccountCode: r.AccountCode, Confidence: r.Confidence,
			Origin: r.Origin, Ref: r.Ref()})
	}
	return append(out, engine.Defaults...), nil
}

// Candidates returns repeated decisions that no active rule covers yet.
// The office's rule candidate setting decides how many decisions and which
// ones count (#333).
func (s *Service) Candidates(ctx context.Context, sub Subject) ([]Candidate, error) {
	set, err := s.store.RuleSettings(ctx, sub.OrganizationID)
	if err != nil {
		return nil, err
	}
	return s.candidates(ctx, sub, set)
}

func (s *Service) candidates(ctx context.Context, sub Subject, set RuleSettings) ([]Candidate, error) {
	cs, err := s.store.Candidates(ctx, sub, int64(set.MinCount), set.includeAccepts())
	if err != nil {
		return nil, err
	}
	rules, err := s.store.ListRules(ctx, sub.OrganizationID, sub.EngagementID, false)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, c := range cs {
		if c.CounterpartyKey == "" {
			continue
		}
		covered := false
		for _, r := range rules {
			if r.Role == engine.RoleLine && r.AccountCode == c.AccountCode && coversKey(r.Conditions, c.CounterpartyKey) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		c.Question = fmt.Sprintf("%q faturaları %d kez %s hesabına işlendi. Kural yapılsın mı?", c.CounterpartyName, c.Decisions, c.AccountCode)
		out = append(out, c)
	}
	return out, nil
}

// PromoteInput turns a candidate into a rule.
type PromoteInput struct {
	CounterpartyKey  string `json:"counterparty_key"`
	CounterpartyName string `json:"counterparty_name"`
	Direction        string `json:"direction"`
	AccountCode      string `json:"account_code"`
}

// Promote creates the engagement rule for a candidate.
func (s *Service) Promote(ctx context.Context, sub Subject, in PromoteInput, userID int64) (Rule, error) {
	cond := engine.Conditions{Direction: in.Direction}
	switch {
	case strings.HasPrefix(in.CounterpartyKey, "name:"):
		cond.CounterpartyNameContains = strings.TrimPrefix(in.CounterpartyKey, "name:")
	case in.CounterpartyKey != "":
		cond.CounterpartyTaxID = in.CounterpartyKey
	default:
		return Rule{}, fmt.Errorf("%w: counterparty_key is required", ErrInvalid)
	}
	name := strings.TrimSpace(in.CounterpartyName)
	if name == "" {
		name = in.CounterpartyKey
	}
	r := RuleInput{Role: engine.RoleLine, Name: name + " → " + in.AccountCode, Conditions: cond, AccountCode: in.AccountCode}
	if err := validateRule(&r, engine.OriginCandidate); err != nil {
		return Rule{}, err
	}
	return s.store.CreateRule(ctx, sub.OrganizationID, sub.EngagementID, r, engine.OriginCandidate, userID)
}

func coversKey(c engine.Conditions, key string) bool {
	if strings.HasPrefix(key, "name:") {
		return c.CounterpartyNameContains != "" && engine.Fold(c.CounterpartyNameContains) == strings.TrimPrefix(key, "name:")
	}
	return c.CounterpartyTaxID == key
}

func counterpartyKey(inv Invoice) string {
	if k := firstNonEmpty(inv.TaxNumber, inv.IdentityNumber); k != "" {
		return k
	}
	if n := engine.Fold(inv.CustomerName); n != "" {
		return "name:" + n
	}
	return ""
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
