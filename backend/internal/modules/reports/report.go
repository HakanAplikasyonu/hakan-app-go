// Package reports is the mükellef report (issue #386, "mükellef bazlı özel
// ilgi"): one month or one year of a taxpayer's revenue, expenses by
// category, VAT and the income-expense trend, with the period's AI comment
// (summaries, #35). The office reads it on the taxpayer page, prints it as a
// PDF and emails it; the taxpayer sees the same numbers in the portal.
//
// Every number comes from portal data (invoices, approved expenses and
// approved Z reports); amounts are TRY (foreign currency invoices at their
// own exchange rate).
package reports

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Errors.
var (
	ErrInvalid  = errors.New("reports: invalid input")
	ErrNotFound = errors.New("reports: not found")
)

// Kinds of report.
const (
	KindMonth = "month"
	KindYear  = "year"
)

// Queries is the db subset the report reads.
type Queries interface {
	ReportInvoiceMonthly(ctx context.Context, arg db.ReportInvoiceMonthlyParams) ([]db.ReportInvoiceMonthlyRow, error)
	ReportExpenseMonthly(ctx context.Context, arg db.ReportExpenseMonthlyParams) ([]db.ReportExpenseMonthlyRow, error)
	ReportRetailMonthly(ctx context.Context, arg db.ReportRetailMonthlyParams) ([]db.ReportRetailMonthlyRow, error)
	ReportExpenseCategories(ctx context.Context, arg db.ReportExpenseCategoriesParams) ([]db.ReportExpenseCategoriesRow, error)
	ReportLatestSummary(ctx context.Context, arg db.ReportLatestSummaryParams) (db.ReportLatestSummaryRow, error)
	ReportTaxpayer(ctx context.Context, taxpayerID int64) (db.ReportTaxpayerRow, error)
}

// Range is the report's period: one month or one calendar year.
type Range struct {
	Kind string // month | year
	Key  string // 2026-10 | 2026
	// From / To are the report's own months (inclusive, YYYY-MM).
	From, To string
	// PrevFrom / PrevTo are the comparison months (previous month or year).
	PrevFrom, PrevTo string
	// TrendFrom is the first month of the trend (12 months up to To).
	TrendFrom string
}

var (
	monthRe = regexp.MustCompile(`^(19|20)\d{2}-(0[1-9]|1[0-2])$`)
	yearRe  = regexp.MustCompile(`^(19|20)\d{2}$`)
)

// ParseRange reads ?period=YYYY-MM or ?year=YYYY (exactly one).
func ParseRange(period, year string) (Range, error) {
	switch {
	case period != "" && year != "":
		return Range{}, fmt.Errorf("%w: give period or year, not both", ErrInvalid)
	case period != "":
		if !monthRe.MatchString(period) {
			return Range{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
		}
		t, _ := time.Parse("2006-01", period)
		prev := t.AddDate(0, -1, 0).Format("2006-01")
		return Range{Kind: KindMonth, Key: period, From: period, To: period, PrevFrom: prev, PrevTo: prev,
			TrendFrom: t.AddDate(0, -11, 0).Format("2006-01")}, nil
	case year != "":
		if !yearRe.MatchString(year) {
			return Range{}, fmt.Errorf("%w: year must be YYYY", ErrInvalid)
		}
		y, _ := strconv.Atoi(year)
		return Range{Kind: KindYear, Key: year, From: year + "-01", To: year + "-12",
			PrevFrom: fmt.Sprintf("%d-01", y-1), PrevTo: fmt.Sprintf("%d-12", y-1), TrendFrom: year + "-01"}, nil
	default:
		return Range{}, fmt.Errorf("%w: period or year is required", ErrInvalid)
	}
}

// Months lists the months from..to (inclusive).
func Months(from, to string) []string {
	f, err1 := time.Parse("2006-01", from)
	t, err2 := time.Parse("2006-01", to)
	if err1 != nil || err2 != nil || f.After(t) {
		return nil
	}
	var out []string
	for m := f; !m.After(t); m = m.AddDate(0, 1, 0) {
		out = append(out, m.Format("2006-01"))
	}
	return out
}

// Totals are the sums of one range. Amounts are decimal strings (2 places).
type Totals struct {
	// Revenue: sales (outgoing invoices, net of sales returns) + Z reports,
	// excluding VAT.
	Revenue string `json:"revenue"`
	// Expenses: purchases (incoming invoices, net of purchase returns) +
	// approved expenses, excluding VAT.
	Expenses string `json:"expenses"`
	// Net = Revenue − Expenses.
	Net string `json:"net"`
	// VAT: calculated (sales), deductible (purchases + expenses), payable =
	// calculated − deductible (negative: carried forward; carry-over from
	// earlier periods not included).
	VATCalculated    string `json:"vat_calculated"`
	VATDeductible    string `json:"vat_deductible"`
	VATPayable       string `json:"vat_payable"`
	SalesInvoices    int64  `json:"sales_invoices"`
	PurchaseInvoices int64  `json:"purchase_invoices"`
	ExpenseRecords   int64  `json:"expense_records"`
	RetailReports    int64  `json:"retail_reports"`
}

// Change compares the range with the previous one (percent, 1 place); null
// when the previous value is zero.
type Change struct {
	Revenue  *string `json:"revenue"`
	Expenses *string `json:"expenses"`
	Net      *string `json:"net"`
}

// Month is one point of the trend.
type Month struct {
	Period        string `json:"period"`
	Label         string `json:"label"`
	Revenue       string `json:"revenue"`
	Expenses      string `json:"expenses"`
	Net           string `json:"net"`
	VATCalculated string `json:"vat_calculated"`
	VATDeductible string `json:"vat_deductible"`
	VATPayable    string `json:"vat_payable"`
}

// Category is one expense category of the range.
type Category struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Amount  string `json:"amount"`
	Records int64  `json:"records"`
	// Share of total expenses, percent (1 place).
	Share string `json:"share"`
}

