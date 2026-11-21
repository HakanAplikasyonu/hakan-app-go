package taxreturns

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// PayrollRun is the period's payroll totals (payroll_runs).
type PayrollRun struct {
	EmployeeCount int32
	IssueCount    int
}

// PayrollLine is one employee row of the period's payroll (TL).
type PayrollLine struct {
	LineNo               int32
	Name                 string
	NationalIDMasked     string
	SGKNo                string
	HireDate             string
	ExitDate             string
	DaysWorked           *int32
	Gross                decimal.Decimal
	SGKBase              decimal.Decimal
	SGKEmployee          decimal.Decimal
	UnemploymentEmployee decimal.Decimal
	IncomeTaxBase        decimal.Decimal
	IncomeTax            decimal.Decimal
	IncomeTaxExemption   decimal.Decimal
	StampTax             decimal.Decimal
	StampTaxExemption    decimal.Decimal
	SGKEmployer          decimal.Decimal
	UnemploymentEmployer decimal.Decimal
	SGKIncentive         decimal.Decimal
}

// Premium is the period's SGK accrual (sgk_premiums).
type Premium struct {
	Accrual   decimal.Decimal
	Incentive decimal.Decimal
	Payable   decimal.Decimal
	Paid      decimal.Decimal
	DueDate   string
}

// IncomeWithholding is income tax withheld on incoming invoices of one
// document class (invoices.income_tax_withholding_amount).
type IncomeWithholding struct {
	DocumentClass string
	Rate          decimal.Decimal
	Count         int64
	Base          decimal.Decimal
	Tax           decimal.Decimal
}

// MUHSGKInput is everything ComputeMUHSGK needs.
type MUHSGKInput struct {
	Run          *PayrollRun
	Lines        []PayrollLine
	Premium      *Premium
	Withholdings []IncomeWithholding
}

// Withholding kinds of the MUHSGK "Vergi bildirimi" rows.
const (
	WithholdingWage         = "wage"
	WithholdingSelfEmployed = "self_employed"
	WithholdingAgricultural = "agricultural"
	WithholdingOther        = "other"
)

// MUHSGKPayroll sums the employee rows.
type MUHSGKPayroll struct {
	EmployeeCount        int32 `json:"employee_count"`
	DaysTotal            int64 `json:"days_total"`
	Gross                Money `json:"gross"`
	IncomeTaxBase        Money `json:"income_tax_base"`
	IncomeTax            Money `json:"income_tax"`
	IncomeTaxExemption   Money `json:"income_tax_exemption"`
	StampTax             Money `json:"stamp_tax"`
	StampTaxExemption    Money `json:"stamp_tax_exemption"`
	SGKBase              Money `json:"sgk_base"`
	SGKEmployee          Money `json:"sgk_employee"`
	UnemploymentEmployee Money `json:"unemployment_employee"`
	SGKEmployer          Money `json:"sgk_employer"`
	UnemploymentEmployer Money `json:"unemployment_employer"`
	SGKIncentive         Money `json:"sgk_incentive"`
}

// MUHSGKWithholding is one row of the tax section.
type MUHSGKWithholding struct {
	Kind  string `json:"kind"`
	Rate  string `json:"rate"`
	Count int64  `json:"count"`
	Base  Money  `json:"base"`
	Tax   Money  `json:"tax"`
}

// MUHSGKEmployee is one row of the employee annex (TCKN masked).
type MUHSGKEmployee struct {
	LineNo             int32  `json:"line_no"`
	Name               string `json:"name"`
	NationalIDMasked   string `json:"national_id_masked"`
	SGKNo              string `json:"sgk_no"`
	HireDate           string `json:"hire_date"`
	ExitDate           string `json:"exit_date"`
	DaysWorked         *int32 `json:"days_worked"`
	Gross              Money  `json:"gross"`
	SGKBase            Money  `json:"sgk_base"`
	IncomeTaxBase      Money  `json:"income_tax_base"`
	IncomeTax          Money  `json:"income_tax"`
	IncomeTaxExemption Money  `json:"income_tax_exemption"`
	StampTax           Money  `json:"stamp_tax"`
	StampTaxExemption  Money  `json:"stamp_tax_exemption"`
}

// MUHSGKPremium compares the payroll with the SGK accrual.
type MUHSGKPremium struct {
	// Expected: işçi + işveren SGK ve işsizlik payları (bordrodan).
	Expected          Money  `json:"expected"`
	ExpectedIncentive Money  `json:"expected_incentive"`
	ExpectedPayable   Money  `json:"expected_payable"`
	Accrual           *Money `json:"accrual"`
	Incentive         *Money `json:"incentive"`
	Payable           *Money `json:"payable"`
	Paid              *Money `json:"paid"`
	Difference        *Money `json:"difference"`
	DueDate           string `json:"due_date"`
}

// MUHSGKTotals are the amounts payable with the return.
type MUHSGKTotals struct {
	IncomeTax      Money `json:"income_tax"`
	StampTax       Money `json:"stamp_tax"`
	TaxPayable     Money `json:"tax_payable"`
	PremiumPayable Money `json:"premium_payable"`
}

