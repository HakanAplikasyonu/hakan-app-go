package taxreturns

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func eq(t *testing.T, what string, got Money, want string) {
	t.Helper()
	if !got.Dec().Equal(d(want)) {
		t.Errorf("%s = %s, want %s", what, got.Dec().StringFixed(2), want)
	}
}

func codes(items []CheckItem) map[string]CheckItem {
	out := map[string]CheckItem{}
	for _, i := range items {
		out[i.Code] = i
	}
	return out
}

func TestComputeKDV1(t *testing.T) {
	prev := "2026-09"
	in := KDV1Input{
		Invoices: []InvoiceLine{
			{Direction: "outgoing", Type: "SATIS", Rate: d("20"), Count: 3, Base: d("10000"), VAT: d("2000")},
			{Direction: "outgoing", Type: "SATIS", Rate: d("10"), Count: 1, Base: d("1000"), VAT: d("100")},
			// Kısmi tevkifat 7/10: satıcı 300 beyan eder.
			{Direction: "outgoing", Type: "TEVKIFAT", Rate: d("20"), WithholdingCode: "603", Count: 1, Base: d("5000"), VAT: d("1000"), Withholding: d("700")},
			// İhracat (tam istisna) matraha girmez.
			{Direction: "outgoing", Type: "ISTISNA", Rate: d("0"), ExemptionCode: "301", Count: 1, Base: d("8000")},
			// İhraç kayıtlı: KDV'si hesaplanmamış; oran × matrah, tecil edilir.
			{Direction: "outgoing", Type: "IHRACKAYITLI", Rate: d("20"), ExemptionCode: "701", Count: 1, Base: d("2000")},
			// Alıştan iade satış sayılır.
			{Direction: "outgoing", Type: "IADE", Rate: d("20"), Count: 1, Base: d("500"), VAT: d("100")},
			{Direction: "incoming", Type: "ALIS", Rate: d("20"), Count: 4, Base: d("6000"), VAT: d("1200")},
			// Alışta tevkifat: 200 bu döneme ait, 300 sorumlu sıfatıyla.
			{Direction: "incoming", Type: "TEVKIFAT", Rate: d("20"), WithholdingCode: "601", Count: 1, Base: d("2500"), VAT: d("500"), Withholding: d("300")},
			// Satıştan iade.
			{Direction: "incoming", Type: "IADE", Rate: d("20"), Count: 1, Base: d("250"), VAT: d("50")},
		},
		InvoiceStats: InvoiceStats{Included: 13, Draft: 2, MissingExchangeRate: 1},
		Retail:       []RetailLine{{Rate: d("20"), Count: 20, Base: d("3000"), VAT: d("600")}, {Rate: d("1"), Count: 20, Base: d("1000"), VAT: d("10")}},
		RetailStats:  RetailStats{Approved: 20, Pending: 2, CardTotal: d("2500")},
		Expenses:     []ExpenseLine{{Rate: d("20"), Count: 5, Base: d("1000"), VAT: d("200")}, {Rate: d("-1"), Count: 1, Base: d("100"), VAT: d("10")}},
		ExpenseStats: ExpenseStats{Pending: 3},
		Carry:        Carry{Source: CarryPrevious, Period: &prev, Amount: d("150")},
	}
	k, items := ComputeKDV1(in)

	if len(k.Sales) != 3 || k.Sales[0].Rate != "1" || k.Sales[2].Rate != "20" {
		t.Fatalf("sales rows: %+v", k.Sales)
	}
	s20 := k.Sales[2]
	eq(t, "20 base", s20.Base, "15500") // 10000 + 2000 ihraç kayıtlı + 3000 Z + 500 iade
	eq(t, "20 vat", s20.VAT, "3100")
	eq(t, "20 retail", s20.RetailVAT, "600")
	eq(t, "20 purchase return", s20.PurchaseReturnVAT, "100")
	if len(k.Withholdings) != 1 || k.Withholdings[0].Ratio != "7/10" {
		t.Fatalf("withholdings: %+v", k.Withholdings)
	}
	eq(t, "declared", k.Withholdings[0].DeclaredVAT, "300")

	tt := k.Totals
	eq(t, "base total", tt.BaseTotal, "22500")       // 15500 + 1000 + 1000 (Z %1) + 5000
	eq(t, "calculated", tt.CalculatedVAT, "3510")    // 3100 + 100 + 10 + 300
	eq(t, "current input", tt.CurrentInput, "1610")  // 1200 + 200 + 200 + 10
	eq(t, "responsible", tt.ResponsibleVAT, "300")   // KDV2
	eq(t, "sales returns", tt.SalesReturnsVAT, "50") // satıştan iade
	eq(t, "deductions", tt.TotalDeductions, "2110")  // 150 + 1610 + 50 + 300
	eq(t, "deferred", tt.DeferredVAT, "400")         // ihraç kayıtlı
	eq(t, "payable", tt.Payable, "1000")             // 3510 − 2110 − 400
	eq(t, "carry out", tt.CarryOut, "0")
	eq(t, "exempt base", tt.ExemptBase, "8000")
	eq(t, "delivery", tt.DeliveryTotal, "30500")
	eq(t, "card", tt.CardSales, "2500")

	if len(k.Exemptions) != 2 || k.Exemptions[0].Group != ExemptionFull || k.Exemptions[1].Group != ExemptionExportRegistered {
		t.Fatalf("exemptions: %+v", k.Exemptions)
	}
	// Oranı olmayan masraf, KDV / matrah oranından (%10) bulunur.
	found := false
	for _, p := range k.Purchases {
		if p.Rate == "10" {
			found = true
			eq(t, "expense 10", p.ExpenseVAT, "10")
		}
	}
	if !found {
		t.Errorf("expense without a rate not placed: %+v", k.Purchases)
	}

	c := codes(items)
	for _, code := range []string{"MISSING_EXCHANGE_RATE", "PENDING_EXPENSES", "PENDING_Z_REPORTS", "DRAFT_INVOICES", "EXPORT_REGISTERED", "RESPONSIBLE_VAT"} {
		if _, ok := c[code]; !ok {
			t.Errorf("checklist misses %s: %+v", code, items)
		}
	}
	if _, ok := c["CARRY_UNKNOWN"]; ok {
		t.Error("carry is known")
	}
	open := applyAcks(items, []string{"MISSING_EXCHANGE_RATE", "PENDING_EXPENSES"})
	if len(open) != 1 || open[0] != "PENDING_Z_REPORTS" {
		t.Errorf("open warnings = %v", open)
	}
}

