package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func at(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", date, Istanbul)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDueDate(t *testing.T) {
	cases := []struct {
		period string
		day    int
		want   string
		rolled bool
	}{
		{"2026-09", 28, "2026-10-28", false}, // Çarşamba
		{"2026-10", 28, "2026-11-30", true},  // 28 Kasım 2026 Cumartesi → Pazartesi
		{"2026-05", 28, "2026-06-29", true},  // 28 Haziran 2026 Pazar → Pazartesi
		{"2026-01", 31, "2026-03-02", true},  // Şubat'ın son günü (28, Cumartesi)
		{"2027-12", 26, "2028-01-26", false}, // yıl dönümü
		{"2026-09", 0, "2026-10-28", false},  // geçersiz gün → 28
	}
	for _, c := range cases {
		got, rolled, err := DueDate(c.period, c.day)
		if err != nil {
			t.Fatal(err)
		}
		if got.Format("2006-01-02") != c.want || rolled != c.rolled {
			t.Errorf("DueDate(%s, %d) = %s rolled=%v, want %s rolled=%v", c.period, c.day, got.Format("2006-01-02"), rolled, c.want, c.rolled)
		}
	}
	if _, _, err := DueDate("2026-13", 28); err == nil {
		t.Fatal("bad period accepted")
	}
}

func TestComputeTotalsTevkifatAndRates(t *testing.T) {
	in := Input{
		Period: "2026-10",
		Lines: []Line{
			{Direction: DirectionOutgoing, Type: "SATIS", TaxRate: d("20"), InvoiceCount: 2, Base: d("1000"), VAT: d("200"), Withholding: d("0")},
			{Direction: DirectionOutgoing, Type: "TEVKIFAT", TaxRate: d("20"), WithholdingCode: "601", InvoiceCount: 1, Base: d("500"), VAT: d("100"), Withholding: d("40")},
			{Direction: DirectionOutgoing, Type: "ISTISNA", TaxRate: d("0"), InvoiceCount: 1, Base: d("300")},
			{Direction: DirectionIncoming, Type: "ALIS", TaxRate: d("10"), InvoiceCount: 1, Base: d("400"), VAT: d("40")},
			{Direction: DirectionIncoming, Type: "IADE", TaxRate: d("20"), InvoiceCount: 1, Base: d("50"), VAT: d("10")},
			{Direction: DirectionIncoming, Type: "TEVKIFAT", TaxRate: d("20"), WithholdingCode: "624", InvoiceCount: 1, Base: d("100"), VAT: d("20"), Withholding: d("4")},
		},
		Excluded: Excluded{Included: 6},
		Settings: Settings{DueDay: 28, DaysBefore: 5},
		Now:      at("2026-11-15 10:00"),
	}
	f, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	tot := f.Totals
	// hesaplanan = 200 + (100 − 40) = 260; indirilecek = 40 + 10 + 20 = 70.
	check := func(name string, got Money, want string) {
		t.Helper()
		if !got.Dec().Equal(d(want)) {
			t.Errorf("%s = %s, want %s", name, got.Dec(), want)
		}
	}
	check("output", tot.OutputVAT, "260")
	check("output withholding", tot.OutputWithholding, "40")
	check("input", tot.InputVAT, "70")
	check("input withholding", tot.InputWithholding, "4")
	check("net", tot.Net, "190")
	check("payable", tot.Payable, "190")
	check("carry out", tot.CarryOut, "0")
	if tot.InvoiceCount != 6 {
		t.Errorf("invoice count %d", tot.InvoiceCount)
	}
	if len(f.Rates) != 3 || f.Rates[0].TaxRate != "0.00" || f.Rates[2].TaxRate != "20.00" {
		t.Fatalf("rates %+v", f.Rates)
	}
	check("rate 20 output", f.Rates[2].OutputVAT, "260")
	check("rate 20 input", f.Rates[2].InputVAT, "30")
	if f.Calendar.DueDate != "2026-11-30" || f.Calendar.DaysLeft != 15 || !f.Calendar.RolledForward || !f.Calendar.HolidaysChecked {
		t.Errorf("calendar %+v", f.Calendar)
	}
	if f.Checks.DeadlineNear || f.Checks.PayableLimit != nil {
		t.Errorf("checks %+v", f.Checks)
	}
	raw, _ := json.Marshal(f.Totals)
	if want := `"payable":"190.00"`; !strings.Contains(string(raw), want) {
		t.Errorf("json %s lacks %s", raw, want)
	}
}

