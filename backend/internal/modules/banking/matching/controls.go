package matching

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// The 21 §6.3 controls computed from bank data alone. The others need the
// pre-accounting side (expense records, cash payments, payroll, cash_box
// balance) that no adapter delivers yet: BNK_FEES_NOT_BOOKED,
// BNK_SALARY_VS_PAYROLL, BNK_PAYMENT_METHOD_MISMATCH, BNK_CASH_LIMIT,
// BNK_POS_VS_SALES, BNK_BALANCE_MISMATCH. They are not owned here, so this
// producer never auto-resolves them.
const (
	CodeUninvoicedCollection = "BNK_UNINVOICED_COLLECTION"
	CodeUndocumentedPayment  = "BNK_UNDOCUMENTED_PAYMENT"
	CodeInterest             = "BNK_INTEREST"
	CodeTaxPaid              = "BNK_TAX_PAID"
	CodeRentWithholding      = "BNK_RENT_WITHHOLDING"
	CodeNonBusiness          = "BNK_NON_BUSINESS"
	CodeUnclassified         = "BNK_UNCLASSIFIED"
)

// OwnedCodes are the codes this producer raises and resolves.
var OwnedCodes = []string{
	CodeUninvoicedCollection, CodeUndocumentedPayment, CodeInterest, CodeTaxPaid,
	CodeRentWithholding, CodeNonBusiness, CodeUnclassified,
}

// Row is one valid movement of the period.
type Row struct {
	ID             int64
	UUID           string
	Date           string
	Direction      banking.Direction
	Amount         decimal.Decimal
	Currency       string
	Detail         banking.Detail
	Matched        bool
	Counterparty   string
	TaxNumber      string // VKN
	IdentityNumber string // TCKN
	PaymentPurpose string
}

// Finding is one control result (code + params for the panel text).
type Finding struct {
	Code   string
	Params map[string]any
}

type group struct {
	Counterparty string   `json:"counterparty"`
	Currency     string   `json:"currency"`
	Total        string   `json:"total"`
	Count        int      `json:"count"`
	Transactions []string `json:"transactions"`
	total        decimal.Decimal
}

// Controls computes the period findings. The warning texts are produced by
// the panel from the code and params (05).
func Controls(rows []Row) []Finding {
	var out []Finding
	byParty := func(detail banking.Detail) []group {
		m := map[string]*group{}
		for _, r := range rows {
			if r.Detail != detail || r.Matched {
				continue
			}
			key := firstNonEmpty(r.TaxNumber, r.IdentityNumber, r.Counterparty, "?") + "|" + r.Currency
			g := m[key]
			if g == nil {
				g = &group{Counterparty: firstNonEmpty(r.Counterparty, r.TaxNumber, r.IdentityNumber), Currency: r.Currency}
				m[key] = g
			}
			g.total = g.total.Add(r.Amount)
			g.Count++
			g.Transactions = append(g.Transactions, r.UUID)
		}
		gs := make([]group, 0, len(m))
		for _, g := range m {
			g.Total = g.total.StringFixed(2)
			gs = append(gs, *g)
		}
		sort.Slice(gs, func(i, j int) bool { return gs[i].total.GreaterThan(gs[j].total) })
		return gs
	}

	// Faturasız tahsilat / belgesiz ödeme, counterparty by counterparty.
	if gs := byParty(banking.DetailCustomerCollection); len(gs) > 0 {
		out = append(out, Finding{Code: CodeUninvoicedCollection, Params: map[string]any{"groups": gs, "total": sumGroups(gs)}})
	}
	if gs := byParty(banking.DetailSupplierPayment); len(gs) > 0 {
		out = append(out, Finding{Code: CodeUndocumentedPayment, Params: map[string]any{"groups": gs, "total": sumGroups(gs)}})
	}

	// Interest and withholding totals for provisional / income tax.
	in, exp, wht := totals(rows, banking.DetailInterestIncome), totals(rows, banking.DetailInterestExpense), totals(rows, banking.DetailWithholdingTax)
	if in.count+exp.count+wht.count > 0 {
		out = append(out, Finding{Code: CodeInterest, Params: map[string]any{
			"interest_income": in.sum.StringFixed(2), "interest_expense": exp.sum.StringFixed(2),
			"withholding_tax": wht.sum.StringFixed(2),
		}})
	}

	// Tax and SGK payments: the list; matching to accruals is the accountant's.
	tax, sgk := totals(rows, banking.DetailTaxPayment), totals(rows, banking.DetailSGKPayment)
	if tax.count+sgk.count > 0 {
		out = append(out, Finding{Code: CodeTaxPaid, Params: map[string]any{
			"tax_total": tax.sum.StringFixed(2), "sgk_total": sgk.sum.StringFixed(2),
			"transactions": append(tax.ids, sgk.ids...),
		}})
	}

	// Rent paid to an individual (TCKN) or as workplace rent (M05): the
	// withholding must be on the muhtasar.
	var rent []string
	rentSum := decimal.Zero
	for _, r := range rows {
		if r.Detail == banking.DetailRentPayment && r.Direction == banking.DirOut &&
			(r.IdentityNumber != "" || r.PaymentPurpose == "M05") {
			rent = append(rent, r.UUID)
			rentSum = rentSum.Add(r.Amount)
		}
	}
	if len(rent) > 0 {
		out = append(out, Finding{Code: CodeRentWithholding, Params: map[string]any{"total": rentSum.StringFixed(2), "transactions": rent}})
	}

	if t := totals(rows, banking.DetailPrivate); t.count > 0 {
		out = append(out, Finding{Code: CodeNonBusiness, Params: map[string]any{"total": t.sum.StringFixed(2), "count": t.count, "transactions": t.ids}})
	}
	if t := totals(rows, banking.DetailUnknown); t.count > 0 {
		out = append(out, Finding{Code: CodeUnclassified, Params: map[string]any{"total": t.sum.StringFixed(2), "count": t.count, "transactions": t.ids}})
	}
	return out
}

type tally struct {
	sum   decimal.Decimal
	count int
	ids   []string
}

func totals(rows []Row, d banking.Detail) tally {
	var t tally
	for _, r := range rows {
		if r.Detail == d {
			t.sum = t.sum.Add(r.Amount)
			t.count++
			t.ids = append(t.ids, r.UUID)
		}
	}
	return t
}

func sumGroups(gs []group) string {
	s := decimal.Zero
	for _, g := range gs {
		s = s.Add(g.total)
	}
	return s.StringFixed(2)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
