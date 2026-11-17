package taxreturns

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Invoice types (invoices.type) and directions used by the KDV1 rules.
const (
	dirOutgoing = "outgoing"
	dirIncoming = "incoming"

	typeReturn           = "IADE"
	typeExempt           = "ISTISNA"
	typeSpecialBase      = "OZELMATRAH"
	typeExportRegistered = "IHRACKAYITLI"
)

// Exemption groups of the KDV1 "İstisnalar" section, from the GİB KDV
// istisna code list: 2xx kısmi istisna, 3xx tam istisna (351 istisna
// olmayan diğer), 7xx ihraç kaydıyla teslim, 8xx özel matrah.
const (
	ExemptionFull             = "full"
	ExemptionPartial          = "partial"
	ExemptionExportRegistered = "export_registered"
	ExemptionOther            = "other"
)

// ExemptionGroup classifies a GİB exemption code.
func ExemptionGroup(code string) string {
	switch {
	case code == "351" || code == "":
		return ExemptionOther
	case strings.HasPrefix(code, "2"):
		return ExemptionPartial
	case strings.HasPrefix(code, "3"):
		return ExemptionFull
	case strings.HasPrefix(code, "7"):
		return ExemptionExportRegistered
	}
	return ExemptionOther
}

// InvoiceLine is one grouped invoice_tax_summaries row (TL).
type InvoiceLine struct {
	Direction       string
	Type            string
	Rate            decimal.Decimal
	WithholdingCode string
	ExemptionCode   string
	Count           int64
	Base            decimal.Decimal
	VAT             decimal.Decimal
	Withholding     decimal.Decimal
}

// InvoiceStats counts the period's invoices for the checklist.
type InvoiceStats struct {
	Included, Draft, Cancelled, MissingExchangeRate, HeaderEstimate int64
}

// RetailLine is the rate breakdown of approved Z reports (TL).
type RetailLine struct {
	Rate  decimal.Decimal
	Count int64
	Base  decimal.Decimal
	VAT   decimal.Decimal
}

// RetailStats summarises the period's Z reports.
type RetailStats struct {
	Approved, Pending, WithoutBreakdown int64
	WithoutBreakdownBase                decimal.Decimal
	WithoutBreakdownVAT                 decimal.Decimal
	CardTotal                           decimal.Decimal
}

// ExpenseLine is the rate breakdown of approved deductible expenses. A
// negative Rate means the receipt carried no rate.
type ExpenseLine struct {
	Rate  decimal.Decimal
	Count int64
	Base  decimal.Decimal
	VAT   decimal.Decimal
}

// ExpenseStats summarises the period's expenses.
type ExpenseStats struct {
	Pending, ForeignCurrency, NotDeductible int64
}

// KDV1Adjustments are the manual fields of a KDV1 draft (tax_returns
// .adjustments). Nil means "not entered".
type KDV1Adjustments struct {
	// CarriedIn overrides the devreden KDV (önceki dönemden devreden).
	CarriedIn *Money `json:"carried_in,omitempty"`
	// AdditionalVAT: ilave edilecek KDV (ör. indirim hakkı geri alınan).
	AdditionalVAT *Money `json:"additional_vat,omitempty"`
	// OtherDeductions: diğer indirimler.
	OtherDeductions *Money `json:"other_deductions,omitempty"`
}

// Carry sources.
const (
	CarryManual   = "manual"
	CarryPrevious = "previous_return"
	CarryOpening  = "opening"
	CarryNone     = "none"
)

// Carry is the resolved devreden KDV and where it comes from.
type Carry struct {
	Source string          `json:"source"`
	Period *string         `json:"period"`
	Amount decimal.Decimal `json:"-"`
}

// KDV1Input is everything ComputeKDV1 needs; the service loads it.
type KDV1Input struct {
	Invoices     []InvoiceLine
	InvoiceStats InvoiceStats
	Retail       []RetailLine
	RetailStats  RetailStats
	Expenses     []ExpenseLine
	ExpenseStats ExpenseStats
	Adjustments  KDV1Adjustments
	Carry        Carry
}