func TestCarryForwardChain(t *testing.T) {
	hist := []PeriodTotal{
		{Period: "2026-06", Output: d("100"), Input: d("300")}, // devir 200
		{Period: "2026-07", Output: d("150"), Input: d("0")},   // 150 − 200 → devir 50
		{Period: "2026-08", Output: d("0"), Input: d("25")},    // devir 75
		{Period: "2026-09", Output: d("500"), Input: d("100")}, // 400 − 75 = 325 ödenir → devir 0
	}
	base := Input{Period: "2026-10", Settings: Settings{DueDay: 28}, Now: at("2026-10-01 00:00")}

	in := base
	in.History = hist[:3]
	f, _ := Compute(in)
	if !f.Totals.CarriedIn.Dec().Equal(d("75")) || f.CarryForward.Source != "computed" || len(f.CarryForward.Periods) != 3 {
		t.Fatalf("chain: carried %s %+v", f.Totals.CarriedIn.Dec(), f.CarryForward)
	}

	in.History = hist
	f, _ = Compute(in)
	if !f.Totals.CarriedIn.Dec().IsZero() {
		t.Fatalf("payable period must reset the carry, got %s", f.Totals.CarriedIn.Dec())
	}

	// Opening into 2026-08 replaces everything before it.
	in.Opening = &Opening{Period: "2026-08", Amount: M(d("1000"))}
	f, _ = Compute(in)
	// 2026-08: 0 − 25 − 1000 → 1025; 2026-09: 400 − 1025 → 625.
	if !f.Totals.CarriedIn.Dec().Equal(d("625")) || f.CarryForward.Source != "opening" || *f.CarryForward.OpeningPeriod != "2026-08" {
		t.Fatalf("opening chain: %s %+v", f.Totals.CarriedIn.Dec(), f.CarryForward)
	}

	// Opening into the period itself is the carry as is.
	in.Opening = &Opening{Period: "2026-10", Amount: M(d("12.34"))}
	in.Lines = []Line{{Direction: DirectionOutgoing, TaxRate: d("20"), VAT: d("10")}}
	f, _ = Compute(in)
	if !f.Totals.CarriedIn.Dec().Equal(d("12.34")) || !f.Totals.CarryOut.Dec().Equal(d("2.34")) || !f.Totals.Payable.Dec().IsZero() {
		t.Fatalf("own-period opening: %+v", f.Totals)
	}

	in = base
	f, _ = Compute(in)
	if f.CarryForward.Source != "none" || f.CarryForward.Periods == nil {
		t.Fatalf("no history: %+v", f.CarryForward)
	}
}

