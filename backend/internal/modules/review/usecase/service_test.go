package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// fieldApplier keeps target values in a map, like an account-code column.
type fieldApplier struct {
	values map[int64]string
	fail   bool
}

func (f *fieldApplier) Apply(_ context.Context, _ Tx, s Suggestion, v json.RawMessage) (json.RawMessage, error) {
	if f.fail {
		return nil, errors.New("boom")
	}
	prev, _ := json.Marshal(f.values[*s.Input.TargetID])
	var str string
	_ = json.Unmarshal(v, &str)
	f.values[*s.Input.TargetID] = str
	return prev, nil
}

func (f *fieldApplier) Revert(_ context.Context, _ Tx, s Suggestion, prev json.RawMessage) error {
	var str string
	_ = json.Unmarshal(prev, &str)
	f.values[*s.Input.TargetID] = str
	return nil
}

var sub = Subject{OrganizationID: 1, EngagementID: 2, TaxpayerID: 3}

func setup() (*Service, *MemoryStore, *fieldApplier) {
	st := NewMemoryStore()
	ap := &fieldApplier{values: map[int64]string{10: "770.01"}}
	return New(st).Register("account_code", ap), st, ap
}

func input(conf string, value string) Input {
	id := int64(10)
	return Input{
		Kind: "account_code", TargetType: "invoice_item", TargetID: &id, TargetField: "account_code",
		Period: "2026-10", Value: json.RawMessage(`"` + value + `"`),
		Confidence: decimal.RequireFromString(conf), Rationale: "Satıcı geçmişte 3 kez 760'a yazıldı",
		Provenance: Provenance{Origin: OriginRule, RuleRef: "office_rule:7", Inputs: map[string]any{"vendor": "X"}},
	}
}

