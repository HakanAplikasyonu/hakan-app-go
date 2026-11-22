package taxreturns

import (
	"testing"

	"github.com/shopspring/decimal"
)

func pd(s string) *decimal.Decimal { v := d(s); return &v }

// foreignLines: yazılım lisansı (100 USD, %15 stopaj, brütleştirme, kur
// 34,50 → matrah 4.058,93; stopaj 608,84; KDV2 811,79), reklam (matrah
// 1.000; KDV2 200), kursuz onaylı bir lisans ve onaysız bir reklam.
func foreignLines() []ForeignServiceLine {
	return []ForeignServiceLine{
		{Status: "confirmed", Treatment: "kdv2_stopaj", Kind: "yazilim_lisans", VATRate: d("20"), StopajRate: d("15"),
			GrossTRY: pd("4058.93"), KDV2TRY: pd("811.79"), StopajTRY: pd("608.84")},
		{Status: "confirmed", Treatment: "kdv2", Kind: "reklam", VATRate: d("20"),
			GrossTRY: pd("600"), KDV2TRY: pd("120"), StopajTRY: pd("0")},
		{Status: "confirmed", Treatment: "kdv2", Kind: "reklam", VATRate: d("20"),
			GrossTRY: pd("400"), KDV2TRY: pd("80"), StopajTRY: pd("0")},
		{Status: "confirmed", Treatment: "kdv2_stopaj", Kind: "yazilim_lisans", VATRate: d("20"), StopajRate: d("15")},
		{Status: "draft", Treatment: "kdv2", Kind: "reklam", VATRate: d("20"),
			GrossTRY: pd("50"), KDV2TRY: pd("10"), StopajTRY: pd("0")},
	}
}

func TestComputeKDV2Foreign(t *testing.T) {
	in := KDV2Input{
		Invoices: []InvoiceLine{{Direction: dirIncoming, Type: "SATIS", Rate: d("20"), WithholdingCode: "603",
			Count: 1, Base: d("1000"), VAT: d("200"), Withholding: d("140")}},
		Foreign: foreignLines(),
	}
	k, items := ComputeKDV2(in)
	if len(k.Rows) != 3 {
		t.Fatalf("rows = %+v", k.Rows)
	}
	if k.Rows[0].Code != "603" || k.Rows[0].Source != "" {
		t.Errorf("domestic row first and unchanged: %+v", k.Rows[0])
	}
	ad, lic := k.Rows[1], k.Rows[2]
	if ad.Source != SourceForeignService || ad.Code != "reklam" || ad.InvoiceCount != 2 || ad.Rate != "20" {
		t.Errorf("ad row = %+v", ad)
	}
	eq(t, "ad base", ad.Base, "1000")
	eq(t, "ad kdv2", ad.Withheld, "200")
	if lic.Code != "yazilim_lisans" || lic.InvoiceCount != 1 {
		t.Errorf("licence row = %+v", lic)
	}
	eq(t, "licence base", lic.Base, "4058.93")
	eq(t, "licence kdv2", lic.VAT, "811.79")
	eq(t, "total base", k.Totals.Base, "6058.93")
	eq(t, "payable", k.Totals.Payable, "1151.79") // 140 + 200 + 811,79
	c := codes(items)
	if c["FOREIGN_MISSING_RATE"].Count != 1 || c["FOREIGN_UNCONFIRMED"].Count != 1 {
		t.Errorf("checks = %+v", items)
	}
	if _, ok := c["FOREIGN_SERVICE_KDV2"]; !ok {
		t.Errorf("foreign info missing: %+v", items)
	}

	// Yalnız yabancı fatura: beyan verilir.
	k, items = ComputeKDV2(KDV2Input{Foreign: foreignLines()[:1]})
	if len(k.Rows) != 1 {
		t.Fatalf("rows = %+v", k.Rows)
	}
	if _, ok := codes(items)["NO_WITHHOLDING"]; ok {
		t.Errorf("no-withholding unexpected")
	}
}

func TestComputeMUHSGKForeign(t *testing.T) {
	m, items := ComputeMUHSGK(MUHSGKInput{
		Withholdings: []IncomeWithholding{{DocumentClass: "smm", Rate: d("20"), Count: 1, Base: d("10000"), Tax: d("2000")}},
		Foreign:      foreignLines(),
	})
	if len(m.Withholdings) != 2 {
		t.Fatalf("withholdings = %+v", m.Withholdings)
	}
	w := m.Withholdings[1]
	if w.Kind != WithholdingForeignService || w.Rate != "15" || w.Count != 1 {
		t.Errorf("foreign row = %+v", w)
	}
	eq(t, "foreign base", w.Base, "4058.93")
	eq(t, "foreign tax", w.Tax, "608.84")
	eq(t, "income tax", m.Totals.IncomeTax, "2608.84")
	eq(t, "tax payable", m.Totals.TaxPayable, "2608.84")
	c := codes(items)
	if c["FOREIGN_MISSING_RATE"].Count != 1 {
		t.Errorf("missing rate check = %+v", items)
	}
	if _, ok := c["FOREIGN_UNCONFIRMED"]; ok {
		t.Errorf("draft row is kdv2 only, no stopaj check expected")
	}
}
