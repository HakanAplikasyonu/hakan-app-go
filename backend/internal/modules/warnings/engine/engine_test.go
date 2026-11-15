package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
)

// staticConfig resolves from in-memory layers, like the Postgres resolver.
type staticConfig struct {
	defaults   map[string]Layer
	office     map[string]Layer
	engagement map[string]Layer
}

func (c *staticConfig) Resolve(_ context.Context, _ Subject, codes []string) (map[string]Effective, error) {
	out := map[string]Effective{}
	for _, code := range codes {
		def, ok := c.defaults[code]
		if !ok {
			continue
		}
		out[code] = Merge(code, def, c.office[code], c.engagement[code])
	}
	return out, nil
}

func sev(s catalog.Severity) *catalog.Severity { return &s }
func boolp(b bool) *bool                       { return &b }

func newEngine() (*Engine, *MemoryStore, *staticConfig) {
	cfg := &staticConfig{
		defaults: map[string]Layer{
			"TAX_TOTAL_MISMATCH": {Severity: sev(catalog.SeverityWarning), Params: json.RawMessage(`{"tolerance":"0.05"}`)},
			"PAYABLE_MISMATCH":   {Severity: sev(catalog.SeverityWarning)},
			"PENDING_RESPONSE":   {Severity: sev(catalog.SeverityInfo)},
			"BNK_GAP":            {Severity: sev(catalog.SeverityBlocker)},
		},
		office:     map[string]Layer{},
		engagement: map[string]Layer{},
	}
	store := NewMemoryStore()
	return New(store, cfg), store, cfg
}

var (
	subj    = Subject{OrganizationID: 1, EngagementID: 10, TaxpayerID: 100}
	invID   = int64(5001)
	invoice = Entity{Type: catalog.EntityInvoice, ID: &invID, Period: "2026-10"}
	owned   = []string{"TAX_TOTAL_MISMATCH", "PAYABLE_MISMATCH"}
)

func open(store *MemoryStore) []StoredWarning {
	var out []StoredWarning
	for _, r := range store.All() {
		if r.Status == catalog.StatusOpen {
			out = append(out, r)
		}
	}
	return out
}

func TestRepeatedSyncDoesNotDuplicate(t *testing.T) {
	e, store, _ := newEngine()
	ctx := context.Background()
	f := []Finding{{Code: "TAX_TOTAL_MISMATCH", Params: map[string]any{"expected": "120.00", "actual": "119.80"}}}
	rep, err := e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if err != nil || rep.Created != 1 {
		t.Fatalf("first apply: %+v %v", rep, err)
	}
	for i := 0; i < 3; i++ {
		rep, err = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
		if err != nil || rep.Created != 0 || rep.Refreshed != 1 {
			t.Fatalf("re-apply %d: %+v %v", i, rep, err)
		}
	}
	if got := len(store.All()); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}

func TestConditionGoneAutoResolves(t *testing.T) {
	e, store, _ := newEngine()
	ctx := context.Background()
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, []Finding{{Code: "TAX_TOTAL_MISMATCH"}, {Code: "PAYABLE_MISMATCH"}})
	rep, err := e.Apply(ctx, subj, "ubl_parser", invoice, owned, []Finding{{Code: "PAYABLE_MISMATCH"}})
	if err != nil || rep.AutoResolved != 1 || rep.Refreshed != 1 {
		t.Fatalf("apply: %+v %v", rep, err)
	}
	rows := store.All()
	if rows[0].Code != "TAX_TOTAL_MISMATCH" || rows[0].Status != catalog.StatusAutoResolved || rows[0].ResolvedAt == nil {
		t.Fatalf("row0 = %+v", rows[0])
	}
	// The condition returns: a new open warning, the old one stays auto_resolved.
	rep, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, []Finding{{Code: "TAX_TOTAL_MISMATCH"}, {Code: "PAYABLE_MISMATCH"}})
	if rep.Created != 1 || len(open(store)) != 2 || len(store.All()) != 3 {
		t.Fatalf("reappear: %+v rows=%d", rep, len(store.All()))
	}
}

func TestAutoResolveOnlyOwnProducerAndEntity(t *testing.T) {
	e, store, _ := newEngine()
	ctx := context.Background()
	other := int64(5002)
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, []Finding{{Code: "TAX_TOTAL_MISMATCH"}})
	_, _ = e.Apply(ctx, subj, "control:totals", invoice, owned, []Finding{{Code: "PAYABLE_MISMATCH"}})
	_, _ = e.Apply(ctx, subj, "ubl_parser", Entity{Type: catalog.EntityInvoice, ID: &other, Period: "2026-10"}, owned, []Finding{{Code: "TAX_TOTAL_MISMATCH"}})
	// ubl_parser sees nothing on the first invoice now.
	rep, _ := e.Apply(ctx, subj, "ubl_parser", invoice, owned, nil)
	if rep.AutoResolved != 1 || len(open(store)) != 2 {
		t.Fatalf("rep = %+v open = %d", rep, len(open(store)))
	}
}

func TestSeverityComesFromConfiguration(t *testing.T) {
	e, store, cfg := newEngine()
	ctx := context.Background()
	f := []Finding{{Code: "TAX_TOTAL_MISMATCH"}}
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if open(store)[0].Severity != catalog.SeverityWarning {
		t.Fatal("default severity not used")
	}
	cfg.office["TAX_TOTAL_MISMATCH"] = Layer{Severity: sev(catalog.SeverityBlocker)}
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if open(store)[0].Severity != catalog.SeverityBlocker {
		t.Fatal("office override not applied")
	}
	cfg.engagement["TAX_TOTAL_MISMATCH"] = Layer{Severity: sev(catalog.SeverityInfo)}
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if got := open(store)[0].Severity; got != catalog.SeverityInfo {
		t.Fatalf("engagement override not applied: %s", got)
	}
	if len(store.All()) != 1 {
		t.Fatal("severity change must refresh, not duplicate")
	}
}