// MUHSGK is the muhtasar ve prim hizmet beyannamesi preview.
type MUHSGK struct {
	Payroll      *MUHSGKPayroll      `json:"payroll"`
	Withholdings []MUHSGKWithholding `json:"withholdings"`
	Employees    []MUHSGKEmployee    `json:"employees"`
	Premium      MUHSGKPremium       `json:"premium"`
	Totals       MUHSGKTotals        `json:"totals"`
	// RentPayments: bank movements marked as rent paid without an invoice,
	// and rent-looking ones not marked (#575, rent.go).
	RentPayments []MUHSGKRentPayment `json:"rent_payments"`
}

func withholdingKind(class string) string {
	switch class {
	case "smm":
		return WithholdingSelfEmployed
	case "mustahsil":
		return WithholdingAgricultural
	}
	return WithholdingOther
}

// ComputeMUHSGK builds the MUHSGK preview. Rules:
//
//   - Ücret: the payroll's income tax base and income tax (after the
//     minimum wage exemption) form the wage row; stamp tax is separate.
//   - Ücret dışı: income tax withheld on incoming invoices, by document
//     class (SMM → serbest meslek, müstahsil → zirai kazanç, others).
//   - SGK: the payroll's employee + employer and unemployment shares are
//     compared with the entered accrual (sgk_premiums); a difference over
//     1 TL is a warning. Premium payable = accrual − incentive (the payroll's
//     estimate when no accrual is entered).
func ComputeMUHSGK(in MUHSGKInput) (MUHSGK, []CheckItem) {
	out := MUHSGK{Withholdings: []MUHSGKWithholding{}, Employees: []MUHSGKEmployee{}}
	var p MUHSGKPayroll
	var gross, itb, it, ite, st, ste, sb, se, ue, ser, uer, inc decimal.Decimal
	var missingSGK, missingDays, derivedBase int64
	var rateMin, rateMax decimal.Decimal
	rated := false
	for _, l := range in.Lines {
		base, derived := wageTaxBase(l)
		if derived {
			derivedBase++
		}
		if r, ok := wageRate(l, base); ok {
			if !rated || r.LessThan(rateMin) {
				rateMin = r
			}
			if !rated || r.GreaterThan(rateMax) {
				rateMax = r
			}
			rated = true
		}
		gross, itb, it, ite = gross.Add(l.Gross), itb.Add(base), it.Add(l.IncomeTax), ite.Add(l.IncomeTaxExemption)
		st, ste, sb = st.Add(l.StampTax), ste.Add(l.StampTaxExemption), sb.Add(l.SGKBase)
		se, ue, ser, uer = se.Add(l.SGKEmployee), ue.Add(l.UnemploymentEmployee), ser.Add(l.SGKEmployer), uer.Add(l.UnemploymentEmployer)
		inc = inc.Add(l.SGKIncentive)
		if l.DaysWorked != nil {
			p.DaysTotal += int64(*l.DaysWorked)
		} else {
			missingDays++
		}
		if l.SGKNo == "" {
			missingSGK++
		}
		out.Employees = append(out.Employees, MUHSGKEmployee{
			LineNo: l.LineNo, Name: l.Name, NationalIDMasked: l.NationalIDMasked, SGKNo: l.SGKNo,
			HireDate: l.HireDate, ExitDate: l.ExitDate, DaysWorked: l.DaysWorked,
			Gross: M(l.Gross), SGKBase: M(l.SGKBase), IncomeTaxBase: M(base), IncomeTax: M(l.IncomeTax),
			IncomeTaxExemption: M(l.IncomeTaxExemption), StampTax: M(l.StampTax), StampTaxExemption: M(l.StampTaxExemption),
		})
	}
	totalTax, totalStamp := decimal.Zero, decimal.Zero
	expected := se.Add(ue).Add(ser).Add(uer)
	if in.Run != nil {
		p.EmployeeCount = in.Run.EmployeeCount
		p.Gross, p.IncomeTaxBase, p.IncomeTax, p.IncomeTaxExemption = M(gross), M(itb), M(it), M(ite)
		p.StampTax, p.StampTaxExemption, p.SGKBase = M(st), M(ste), M(sb)
		p.SGKEmployee, p.UnemploymentEmployee, p.SGKEmployer, p.UnemploymentEmployer = M(se), M(ue), M(ser), M(uer)
		p.SGKIncentive = M(inc)
		out.Payroll = &p
		out.Withholdings = append(out.Withholdings, MUHSGKWithholding{
			Kind: WithholdingWage, Rate: wageRateString(rated, rateMin, rateMax), Count: int64(len(in.Lines)), Base: M(itb), Tax: M(it),
		})
		totalTax, totalStamp = it, st
	}
	var otherTax decimal.Decimal
	for _, w := range in.Withholdings {
		out.Withholdings = append(out.Withholdings, MUHSGKWithholding{
			Kind: withholdingKind(w.DocumentClass), Rate: rateString(w.Rate), Count: w.Count, Base: M(w.Base), Tax: M(w.Tax),
		})
		otherTax = otherTax.Add(w.Tax)
	}
	totalTax = totalTax.Add(otherTax)

	out.Premium = MUHSGKPremium{Expected: M(expected), ExpectedIncentive: M(inc), ExpectedPayable: M(expected.Sub(inc))}
	premiumPayable := expected.Sub(inc)
	var diff decimal.Decimal
	if in.Premium != nil {
		a, i, pay, paid := M(in.Premium.Accrual), M(in.Premium.Incentive), M(in.Premium.Payable), M(in.Premium.Paid)
		diff = in.Premium.Accrual.Sub(expected)
		d := M(diff)
		out.Premium.Accrual, out.Premium.Incentive, out.Premium.Payable, out.Premium.Paid = &a, &i, &pay, &paid
		out.Premium.Difference, out.Premium.DueDate = &d, in.Premium.DueDate
		premiumPayable = in.Premium.Payable
	}
	out.Totals = MUHSGKTotals{
		IncomeTax: M(totalTax), StampTax: M(totalStamp), TaxPayable: M(totalTax.Add(totalStamp)), PremiumPayable: M(premiumPayable),
	}

	var c []CheckItem
	if in.Run == nil {
		c = append(c, item("NO_PAYROLL", SeverityWarning,
			"Bu döneme bordro yüklenmedi; çalışan yoksa beyan yalnızca ücret dışı stopajla (ya da boş) verilir.", 0))
	} else {
		if in.Run.IssueCount > 0 {
			c = append(c, item("PAYROLL_ISSUES", SeverityWarning,
				fmt.Sprintf("Bordroda %d satır denetimi bulgusu var (net uyuşmazlığı, TCKN ...).", in.Run.IssueCount), int64(in.Run.IssueCount)))
		}
		if in.Premium == nil {
			c = append(c, item("SGK_ACCRUAL_MISSING", SeverityWarning,
				"Dönemin SGK tahakkuku girilmedi; prim tutarı bordrodan tahmin edildi.", 0))
		} else if diff.Abs().GreaterThan(decimal.NewFromInt(1)) {
			c = append(c, itemAmount("SGK_ACCRUAL_MISMATCH", SeverityWarning,
				"SGK tahakkuku bordrodaki prim toplamından farklı.", 0, diff))
		}
		if missingSGK > 0 {
			c = append(c, item("MISSING_SGK_NO", SeverityWarning,
				fmt.Sprintf("%d çalışanın SGK sicil numarası yok.", missingSGK), missingSGK))
		}
		if derivedBase > 0 {
			c = append(c, item("INCOME_TAX_BASE_DERIVED", SeverityInfo,
				fmt.Sprintf("%d çalışanın gelir vergisi matrahı bordroda yoktu; brüt ücretten SGK ve işsizlik işçi payları düşülerek hesaplandı. Engelli indirimi varsa bordro programındaki matrahla karşılaştırın.", derivedBase), derivedBase))
		}
		if missingDays > 0 {
			c = append(c, item("MISSING_DAYS", SeverityWarning,
				fmt.Sprintf("%d çalışanın prim günü bordroda yok.", missingDays), missingDays))
		}
		c = append(c, item("MASKED_NATIONAL_IDS", SeverityInfo,
			"Çalışan TCKN'leri maskeli saklanır; e-bildirge ekinde tam TCKN bordro programından gelir.", 0))
	}
	if otherTax.IsPositive() {
		c = append(c, itemAmount("INCOME_WITHHOLDING", SeverityInfo,
			"Gelen faturalarda kesilen gelir vergisi (serbest meslek, müstahsil, kira ...) muhtasara eklendi.", 0, otherTax))
	}
	return out, c
}