func TestComputeKDV1CarryOutAndAdjustments(t *testing.T) {
	extra, other, carried := M(d("50")), M(d("25")), M(d("1000"))
	in := KDV1Input{
		Invoices: []InvoiceLine{
			{Direction: "outgoing", Type: "SATIS", Rate: d("20"), Count: 1, Base: d("1000"), VAT: d("200")},
			{Direction: "incoming", Type: "ALIS", Rate: d("20"), Count: 1, Base: d("500"), VAT: d("100")},
		},
		InvoiceStats: InvoiceStats{Included: 2},
		Adjustments:  KDV1Adjustments{AdditionalVAT: &extra, OtherDeductions: &other, CarriedIn: &carried},
		Carry:        Carry{Source: CarryManual, Amount: d("1000")},
	}
	k, _ := ComputeKDV1(in)
	eq(t, "total vat", k.Totals.TotalVAT, "250")
	eq(t, "payable", k.Totals.Payable, "0")
	eq(t, "carry out", k.Totals.CarryOut, "875") // 1000 + 100 + 25 − 250
}

func TestComputeKDV1Empty(t *testing.T) {
	k, items := ComputeKDV1(KDV1Input{Carry: Carry{Source: CarryNone}})
	if k.Sales == nil || k.Purchases == nil || k.Exemptions == nil {
		t.Fatal("empty slices must not be nil")
	}
	c := codes(items)
	if c["CARRY_UNKNOWN"].Severity != SeverityWarning || c["NO_DATA"].Severity != SeverityInfo {
		t.Errorf("checklist = %+v", items)
	}
}

