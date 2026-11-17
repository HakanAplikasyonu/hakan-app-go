// Package reconciliation is mutabakat (#389): the trial balance (mizan,
// accountplan) compared account by account with what the portal's sources
// (invoices, bank, expenses, Z reports, payroll) put into journal drafts,
// with the source records that are in no draft yet (missing entry
// candidates) and a rule based explanation of each difference; and the
// counterparty (cari) reconciliation with the BA/BS check.
package reconciliation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Source kinds of a journal draft line, as the comparison groups them.
const (
	KindInvoice = "invoice"
	KindBank    = "bank"
	KindExpense = "expense"
	KindZReport = "zreport"
	KindPayroll = "payroll"
	KindManual  = "manual"
)

// Kinds is the display order of the source kinds.
var Kinds = []string{KindInvoice, KindBank, KindExpense, KindZReport, KindPayroll, KindManual}

// SourceKindOf maps a journal_draft_lines.source_kind (and whether its
// source id is a Z report) to a comparison kind.
func SourceKindOf(sourceKind string, retail bool) string {
	switch sourceKind {
	case "invoice", "invoice_item":
		return KindInvoice
	case "bank_transaction":
		return KindBank
	case "receipt":
		if retail {
			return KindZReport
		}
		return KindExpense
	case "payroll":
		return KindPayroll
	}
	return KindManual
}

// Results of an account comparison.
const (
	ResultMatched          = "matched"
	ResultDifference       = "difference"
	ResultMissingInLedger  = "missing_in_ledger"
	ResultMissingInSources = "missing_in_sources"
)

// Statuses of a reconciliation item (the accountant's decision).
const (
	StatusOpen      = "open"
	StatusExplained = "explained"
	StatusInReview  = "in_review"
	StatusResolved  = "resolved"
	StatusIgnored   = "ignored"
)

// ValidDecision reports whether s is a status a person may set directly
// (in_review goes through the review queue).
func ValidDecision(s string) bool {
	switch s {
	case StatusOpen, StatusExplained, StatusResolved, StatusIgnored:
		return true
	}
	return false
}

// Suggestion codes of the explanation.
const (
	SuggestDraftNotExported = "draft_not_exported"
	SuggestUnposted         = "unposted_sources"
	SuggestSingleUnposted   = "single_unposted"
	SuggestRounding         = "rounding"
	SuggestOpeningBalance   = "opening_balance"
	SuggestMissingSource    = "missing_source"
	SuggestNotExported      = "not_in_ledger"
	SuggestTiming           = "timing_or_missing_document"
)

// LedgerLine is one trial balance account (amounts are the file's
// cumulative totals).
type LedgerLine struct {
	Code     string
	Name     string
	MainCode string
	Level    int
	IsLeaf   bool
	Debit    decimal.Decimal
	Credit   decimal.Decimal
	Balance  decimal.Decimal
}

// SourceLine is the total of journal draft lines of one account and kind.
type SourceLine struct {
	Account string
	Kind    string
	// Draft: the lines' draft is not approved / exported yet.
	Draft  bool
	Debit  decimal.Decimal
	Credit decimal.Decimal
	Lines  int64
}

// Unposted is a source record in no journal draft. Effects are what it
// would post, per main account, as debit − credit.
type Unposted struct {
	Kind    string                     `json:"kind"`
	ID      string                     `json:"id"`
	Date    string                     `json:"date"`
	Label   string                     `json:"label"`
	Amount  decimal.Decimal            `json:"amount"`
	Effects map[string]decimal.Decimal `json:"-"`
}

// Input of Compare.
type Input struct {
	Current []LedgerLine
	// Previous is the trial balance subtracted to get the period's
	// movement; nil = the current file's totals are compared as they are
	// (cumulative from the start of the year).
	Previous  []LedgerLine
	Sources   []SourceLine
	Unposted  []Unposted
	Tolerance decimal.Decimal
}

// KindTotal is the per kind breakdown of an item's source side.
type KindTotal struct {
	Kind   string          `json:"kind"`
	Debit  decimal.Decimal `json:"debit"`
	Credit decimal.Decimal `json:"credit"`
	Lines  int64           `json:"lines"`
	// DraftNet is the net (debit − credit) of lines whose draft is not
	// approved yet: not in the accounting program.
	DraftNet decimal.Decimal `json:"draft_net"`
}

// UnpostedRef is an unposted record as it affects one account.
type UnpostedRef struct {
	Kind   string          `json:"kind"`
	ID     string          `json:"id"`
	Date   string          `json:"date"`
	Label  string          `json:"label"`
	Amount decimal.Decimal `json:"amount"`
	// Effect on this account (debit − credit).
	Effect decimal.Decimal `json:"effect"`
}

