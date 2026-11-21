package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Cari mizanı (#780 C): the current accounts (alıcılar, satıcılar, avanslar,
// ortaklar, personel) one by one with their debit, credit and balance, grouped
// under their main account. It is built from the same journal lines as the
// live trial balance, so each group's total is that main account's line in
// the trial balance.

// CariMainCodes are the current-account main accounts of the Turkish chart.
var CariMainCodes = []string{
	"120", "121", "126", "127", "131", "132", "133", "135", "136", "159",
	"195", "196", "320", "321", "326", "329", "331", "332", "333", "335", "336", "340",
}

// cariMainNames are the TDHP names of CariMainCodes, used when the
// engagement's plan does not name the main account.
var cariMainNames = map[string]string{
	"120": "Alıcılar", "121": "Alacak senetleri", "126": "Verilen depozito ve teminatlar",
	"127": "Diğer ticari alacaklar", "131": "Ortaklardan alacaklar", "132": "İştiraklerden alacaklar",
	"133": "Bağlı ortaklıklardan alacaklar", "135": "Personelden alacaklar", "136": "Diğer çeşitli alacaklar",
	"159": "Verilen sipariş avansları", "195": "İş avansları", "196": "Personel avansları",
	"320": "Satıcılar", "321": "Borç senetleri", "326": "Alınan depozito ve teminatlar",
	"329": "Diğer ticari borçlar", "331": "Ortaklara borçlar", "332": "İştiraklere borçlar",
	"333": "Bağlı ortaklıklara borçlar", "335": "Personele borçlar", "336": "Diğer çeşitli borçlar",
	"340": "Alınan sipariş avansları",
}

// IsCariCode reports whether code is under a current-account main account.
func IsCariCode(code string) bool {
	_, ok := cariMainNames[mainOf(code)]
	return ok
}

func mainOf(code string) string {
	if i := strings.IndexByte(code, '.'); i >= 0 {
		return code[:i]
	}
	return code
}

// CariInfo names a cari sub account (from the plan, the generated sub
// accounts or the cari hafızası) and gives its VKN / TCKN when known.
type CariInfo struct {
	Name  string
	TaxID string
}

// Balance sides.
const (
	SideDebit  = "debit"
	SideCredit = "credit"
	SideZero   = "zero"
)

// CariAccount is one current account's line in the cari mizanı.
type CariAccount struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	TaxID   *string `json:"tax_id"`
	Debit   string  `json:"debit"`
	Credit  string  `json:"credit"`
	Balance string  `json:"balance"`
	// Side: debit (borç bakiyesi), credit (alacak bakiyesi) or zero (kapandı).
	Side string `json:"side"`
	// Share is the account's part of its group's balance, in percent, for
	// accounts whose balance is on the group's side; null otherwise.
	Share *string `json:"share"`
	// Reverse: the balance is on the other side than the main account
	// normally carries (a customer with a credit balance in 120: avans ya da
	// fazla tahsilat; a supplier with a debit balance in 320).
	Reverse bool `json:"reverse"`
	// Unassigned: amounts written on the main account itself (or on a code
	// with sub accounts) and not on a single cari.
	Unassigned bool `json:"unassigned"`
}

// CariGroup is a main account with its current accounts. Debit, credit and
// balance equal the main account's line in the live trial balance.
type CariGroup struct {
	MainCode string        `json:"main_code"`
	Name     string        `json:"name"`
	Debit    string        `json:"debit"`
	Credit   string        `json:"credit"`
	Balance  string        `json:"balance"`
	Side     string        `json:"side"`
	Count    int           `json:"count"`
	Reverse  int           `json:"reverse_count"`
	Accounts []CariAccount `json:"accounts"`
}

// CariTotals sums every group.
type CariTotals struct {
	Debit         string `json:"debit"`
	Credit        string `json:"credit"`
	DebitBalance  string `json:"debit_balance"`
	CreditBalance string `json:"credit_balance"`
	Count         int    `json:"count"`
	Reverse       int    `json:"reverse_count"`
	Unassigned    int    `json:"unassigned_count"`
}

// CariReport is the cari mizanı of a period: January of its year to period.
type CariReport struct {
	Period        string      `json:"period"`
	FromPeriod    string      `json:"from_period"`
	IncludeDrafts bool        `json:"include_drafts"`
	Groups        []CariGroup `json:"groups"`
	Totals        CariTotals  `json:"totals"`
}

// creditNormal: main accounts that normally carry a credit balance (borçlar
// and alınan avanslar); the rest are receivables / advances given.
func creditNormal(main string) bool { return main >= "300" }

func side(b decimal.Decimal) string {
	switch b.Sign() {
	case 1:
		return SideDebit
	case -1:
		return SideCredit
	}
	return SideZero
}