// wageTaxBase is the line's income tax base. Payroll lines stored before
// the importer derived it (#575) carry 0: the base is then gross minus the
// employee SGK and unemployment shares (GVK 63), and derived is true.
func wageTaxBase(l PayrollLine) (base decimal.Decimal, derived bool) {
	if l.IncomeTaxBase.IsPositive() || !l.Gross.IsPositive() {
		return l.IncomeTaxBase, false
	}
	b := l.Gross.Sub(l.SGKEmployee).Sub(l.UnemploymentEmployee)
	if b.IsNegative() {
		b = decimal.Zero
	}
	return b.Round(2), true
}

// wageRate is the line's tariff rate in percent: income tax before the
// minimum wage exemption over the base, rounded to a whole percent (the
// brackets are 15 / 20 / 27 / 35 / 40).
func wageRate(l PayrollLine, base decimal.Decimal) (decimal.Decimal, bool) {
	if !base.IsPositive() {
		return decimal.Zero, false
	}
	tax := l.IncomeTax.Add(l.IncomeTaxExemption)
	if !tax.IsPositive() {
		return decimal.Zero, false
	}
	return tax.Mul(decimal.NewFromInt(100)).Div(base).Round(0), true
}

// wageRateString: "15", or "15–20" when employees fall in different
// brackets; "" when no line has a tax.
func wageRateString(rated bool, lo, hi decimal.Decimal) string {
	if !rated {
		return ""
	}
	if lo.Equal(hi) {
		return lo.String()
	}
	return lo.String() + "–" + hi.String()
}