func TestBelowThresholdWaitsInQueue(t *testing.T) {
	svc, st, ap := setup()
	ctx := context.Background()
	s, err := svc.Submit(ctx, sub, input("0.80", "760.01"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusPending || ap.values[10] != "770.01" {
		t.Fatalf("status %s value %s", s.Status, ap.values[10])
	}
	rows, _, _ := st.List(ctx, Filter{OrganizationID: 1, Status: "pending"})
	if len(rows) != 1 {
		t.Fatalf("queue = %d", len(rows))
	}
}

func TestAboveThresholdAutoAppliedAndRevertible(t *testing.T) {
	svc, st, ap := setup()
	ctx := context.Background()
	s, err := svc.Submit(ctx, sub, input("0.97", "760.01"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusAutoAccepted || ap.values[10] != "760.01" {
		t.Fatalf("status %s value %s", s.Status, ap.values[10])
	}
	_, log, _ := st.Get(ctx, sub, s.UUID)
	if len(log) != 1 || log[0].Action != ActionAutoAccept || log[0].ActorUserID != nil || log[0].Threshold == nil || log[0].Reason == "" {
		t.Fatalf("log = %+v", log)
	}
	if _, err := svc.Revert(ctx, sub, s.UUID, 5, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revert without reason: %v", err)
	}
	r, err := svc.Revert(ctx, sub, s.UUID, 5, "yanlış hesap")
	if err != nil || r.Status != StatusReverted || ap.values[10] != "770.01" {
		t.Fatalf("revert: %v %s %s", err, r.Status, ap.values[10])
	}
	_, log, _ = st.Get(ctx, sub, s.UUID)
	if len(log) != 2 || log[1].Action != ActionRevert || *log[1].ActorUserID != 5 || log[1].Reason != "yanlış hesap" {
		t.Fatalf("log = %+v", log)
	}
}

func TestThresholdFromOfficeAndEngagement(t *testing.T) {
	svc, st, _ := setup()
	ctx := context.Background()
	thr := decimal.RequireFromString("0.70")
	off := false
	_ = st.UpsertSetting(ctx, 1, 0, SettingRow{Kind: AnyKind, Threshold: &thr}, 1)
	s, _ := svc.Submit(ctx, sub, input("0.80", "760.01"))
	if s.Status != StatusAutoAccepted {
		t.Fatalf("office threshold ignored: %s", s.Status)
	}
	_ = st.UpsertSetting(ctx, 1, 2, SettingRow{Kind: "account_code", AutoAccept: &off}, 1)
	s, _ = svc.Submit(ctx, sub, input("0.99", "761.01"))
	if s.Status != StatusPending {
		t.Fatalf("engagement auto-accept off ignored: %s", s.Status)
	}
	p := ResolvePolicy([]SettingRow{{Kind: AnyKind, Threshold: &thr}, {EngagementID: 2, Kind: "account_code", AutoAccept: &off}}, 2, "account_code")
	if p.AutoAccept || !p.Threshold.Equal(thr) {
		t.Fatalf("policy = %+v", p)
	}
	if d := ResolvePolicy(nil, 0, "x"); !d.AutoAccept || !d.Threshold.Equal(DefaultThreshold) {
		t.Fatalf("default = %+v", d)
	}
}

func TestNoApplierNeverAutoAccepts(t *testing.T) {
	st := NewMemoryStore()
	svc := New(st)
	s, err := svc.Submit(context.Background(), sub, input("1", "760.01"))
	if err != nil || s.Status != StatusPending {
		t.Fatalf("%v %s", err, s.Status)
	}
	if _, err := svc.Accept(context.Background(), sub, s.UUID, 5, ""); !errors.Is(err, ErrNoApplier) {
		t.Fatalf("accept err = %v", err)
	}
}

func TestAcceptRejectCorrect(t *testing.T) {
	svc, st, ap := setup()
	ctx := context.Background()
	a, _ := svc.Submit(ctx, sub, input("0.5", "760.01"))
	if r, err := svc.Accept(ctx, sub, a.UUID, 5, ""); err != nil || r.Status != StatusAccepted || ap.values[10] != "760.01" || *r.DecidedBy != 5 {
		t.Fatalf("accept: %v %+v", err, r)
	}
	if _, err := svc.Reject(ctx, sub, a.UUID, 5, "x"); !errors.Is(err, ErrState) {
		t.Fatalf("reject accepted: %v", err)
	}
	// Correct an applied one: revert later restores the original value.
	c, err := svc.Correct(ctx, sub, a.UUID, 5, json.RawMessage(`"740.01"`), "740 doğru")
	if err != nil || c.Status != StatusCorrected || !c.RuleCandidate || ap.values[10] != "740.01" {
		t.Fatalf("correct: %v %+v", err, c)
	}
	if _, err := svc.Revert(ctx, sub, a.UUID, 5, "geri"); err != nil || ap.values[10] != "770.01" {
		t.Fatalf("revert after correct: %v %s", err, ap.values[10])
	}
	b, _ := svc.Submit(ctx, sub, input("0.5", "760.02"))
	if _, err := svc.Reject(ctx, sub, b.UUID, 5, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("reject needs reason")
	}
	if r, err := svc.Reject(ctx, sub, b.UUID, 5, "alakasız"); err != nil || r.Status != StatusRejected {
		t.Fatalf("reject: %v %+v", err, r)
	}
	rc := true
	rows, _, _ := st.List(ctx, Filter{OrganizationID: 1, RuleCandidate: &rc})
	if len(rows) != 1 || rows[0].UUID != a.UUID {
		t.Fatalf("rule candidates = %d", len(rows))
	}
	_, log, _ := st.Get(ctx, sub, a.UUID)
	if len(log) != 3 {
		t.Fatalf("log = %d", len(log))
	}
}

func TestNewSuggestionSupersedesPending(t *testing.T) {
	svc, st, _ := setup()
	ctx := context.Background()
	a, _ := svc.Submit(ctx, sub, input("0.5", "760.01"))
	_, _ = svc.Submit(ctx, sub, input("0.6", "760.02"))
	old, log, _ := st.Get(ctx, sub, a.UUID)
	if old.Status != StatusSuperseded || len(log) != 1 || log[0].Action != ActionSupersede {
		t.Fatalf("old = %s log = %+v", old.Status, log)
	}
	rows, _, _ := st.List(ctx, Filter{OrganizationID: 1, Status: "pending"})
	if len(rows) != 1 {
		t.Fatalf("pending = %d", len(rows))
	}
}

func TestApplyFailureRollsBack(t *testing.T) {
	svc, st, ap := setup()
	ap.fail = true
	ctx := context.Background()
	if _, err := svc.Submit(ctx, sub, input("0.99", "760.01")); !errors.Is(err, ErrApplyFailed) {
		t.Fatalf("err = %v", err)
	}
	if rows, _, _ := st.List(ctx, Filter{OrganizationID: 1}); len(rows) != 0 {
		t.Fatalf("rolled-back insert left %d rows", len(rows))
	}
}

func TestValidation(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()
	mut := []func(*Input){
		func(i *Input) { i.Kind = "Bad Kind" },
		func(i *Input) { i.Confidence = decimal.RequireFromString("1.01") },
		func(i *Input) { i.Confidence = decimal.RequireFromString("-0.1") },
		func(i *Input) { i.Rationale = "  " },
		func(i *Input) { i.Value = json.RawMessage(`{`) },
		func(i *Input) { i.Provenance = Provenance{Origin: OriginDocument} },
		func(i *Input) { i.Provenance = Provenance{Origin: OriginModel} },
		func(i *Input) { i.Provenance = Provenance{Origin: OriginRule} },
		func(i *Input) { i.Provenance = Provenance{Origin: "guess"} },
		func(i *Input) { i.Period = "2026-13" },
	}
	for n, m := range mut {
		in := input("0.5", "760.01")
		m(&in)
		if _, err := svc.Submit(ctx, sub, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: err = %v", n, err)
		}
	}
	if _, err := svc.Submit(ctx, Subject{OrganizationID: 1}, input("0.5", "x")); !errors.Is(err, ErrBadSubject) {
		t.Errorf("subject err = %v", err)
	}
}