// Commentary is the period's AI comment (period summary, #35).
type Commentary struct {
	SummaryUUID uuid.UUID `json:"summary_uuid"`
	Period      string    `json:"period"`
	// Status: approved | draft. A draft is shown to the office only.
	Status string `json:"status"`
	Body   string `json:"body"`
}

// Taxpayer is the report's header.
type Taxpayer struct {
	Title string `json:"title"`
	TaxID string `json:"tax_id"`
}

// Report is the API view.
type Report struct {
	Kind     string   `json:"kind"`
	Period   string   `json:"period"`
	Label    string   `json:"label"`
	Currency string   `json:"currency"`
	Taxpayer Taxpayer `json:"taxpayer"`
	Totals   Totals   `json:"totals"`
	Previous Totals   `json:"previous"`
	Change   Change   `json:"change"`
	// Months: the trend (12 months up to the period, or the year's months).
	Months     []Month     `json:"months"`
	Categories []Category  `json:"categories"`
	Commentary *Commentary `json:"commentary"`
	// Notes explain what the numbers include (shown under the charts).
	Notes       []string  `json:"notes"`
	GeneratedAt time.Time `json:"generated_at"`
}

// Options steer what Build adds besides the numbers.
type Options struct {
	// OrganizationID / EngagementID select the AI comment; zero skips it.
	OrganizationID, EngagementID int64
	// ApprovedOnly hides draft comments (the taxpayer and the email).
	ApprovedOnly bool
}

type sums struct {
	sales, salesRet, purch, purchRet, exp, retail              decimal.Decimal
	salesVAT, salesRetVAT, purchVAT, purchRetVAT, expVAT, rVAT decimal.Decimal
	salesN, purchN, expN, retailN                              int64
}

func (s sums) revenue() decimal.Decimal  { return s.sales.Sub(s.salesRet).Add(s.retail) }
func (s sums) expenses() decimal.Decimal { return s.purch.Sub(s.purchRet).Add(s.exp) }
func (s sums) vatCalc() decimal.Decimal  { return s.salesVAT.Sub(s.salesRetVAT).Add(s.rVAT) }
func (s sums) vatDed() decimal.Decimal   { return s.purchVAT.Sub(s.purchRetVAT).Add(s.expVAT) }

func (s *sums) add(o sums) {
	s.sales, s.salesRet = s.sales.Add(o.sales), s.salesRet.Add(o.salesRet)
	s.purch, s.purchRet = s.purch.Add(o.purch), s.purchRet.Add(o.purchRet)
	s.exp, s.retail = s.exp.Add(o.exp), s.retail.Add(o.retail)
	s.salesVAT, s.salesRetVAT = s.salesVAT.Add(o.salesVAT), s.salesRetVAT.Add(o.salesRetVAT)
	s.purchVAT, s.purchRetVAT = s.purchVAT.Add(o.purchVAT), s.purchRetVAT.Add(o.purchRetVAT)
	s.expVAT, s.rVAT = s.expVAT.Add(o.expVAT), s.rVAT.Add(o.rVAT)
	s.salesN, s.purchN, s.expN, s.retailN = s.salesN+o.salesN, s.purchN+o.purchN, s.expN+o.expN, s.retailN+o.retailN
}

