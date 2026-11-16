//go:build ignore

// Mizan hafızası fikstürleri (#310). Yeniden üretmek için backend/ içinde:
//
//	go run testdata/mizan/generate.go
//
// Üretir:
//   - oguz-luca-mizan.csv / .xlsx: Luca'dan alınmış gibi anonim bir mizan
//     (120/150/153/191/320/391/600 alt hesapları, birkaç cari).
//   - ubl/*.xml: Oğuz Gıda'nın üç karşı firmayla iki aylık (2026-10, 2026-11)
//     e-faturaları.
//
// Senaryo:
//   - Korkmaz Gıda (müşteri) planda 120.01.006: cari önerisi mevcut hesap.
//   - Deniz Ambalaj (satıcı) planda 320.01.002. Kalemleri varsayılan 153'e
//     düşer, müşavir 150.01'e düzeltir → kural adayı → terfi → ikinci ayda
//     otomatik.
//   - Yeni Lojistik (satıcı) planda yok: sıradaki boş 320.01.007 önerilir ve
//     "açılmalı" işaretlenir.
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/xuri/excelize/v2"
)

const dir = "testdata/mizan"

// Taxpayer is the mükellef whose plan and invoices these are.
var Taxpayer = Party{Name: "Oğuz Gıda Pazarlama Ltd. Şti.", VKN: vkn("473007455")}

type Party struct{ Name, VKN string }

var (
	korkmaz = Party{Name: "Korkmaz Gıda San. ve Tic. A.Ş.", VKN: vkn("561002343")}
	deniz   = Party{Name: "Deniz Ambalaj Ltd. Şti.", VKN: vkn("298105617")}
	yeni    = Party{Name: "Yeni Lojistik Taşımacılık A.Ş.", VKN: vkn("904410028")}
)

// vkn appends the 10th (check) digit to a 9-digit base (GİB algorithm).
func vkn(base string) string {
	sum := 0
	for i := 0; i < 9; i++ {
		d := int(base[i] - '0')
		t := (d + 9 - i) % 10
		if t == 0 {
			continue
		}
		v := (t * (1 << (9 - i))) % 9
		if v == 0 {
			v = 9
		}
		sum += v
	}
	return base + strconv.Itoa((10-sum%10)%10)
}

var plan = [][]string{
	{"100", "KASA", "125.400,00", "118.250,00", "7.150,00", ""},
	{"100.01", "MERKEZ KASA TL", "125.400,00", "118.250,00", "7.150,00", ""},
	{"102", "BANKALAR", "1.450.300,00", "1.312.800,00", "137.500,00", ""},
	{"102.01", "ZİRAAT BANKASI TL", "1.450.300,00", "1.312.800,00", "137.500,00", ""},
	{"120", "ALICILAR", "842.100,00", "760.400,00", "81.700,00", ""},
	{"120.01", "YURTİÇİ ALICILAR", "842.100,00", "760.400,00", "81.700,00", ""},
	{"120.01.001", "AKDENİZ MARKET LTD. ŞTİ.", "120.000,00", "110.000,00", "10.000,00", ""},
	{"120.01.002", "BEREKET GIDA A.Ş.", "98.500,00", "98.500,00", "", ""},
	{"120.01.003", "CEYLAN TOPTAN GIDA", "210.300,00", "180.000,00", "30.300,00", ""},
	{"120.01.004", "DOĞA ORGANİK ÜRÜNLER LTD. ŞTİ.", "45.000,00", "45.000,00", "", ""},
	{"120.01.005", "ERCİYES BAKKALİYE", "18.300,00", "15.900,00", "2.400,00", ""},
	{"120.01.006", "KORKMAZ GIDA SAN. VE TİC. A.Ş.", "350.000,00", "311.000,00", "39.000,00", ""},
	{"150", "İLK MADDE VE MALZEME", "64.200,00", "51.000,00", "13.200,00", ""},
	{"150.01", "AMBALAJ MALZEMELERİ", "64.200,00", "51.000,00", "13.200,00", ""},
	{"153", "TİCARİ MALLAR", "1.120.000,00", "985.000,00", "135.000,00", ""},
	{"153.01", "GIDA ÜRÜNLERİ", "1.120.000,00", "985.000,00", "135.000,00", ""},
	{"191", "İNDİRİLECEK KDV", "96.400,00", "96.400,00", "", ""},
	{"191.01", "İNDİRİLECEK KDV %20", "71.300,00", "71.300,00", "", ""},
	{"191.02", "İNDİRİLECEK KDV %10", "16.100,00", "16.100,00", "", ""},
	{"191.03", "İNDİRİLECEK KDV %1", "9.000,00", "9.000,00", "", ""},
	{"320", "SATICILAR", "905.000,00", "1.012.300,00", "", "107.300,00"},
	{"320.01", "YURTİÇİ SATICILAR", "905.000,00", "1.012.300,00", "", "107.300,00"},
	{"320.01.001", "ANADOLU UN SANAYİ A.Ş.", "300.000,00", "330.000,00", "", "30.000,00"},
	{"320.01.002", "DENİZ AMBALAJ LTD. ŞTİ.", "61.000,00", "64.200,00", "", "3.200,00"},
	{"320.01.003", "GÜNEŞ YAĞ SANAYİ A.Ş.", "250.000,00", "281.400,00", "", "31.400,00"},
	{"320.01.004", "KARADENİZ FINDIK KOOP.", "120.000,00", "150.700,00", "", "30.700,00"},
	{"320.01.005", "MARMARA ŞEKER LTD. ŞTİ.", "98.000,00", "98.000,00", "", ""},
	{"320.01.006", "TRAKYA SÜT ÜRÜNLERİ A.Ş.", "76.000,00", "88.000,00", "", "12.000,00"},
	{"391", "HESAPLANAN KDV", "122.000,00", "140.000,00", "", "18.000,00"},
	{"391.01", "HESAPLANAN KDV %20", "40.000,00", "46.000,00", "", "6.000,00"},
	{"391.02", "HESAPLANAN KDV %10", "12.000,00", "14.000,00", "", "2.000,00"},
	{"391.03", "HESAPLANAN KDV %1", "70.000,00", "80.000,00", "", "10.000,00"},
	{"600", "YURTİÇİ SATIŞLAR", "", "1.398.000,00", "", "1.398.000,00"},
	{"600.01", "GIDA SATIŞLARI", "", "1.398.000,00", "", "1.398.000,00"},
}