// SubAccount is one leaf account of the item.
type SubAccount struct {
	Code      string          `json:"code"`
	Name      string          `json:"name"`
	LedgerNet decimal.Decimal `json:"ledger_net"`
	SourceNet decimal.Decimal `json:"source_net"`
}

// Item is the comparison of one main account.
type Item struct {
	MainCode     string
	AccountName  string
	LedgerDebit  decimal.Decimal
	LedgerCredit decimal.Decimal
	SourceDebit  decimal.Decimal
	SourceCredit decimal.Decimal
	// Difference = (ledger debit − credit) − (source debit − credit).
	Difference decimal.Decimal
	Result     string
	Sources    []KindTotal
	Unposted   []UnpostedRef
	// UnpostedTotal is the sum of the unposted effects on the account (all
	// of them, not only the listed ones).
	UnpostedTotal decimal.Decimal
	UnpostedCount int
	SubAccounts   []SubAccount
	Suggestion    string
	SuggestCode   string
	Confidence    decimal.Decimal
}

// LedgerNet is the ledger movement's net.
func (it Item) LedgerNet() decimal.Decimal { return it.LedgerDebit.Sub(it.LedgerCredit) }

// SourceNet is the sources' net.
func (it Item) SourceNet() decimal.Decimal { return it.SourceDebit.Sub(it.SourceCredit) }

// MaxListed caps the unposted records and sub accounts stored per item.
const MaxListed = 50

// Covered are the main accounts the portal's sources are expected to
// reach: a ledger movement on one of them with no source is reported
// (missing_in_sources); others (capital, fixed assets ...) are not.
var Covered = map[string]string{
	"100": "Kasa",
	"102": "Bankalar",
	"108": "Diğer hazır değerler",
	"120": "Alıcılar",
	"153": "Ticari mallar",
	"191": "İndirilecek KDV",
	"320": "Satıcılar",
	"335": "Personele borçlar",
	"360": "Ödenecek vergi ve fonlar",
	"361": "Ödenecek sosyal güvenlik kesintileri",
	"391": "Hesaplanan KDV",
	"600": "Yurt içi satışlar",
	"770": "Genel yönetim giderleri",
}

// expectedSource names the portal source an account's movement usually
// comes from (the explanation of a missing source).
var expectedSource = map[string]string{
	"100": "Z raporları ve nakit masraflar",
	"102": "banka ekstresi",
	"108": "Z raporlarının kart tahsilatları",
	"120": "satış faturaları",
	"153": "alış faturaları",
	"191": "alış faturaları ve masraf fişleri",
	"320": "alış faturaları",
	"335": "bordro",
	"360": "bordro",
	"361": "bordro",
	"391": "satış faturaları ve Z raporları",
	"600": "satış faturaları ve Z raporları",
	"770": "masraf fişleri ve bordro",
}

// MainCodeOf returns the 3 digit main account of code ("" when code does
// not start with one).
func MainCodeOf(code string) string {
	code = strings.TrimSpace(code)
	if len(code) < 3 {
		return ""
	}
	for i := 0; i < 3; i++ {
		if code[i] < '0' || code[i] > '9' {
			return ""
		}
	}
	if len(code) > 3 && code[3] != '.' && (code[3] < '0' || code[3] > '9') {
		return ""
	}
	return code[:3]
}

type acc struct {
	debit, credit decimal.Decimal
}

func leafTotals(lines []LedgerLine) (map[string]acc, map[string]acc, map[string]string) {
	byMain := map[string]acc{}
	byLeaf := map[string]acc{}
	names := map[string]string{}
	for _, l := range lines {
		main := l.MainCode
		if main == "" {
			main = MainCodeOf(l.Code)
		}
		if main == "" {
			continue
		}
		if l.Code == main && l.Name != "" {
			names[main] = l.Name
		}
		if l.Name != "" {
			names[l.Code] = l.Name
		}
		if !l.IsLeaf {
			continue
		}
		a := byMain[main]
		a.debit, a.credit = a.debit.Add(l.Debit), a.credit.Add(l.Credit)
		byMain[main] = a
		b := byLeaf[l.Code]
		b.debit, b.credit = b.debit.Add(l.Debit), b.credit.Add(l.Credit)
		byLeaf[l.Code] = b
	}
	return byMain, byLeaf, names
}

