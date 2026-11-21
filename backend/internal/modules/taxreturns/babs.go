package taxreturns

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Counterparty is one counterparty's invoices of the period (TL, KDV hariç).
// ExpenseCount of Count are purchase documents recorded as expenses (#575).
type Counterparty struct {
	Direction    string
	ID           string
	Name         string
	Count        int64
	ExpenseCount int64
	Amount       decimal.Decimal
}

// ExpenseCounterparty is one vendor's approved purchase documents recorded
// as expenses (paper invoice, serbest meslek makbuzu) of the period.
type ExpenseCounterparty struct {
	ID     string
	Name   string
	Count  int64
	Amount decimal.Decimal
}

// MissingCounterparty counts invoices with no counterparty id.
type MissingCounterparty struct {
	Direction string
	Count     int64
	Amount    decimal.Decimal
}

// BABSInput is everything ComputeBABS needs.
type BABSInput struct {
	Counterparties []Counterparty
	Missing        []MissingCounterparty
	InvoiceStats   InvoiceStats
	// Expenses and ExpensesMissingVendor: purchase documents recorded as
	// expenses (#575); they join Form Ba with the vendor's e-invoices.
	Expenses              []ExpenseCounterparty
	ExpensesMissingVendor MissingCounterparty
}

// BABSRow is one row of Form Ba or Bs.
type BABSRow struct {
	CounterpartyID string `json:"counterparty_id"`
	// IDType: vkn (10 hane) or tckn (11 hane).
	IDType        string `json:"id_type"`
	Name          string `json:"name"`
	DocumentCount int64  `json:"document_count"`
	// ExpenseCount of DocumentCount are purchase documents recorded as
	// expenses (paper invoice, serbest meslek makbuzu), not e-invoices.
	ExpenseCount int64 `json:"expense_count"`
	Amount       Money `json:"amount"`
}

// BABSForm is one form with its totals.
type BABSForm struct {
	Rows []BABSRow `json:"rows"`
	// DocumentCount and Amount sum the reported rows.
	DocumentCount int64 `json:"document_count"`
	Amount        Money `json:"amount"`
	// BelowThreshold: counterparties under the threshold (not reported).
	BelowThreshold int64 `json:"below_threshold"`
}

// BABS is the Form Ba (alışlar) and Bs (satışlar) preview.
type BABS struct {
	Threshold Money    `json:"threshold"`
	BA        BABSForm `json:"ba"`
	BS        BABSForm `json:"bs"`
}