func (s sums) totals() Totals {
	rev, exp := s.revenue(), s.expenses()
	calc, ded := s.vatCalc(), s.vatDed()
	return Totals{Revenue: money(rev), Expenses: money(exp), Net: money(rev.Sub(exp)),
		VATCalculated: money(calc), VATDeductible: money(ded), VATPayable: money(calc.Sub(ded)),
		SalesInvoices: s.salesN, PurchaseInvoices: s.purchN, ExpenseRecords: s.expN, RetailReports: s.retailN}
}

// Build computes the report of taxpayerID for rg.
func Build(ctx context.Context, q Queries, taxpayerID int64, rg Range, opt Options, now time.Time) (Report, error) {
	from := rg.TrendFrom
	if rg.PrevFrom < from {
		from = rg.PrevFrom
	}
	tp, fp, tpd := taxpayerID, from, rg.To
	invs, err := q.ReportInvoiceMonthly(ctx, db.ReportInvoiceMonthlyParams{TaxpayerID: tp, FromPeriod: fp, ToPeriod: tpd})
	if err != nil {
		return Report{}, fmt.Errorf("reports: invoices: %w", err)
	}
	exps, err := q.ReportExpenseMonthly(ctx, db.ReportExpenseMonthlyParams{TaxpayerID: tp, FromPeriod: fp, ToPeriod: tpd})
	if err != nil {
		return Report{}, fmt.Errorf("reports: expenses: %w", err)
	}
	rets, err := q.ReportRetailMonthly(ctx, db.ReportRetailMonthlyParams{TaxpayerID: tp, FromPeriod: fp, ToPeriod: tpd})
	if err != nil {
		return Report{}, fmt.Errorf("reports: retail sales: %w", err)
	}
	cats, err := q.ReportExpenseCategories(ctx, db.ReportExpenseCategoriesParams{TaxpayerID: tp, FromPeriod: rg.From, ToPeriod: rg.To})
	if err != nil {
		return Report{}, fmt.Errorf("reports: categories: %w", err)
	}
	head, err := q.ReportTaxpayer(ctx, taxpayerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("reports: taxpayer: %w", err)
	}

	byMonth := map[string]*sums{}
	at := func(p string) *sums {
		if byMonth[p] == nil {
			byMonth[p] = &sums{}
		}
		return byMonth[p]
	}
	for _, r := range invs {
		s, base, vat := at(r.Period), dec(r.Base), dec(r.Vat)
		switch {
		case r.Direction == "outgoing" && !r.IsReturn:
			s.sales, s.salesVAT, s.salesN = s.sales.Add(base), s.salesVAT.Add(vat), s.salesN+r.Invoices
		case r.Direction == "incoming" && r.IsReturn: // customer returned goods
			s.salesRet, s.salesRetVAT = s.salesRet.Add(base), s.salesRetVAT.Add(vat)
		case r.Direction == "incoming":
			s.purch, s.purchVAT, s.purchN = s.purch.Add(base), s.purchVAT.Add(vat), s.purchN+r.Invoices
		case r.Direction == "outgoing" && r.IsReturn: // returned to the supplier
			s.purchRet, s.purchRetVAT = s.purchRet.Add(base), s.purchRetVAT.Add(vat)
		}
	}
	for _, r := range exps {
		s := at(r.Period)
		s.exp, s.expVAT, s.expN = s.exp.Add(dec(r.Base)), s.expVAT.Add(dec(r.Vat)), s.expN+r.Expenses
	}
	for _, r := range rets {
		s := at(r.Period)
		s.retail, s.rVAT, s.retailN = s.retail.Add(dec(r.Base)), s.rVAT.Add(dec(r.Vat)), s.retailN+r.Reports
	}
	sumOf := func(from, to string) sums {
		var t sums
		for _, m := range Months(from, to) {
			if s := byMonth[m]; s != nil {
				t.add(*s)
			}
		}
		return t
	}
	cur, prev := sumOf(rg.From, rg.To), sumOf(rg.PrevFrom, rg.PrevTo)

	out := Report{Kind: rg.Kind, Period: rg.Key, Label: RangeLabel(rg), Currency: "TRY",
		Taxpayer: Taxpayer{Title: head.Title, TaxID: head.TaxID},
		Totals:   cur.totals(), Previous: prev.totals(), GeneratedAt: now.UTC(),
		Change: Change{
			Revenue:  pct(cur.revenue(), prev.revenue()),
			Expenses: pct(cur.expenses(), prev.expenses()),
			Net:      pct(cur.revenue().Sub(cur.expenses()), prev.revenue().Sub(prev.expenses())),
		},
		Months: []Month{}, Categories: []Category{}}
	for _, m := range Months(rg.TrendFrom, rg.To) {
		s := sums{}
		if byMonth[m] != nil {
			s = *byMonth[m]
		}
		rev, exp, calc, ded := s.revenue(), s.expenses(), s.vatCalc(), s.vatDed()
		out.Months = append(out.Months, Month{Period: m, Label: MonthShort(m), Revenue: money(rev), Expenses: money(exp),
			Net: money(rev.Sub(exp)), VATCalculated: money(calc), VATDeductible: money(ded), VATPayable: money(calc.Sub(ded))})
	}
	out.Categories = categories(cur, cats)
	out.Notes = notes(cur)

	if opt.OrganizationID > 0 && opt.EngagementID > 0 && rg.Kind == KindMonth {
		row, err := q.ReportLatestSummary(ctx, db.ReportLatestSummaryParams{OrganizationID: opt.OrganizationID, EngagementID: opt.EngagementID, Period: rg.Key})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return Report{}, fmt.Errorf("reports: summary: %w", err)
		case opt.ApprovedOnly && row.Status != "approved":
		default:
			out.Commentary = &Commentary{SummaryUUID: row.Uuid, Period: row.Period, Status: row.Status, Body: row.Body}
		}
	}
	return out, nil
}