func TestComputeMUHSGK(t *testing.T) {
	days := int32(30)
	in := MUHSGKInput{
		Run: &PayrollRun{EmployeeCount: 2, IssueCount: 1},
		Lines: []PayrollLine{
			{LineNo: 1, Name: "Ayşe Kaya", SGKNo: "123", DaysWorked: &days, Gross: d("30000"), SGKBase: d("30000"),
				SGKEmployee: d("4200"), UnemploymentEmployee: d("300"), IncomeTaxBase: d("25500"), IncomeTax: d("1500"),
				IncomeTaxExemption: d("2000"), StampTax: d("100"), StampTaxExemption: d("150"), SGKEmployer: d("4650"), UnemploymentEmployer: d("600")},
			{LineNo: 2, Name: "Ali Demir", Gross: d("26000"), SGKBase: d("26000"), SGKEmployee: d("3640"), UnemploymentEmployee: d("260"),
				IncomeTaxBase: d("22100"), IncomeTax: d("1000"), StampTax: d("50"), SGKEmployer: d("4030"), UnemploymentEmployer: d("520"), SGKIncentive: d("1300")},
		},
		Premium:      &Premium{Accrual: d("18000"), Incentive: d("1300"), Payable: d("16700"), DueDate: "2026-11-26"},
		Withholdings: []IncomeWithholding{{DocumentClass: "smm", Rate: d("20"), Count: 1, Base: d("10000"), Tax: d("2000")}},
	}
	m, items := ComputeMUHSGK(in)
	if m.Payroll == nil || m.Payroll.EmployeeCount != 2 || m.Payroll.DaysTotal != 30 {
		t.Fatalf("payroll = %+v", m.Payroll)
	}
	eq(t, "income tax", m.Totals.IncomeTax, "4500") // 1500 + 1000 + 2000 SMM
	eq(t, "stamp", m.Totals.StampTax, "150")
	eq(t, "tax payable", m.Totals.TaxPayable, "4650")
	eq(t, "expected premium", m.Premium.Expected, "18200")
	eq(t, "premium payable", m.Totals.PremiumPayable, "16700")
	if m.Premium.Difference == nil || !m.Premium.Difference.Dec().Equal(d("-200")) {
		t.Errorf("difference = %v", m.Premium.Difference)
	}
	if len(m.Withholdings) != 2 || m.Withholdings[0].Kind != WithholdingWage || m.Withholdings[1].Kind != WithholdingSelfEmployed {
		t.Errorf("withholdings = %+v", m.Withholdings)
	}
	c := codes(items)
	for _, code := range []string{"PAYROLL_ISSUES", "SGK_ACCRUAL_MISMATCH", "MISSING_SGK_NO", "MISSING_DAYS", "MASKED_NATIONAL_IDS", "INCOME_WITHHOLDING"} {
		if _, ok := c[code]; !ok {
			t.Errorf("checklist misses %s", code)
		}
	}

	none, items := ComputeMUHSGK(MUHSGKInput{})
	if none.Payroll != nil || codes(items)["NO_PAYROLL"].Severity != SeverityWarning {
		t.Errorf("no payroll: %+v %+v", none, items)
	}
}

func TestComputeMUHSGKWageBaseAndRate(t *testing.T) {
	// Matrah bordroda var: aynen alınır; oran = (GV + asgari ücret istisnası) / matrah.
	m, items := ComputeMUHSGK(MUHSGKInput{
		Run: &PayrollRun{EmployeeCount: 1},
		Lines: []PayrollLine{{LineNo: 1, Name: "Ayşe Kaya", SGKNo: "1", Gross: d("30000"), SGKEmployee: d("4200"),
			UnemploymentEmployee: d("300"), IncomeTaxBase: d("25500"), IncomeTax: d("1500"), IncomeTaxExemption: d("2325")}},
	})
	eq(t, "base", m.Payroll.IncomeTaxBase, "25500")
	if m.Withholdings[0].Rate != "15" {
		t.Errorf("rate = %q", m.Withholdings[0].Rate)
	}
	if _, ok := codes(items)["INCOME_TAX_BASE_DERIVED"]; ok {
		t.Errorf("base present, derived item unexpected")
	}

	// Matrahı 0 kaydedilmiş eski bordro (#575 demo): brütten hesaplanır, kontrol listesinde söylenir.
	m, items = ComputeMUHSGK(MUHSGKInput{
		Run: &PayrollRun{EmployeeCount: 2},
		Lines: []PayrollLine{
			{LineNo: 1, Name: "Sultan Öztürk", SGKNo: "1", Gross: d("26005.50"), SGKEmployee: d("3640.77"),
				UnemploymentEmployee: d("260.06"), IncomeTax: d("0"), IncomeTaxExemption: d("3315.70")},
			{LineNo: 2, Name: "Mert Can", SGKNo: "2", Gross: d("60000"), SGKEmployee: d("8400"),
				UnemploymentEmployee: d("600"), IncomeTax: d("6885"), IncomeTaxExemption: d("3315.70")},
		},
	})
	eq(t, "derived base", m.Payroll.IncomeTaxBase, "73104.67") // 22104.67 + 51000
	if m.Employees[0].IncomeTaxBase.Dec().String() != "22104.67" {
		t.Errorf("employee base = %v", m.Employees[0].IncomeTaxBase)
	}
	if m.Withholdings[0].Rate != "15–20" {
		t.Errorf("rate = %q", m.Withholdings[0].Rate)
	}
	if codes(items)["INCOME_TAX_BASE_DERIVED"].Count != 2 {
		t.Errorf("checklist = %+v", items)
	}
}