// ComputeBABS builds Form Ba and Bs: per counterparty (VKN / TCKN) the
// period's active invoices; a counterparty is reported when its KDV hariç
// total reaches BABSThreshold. Incoming invoices are Ba, outgoing Bs (a
// return follows its direction: an outgoing IADE is in Bs).
//
// Purchase documents recorded as expenses (paper invoice, serbest meslek
// makbuzu) join Form Ba under the vendor's VKN / TCKN (#575): the threshold
// applies to the vendor's e-invoices and expense documents together.
func ComputeBABS(in BABSInput) (BABS, []CheckItem) {
	in.Counterparties = mergeExpenses(in.Counterparties, in.Expenses)
	out := BABS{Threshold: M(BABSThreshold), BA: BABSForm{Rows: []BABSRow{}}, BS: BABSForm{Rows: []BABSRow{}}}
	var invalid int64
	var baAmt, bsAmt decimal.Decimal
	for _, cp := range in.Counterparties {
		f, amt := &out.BS, &bsAmt
		if cp.Direction == dirIncoming {
			f, amt = &out.BA, &baAmt
		}
		if cp.Amount.LessThan(BABSThreshold) {
			f.BelowThreshold++
			continue
		}
		idType := ""
		switch len(cp.ID) {
		case 10:
			idType = "vkn"
		case 11:
			idType = "tckn"
		default:
			invalid++
		}
		f.Rows = append(f.Rows, BABSRow{CounterpartyID: cp.ID, IDType: idType, Name: cp.Name, DocumentCount: cp.Count,
			ExpenseCount: cp.ExpenseCount, Amount: M(cp.Amount)})
		f.DocumentCount += cp.Count
		*amt = amt.Add(cp.Amount)
	}
	out.BA.Amount, out.BS.Amount = M(baAmt), M(bsAmt)

	var c []CheckItem
	for _, m := range in.Missing {
		if m.Count == 0 {
			continue
		}
		form := "Bs"
		if m.Direction == dirIncoming {
			form = "Ba"
		}
		c = append(c, itemAmount("MISSING_COUNTERPARTY_"+form, SeverityWarning,
			fmt.Sprintf("Form %s: %d faturada karşı tarafın VKN / TCKN'si yok (yabancı olabilir); ülke ve unvanı elle girin.", form, m.Count),
			m.Count, m.Amount))
	}
	if m := in.ExpensesMissingVendor; m.Count > 0 {
		c = append(c, itemAmount("EXPENSE_MISSING_VENDOR_ID", SeverityWarning,
			fmt.Sprintf("Form Ba: masraf olarak işlenen %d alış belgesinde (fatura, serbest meslek makbuzu) satıcının VKN / TCKN'si yok; Ba'ya alınamadı. Masraf kaydına satıcı VKN'sini girin.", m.Count),
			m.Count, m.Amount))
	}
	var expenseDocs int64
	for _, r := range out.BA.Rows {
		expenseDocs += r.ExpenseCount
	}
	if expenseDocs > 0 {
		c = append(c, item("EXPENSES_IN_BA", SeverityInfo,
			fmt.Sprintf("Masraf olarak işlenen %d alış belgesi (fatura, serbest meslek makbuzu) Form Ba'ya eklendi.", expenseDocs), expenseDocs))
	}
	if invalid > 0 {
		c = append(c, item("INVALID_COUNTERPARTY_ID", SeverityWarning,
			fmt.Sprintf("%d karşı tarafın kimlik numarası 10 ya da 11 hane değil.", invalid), invalid))
	}
	if n := in.InvoiceStats.MissingExchangeRate; n > 0 {
		c = append(c, item("MISSING_EXCHANGE_RATE", SeverityWarning,
			fmt.Sprintf("%d döviz faturasında kur yok; formlara alınmadı.", n), n))
	}
	if n := in.InvoiceStats.Draft; n > 0 {
		c = append(c, item("DRAFT_INVOICES", SeverityInfo, fmt.Sprintf("%d taslak fatura formlara alınmadı.", n), n))
	}
	if n := out.BA.BelowThreshold + out.BS.BelowThreshold; n > 0 {
		c = append(c, item("BELOW_THRESHOLD", SeverityInfo,
			fmt.Sprintf("%d karşı taraf 5.000 TL sınırının altında kaldı; bildirilmez.", n), n))
	}
	return out, c
}

// mergeExpenses adds the expense documents to the incoming counterparties:
// same VKN / TCKN → one row (counts and amounts summed, the invoice name
// kept); a vendor with no e-invoice gets its own row.
func mergeExpenses(cps []Counterparty, exps []ExpenseCounterparty) []Counterparty {
	if len(exps) == 0 {
		return cps
	}
	out := make([]Counterparty, len(cps), len(cps)+len(exps))
	copy(out, cps)
	idx := map[string]int{}
	for i, cp := range out {
		if cp.Direction == dirIncoming {
			idx[cp.ID] = i
		}
	}
	for _, e := range exps {
		if i, ok := idx[e.ID]; ok {
			out[i].Count += e.Count
			out[i].ExpenseCount += e.Count
			out[i].Amount = out[i].Amount.Add(e.Amount)
			if out[i].Name == "" {
				out[i].Name = e.Name
			}
			continue
		}
		idx[e.ID] = len(out)
		out = append(out, Counterparty{Direction: dirIncoming, ID: e.ID, Name: e.Name, Count: e.Count, ExpenseCount: e.Count, Amount: e.Amount})
	}
	// Same order as the invoice query: direction, amount (largest first), id.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Direction != b.Direction {
			return a.Direction < b.Direction
		}
		if c := a.Amount.Cmp(b.Amount); c != 0 {
			return c > 0
		}
		return a.ID < b.ID
	})
	return out
}