// SalesRate is one rate row of "Matrah – teslim ve hizmetler".
type SalesRate struct {
	Rate               string `json:"rate"`
	Base               Money  `json:"base"`
	VAT                Money  `json:"vat"`
	InvoiceBase        Money  `json:"invoice_base"`
	InvoiceVAT         Money  `json:"invoice_vat"`
	RetailBase         Money  `json:"retail_base"`
	RetailVAT          Money  `json:"retail_vat"`
	PurchaseReturnBase Money  `json:"purchase_return_base"`
	PurchaseReturnVAT  Money  `json:"purchase_return_vat"`
}

// WithholdingRow is one row of "Kısmi tevkifat uygulanan işlemler".
type WithholdingRow struct {
	Code string `json:"code"`
	Rate string `json:"rate"`
	// Ratio is the tevkifat oranı ("7/10").
	Ratio        string `json:"ratio"`
	InvoiceCount int64  `json:"invoice_count"`
	Base         Money  `json:"base"`
	VAT          Money  `json:"vat"`
	Withheld     Money  `json:"withheld"`
	// DeclaredVAT is the seller's part (vat − withheld), in hesaplanan KDV.
	DeclaredVAT Money `json:"declared_vat"`
}

// SpecialBaseRow is one özel matrah row.
type SpecialBaseRow struct {
	Code         string `json:"code"`
	Rate         string `json:"rate"`
	InvoiceCount int64  `json:"invoice_count"`
	Base         Money  `json:"base"`
	VAT          Money  `json:"vat"`
}

// PurchaseRate is one rate row of "Bu döneme ait indirilecek KDV".
type PurchaseRate struct {
	// Rate is empty when an expense carried no rate.
	Rate         string `json:"rate"`
	Base         Money  `json:"base"`
	VAT          Money  `json:"vat"`
	InvoiceBase  Money  `json:"invoice_base"`
	InvoiceVAT   Money  `json:"invoice_vat"`
	ExpenseBase  Money  `json:"expense_base"`
	ExpenseVAT   Money  `json:"expense_vat"`
	InvoiceCount int64  `json:"invoice_count"`
	ExpenseCount int64  `json:"expense_count"`
}

// ExemptionRow is one row of the "İstisnalar" section.
type ExemptionRow struct {
	Group        string `json:"group"`
	Code         string `json:"code"`
	InvoiceCount int64  `json:"invoice_count"`
	Base         Money  `json:"base"`
	// VAT: for ihraç kayıtlı teslim the computed and deferred (tecil) VAT.
	VAT Money `json:"vat"`
}

// KDV1Totals are the result fields of the form.
type KDV1Totals struct {
	BaseTotal       Money `json:"base_total"`
	CalculatedVAT   Money `json:"calculated_vat"`
	AdditionalVAT   Money `json:"additional_vat"`
	TotalVAT        Money `json:"total_vat"`
	CarriedIn       Money `json:"carried_in"`
	CurrentInput    Money `json:"current_input"`
	SalesReturnsVAT Money `json:"sales_returns_vat"`
	// ResponsibleVAT: alışlarda tevkif edilip KDV2 ile beyan edilen KDV.
	ResponsibleVAT  Money `json:"responsible_vat"`
	OtherDeductions Money `json:"other_deductions"`
	TotalDeductions Money `json:"total_deductions"`
	ExemptBase      Money `json:"exempt_base"`
	DeferredVAT     Money `json:"deferred_vat"`
	Payable         Money `json:"payable"`
	CarryOut        Money `json:"carry_out"`
	// DeliveryTotal: teslim ve hizmetlerin karşılığını teşkil eden bedel
	// (matrah + istisna kapsamındaki teslimler).
	DeliveryTotal Money `json:"delivery_total"`
	// CardSales: Z raporlarında kredi kartıyla tahsil edilen, KDV dahil.
	CardSales Money `json:"card_sales"`
}

// KDV1 is the 1 No.lu KDV beyannamesi preview.
type KDV1 struct {
	Sales        []SalesRate      `json:"sales"`
	Withholdings []WithholdingRow `json:"withholdings"`
	SpecialBases []SpecialBaseRow `json:"special_bases"`
	Purchases    []PurchaseRate   `json:"purchases"`
	Exemptions   []ExemptionRow   `json:"exemptions"`
	Totals       KDV1Totals       `json:"totals"`
	Carry        Carry            `json:"carry"`
	Adjustments  KDV1Adjustments  `json:"adjustments"`
}

