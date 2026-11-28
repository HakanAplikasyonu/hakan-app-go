package rules

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/shopspring/decimal"
)

// seedConfig mirrors the warning_codes seed for the codes under test.
type seedConfig struct{ office map[string]engine.Layer }

var seed = map[string]string{
	CodeLineTaxable:   `{"tolerance":"0.01"}`,
	CodeTaxTotal:      `{"tolerance":"0.05"}`,
	CodeBaseTotal:     `{"tolerance":"0.05"}`,
	CodePayable:       `{"tolerance":"0.05"}`,
	CodeHeaderEst:     `{}`,
	CodeInvalidTaxID:  `{}`,
	CodeUnusualAmount: `{"multiplier":"10","min_history":3,"min_amount":"50000"}`,
	CodeNewPartyHigh:  `{"limit":"100000"}`,
	CodeOutOfPeriod:   `{}`,
}

func (c seedConfig) Resolve(_ context.Context, _ engine.Subject, codes []string) (map[string]engine.Effective, error) {
	w := catalog.SeverityWarning
	out := map[string]engine.Effective{}
	for _, code := range codes {
		out[code] = engine.Merge(code, engine.Layer{Severity: &w, Params: json.RawMessage(seed[code])}, c.office[code])
	}
	return out, nil
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func dp(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

var nextID int64

func newID() int64 { nextID++; return nextID }

var subj = engine.Subject{OrganizationID: 1, EngagementID: 2, TaxpayerID: 3}

func openCodes(store *engine.MemoryStore) map[string]int {
	out := map[string]int{}
	for _, r := range store.All() {
		if r.Status == catalog.StatusOpen {
			out[r.Code]++
		}
	}
	return out
}

func goodInvoice() InvoiceTotalsInput {
	return InvoiceTotalsInput{
		InvoiceID: newID(), Period: "2026-10",
		Lines: []InvoiceLine{
			{ID: newID(), LineExtension: d("100"), Taxable: dp("100"), Rate: "20", Tax: d("20")},
			{ID: newID(), LineExtension: d("50"), Taxable: dp("50"), Rate: "10", Tax: d("5")},
		},
		TaxExclusive:    d("150"),
		HeaderTax:       d("25"),
		HeaderTaxByRate: map[string]decimal.Decimal{"20": d("20"), "10": d("5")},
		Payable:         d("175"),
	}
}

func TestInvoiceTotalsCleanInvoice(t *testing.T) {
	store := engine.NewMemoryStore()
	e := engine.New(store, seedConfig{})
	if _, err := engine.Run(context.Background(), e, subj, InvoiceTotals{}, goodInvoice()); err != nil {
		t.Fatal(err)
	}
	if len(store.All()) != 0 {
		t.Fatalf("clean invoice raised %v", openCodes(store))
	}
}

func TestInvoiceTotalsMismatchesThenFixed(t *testing.T) {
	store := engine.NewMemoryStore()
	e := engine.New(store, seedConfig{})
	ctx := context.Background()
	in := goodInvoice()
	in.Lines[0].Taxable = dp("99.98") // line taxable off by 0.02 (> 0.01)
	in.HeaderTax = d("25.10")         // Σ line tax 25 vs 25.10
	in.HeaderTaxByRate["20"] = d("20.10")
	in.TaxExclusive = d("150.20") // Σ base 149.98 vs 150.20
	in.Payable = d("170")         // expected 150.20+25.10 = 175.30
	in.Withholding = d("0")
	rep, err := engine.Run(ctx, e, subj, InvoiceTotals{}, in)
	if err != nil {
		t.Fatal(err)
	}
	got := openCodes(store)
	for _, c := range []string{CodeLineTaxable, CodeTaxTotal, CodeBaseTotal, CodePayable} {
		if got[c] != 1 {
			t.Errorf("%s: open = %d (all %v)", c, got[c], got)
		}
	}
	if got[CodeHeaderEst] != 0 || rep.Created != 4 {
		t.Fatalf("rep = %+v open = %v", rep, got)
	}
	// Same data again: nothing new.
	rep, _ = engine.Run(ctx, e, subj, InvoiceTotals{}, in)
	if rep.Created != 0 || rep.Refreshed != 4 {
		t.Fatalf("re-run rep = %+v", rep)
	}
	// Data corrected: everything auto-resolves (the line ids are kept).
	fixed := goodInvoice()
	fixed.InvoiceID, fixed.Lines[0].ID, fixed.Lines[1].ID = in.InvoiceID, in.Lines[0].ID, in.Lines[1].ID
	rep, _ = engine.Run(ctx, e, subj, InvoiceTotals{}, fixed)
	if rep.AutoResolved != 4 || len(openCodes(store)) != 0 {
		t.Fatalf("fixed rep = %+v open = %v", rep, openCodes(store))
	}
}

func TestInvoiceTotalsWithinToleranceAndWithholding(t *testing.T) {
	store := engine.NewMemoryStore()
	e := engine.New(store, seedConfig{})
	in := goodInvoice()
	in.HeaderTax = d("25.04") // within 0.05
	in.Withholding = d("10")  // tevkifat
	in.Payable = d("165.04")  // 150 + 25.04 - 10
	if _, err := engine.Run(context.Background(), e, subj, InvoiceTotals{}, in); err != nil {
		t.Fatal(err)
	}
	if len(store.All()) != 0 {
		t.Fatalf("raised %v", openCodes(store))
	}
}

func TestInvoiceTotalsToleranceFromConfig(t *testing.T) {
	store := engine.NewMemoryStore()
	// The office tightens TAX_TOTAL tolerance to 0.
	e := engine.New(store, seedConfig{office: map[string]engine.Layer{
		CodeTaxTotal: {Params: json.RawMessage(`{"tolerance":"0"}`)},
	}})
	in := goodInvoice()
	in.HeaderTax = d("25.01")
	in.Payable = d("175.01")
	if _, err := engine.Run(context.Background(), e, subj, InvoiceTotals{}, in); err != nil {
		t.Fatal(err)
	}
	if openCodes(store)[CodeTaxTotal] != 1 {
		t.Fatalf("office tolerance ignored: %v", openCodes(store))
	}
}

func TestHeaderEstimateWithoutLines(t *testing.T) {
	store := engine.NewMemoryStore()
	e := engine.New(store, seedConfig{})
	in := InvoiceTotalsInput{InvoiceID: newID(), Period: "2026-10", TaxExclusive: d("100"), HeaderTax: d("20"), Payable: d("120"), HeaderEstimate: true}
	if _, err := engine.Run(context.Background(), e, subj, InvoiceTotals{}, in); err != nil {
		t.Fatal(err)
	}
	if got := openCodes(store); got[CodeHeaderEst] != 1 || len(got) != 1 {
		t.Fatalf("open = %v", got)
	}
}

func TestTaxID(t *testing.T) {
	ctx := context.Background()
	id := newID()
	for _, c := range []struct {
		taxID string
		bad   bool
	}{{"1234567890", false}, {"12345678901", false}, {"123456789", true}, {"12345678a0", true}, {"", true}} {
		store := engine.NewMemoryStore()
		e := engine.New(store, seedConfig{})
		if _, err := engine.Run(ctx, e, subj, TaxID{}, TaxIDInput{EntityType: catalog.EntityCustomer, EntityID: id, Period: "2026-10", TaxID: c.taxID}); err != nil {
			t.Fatal(err)
		}
		if got := openCodes(store)[CodeInvalidTaxID] == 1; got != c.bad {
			t.Errorf("%q: flagged=%v want %v", c.taxID, got, c.bad)
		}
	}
}

func TestAmounts(t *testing.T) {
	ctx := context.Background()
	// A supplier invoicing about 10.000 TL: median 10.000, max 12.400.
	hist := []decimal.Decimal{d("9000"), d("10000"), d("12400"), d("10000")}
	small := []decimal.Decimal{d("1000"), d("1200"), d("800")} // median 1.000
	cases := []struct {
		name string
		in   AmountInput
		want map[string]int
	}{
		{"normal", AmountInput{Amount: d("11000"), PartyHistory: hist}, map[string]int{}},
		{"10.000 → 1.000.000", AmountInput{Amount: d("1000000"), PartyHistory: hist}, map[string]int{CodeUnusualAmount: 1}},
		{"just under 10×", AmountInput{Amount: d("100000"), PartyHistory: hist}, map[string]int{}},
		// 30× the median but under the 50.000 TL floor.
		{"under the floor", AmountInput{Amount: d("30000"), PartyHistory: small}, map[string]int{}},
		{"too little history", AmountInput{Amount: d("1000000"), PartyHistory: hist[:2]}, map[string]int{}},
		// Two invoices with the party: falls back to the taxpayer's purchases.
		{"party thin, direction basis", AmountInput{Amount: d("1000000"), PartyHistory: hist[:2], DirectionHistory: hist}, map[string]int{CodeUnusualAmount: 1}},
		{"no history at all", AmountInput{Amount: d("1000000")}, map[string]int{}},
		{"new party high", AmountInput{Amount: d("150000"), FirstSeen: true}, map[string]int{CodeNewPartyHigh: 1}},
		{"new party low", AmountInput{Amount: d("90000"), FirstSeen: true}, map[string]int{}},
	}
	for _, c := range cases {
		store := engine.NewMemoryStore()
		e := engine.New(store, seedConfig{})
		c.in.InvoiceID, c.in.Period, c.in.Direction = newID(), "2026-10", "incoming"
		if _, err := engine.Run(ctx, e, subj, Amounts{}, c.in); err != nil {
			t.Fatal(err)
		}
		got := openCodes(store)
		if len(got) != len(c.want) {
			t.Errorf("%s: open = %v want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: open = %v want %v", c.name, got, c.want)
			}
		}
	}

	// The warning carries the comparison the text shows.
	store := engine.NewMemoryStore()
	e := engine.New(store, seedConfig{})
	if _, err := engine.Run(ctx, e, subj, Amounts{}, AmountInput{
		InvoiceID: newID(), Period: "2026-10", Direction: "incoming", Amount: d("1000000"), PartyHistory: hist,
	}); err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(store.All()[0].Params, &params); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"amount": "1000000.00", "median": "10000.00", "min": "9000.00", "max": "12400.00", "count": float64(4),
		"multiplier": "10", "min_amount": "50000.00", "basis": BasisParty, "direction": "incoming", "variant": "party_incoming",
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("param %s = %v want %v", k, params[k], v)
		}
	}

	// A null limit (office has not set it) disables the check.
	store = engine.NewMemoryStore()
	e = engine.New(store, seedConfig{office: map[string]engine.Layer{CodeNewPartyHigh: {Params: json.RawMessage(`{"limit":null}`)}}})
	_, _ = engine.Run(ctx, e, subj, Amounts{}, AmountInput{InvoiceID: newID(), Period: "2026-10", Amount: d("1e9"), FirstSeen: true})
	if len(store.All()) != 0 {
		t.Fatal("null limit must skip the check")
	}
}