func TestSettingsFrom(t *testing.T) {
	cfg := map[string]engine.Effective{
		CodePayableThreshold: {Code: CodePayableThreshold, Enabled: true, Params: map[string]any{"limit": "50000"}},
		CodeDeadlineNear:     {Code: CodeDeadlineNear, Enabled: false, Params: map[string]any{"days_before": float64(3), "due_day": float64(26)}},
	}
	s, err := SettingsFrom(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.PayableLimit == nil || !s.PayableLimit.Equal(d("50000")) || s.DaysBefore != 3 || s.DueDay != 26 {
		t.Fatalf("%+v", s)
	}
	cfg[CodeDeadlineNear] = engine.Effective{Params: map[string]any{"days_before": 2.5, "due_day": float64(40)}}
	cfg[CodePayableThreshold] = engine.Effective{Params: map[string]any{"limit": nil}}
	s, _ = SettingsFrom(cfg)
	if s.PayableLimit != nil || s.DaysBefore != DefaultDaysBefore || s.DueDay != DefaultDueDay {
		t.Fatalf("invalid params must fall back: %+v", s)
	}
	cfg[CodePayableThreshold] = engine.Effective{Params: map[string]any{"limit": "abc"}}
	if _, err := SettingsFrom(cfg); err == nil {
		t.Fatal("bad limit accepted")
	}
}

// --- service + rule over the engine's memory store --------------------------

type fakeStore struct {
	lines   map[string][]Line
	opening *Opening
}

func (f *fakeStore) Lines(_ context.Context, _ int64, p string) ([]Line, error) {
	return f.lines[p], nil
}
func (f *fakeStore) Excluded(context.Context, int64, string) (Excluded, error) {
	return Excluded{}, nil
}
func (f *fakeStore) History(_ context.Context, _ int64, from, before string) ([]PeriodTotal, error) {
	var out []PeriodTotal
	for _, p := range []string{"2026-08", "2026-09", "2026-10", "2026-11"} {
		if p < from || p >= before {
			continue
		}
		var pt PeriodTotal
		pt.Period = p
		for _, l := range f.lines[p] {
			if l.Direction == DirectionOutgoing {
				pt.Output = pt.Output.Add(l.VAT.Sub(l.Withholding))
			} else {
				pt.Input = pt.Input.Add(l.VAT)
			}
		}
		if len(f.lines[p]) > 0 {
			out = append(out, pt)
		}
	}
	return out, nil
}
func (f *fakeStore) OpeningAtOrBefore(_ context.Context, _, _ int64, p string) (*Opening, error) {
	if f.opening != nil && f.opening.Period <= p {
		return f.opening, nil
	}
	return nil, nil
}
func (f *fakeStore) ListOpenings(context.Context, int64, int64) ([]Opening, error) { return nil, nil }
func (f *fakeStore) UpsertOpening(_ context.Context, _, _ int64, o Opening, _ int64) (Opening, error) {
	f.opening = &o
	return o, nil
}
func (f *fakeStore) DeleteOpening(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (f *fakeStore) ActiveEngagements(context.Context, OfficeFilter) ([]EngagementRef, int64, error) {
	return []EngagementRef{{ID: 7, UUID: uuid.New(), TaxpayerID: 9, TaxpayerUUID: uuid.New()}}, 1, nil
}

type mapConfig map[string]engine.Effective

func (m mapConfig) Resolve(_ context.Context, _ engine.Subject, codes []string) (map[string]engine.Effective, error) {
	out := map[string]engine.Effective{}
	for _, c := range codes {
		if e, ok := m[c]; ok {
			out[c] = e
		}
	}
	return out, nil
}

func TestEvaluateRaisesAndAutoResolves(t *testing.T) {
	cfg := mapConfig{
		CodePayableThreshold: {Code: CodePayableThreshold, Enabled: true, Severity: catalog.SeverityWarning, Params: map[string]any{"limit": "100"}},
		CodeDeadlineNear:     {Code: CodeDeadlineNear, Enabled: true, Severity: catalog.SeverityWarning, Params: map[string]any{"days_before": float64(5), "due_day": float64(28)}},
	}
	store := &fakeStore{lines: map[string][]Line{
		"2026-10": {{Direction: DirectionOutgoing, TaxRate: d("20"), VAT: d("300"), InvoiceCount: 1}},
	}}
	mem := engine.NewMemoryStore()
	now := at("2026-11-26 09:00") // 2026-10 son günü 30 Kasım: 4 gün
	svc := New(store, cfg, engine.New(mem, cfg)).WithClock(func() time.Time { return now })
	sub := Subject{OrganizationID: 1, EngagementID: 7, TaxpayerID: 9}

	if got := svc.DefaultEvaluationPeriods(); got[0] != "2026-10" || got[1] != "2026-11" {
		t.Fatalf("default periods %v", got)
	}
	ev, err := svc.Evaluate(context.Background(), sub, []string{"2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if ev[0].Created != 2 || ev[0].DaysLeft != 4 || ev[0].Payable.Dec().StringFixed(2) != "300.00" {
		t.Fatalf("evaluation %+v", ev[0])
	}
	rows := mem.All()
	if len(rows) != 2 {
		t.Fatalf("warnings %d", len(rows))
	}
	for _, r := range rows {
		if r.Producer != Producer || r.EntityType != catalog.EntityPeriod || r.Period != "2026-10" || r.EntityID != nil {
			t.Errorf("warning %+v", r.Warning)
		}
	}

	// Same day again: refreshed, not duplicated (params are stable).
	ev, _ = svc.Evaluate(context.Background(), sub, []string{"2026-10"})
	if ev[0].Created != 0 || ev[0].Refreshed != 2 {
		t.Fatalf("re-evaluation %+v", ev[0])
	}

	// An opening carry-forward wipes the payable; the deadline has passed.
	store.opening = &Opening{Period: "2026-10", Amount: M(d("1000"))}
	now = at("2026-12-01 09:00")
	ev, _ = svc.Evaluate(context.Background(), sub, []string{"2026-10"})
	if ev[0].AutoResolved != 2 || ev[0].DaysLeft >= 0 {
		t.Fatalf("auto-resolve %+v", ev[0])
	}

	items, total, err := svc.Office(context.Background(), OfficeFilter{OrganizationID: 1, Limit: 20}, "2026-10")
	if err != nil || total != 1 || len(items) != 1 || !items[0].Totals.CarriedIn.Dec().Equal(d("1000")) {
		t.Fatalf("office %v %d %+v", err, total, items)
	}
}

func TestSetOpeningValidation(t *testing.T) {
	svc := New(&fakeStore{}, mapConfig{}, nil)
	sub := Subject{OrganizationID: 1, EngagementID: 2}
	for _, in := range []OpeningInput{
		{Period: "2026-1", Amount: "10"},
		{Period: "2026-10", Amount: "-1"},
		{Period: "2026-10", Amount: "1.234"},
		{Period: "2026-10", Amount: "abc"},
	} {
		if _, err := svc.SetOpening(context.Background(), sub, in, 0); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	o, err := svc.SetOpening(context.Background(), sub, OpeningInput{Period: "2026-10", Amount: " 1500.50 "}, 0)
	if err != nil || !o.Amount.Dec().Equal(d("1500.5")) {
		t.Fatalf("%v %+v", err, o)
	}
}