var hundred = decimal.NewFromInt(100)

// ComputeKDV1 builds the KDV1 preview. Rules:
//
//   - Sales (outgoing invoices, approved Z reports not linked to an invoice)
//     go to "Matrah" by rate. Outgoing IADE (alıştan iade) is a delivery and
//     joins the sales rows.
//   - An outgoing summary with withholding goes to "Kısmi tevkifat"; the
//     seller declares vat − withheld.
//   - OZELMATRAH goes to "Özel matrah"; its VAT is hesaplanan.
//   - IHRACKAYITLI is in the sales rows and, with its VAT, in the ihraç
//     kayıtlı table: that VAT is deferred (tecil), at most the payable.
//     A summary with rate but no VAT gets base × rate.
//   - Other outgoing summaries with no VAT and an exemption code (or type
//     ISTISNA) go to "İstisnalar" by code group, outside the matrah.
//   - Purchases: incoming invoices and approved deductible expenses.
//     Incoming withholding is split: vat − withheld is "bu döneme ait
//     indirilecek", the withheld part is "sorumlu sıfatıyla beyan edilen"
//     (KDV2) and also deducted. Incoming IADE (satıştan iade) is its own
//     deduction line.
//   - Net = total VAT − deductions; payable after deferral, else carry out.
func ComputeKDV1(in KDV1Input) (KDV1, []CheckItem) {
	out := KDV1{
		Sales: []SalesRate{}, Withholdings: []WithholdingRow{}, SpecialBases: []SpecialBaseRow{},
		Purchases: []PurchaseRate{}, Exemptions: []ExemptionRow{}, Carry: in.Carry, Adjustments: in.Adjustments,
	}
	type salesAcc struct{ ib, iv, rb, rv, pb, pv decimal.Decimal }
	sales := map[string]*salesAcc{}
	salesKey := func(r decimal.Decimal) *salesAcc {
		k := rateString(r)
		if sales[k] == nil {
			sales[k] = &salesAcc{}
		}
		return sales[k]
	}
	type purchAcc struct {
		ib, iv, eb, ev decimal.Decimal
		ic, ec         int64
	}
	purch := map[string]*purchAcc{}
	purchKey := func(k string) *purchAcc {
		if purch[k] == nil {
			purch[k] = &purchAcc{}
		}
		return purch[k]
	}
	type whAcc struct {
		code             string
		rate             decimal.Decimal
		count            int64
		base, vat, wheld decimal.Decimal
	}
	wh := map[string]*whAcc{}
	type exAcc struct {
		group, code string
		count       int64
		base, vat   decimal.Decimal
	}
	ex := map[string]*exAcc{}
	addEx := func(group, code string, l InvoiceLine, vat decimal.Decimal) {
		k := group + "|" + code
		a := ex[k]
		if a == nil {
			a = &exAcc{group: group, code: code}
			ex[k] = a
		}
		a.count += l.Count
		a.base, a.vat = a.base.Add(l.Base), a.vat.Add(vat)
	}
	type spAcc struct {
		code      string
		rate      decimal.Decimal
		count     int64
		base, vat decimal.Decimal
	}
	sp := map[string]*spAcc{}

	var salesReturns, responsible, exportVAT decimal.Decimal
	for _, l := range in.Invoices {
		switch l.Direction {
		case dirOutgoing:
			switch {
			case l.Type == typeExportRegistered:
				vat := l.VAT
				if vat.IsZero() && l.Rate.IsPositive() {
					vat = l.Base.Mul(l.Rate).Div(hundred).Round(2)
				}
				if l.Rate.IsPositive() {
					a := salesKey(l.Rate)
					a.ib, a.iv = a.ib.Add(l.Base), a.iv.Add(vat)
				}
				exportVAT = exportVAT.Add(vat)
				code := l.ExemptionCode
				if code == "" {
					code = "701"
				}
				addEx(ExemptionExportRegistered, code, l, vat)
			case l.Withholding.IsPositive():
				k := l.WithholdingCode + "|" + rateString(l.Rate)
				a := wh[k]
				if a == nil {
					a = &whAcc{code: l.WithholdingCode, rate: l.Rate}
					wh[k] = a
				}
				a.count += l.Count
				a.base, a.vat, a.wheld = a.base.Add(l.Base), a.vat.Add(l.VAT), a.wheld.Add(l.Withholding)
			case l.Type == typeSpecialBase:
				k := l.ExemptionCode + "|" + rateString(l.Rate)
				a := sp[k]
				if a == nil {
					a = &spAcc{code: l.ExemptionCode, rate: l.Rate}
					sp[k] = a
				}
				a.count += l.Count
				a.base, a.vat = a.base.Add(l.Base), a.vat.Add(l.VAT)
			case l.VAT.IsZero() && (l.Type == typeExempt || l.ExemptionCode != ""):
				group := ExemptionGroup(l.ExemptionCode)
				if group == ExemptionExportRegistered {
					group = ExemptionOther
				}
				addEx(group, l.ExemptionCode, l, decimal.Zero)
			case l.Type == typeReturn:
				a := salesKey(l.Rate)
				a.pb, a.pv = a.pb.Add(l.Base), a.pv.Add(l.VAT)
			default:
				a := salesKey(l.Rate)
				a.ib, a.iv = a.ib.Add(l.Base), a.iv.Add(l.VAT)
			}
		case dirIncoming:
			if l.Type == typeReturn {
				salesReturns = salesReturns.Add(l.VAT)
				continue
			}
			a := purchKey(rateString(l.Rate))
			a.ib, a.iv = a.ib.Add(l.Base), a.iv.Add(l.VAT.Sub(l.Withholding))
			a.ic += l.Count
			responsible = responsible.Add(l.Withholding)
		}
	}
	for _, r := range in.Retail {
		a := salesKey(r.Rate)
		a.rb, a.rv = a.rb.Add(r.Base), a.rv.Add(r.VAT)
	}
	if in.RetailStats.WithoutBreakdown > 0 && in.RetailStats.WithoutBreakdownBase.IsPositive() {
		rate := in.RetailStats.WithoutBreakdownVAT.Mul(hundred).Div(in.RetailStats.WithoutBreakdownBase).Round(0)
		a := salesKey(rate)
		a.rb, a.rv = a.rb.Add(in.RetailStats.WithoutBreakdownBase), a.rv.Add(in.RetailStats.WithoutBreakdownVAT)
	}
	for _, e := range in.Expenses {
		key := ""
		switch {
		case !e.Rate.IsNegative():
			key = rateString(e.Rate)
		case e.Base.IsPositive():
			key = rateString(e.VAT.Mul(hundred).Div(e.Base).Round(0))
		}
		a := purchKey(key)
		a.eb, a.ev = a.eb.Add(e.Base), a.ev.Add(e.VAT)
		a.ec += e.Count
	}

	var baseTotal, calculated, currentInput, exemptBase decimal.Decimal
	for _, k := range sortedRates(keys(sales)) {
		a := sales[k]
		base, vat := a.ib.Add(a.rb).Add(a.pb), a.iv.Add(a.rv).Add(a.pv)
		baseTotal, calculated = baseTotal.Add(base), calculated.Add(vat)
		out.Sales = append(out.Sales, SalesRate{
			Rate: k, Base: M(base), VAT: M(vat), InvoiceBase: M(a.ib), InvoiceVAT: M(a.iv),
			RetailBase: M(a.rb), RetailVAT: M(a.rv), PurchaseReturnBase: M(a.pb), PurchaseReturnVAT: M(a.pv),
		})
	}
	whKeys := make([]string, 0, len(wh))
	for k := range wh {
		whKeys = append(whKeys, k)
	}
	sort.Strings(whKeys)
	for _, k := range whKeys {
		a := wh[k]
		declared := a.vat.Sub(a.wheld)
		baseTotal, calculated = baseTotal.Add(a.base), calculated.Add(declared)
		out.Withholdings = append(out.Withholdings, WithholdingRow{
			Code: a.code, Rate: rateString(a.rate), Ratio: ratio(a.wheld, a.vat), InvoiceCount: a.count,
			Base: M(a.base), VAT: M(a.vat), Withheld: M(a.wheld), DeclaredVAT: M(declared),
		})
	}
	spKeys := make([]string, 0, len(sp))
	for k := range sp {
		spKeys = append(spKeys, k)
	}
	sort.Strings(spKeys)
	for _, k := range spKeys {
		a := sp[k]
		baseTotal, calculated = baseTotal.Add(a.base), calculated.Add(a.vat)
		out.SpecialBases = append(out.SpecialBases, SpecialBaseRow{
			Code: a.code, Rate: rateString(a.rate), InvoiceCount: a.count, Base: M(a.base), VAT: M(a.vat),
		})
	}
	pKeys := make([]string, 0, len(purch))
	for k := range purch {
		pKeys = append(pKeys, k)
	}
	pKeys = sortedRates(pKeys)
	for _, k := range pKeys {
		a := purch[k]
		currentInput = currentInput.Add(a.iv).Add(a.ev)
		out.Purchases = append(out.Purchases, PurchaseRate{
			Rate: k, Base: M(a.ib.Add(a.eb)), VAT: M(a.iv.Add(a.ev)), InvoiceBase: M(a.ib), InvoiceVAT: M(a.iv),
			ExpenseBase: M(a.eb), ExpenseVAT: M(a.ev), InvoiceCount: a.ic, ExpenseCount: a.ec,
		})
	}
	exKeys := make([]string, 0, len(ex))
	for k := range ex {
		exKeys = append(exKeys, k)
	}
	groupOrder := map[string]int{ExemptionFull: 0, ExemptionPartial: 1, ExemptionExportRegistered: 2, ExemptionOther: 3}
	sort.Slice(exKeys, func(i, j int) bool {
		a, b := ex[exKeys[i]], ex[exKeys[j]]
		if a.group != b.group {
			return groupOrder[a.group] < groupOrder[b.group]
		}
		return a.code < b.code
	})
	for _, k := range exKeys {
		a := ex[k]
		if a.group != ExemptionExportRegistered {
			exemptBase = exemptBase.Add(a.base)
		}
		out.Exemptions = append(out.Exemptions, ExemptionRow{
			Group: a.group, Code: a.code, InvoiceCount: a.count, Base: M(a.base), VAT: M(a.vat),
		})
	}

	additional := adj(in.Adjustments.AdditionalVAT)
	other := adj(in.Adjustments.OtherDeductions)
	carried := in.Carry.Amount
	totalVAT := calculated.Add(additional)
	deductions := carried.Add(currentInput).Add(salesReturns).Add(responsible).Add(other)
	net := totalVAT.Sub(deductions)
	var deferred, payable, carryOut decimal.Decimal
	if net.IsPositive() {
		deferred = decimal.Min(exportVAT, net)
		payable = net.Sub(deferred)
	} else {
		carryOut = net.Neg()
	}
	out.Totals = KDV1Totals{
		BaseTotal: M(baseTotal), CalculatedVAT: M(calculated), AdditionalVAT: M(additional), TotalVAT: M(totalVAT),
		CarriedIn: M(carried), CurrentInput: M(currentInput), SalesReturnsVAT: M(salesReturns), ResponsibleVAT: M(responsible),
		OtherDeductions: M(other), TotalDeductions: M(deductions), ExemptBase: M(exemptBase),
		DeferredVAT: M(deferred), Payable: M(payable), CarryOut: M(carryOut),
		DeliveryTotal: M(baseTotal.Add(exemptBase)), CardSales: M(in.RetailStats.CardTotal),
	}
	return out, kdv1Checklist(in, out, exportVAT)
}