// categoryLabels names the expense categories. Today the key is the
// document type of an approved expense; when expense categories arrive the
// same table takes their keys, and an unknown key is shown as it is.
var categoryLabels = map[string]string{
	"purchase_invoices": "Alış faturaları",
	"okc_receipt":       "Yazar kasa fişleri",
	"fuel_receipt":      "Akaryakıt",
	"expense_voucher":   "Gider pusulası",
	"smm":               "Serbest meslek makbuzu",
	"bank_fee":          "Banka masrafları",
	"salary":            "Personel",
	"tax_payment":       "Vergi ve harç ödemeleri",
	"other":             "Diğer giderler",
}

// CategoryLabel is the Turkish name of a category key.
func CategoryLabel(key string) string {
	if l, ok := categoryLabels[key]; ok {
		return l
	}
	return key
}

func categories(cur sums, rows []db.ReportExpenseCategoriesRow) []Category {
	type c struct {
		key string
		amt decimal.Decimal
		n   int64
	}
	var list []c
	if p := cur.purch.Sub(cur.purchRet); !p.IsZero() || cur.purchN > 0 {
		list = append(list, c{"purchase_invoices", p, cur.purchN})
	}
	for _, r := range rows {
		list = append(list, c{r.Category, dec(r.Base), r.Records})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].amt.GreaterThan(list[j].amt) })
	total := decimal.Zero
	for _, x := range list {
		total = total.Add(x.amt)
	}
	out := make([]Category, 0, len(list))
	for _, x := range list {
		share := decimal.Zero
		if total.IsPositive() {
			share = x.amt.Div(total).Mul(decimal.NewFromInt(100))
		}
		out = append(out, Category{Key: x.key, Label: CategoryLabel(x.key), Amount: money(x.amt), Records: x.n, Share: share.StringFixed(1)})
	}
	return out
}

func notes(cur sums) []string {
	n := []string{
		"Tutarlar KDV hariç ve TL'dir; döviz faturalar kendi kurlarıyla çevrilir.",
		"Gelir: satış faturaları (iadeler düşülür) ve faturaya bağlanmamış onaylı Z raporları.",
		"Gider: alış faturaları (iadeler düşülür) ve onaylı masraflar.",
		"KDV tahminidir; önceki dönemden devreden KDV dahil değildir.",
	}
	if cur.salesN == 0 && cur.purchN == 0 && cur.expN == 0 && cur.retailN == 0 {
		n = append(n, "Bu dönemde portala kayıtlı veri yok.")
	}
	return n
}

var months = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// MonthName: 2026-10 → "Ekim 2026".
func MonthName(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return p
	}
	return months[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// MonthShort: 2026-10 → "Eki 26".
func MonthShort(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return p
	}
	return string([]rune(months[t.Month()-1])[:3]) + " " + t.Format("06")
}

// RangeLabel names the range in Turkish.
func RangeLabel(rg Range) string {
	if rg.Kind == KindYear {
		return rg.Key + " yılı"
	}
	return MonthName(rg.Key)
}

func pct(now, was decimal.Decimal) *string {
	if was.IsZero() {
		return nil
	}
	s := now.Sub(was).Div(was.Abs()).Mul(decimal.NewFromInt(100)).StringFixed(1)
	return &s
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}