// Compare runs the account comparison. Items are sorted by main code.
func Compare(in Input) []Item {
	tol := in.Tolerance
	if tol.IsNegative() {
		tol = decimal.Zero
	}
	curMain, curLeaf, names := leafTotals(in.Current)
	if in.Previous != nil {
		prevMain, prevLeaf, _ := leafTotals(in.Previous)
		for k, p := range prevMain {
			c := curMain[k]
			curMain[k] = acc{c.debit.Sub(p.debit), c.credit.Sub(p.credit)}
		}
		for k, p := range prevLeaf {
			c := curLeaf[k]
			curLeaf[k] = acc{c.debit.Sub(p.debit), c.credit.Sub(p.credit)}
		}
	}

	type side struct {
		debit, credit decimal.Decimal
		kinds         map[string]*KindTotal
		leaf          map[string]decimal.Decimal
	}
	src := map[string]*side{}
	for _, s := range in.Sources {
		main := MainCodeOf(s.Account)
		if main == "" {
			continue
		}
		sd := src[main]
		if sd == nil {
			sd = &side{kinds: map[string]*KindTotal{}, leaf: map[string]decimal.Decimal{}}
			src[main] = sd
		}
		sd.debit, sd.credit = sd.debit.Add(s.Debit), sd.credit.Add(s.Credit)
		k := sd.kinds[s.Kind]
		if k == nil {
			k = &KindTotal{Kind: s.Kind}
			sd.kinds[s.Kind] = k
		}
		k.Debit, k.Credit, k.Lines = k.Debit.Add(s.Debit), k.Credit.Add(s.Credit), k.Lines+s.Lines
		if s.Draft {
			k.DraftNet = k.DraftNet.Add(s.Debit.Sub(s.Credit))
		}
		code := strings.TrimSpace(s.Account)
		sd.leaf[code] = sd.leaf[code].Add(s.Debit.Sub(s.Credit))
	}

	unposted := map[string][]UnpostedRef{}
	for _, u := range in.Unposted {
		for main, eff := range u.Effects {
			if eff.IsZero() {
				continue
			}
			unposted[main] = append(unposted[main], UnpostedRef{Kind: u.Kind, ID: u.ID, Date: u.Date, Label: u.Label, Amount: u.Amount, Effect: eff})
		}
	}

	mains := map[string]bool{}
	for k := range src {
		mains[k] = true
	}
	for k, a := range curMain {
		if _, ok := Covered[k]; ok && (!a.debit.IsZero() || !a.credit.IsZero()) {
			mains[k] = true
		}
	}
	for k := range unposted {
		if _, ok := Covered[k]; ok {
			mains[k] = true
		}
	}

	out := make([]Item, 0, len(mains))
	for main := range mains {
		l := curMain[main]
		it := Item{MainCode: main, AccountName: names[main], LedgerDebit: l.debit.Round(2), LedgerCredit: l.credit.Round(2)}
		if it.AccountName == "" {
			it.AccountName = Covered[main]
		}
		leafSrc := map[string]decimal.Decimal{}
		if sd := src[main]; sd != nil {
			it.SourceDebit, it.SourceCredit = sd.debit.Round(2), sd.credit.Round(2)
			for _, kind := range Kinds {
				if k := sd.kinds[kind]; k != nil {
					k.Debit, k.Credit, k.DraftNet = k.Debit.Round(2), k.Credit.Round(2), k.DraftNet.Round(2)
					it.Sources = append(it.Sources, *k)
				}
			}
			leafSrc = sd.leaf
		}
		refs := unposted[main]
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].Date != refs[j].Date {
				return refs[i].Date < refs[j].Date
			}
			return refs[i].ID < refs[j].ID
		})
		for _, r := range refs {
			it.UnpostedTotal = it.UnpostedTotal.Add(r.Effect)
		}
		it.UnpostedTotal = it.UnpostedTotal.Round(2)
		it.UnpostedCount = len(refs)
		if len(refs) > MaxListed {
			refs = refs[:MaxListed]
		}
		it.Unposted = refs
		it.SubAccounts = subAccounts(main, curLeaf, leafSrc, names)
		it.Difference = it.LedgerNet().Sub(it.SourceNet()).Round(2)
		ledgerEmpty := it.LedgerDebit.IsZero() && it.LedgerCredit.IsZero()
		sourceEmpty := it.SourceDebit.IsZero() && it.SourceCredit.IsZero()
		switch {
		case it.Difference.Abs().LessThanOrEqual(tol):
			it.Result = ResultMatched
		case ledgerEmpty && !sourceEmpty:
			it.Result = ResultMissingInLedger
		case sourceEmpty && !ledgerEmpty:
			it.Result = ResultMissingInSources
		default:
			it.Result = ResultDifference
		}
		explain(&it, tol, in.Previous == nil)
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MainCode < out[j].MainCode })
	return out
}