var header = []string{"Hesap Kodu", "Hesap Adı", "Borç", "Alacak", "Borç Bakiye", "Alacak Bakiye"}
var title = "OĞUZ GIDA PAZARLAMA LTD. ŞTİ. GENEL GEÇİCİ MİZAN 01.01.2026 - 30.09.2026"

type line struct {
	Name     string
	Qty      int
	Price    float64
	VATRate  int
	Net, VAT string
}

type invoice struct {
	File, ID, UUID, Date, Type string
	Supplier, Customer         Party
	Lines                      []line
	Net, VAT, Total            string
	Subtotals                  []subtotal
}

type subtotal struct {
	Rate     int
	Net, VAT string
}

func money(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func build(file, id, uuid, date string, supplier, customer Party, lines []line) invoice {
	inv := invoice{File: file, ID: id, UUID: uuid, Date: date, Type: "SATIS", Supplier: supplier, Customer: customer}
	var net, vat float64
	byRate := map[int][2]float64{}
	var rates []int
	for i := range lines {
		n := float64(lines[i].Qty) * lines[i].Price
		v := n * float64(lines[i].VATRate) / 100
		lines[i].Net, lines[i].VAT = money(n), money(v)
		net += n
		vat += v
		if _, ok := byRate[lines[i].VATRate]; !ok {
			rates = append(rates, lines[i].VATRate)
		}
		s := byRate[lines[i].VATRate]
		byRate[lines[i].VATRate] = [2]float64{s[0] + n, s[1] + v}
	}
	for _, r := range rates {
		inv.Subtotals = append(inv.Subtotals, subtotal{Rate: r, Net: money(byRate[r][0]), VAT: money(byRate[r][1])})
	}
	inv.Lines, inv.Net, inv.VAT, inv.Total = lines, money(net), money(vat), money(net+vat)
	return inv
}

func invoices() []invoice {
	t := Taxpayer
	return []invoice{
		build("2026-10-korkmaz-satis.xml", "OGZ2026000001001", "6F1D3C0A-7B2E-4C1D-9E8F-310000000001", "2026-10-07", t, korkmaz,
			[]line{{Name: "Kuru Kayısı 5 kg", Qty: 40, Price: 650, VATRate: 1}, {Name: "Antep Fıstığı 1 kg", Qty: 25, Price: 900, VATRate: 1}}),
		build("2026-10-deniz-alis.xml", "DNZ2026000004521", "6F1D3C0A-7B2E-4C1D-9E8F-310000000002", "2026-10-09", deniz, t,
			[]line{{Name: "Oluklu Karton Koli 40x30x20", Qty: 500, Price: 12.5, VATRate: 20}, {Name: "Streç Film 50 cm", Qty: 40, Price: 185, VATRate: 20}, {Name: "Etiket Rulosu 100x150", Qty: 60, Price: 75, VATRate: 20}}),
		build("2026-10-yeni-lojistik-alis.xml", "YNL2026000000318", "6F1D3C0A-7B2E-4C1D-9E8F-310000000003", "2026-10-14", yeni, t,
			[]line{{Name: "Ankara - İstanbul Parsiyel Taşıma", Qty: 1, Price: 8500, VATRate: 20}}),
		build("2026-11-korkmaz-satis.xml", "OGZ2026000001044", "6F1D3C0A-7B2E-4C1D-9E8F-310000000004", "2026-11-06", t, korkmaz,
			[]line{{Name: "Kuru Kayısı 5 kg", Qty: 30, Price: 650, VATRate: 1}, {Name: "Ceviz İçi 1 kg", Qty: 20, Price: 520, VATRate: 1}}),
		build("2026-11-deniz-alis.xml", "DNZ2026000004890", "6F1D3C0A-7B2E-4C1D-9E8F-310000000005", "2026-11-10", deniz, t,
			[]line{{Name: "Oluklu Karton Koli 40x30x20", Qty: 600, Price: 12.5, VATRate: 20}, {Name: "Streç Film 50 cm", Qty: 30, Price: 185, VATRate: 20}, {Name: "Etiket Rulosu 100x150", Qty: 80, Price: 75, VATRate: 20}}),
		build("2026-11-yeni-lojistik-alis.xml", "YNL2026000000402", "6F1D3C0A-7B2E-4C1D-9E8F-310000000006", "2026-11-18", yeni, t,
			[]line{{Name: "Ankara - İstanbul Parsiyel Taşıma", Qty: 1, Price: 9200, VATRate: 20}}),
	}
}

const ublTpl = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2">
  <ext:UBLExtensions><ext:UBLExtension><ext:ExtensionContent><UBLTR xmlns="urn:ubltr:extension"/></ext:ExtensionContent></ext:UBLExtension></ext:UBLExtensions>
  <cbc:UBLVersionID>2.1</cbc:UBLVersionID>
  <cbc:CustomizationID>TR1.2</cbc:CustomizationID>
  <cbc:ProfileID>TICARIFATURA</cbc:ProfileID>
  <cbc:ID>{{.ID}}</cbc:ID>
  <cbc:CopyIndicator>false</cbc:CopyIndicator>
  <cbc:UUID>{{.UUID}}</cbc:UUID>
  <cbc:IssueDate>{{.Date}}</cbc:IssueDate>
  <cbc:IssueTime>10:00:00</cbc:IssueTime>
  <cbc:InvoiceTypeCode>{{.Type}}</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>TRY</cbc:DocumentCurrencyCode>
  <cbc:LineCountNumeric>{{len .Lines}}</cbc:LineCountNumeric>
  <cac:Signature>
    <cbc:ID schemeID="VKN_TCKN">{{.Supplier.VKN}}</cbc:ID>
    <cac:SignatoryParty>{{template "party" .Supplier}}</cac:SignatoryParty>
    <cac:DigitalSignatureAttachment><cac:ExternalReference><cbc:URI>#Signature</cbc:URI></cac:ExternalReference></cac:DigitalSignatureAttachment>
  </cac:Signature>
  <cac:AccountingSupplierParty><cac:Party>{{template "party" .Supplier}}</cac:Party></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><cac:Party>{{template "party" .Customer}}</cac:Party></cac:AccountingCustomerParty>
  <cac:TaxTotal>
    <cbc:TaxAmount currencyID="TRY">{{.VAT}}</cbc:TaxAmount>{{range .Subtotals}}
    <cac:TaxSubtotal>
      <cbc:TaxableAmount currencyID="TRY">{{.Net}}</cbc:TaxableAmount>
      <cbc:TaxAmount currencyID="TRY">{{.VAT}}</cbc:TaxAmount>
      <cbc:Percent>{{.Rate}}</cbc:Percent>
      <cac:TaxCategory><cac:TaxScheme><cbc:Name>KDV</cbc:Name><cbc:TaxTypeCode>0015</cbc:TaxTypeCode></cac:TaxScheme></cac:TaxCategory>
    </cac:TaxSubtotal>{{end}}
  </cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:LineExtensionAmount currencyID="TRY">{{.Net}}</cbc:LineExtensionAmount>
    <cbc:TaxExclusiveAmount currencyID="TRY">{{.Net}}</cbc:TaxExclusiveAmount>
    <cbc:TaxInclusiveAmount currencyID="TRY">{{.Total}}</cbc:TaxInclusiveAmount>
    <cbc:PayableAmount currencyID="TRY">{{.Total}}</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>{{range $i, $l := .Lines}}
  <cac:InvoiceLine>
    <cbc:ID>{{inc $i}}</cbc:ID>
    <cbc:InvoicedQuantity unitCode="C62">{{$l.Qty}}</cbc:InvoicedQuantity>
    <cbc:LineExtensionAmount currencyID="TRY">{{$l.Net}}</cbc:LineExtensionAmount>
    <cac:TaxTotal>
      <cbc:TaxAmount currencyID="TRY">{{$l.VAT}}</cbc:TaxAmount>
      <cac:TaxSubtotal>
        <cbc:TaxableAmount currencyID="TRY">{{$l.Net}}</cbc:TaxableAmount>
        <cbc:TaxAmount currencyID="TRY">{{$l.VAT}}</cbc:TaxAmount>
        <cbc:Percent>{{$l.VATRate}}</cbc:Percent>
        <cac:TaxCategory><cac:TaxScheme><cbc:Name>KDV</cbc:Name><cbc:TaxTypeCode>0015</cbc:TaxTypeCode></cac:TaxScheme></cac:TaxCategory>
      </cac:TaxSubtotal>
    </cac:TaxTotal>
    <cac:Item><cbc:Name>{{$l.Name}}</cbc:Name></cac:Item>
    <cac:Price><cbc:PriceAmount currencyID="TRY">{{price $l.Price}}</cbc:PriceAmount></cac:Price>
  </cac:InvoiceLine>{{end}}
</Invoice>
{{define "party"}}
      <cac:PartyIdentification><cbc:ID schemeID="VKN">{{.VKN}}</cbc:ID></cac:PartyIdentification>
      <cac:PartyName><cbc:Name>{{.Name}}</cbc:Name></cac:PartyName>
      <cac:PostalAddress>
        <cbc:CitySubdivisionName>Çankaya</cbc:CitySubdivisionName>
        <cbc:CityName>Ankara</cbc:CityName>
        <cac:Country><cbc:IdentificationCode>TR</cbc:IdentificationCode><cbc:Name>Türkiye</cbc:Name></cac:Country>
      </cac:PostalAddress>
      <cac:PartyTaxScheme><cac:TaxScheme><cbc:Name>Çankaya</cbc:Name></cac:TaxScheme></cac:PartyTaxScheme>
{{end}}`

func main() {
	must(os.MkdirAll(filepath.Join(dir, "ubl"), 0o755))

	// CSV: Luca "Excel'e aktar" çıktısı gibi, noktalı virgüllü, UTF-8 BOM'lu.
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	must(w.Write([]string{title}))
	must(w.Write(header))
	for _, r := range plan {
		must(w.Write(r))
	}
	must(w.Write([]string{"", "GENEL TOPLAM", "5.170.400,00", "5.170.400,00", "", ""}))
	w.Flush()
	must(w.Error())
	must(os.WriteFile(filepath.Join(dir, "oguz-luca-mizan.csv"), buf.Bytes(), 0o644))

	x := excelize.NewFile()
	sheet := "Mizan"
	must(x.SetSheetName("Sheet1", sheet))
	must(x.SetCellValue(sheet, "A1", title))
	must(x.SetSheetRow(sheet, "A3", &header))
	for i, r := range plan {
		row := make([]any, len(r))
		for j, v := range r {
			row[j] = v
		}
		must(x.SetSheetRow(sheet, fmt.Sprintf("A%d", i+4), &row))
	}
	must(x.SaveAs(filepath.Join(dir, "oguz-luca-mizan.xlsx")))

	tpl := template.Must(template.New("ubl").Funcs(template.FuncMap{
		"inc":   func(i int) int { return i + 1 },
		"price": func(v float64) string { return strings.TrimSuffix(strings.TrimSuffix(money(v), "0"), ".0") },
	}).Parse(ublTpl))
	for _, inv := range invoices() {
		var b bytes.Buffer
		must(tpl.Execute(&b, inv))
		must(os.WriteFile(filepath.Join(dir, "ubl", inv.File), b.Bytes(), 0o644))
	}
	fmt.Printf("taxpayer %s %s; korkmaz %s, deniz %s, yeni %s\n", Taxpayer.Name, Taxpayer.VKN, korkmaz.VKN, deniz.VKN, yeni.VKN)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