// CariBalances builds the period's cari mizanı from the engagement's journal
// entries (approved and exported; drafts too when asked).
func (s *Service) CariBalances(ctx context.Context, sub Subject, period string, includeDrafts bool) (CariReport, error) {
	if !ValidPeriod(period) {
		return CariReport{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	from := period[:4] + "-01"
	totals, err := s.store.LiveTotals(ctx, sub, from, period, includeDrafts)
	if err != nil {
		return CariReport{}, err
	}
	report := CariReport{Period: period, FromPeriod: from, IncludeDrafts: includeDrafts}
	accounts := CariAccounts(totals)
	codes := make([]string, 0, len(accounts)+len(CariMainCodes))
	codes = append(codes, CariMainCodes...)
	for _, a := range accounts {
		codes = append(codes, a.Code)
	}
	info, err := s.store.CariInfo(ctx, sub, codes)
	if err != nil {
		return CariReport{}, err
	}
	report.Groups, report.Totals = cariGroups(accounts, info)
	return report, nil
}

// cariPosted is one posted cari code's sums.
type cariPosted struct {
	Code          string
	Debit, Credit decimal.Decimal
	Unassigned    bool
}

// CariAccounts keeps the posted codes under a cari main account, summed by
// normalized code. A code that has posted sub codes ("120" next to
// "120.01") is unassigned: its own amounts are on no single cari.
func CariAccounts(totals []LiveTotal) []cariPosted {
	byCode := map[string]*cariPosted{}
	for _, t := range totals {
		code := NormalizeCode(t.Code)
		if code == "" || !IsCariCode(code) {
			continue
		}
		p, ok := byCode[code]
		if !ok {
			p = &cariPosted{Code: code}
			byCode[code] = p
		}
		p.Debit, p.Credit = p.Debit.Add(t.Debit), p.Credit.Add(t.Credit)
	}
	out := make([]cariPosted, 0, len(byCode))
	for code, p := range byCode {
		prefix := code + "."
		for c := range byCode {
			if strings.HasPrefix(c, prefix) {
				p.Unassigned = true
				break
			}
		}
		// A line on the bare main account belongs to no cari.
		if !strings.Contains(code, ".") {
			p.Unassigned = true
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return codeLess(out[i].Code, out[j].Code) })
	return out
}

func cariGroups(accounts []cariPosted, info map[string]CariInfo) ([]CariGroup, CariTotals) {
	var (
		groups []CariGroup
		tot    CariTotals
		td, tc decimal.Decimal
		db, cb decimal.Decimal
	)
	byMain := map[string][]cariPosted{}
	var mains []string
	for _, a := range accounts {
		m := mainOf(a.Code)
		if _, ok := byMain[m]; !ok {
			mains = append(mains, m)
		}
		byMain[m] = append(byMain[m], a)
	}
	sort.Strings(mains)
	for _, m := range mains {
		g := CariGroup{MainCode: m, Name: info[m].Name, Accounts: []CariAccount{}}
		if g.Name == "" {
			g.Name = cariMainNames[m]
		}
		var gd, gc decimal.Decimal
		for _, a := range byMain[m] {
			gd, gc = gd.Add(a.Debit), gc.Add(a.Credit)
		}
		gb := gd.Sub(gc)
		// Shares are taken against the sum of balances on the group's side,
		// so they add up to 100 even when some caris are reversed.
		var sideSum decimal.Decimal
		for _, a := range byMain[m] {
			if b := a.Debit.Sub(a.Credit); b.Sign() != 0 && b.Sign() == gb.Sign() {
				sideSum = sideSum.Add(b)
			}
		}
		for _, a := range byMain[m] {
			b := a.Debit.Sub(a.Credit)
			ca := CariAccount{Code: a.Code, Name: info[a.Code].Name, Debit: a.Debit.StringFixed(2), Credit: a.Credit.StringFixed(2),
				Balance: b.StringFixed(2), Side: side(b), Unassigned: a.Unassigned}
			if t := info[a.Code].TaxID; t != "" {
				ca.TaxID = &t
			}
			if ca.Name == "" && a.Unassigned {
				ca.Name = g.Name
			}
			if b.Sign() != 0 && b.Sign() == gb.Sign() && !sideSum.IsZero() {
				sh := b.Div(sideSum).Mul(decimal.NewFromInt(100)).StringFixed(1)
				ca.Share = &sh
			}
			ca.Reverse = b.Sign() != 0 && !a.Unassigned && (b.Sign() > 0) == creditNormal(m)
			if ca.Reverse {
				g.Reverse++
			}
			if a.Unassigned {
				tot.Unassigned++
			} else {
				g.Count++
			}
			if b.Sign() > 0 {
				db = db.Add(b)
			} else {
				cb = cb.Add(b.Neg())
			}
			g.Accounts = append(g.Accounts, ca)
		}
		g.Debit, g.Credit, g.Balance, g.Side = gd.StringFixed(2), gc.StringFixed(2), gb.StringFixed(2), side(gb)
		td, tc = td.Add(gd), tc.Add(gc)
		tot.Count += g.Count
		tot.Reverse += g.Reverse
		groups = append(groups, g)
	}
	tot.Debit, tot.Credit = td.StringFixed(2), tc.StringFixed(2)
	tot.DebitBalance, tot.CreditBalance = db.StringFixed(2), cb.StringFixed(2)
	if groups == nil {
		groups = []CariGroup{}
	}
	return groups, tot
}