func subAccounts(main string, ledger map[string]acc, source map[string]decimal.Decimal, names map[string]string) []SubAccount {
	codes := map[string]bool{}
	for c, a := range ledger {
		if MainCodeOf(c) == main && (!a.debit.IsZero() || !a.credit.IsZero()) {
			codes[c] = true
		}
	}
	for c, v := range source {
		if !v.IsZero() {
			codes[c] = true
		}
	}
	out := make([]SubAccount, 0, len(codes))
	for c := range codes {
		a := ledger[c]
		out = append(out, SubAccount{Code: c, Name: names[c], LedgerNet: a.debit.Sub(a.credit).Round(2), SourceNet: source[c].Round(2)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	if len(out) > MaxListed {
		out = out[:MaxListed]
	}
	return out
}

// RoundingLimit: a difference below it (and above the tolerance) is
// likely rounding.
var RoundingLimit = decimal.NewFromInt(10)

var kindLabels = map[string]string{
	KindInvoice: "fatura", KindBank: "banka hareketi", KindExpense: "masraf fişi",
	KindZReport: "Z raporu", KindPayroll: "bordro", KindManual: "elle fiş",
}

// KindLabel is the Turkish name of a source kind.
func KindLabel(k string) string {
	if l, ok := kindLabels[k]; ok {
		return l
	}
	return k
}

func explain(it *Item, tol decimal.Decimal, cumulative bool) {
	if it.Result == ResultMatched {
		return
	}
	set := func(code string, conf float64, format string, args ...any) {
		it.SuggestCode, it.Confidence, it.Suggestion = code, decimal.NewFromFloat(conf), fmt.Sprintf(format, args...)
	}
	diff := it.Difference
	near := func(v decimal.Decimal) bool { return diff.Sub(v).Abs().LessThanOrEqual(tol) }

	draftNet := decimal.Zero
	for _, k := range it.Sources {
		draftNet = draftNet.Add(k.DraftNet)
	}
	if !draftNet.IsZero() && near(draftNet.Neg()) {
		set(SuggestDraftNotExported, 0.9,
			"Fark, muhasebe programına henüz aktarılmamış fiş taslaklarının tutarına eşit (%s). Taslakları onaylayıp dışa aktarın.",
			Money(draftNet.Abs()))
		return
	}
	if it.UnpostedCount > 0 && !it.UnpostedTotal.IsZero() && near(it.UnpostedTotal) {
		set(SuggestUnposted, 0.85,
			"Fark, portalda fişlenmemiş %d kaydın toplamına eşit (%s). Kayıtlar muhasebe programına doğrudan girilmiş olabilir; portalda da fişleyin.",
			it.UnpostedCount, Money(it.UnpostedTotal.Abs()))
		return
	}
	for _, u := range it.Unposted {
		if near(u.Effect) {
			set(SuggestSingleUnposted, 0.75,
				"Fark, fişlenmemiş tek bir %s ile aynı tutarda: %s %s (%s).",
				KindLabel(u.Kind), u.Date, u.Label, Money(u.Effect.Abs()))
			return
		}
	}
	if diff.Abs().LessThan(RoundingLimit) {
		set(SuggestRounding, 0.6, "Küçük fark (%s): yuvarlama ya da kuruş farkı olabilir.", Money(diff.Abs()))
		return
	}
	if cumulative && it.MainCode < "600" && (!it.LedgerDebit.IsZero() || !it.LedgerCredit.IsZero()) {
		set(SuggestOpeningBalance, 0.5,
			"Karşılaştırma yıl başından birikimli: açılış fişi mizanda var, kaynaklarda yok. Önceki ayın mizanını yükleyin ya da açılış tutarını not edin.")
		return
	}
	switch it.Result {
	case ResultMissingInSources:
		src := expectedSource[it.MainCode]
		if src == "" {
			src = "kaynak belgeler"
		}
		set(SuggestMissingSource, 0.4,
			"Mizanda %s hareket var, portal kaynaklarında karşılığı yok: %s portala yüklenmemiş ya da kayıt elle girilmiş olabilir.",
			Money(it.LedgerNet().Abs()), src)
	case ResultMissingInLedger:
		set(SuggestNotExported, 0.5,
			"Portal kaynaklarında %s hareket var, mizanda bu hesap hareketsiz: fişler muhasebe programına aktarılmamış ya da başka hesaba işlenmiş olabilir.",
			Money(it.SourceNet().Abs()))
	default:
		side := "fazla"
		if diff.IsNegative() {
			side = "eksik"
		}
		set(SuggestTiming, 0.3,
			"Mizan, kaynaklardan %s %s: dönem kayması (belge tarihi), kur farkı ya da portala yüklenmemiş belge olabilir.",
			Money(diff.Abs()), side)
	}
}

// Money formats an amount the Turkish way: 1.234,50 TL.
func Money(d decimal.Decimal) string {
	s := d.Round(2).StringFixed(2)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + frac + " TL"
	if neg {
		return "-" + out
	}
	return out
}
