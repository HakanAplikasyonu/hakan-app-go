package taxreturns

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Counterparty is one counterparty's invoices of the period (TL, KDV hariç).
type Counterparty struct {
	Direction string
	ID        string
	Name      string
	Count     int64
	Amount    decimal.Decimal
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
}

// BABSRow is one row of Form Ba or Bs.
type BABSRow struct {
	CounterpartyID string `json:"counterparty_id"`
	// IDType: vkn (10 hane) or tckn (11 hane).
	IDType        string `json:"id_type"`
	Name          string `json:"name"`
	DocumentCount int64  `json:"document_count"`
	Amount        Money  `json:"amount"`
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
func ComputeBABS(in BABSInput) (BABS, []CheckItem) {
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
		f.Rows = append(f.Rows, BABSRow{CounterpartyID: cp.ID, IDType: idType, Name: cp.Name, DocumentCount: cp.Count, Amount: M(cp.Amount)})
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
