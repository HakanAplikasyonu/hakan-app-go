package taxreturns

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// KDV2Input is everything ComputeKDV2 needs: the period's grouped invoice
// lines (the same rows as KDV1).
type KDV2Input struct {
	Invoices     []InvoiceLine
	InvoiceStats InvoiceStats
}

// KDV2Row is one row of "Tevkifat uygulanan işlemler": alışlarda tevkif
// edilen KDV, işlem türü (tevkifat kodu) ve oranına göre.
type KDV2Row struct {
	Code string `json:"code"`
	Rate string `json:"rate"`
	// Ratio is the tevkifat oranı ("9/10").
	Ratio        string `json:"ratio"`
	InvoiceCount int64  `json:"invoice_count"`
	Base         Money  `json:"base"`
	VAT          Money  `json:"vat"`
	Withheld     Money  `json:"withheld"`
}

// KDV2Totals are the declaration totals.
type KDV2Totals struct {
	Base Money `json:"base"`
	VAT  Money `json:"vat"`
	// Payable is the tevkif edilen KDV: ödenecek KDV of the 2 No.lu beyan.
	Payable Money `json:"payable"`
}

// KDV2 is the 2 No.lu KDV beyannamesi preview (#575).
type KDV2 struct {
	Rows   []KDV2Row  `json:"rows"`
	Totals KDV2Totals `json:"totals"`
}

// ComputeKDV2 builds the 2 No.lu KDV beyannamesi: the taxpayer, as the
// buyer, withheld part of the seller's VAT (KDV tevkifatı) and declares it
// "sorumlu sıfatıyla". Rules:
//
//   - Incoming invoices with a withholding amount, except returns (an
//     incoming IADE is a sales return), grouped by withholding code and
//     VAT rate.
//   - Payable = the withheld VAT. The same amount is deducted in KDV1
//     ("sorumlu sıfatıyla beyan edilen KDV"), so both returns are filed
//     together.
func ComputeKDV2(in KDV2Input) (KDV2, []CheckItem) {
	type acc struct {
		code                string
		rate                decimal.Decimal
		count               int64
		base, vat, withheld decimal.Decimal
	}
	groups := map[string]*acc{}
	var missingCode int64
	for _, l := range in.Invoices {
		if l.Direction != dirIncoming || l.Type == typeReturn || !l.Withholding.IsPositive() {
			continue
		}
		if l.WithholdingCode == "" {
			missingCode += l.Count
		}
		k := l.WithholdingCode + "|" + rateString(l.Rate)
		a := groups[k]
		if a == nil {
			a = &acc{code: l.WithholdingCode, rate: l.Rate}
			groups[k] = a
		}
		a.count += l.Count
		a.base, a.vat, a.withheld = a.base.Add(l.Base), a.vat.Add(l.VAT), a.withheld.Add(l.Withholding)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := KDV2{Rows: []KDV2Row{}}
	var base, vat, withheld decimal.Decimal
	for _, k := range keys {
		a := groups[k]
		out.Rows = append(out.Rows, KDV2Row{
			Code: a.code, Rate: rateString(a.rate), Ratio: ratio(a.withheld, a.vat), InvoiceCount: a.count,
			Base: M(a.base), VAT: M(a.vat), Withheld: M(a.withheld),
		})
		base, vat, withheld = base.Add(a.base), vat.Add(a.vat), withheld.Add(a.withheld)
	}
	out.Totals = KDV2Totals{Base: M(base), VAT: M(vat), Payable: M(withheld)}

	var c []CheckItem
	if len(out.Rows) == 0 {
		c = append(c, item("NO_WITHHOLDING", SeverityInfo,
			"Bu dönem tevkifatlı alış faturası yok; 2 No.lu KDV beyannamesi verilmez.", 0))
	} else {
		c = append(c, itemAmount("KDV1_RESPONSIBLE", SeverityInfo,
			"Tevkif edilen KDV, 1 No.lu beyanda \"sorumlu sıfatıyla beyan edilen KDV\" olarak indirilir; iki beyanı birlikte verin.", 0, withheld))
	}
	if missingCode > 0 {
		c = append(c, item("MISSING_WITHHOLDING_CODE", SeverityWarning,
			fmt.Sprintf("%d tevkifatlı alış faturasında tevkifat kodu (işlem türü) yok; beyanda işlem türünü elle seçin.", missingCode), missingCode))
	}
	if n := in.InvoiceStats.MissingExchangeRate; n > 0 {
		c = append(c, item("MISSING_EXCHANGE_RATE", SeverityWarning,
			fmt.Sprintf("%d döviz faturasında kur yok; beyana alınmadı.", n), n))
	}
	if n := in.InvoiceStats.Draft; n > 0 {
		c = append(c, item("DRAFT_INVOICES", SeverityInfo, fmt.Sprintf("%d taslak fatura beyana alınmadı.", n), n))
	}
	return out, c
}