func TestDisabledCodeIsNotRaisedAndResolvesOpenOne(t *testing.T) {
	e, store, cfg := newEngine()
	ctx := context.Background()
	f := []Finding{{Code: "TAX_TOTAL_MISMATCH"}}
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	cfg.engagement["TAX_TOTAL_MISMATCH"] = Layer{Enabled: boolp(false)}
	rep, _ := e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if rep.Disabled != 1 || rep.AutoResolved != 1 || len(open(store)) != 0 {
		t.Fatalf("rep = %+v", rep)
	}
}

func TestAcknowledgedStaysUntilParamsChange(t *testing.T) {
	e, store, _ := newEngine()
	ctx := context.Background()
	f := []Finding{{Code: "TAX_TOTAL_MISMATCH", Params: map[string]any{"actual": "119.80"}}}
	_, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	store.SetStatus(keyFor(subj, invoice, "TAX_TOTAL_MISMATCH"), catalog.StatusAcknowledged)
	rep, _ := e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if rep.Suppressed != 1 || len(store.All()) != 1 {
		t.Fatalf("acknowledged reopened: %+v", rep)
	}
	f[0].Params["actual"] = "100.00"
	rep, _ = e.Apply(ctx, subj, "ubl_parser", invoice, owned, f)
	if rep.Created != 1 || len(open(store)) != 1 {
		t.Fatalf("changed condition must open a new warning: %+v", rep)
	}
}

func TestPeriodWideWarningsKeyedByPeriod(t *testing.T) {
	e, store, _ := newEngine()
	ctx := context.Background()
	oct := Entity{Type: catalog.EntityPeriod, Period: "2026-10"}
	nov := Entity{Type: catalog.EntityPeriod, Period: "2026-11"}
	bnk := []string{"BNK_GAP"}
	_, _ = e.Apply(ctx, subj, "control:bank_gap", oct, bnk, []Finding{{Code: "BNK_GAP"}})
	_, _ = e.Apply(ctx, subj, "control:bank_gap", nov, bnk, []Finding{{Code: "BNK_GAP"}})
	if len(open(store)) != 2 {
		t.Fatal("periods must not share a warning")
	}
	rep, _ := e.Apply(ctx, subj, "control:bank_gap", oct, bnk, nil)
	if rep.AutoResolved != 1 || len(open(store)) != 1 || open(store)[0].Period != "2026-11" {
		t.Fatalf("rep = %+v", rep)
	}
	if open(store)[0].Severity != catalog.SeverityBlocker {
		t.Fatal("BNK_GAP severity")
	}
}

func TestApplyValidation(t *testing.T) {
	e, _, _ := newEngine()
	ctx := context.Background()
	cases := []struct {
		name     string
		producer string
		ent      Entity
		owned    []string
		findings []Finding
		want     error
	}{
		{"bad producer", "ubl", invoice, owned, nil, ErrBadProducer},
		{"unknown code", "ubl_parser", invoice, []string{"NOPE"}, nil, ErrUnknownCode},
		{"wrong entity", "ubl_parser", Entity{Type: catalog.EntityInvoiceItem, ID: &invID}, owned, nil, ErrEntityNotAllow},
		{"not owned", "ubl_parser", invoice, owned, []Finding{{Code: "PENDING_RESPONSE"}}, ErrNotOwned},
		{"bad entity type", "ubl_parser", Entity{Type: "nope", ID: &invID}, owned, nil, ErrBadEntity},
		{"bad period", "ubl_parser", Entity{Type: catalog.EntityInvoice, ID: &invID, Period: "2026-13"}, owned, nil, ErrBadEntity},
		{"no id no period", "ubl_parser", Entity{Type: catalog.EntityInvoice}, owned, nil, ErrBadEntity},
		{"bad subject", "", invoice, owned, nil, ErrBadSubject},
		{"zero id", "ubl_parser", Entity{Type: catalog.EntityInvoice, ID: new(int64), Period: "2026-10"}, owned, nil, ErrBadEntity},
		{"period with id", "control:x", Entity{Type: catalog.EntityPeriod, ID: &invID, Period: "2026-10"}, []string{"BNK_GAP"}, nil, ErrBadEntity},
	}
	for _, c := range cases {
		s := subj
		if c.want == ErrBadSubject {
			s = Subject{OrganizationID: 1}
		}
		if _, err := e.Apply(ctx, s, c.producer, c.ent, c.owned, c.findings); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	for _, p := range []string{"adapter:isnet", "ubl_parser", "excel_import", "sync_engine", "control:tax_totals"} {
		if !ValidProducer(p) {
			t.Errorf("producer %q should be valid", p)
		}
	}
}

func TestMergeParams(t *testing.T) {
	eff := Merge("X",
		Layer{Severity: sev(catalog.SeverityWarning), Params: json.RawMessage(`{"limit":"50000","days":3}`)},
		Layer{Params: json.RawMessage(`{"limit":"75000"}`)},
		Layer{Params: json.RawMessage(`{"days":null}`), Enabled: boolp(false)},
	)
	if eff.Severity != catalog.SeverityWarning || eff.Enabled || eff.Params["limit"] != "75000" || eff.Params["days"] != nil {
		t.Fatalf("eff = %+v", eff)
	}
	if _, ok := eff.Params["days"]; !ok {
		t.Fatal("null must clear the value, not drop the key")
	}
}