func TestStatsOfMedian(t *testing.T) {
	st := statsOf([]decimal.Decimal{d("5"), d("1"), d("3"), d("100")})
	if !st.median.Equal(d("4")) || !st.min.Equal(d("1")) || !st.max.Equal(d("100")) {
		t.Fatalf("stats %+v", st)
	}
	if st := statsOf([]decimal.Decimal{d("7")}); !st.median.Equal(d("7")) {
		t.Fatalf("single %+v", st)
	}
}

func TestOutOfPeriod(t *testing.T) {
	ctx := context.Background()
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	closed := at("2026-04-20T10:00:00Z")
	march := at("2026-03-15T00:00:00Z")
	cases := []struct {
		name string
		in   OutOfPeriodInput
		want bool
	}{
		{"arrived after the filed return", OutOfPeriodInput{IssueDate: march, ArrivedAt: at("2026-05-03T09:00:00Z"), RecordedPeriod: "2026-05", ClosedAt: &closed, CloseReason: CloseReasonFiled}, true},
		{"arrived before closing", OutOfPeriodInput{IssueDate: march, ArrivedAt: at("2026-04-02T09:00:00Z"), RecordedPeriod: "2026-04", ClosedAt: &closed, CloseReason: CloseReasonFiled}, false},
		{"month still open", OutOfPeriodInput{IssueDate: march, ArrivedAt: at("2026-05-03T09:00:00Z"), RecordedPeriod: "2026-05"}, false},
		{"same month", OutOfPeriodInput{IssueDate: march, ArrivedAt: at("2026-03-28T09:00:00Z"), RecordedPeriod: "2026-03", ClosedAt: &closed, CloseReason: CloseReasonLock}, false},
	}
	for _, c := range cases {
		store := engine.NewMemoryStore()
		e := engine.New(store, seedConfig{})
		c.in.InvoiceID = newID()
		if _, err := engine.Run(ctx, e, subj, OutOfPeriod{}, c.in); err != nil {
			t.Fatal(err)
		}
		got := openCodes(store)[CodeOutOfPeriod] == 1
		if got != c.want {
			t.Errorf("%s: raised=%v want %v", c.name, got, c.want)
			continue
		}
		if !got {
			continue
		}
		w := store.All()[0]
		if w.Period != c.in.RecordedPeriod {
			t.Errorf("%s: period %q want %q", c.name, w.Period, c.in.RecordedPeriod)
		}
		var params map[string]any
		if err := json.Unmarshal(w.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params["issue_period"] != "2026-03" || params["closed_on"] != "2026-04-20" || params["variant"] != CloseReasonFiled {
			t.Errorf("%s: params %v", c.name, params)
		}
	}
}