func TestComputeBABS(t *testing.T) {
	in := BABSInput{
		Counterparties: []Counterparty{
			{Direction: "incoming", ID: "1234567890", Name: "Tedarikçi A.Ş.", Count: 3, Amount: d("12000")},
			{Direction: "incoming", ID: "12345678901", Name: "Oğuz Yılmaz", Count: 1, Amount: d("5000")},
			{Direction: "incoming", ID: "1111111111", Name: "Küçük", Count: 1, Amount: d("4999.99")},
			{Direction: "outgoing", ID: "9876543210", Name: "Müşteri Ltd.", Count: 2, Amount: d("8000")},
		},
		Missing: []MissingCounterparty{{Direction: "outgoing", Count: 1, Amount: d("8000")}},
	}
	b, items := ComputeBABS(in)
	if len(b.BA.Rows) != 2 || b.BA.Rows[1].IDType != "tckn" || b.BA.BelowThreshold != 1 {
		t.Fatalf("ba = %+v", b.BA)
	}
	eq(t, "ba amount", b.BA.Amount, "17000")
	if b.BA.DocumentCount != 4 || len(b.BS.Rows) != 1 {
		t.Errorf("ba docs %d, bs rows %d", b.BA.DocumentCount, len(b.BS.Rows))
	}
	c := codes(items)
	if c["MISSING_COUNTERPARTY_Bs"].Severity != SeverityWarning || c["BELOW_THRESHOLD"].Count != 1 {
		t.Errorf("checklist = %+v", items)
	}
}

func TestDueDate(t *testing.T) {
	cases := []struct{ kind, period, want string }{
		{KindKDV1, "2026-10", "2026-11-30"},   // 28 Kasım 2026 cumartesi → pazartesi
		{KindMUHSGK, "2026-10", "2026-11-26"}, // perşembe
		{KindBABS, "2026-10", "2026-11-30"},   // ayın son günü, pazartesi
		{KindBABS, "2027-01", "2027-03-01"},   // 28 Şubat 2027 pazar → 1 Mart
		{KindKDV1, "2026-04", "2026-06-01"},   // 28 Mayıs 2026 Kurban Bayramı → ilk iş günü
		{KindMUHSGK, "2026-04", "2026-05-26"}, // arife yarım gün: kaymaz
	}
	for _, c := range cases {
		got, _, err := DueDate(c.kind, c.period)
		if err != nil || got.Format(time.DateOnly) != c.want {
			t.Errorf("DueDate(%s, %s) = %s, %v; want %s", c.kind, c.period, got.Format(time.DateOnly), err, c.want)
		}
	}
	if _, _, err := DueDate("kdv2", "2026-10"); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	k, items := ComputeKDV1(KDV1Input{
		Invoices: []InvoiceLine{{Direction: "outgoing", Type: "SATIS", Rate: d("20"), Count: 1, Base: d("100"), VAT: d("20")}},
		Carry:    Carry{Source: CarryNone},
	})
	raw, err := json.Marshal(body{Checklist: items, KDV1: &k})
	if err != nil {
		t.Fatal(err)
	}
	var back body
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	eq(t, "payable", back.KDV1.Totals.Payable, "20")
	if back.KDV1.Carry.Source != CarryNone {
		t.Errorf("carry = %+v", back.KDV1.Carry)
	}
}

func TestParseKDV1Adjustments(t *testing.T) {
	if _, err := ParseKDV1Adjustments([]byte(`{"carried_in":"-1"}`)); err == nil {
		t.Error("negative accepted")
	}
	if _, err := ParseKDV1Adjustments([]byte(`{"foo":"1"}`)); err == nil {
		t.Error("unknown field accepted")
	}
	a, err := ParseKDV1Adjustments([]byte(`{"carried_in":"12.5"}`))
	if err != nil || a.CarriedIn == nil || !a.CarriedIn.Dec().Equal(d("12.5")) {
		t.Errorf("parse = %+v, %v", a, err)
	}
}

// KDV indirim dönemi (#640): başka aydan alınan ve sonraki aya aktarılan
// alış faturaları kontrol listesinde bilgi olarak görünür, onay istemez.
func TestComputeKDV1VATDeductionMoved(t *testing.T) {
	_, items := ComputeKDV1(KDV1Input{
		InvoiceStats: InvoiceStats{Included: 1, MovedIn: 2, MovedOut: 1},
		Carry:        Carry{Source: CarryManual},
	})
	c := codes(items)
	in, out := c["VAT_DEDUCTION_MOVED_IN"], c["VAT_DEDUCTION_MOVED_OUT"]
	if in.Severity != SeverityInfo || in.Count != 2 || out.Severity != SeverityInfo || out.Count != 1 {
		t.Errorf("checklist = %+v", items)
	}
	if open := applyAcks(items, nil); len(open) != 0 {
		t.Errorf("info items must not block: %v", open)
	}
}