func kdv1Checklist(in KDV1Input, k KDV1, exportVAT decimal.Decimal) []CheckItem {
	var c []CheckItem
	s := in.InvoiceStats
	if s.MissingExchangeRate > 0 {
		c = append(c, item("MISSING_EXCHANGE_RATE", SeverityWarning,
			fmt.Sprintf("%d döviz faturasında kur yok; beyana alınmadı.", s.MissingExchangeRate), s.MissingExchangeRate))
	}
	if s.HeaderEstimate > 0 {
		c = append(c, item("HEADER_ESTIMATE", SeverityWarning,
			fmt.Sprintf("%d faturanın KDV kırılımı başlık tutarından tahmin edildi; kalemleri kontrol edin.", s.HeaderEstimate), s.HeaderEstimate))
	}
	if in.ExpenseStats.Pending > 0 {
		c = append(c, item("PENDING_EXPENSES", SeverityWarning,
			fmt.Sprintf("%d masraf onay bekliyor; onaylanmadan indirime alınmaz.", in.ExpenseStats.Pending), in.ExpenseStats.Pending))
	}
	if in.RetailStats.Pending > 0 {
		c = append(c, item("PENDING_Z_REPORTS", SeverityWarning,
			fmt.Sprintf("%d Z raporu onay bekliyor; onaylanmadan satışlara alınmaz.", in.RetailStats.Pending), in.RetailStats.Pending))
	}
	if in.RetailStats.WithoutBreakdown > 0 {
		c = append(c, item("Z_WITHOUT_BREAKDOWN", SeverityWarning,
			fmt.Sprintf("%d Z raporunda KDV oranı kırılımı yok; oran toplam tutardan çıkarıldı.", in.RetailStats.WithoutBreakdown), in.RetailStats.WithoutBreakdown))
	}
	if in.Carry.Source == CarryNone {
		c = append(c, item("CARRY_UNKNOWN", SeverityWarning,
			"Önceki dönemin KDV1'i hazırlanmadı ve açılış devri girilmedi; devreden KDV 0 alındı.", 0))
	}
	if s.Draft > 0 {
		c = append(c, item("DRAFT_INVOICES", SeverityInfo,
			fmt.Sprintf("%d taslak fatura beyana alınmadı.", s.Draft), s.Draft))
	}
	if in.ExpenseStats.ForeignCurrency > 0 {
		c = append(c, item("FOREIGN_CURRENCY_EXPENSES", SeverityInfo,
			fmt.Sprintf("%d dövizli masraf beyana alınmadı.", in.ExpenseStats.ForeignCurrency), in.ExpenseStats.ForeignCurrency))
	}
	if in.ExpenseStats.NotDeductible > 0 {
		c = append(c, item("NOT_DEDUCTIBLE_EXPENSES", SeverityInfo,
			fmt.Sprintf("%d masrafın KDV'si indirilemez olarak işaretli; indirime alınmadı.", in.ExpenseStats.NotDeductible), in.ExpenseStats.NotDeductible))
	}
	if exportVAT.IsPositive() {
		c = append(c, itemAmount("EXPORT_REGISTERED", SeverityInfo,
			"İhraç kaydıyla teslim KDV'si tecil edildi; üç ay içinde ihracat gerçekleşmezse tecil kaldırılır.", 0, exportVAT))
	}
	if r := k.Totals.ResponsibleVAT.Dec(); r.IsPositive() {
		c = append(c, itemAmount("RESPONSIBLE_VAT", SeverityInfo,
			"Alışlarda tevkif edilen KDV, 2 No.lu KDV beyannamesiyle ayrıca beyan edilir; ödendiği varsayılarak indirime alındı.", 0, r))
	}
	if s.Included == 0 && in.RetailStats.Approved == 0 && len(in.Expenses) == 0 {
		c = append(c, item("NO_DATA", SeverityInfo, "Bu döneme ait fatura, Z raporu ya da masraf yok; beyan boş verilir.", 0))
	}
	return c
}

func adj(m *Money) decimal.Decimal {
	if m == nil {
		return decimal.Zero
	}
	return m.Dec()
}

// ratio prints the withholding ratio withheld / vat as "n/10" when it is a
// whole number of tenths, else as a decimal.
func ratio(withheld, vat decimal.Decimal) string {
	if !vat.IsPositive() {
		return ""
	}
	r := withheld.Mul(decimal.NewFromInt(10)).Div(vat)
	if n := r.Round(0); r.Sub(n).Abs().LessThan(decimal.RequireFromString("0.05")) {
		return n.String() + "/10"
	}
	return withheld.Div(vat).Round(2).String()
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// sortedRates sorts rate strings numerically; empty (unknown) goes last.
func sortedRates(ks []string) []string {
	sort.Slice(ks, func(i, j int) bool {
		if ks[i] == "" || ks[j] == "" {
			return ks[j] == "" && ks[i] != ""
		}
		a, _ := decimal.NewFromString(ks[i])
		b, _ := decimal.NewFromString(ks[j])
		return a.LessThan(b)
	})
	return ks
}
