package ublmap

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

const ownVKN = "1288331521" // supplier in most go-ubltr samples

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mapFixture(t *testing.T, name string, opt Options) *Result {
	t.Helper()
	if opt.OwnTaxID == "" {
		opt.OwnTaxID = ownVKN
	}
	r, err := MapXML(fixture(t, name), opt)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func eq(t *testing.T, field string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(d(want)) {
		t.Errorf("%s = %s, want %s", field, got, want)
	}
}

func codes(ws []model.Warning) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

func noWarnings(t *testing.T, r *Result) {
	t.Helper()
	if len(r.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v (%+v)", codes(r.Warnings), r.Warnings)
	}
}

func hasWarning(t *testing.T, r *Result, code string) model.Warning {
	t.Helper()
	for _, w := range r.Warnings {
		if w.Code == code {
			return w
		}
	}
	t.Fatalf("warning %s not raised; got %v", code, codes(r.Warnings))
	return model.Warning{}
}

// 1. Temel fatura: başlık, karşı taraf, satırlar, özet.
func TestAcceptance01TemelFatura(t *testing.T) {
	r := mapFixture(t, "temelfatura-satis.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if inv.UUID != "6a2c1d2e-3f40-4b51-8c62-7d8e9fa0b1c2" || inv.SourceID != inv.UUID {
		t.Errorf("uuid/source_id = %q/%q", inv.UUID, inv.SourceID)
	}
	if inv.InvoiceNumber != "GIB2026000000001" || inv.Direction != model.DirectionOutgoing || inv.Type != model.TypeSatis {
		t.Errorf("header: %+v", inv)
	}
	if *inv.Profile != model.ProfileTemel || inv.Period != "2026-03" || inv.IssueDate.Format("2006-01-02") != "2026-03-01" || inv.IssueTime != "10:00:00" {
		t.Errorf("profile/date: %v %s %v %s", *inv.Profile, inv.Period, inv.IssueDate, inv.IssueTime)
	}
	if inv.SourceSystem != "manual_upload" || inv.CaptureMethod != model.CaptureUBLUpload || !inv.HasItems || inv.TaxSummarySource != model.TaxSummaryFromItems {
		t.Errorf("fixed values: %s %s %v %s", inv.SourceSystem, inv.CaptureMethod, inv.HasItems, inv.TaxSummarySource)
	}
	cp := inv.Counterparty
	if cp.IdentityNumber != "12345678901" || cp.Name != "Ali Yılmaz" || cp.LegalType != "individual" || cp.City != "İstanbul" || cp.District != "Beşiktaş" {
		t.Errorf("counterparty: %+v", cp)
	}
	eq(t, "tax_exclusive", inv.Amounts.TaxExclusiveAmount, "420")
	eq(t, "tax_amount", inv.Amounts.TaxAmount, "72")
	eq(t, "payable", inv.Amounts.PayableAmount, "492")
	if len(inv.Items) != 2 {
		t.Fatalf("items = %d", len(inv.Items))
	}
	it := inv.Items[0]
	if it.LineNo != 1 || it.Name != "Masa Lambası" || it.UnitCode != "C62" {
		t.Errorf("item: %+v", it)
	}
	eq(t, "qty", it.Quantity, "2")
	eq(t, "unit_price", it.UnitPrice, "150")
	eq(t, "line_extension", it.LineExtensionAmount, "300")
	eq(t, "item tax_exclusive", it.TaxExclusiveAmount, "300")
	eq(t, "item rate", it.TaxRate, "20")
	eq(t, "item vat", it.TotalTaxAmount, "60")
	eq(t, "item inclusive", it.TaxInclusiveAmount, "360")
	if len(it.TaxBreakdowns) != 1 || it.TaxBreakdowns[0].TaxTypeCode != "0015" || it.TaxBreakdowns[0].TaxAmount != "60.00" {
		t.Errorf("breakdowns: %+v", it.TaxBreakdowns)
	}
	if len(inv.TaxSummaries) != 2 {
		t.Fatalf("summary: %+v", inv.TaxSummaries)
	}
	eq(t, "summary[0].rate", inv.TaxSummaries[0].TaxRate, "10")
	eq(t, "summary[1].base", inv.TaxSummaries[1].Base, "300")
	if len(inv.ContentHash) != 64 {
		t.Errorf("content hash = %q", inv.ContentHash)
	}
}

// 2. Ticari fatura, üç oran (%1, %10, %20) ve satır iskontosu.
func TestAcceptance02UcOranIskonto(t *testing.T) {
	r := mapFixture(t, "ticarifatura-uc-oran-iskonto.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if *inv.Profile != model.ProfileTicari || len(inv.TaxSummaries) != 3 {
		t.Fatalf("profile %v, summary %+v", *inv.Profile, inv.TaxSummaries)
	}
	want := [][3]string{{"1", "250", "2.5"}, {"10", "540", "54"}, {"20", "2250", "450"}}
	for i, w := range want {
		s := inv.TaxSummaries[i]
		eq(t, "rate", s.TaxRate, w[0])
		eq(t, "base", s.Base, w[1])
		eq(t, "vat", s.VAT, w[2])
	}
	kitap := inv.Items[1]
	eq(t, "kitap line_extension (qty×price)", kitap.LineExtensionAmount, "600")
	eq(t, "kitap own_allowance", kitap.OwnAllowanceAmount, "60")
	eq(t, "kitap tax_exclusive (net)", kitap.TaxExclusiveAmount, "540")
	klavye := inv.Items[2]
	eq(t, "klavye own_allowance (10%)", klavye.OwnAllowanceAmount, "250")
	eq(t, "tax_amount", inv.Amounts.TaxAmount, "506.5")
	if inv.PaymentDueDate == nil || inv.PaymentDueDate.Format("2006-01-02") != "2026-11-15" {
		t.Errorf("due date: %v", inv.PaymentDueDate)
	}
	if inv.OrderReference["id"] != "SIP-2026-0042" {
		t.Errorf("order ref: %v", inv.OrderReference)
	}
}

// 3. Tevkifatlı fatura: satır ve başlık tevkifatı, PayableAmount kontrolü.
func TestAcceptance03Tevkifat(t *testing.T) {
	r := mapFixture(t, "temelfatura-tevkifat.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if inv.Type != model.TypeTevkifat {
		t.Errorf("type = %s", inv.Type)
	}
	eq(t, "withholding (header)", inv.Amounts.WithholdingAmount, "1600")
	eq(t, "payable", inv.Amounts.PayableAmount, "16400")
	it := inv.Items[0]
	if it.WithholdingCode != "603" {
		t.Errorf("withholding code = %q", it.WithholdingCode)
	}
	eq(t, "withholding_rate (percent)", it.WithholdingRate, "70")
	eq(t, "withholding_amount", it.WithholdingAmount, "1400")
	if len(inv.TaxSummaries) != 2 || inv.TaxSummaries[0].WithholdingCode != "603" || inv.TaxSummaries[1].WithholdingCode != "624" {
		t.Fatalf("summary: %+v", inv.TaxSummaries)
	}
	eq(t, "summary 603 withholding", inv.TaxSummaries[0].Withholding, "1400")
	eq(t, "summary 624 withholding", inv.TaxSummaries[1].Withholding, "200")

	// Break PayableAmount: the check must fire.
	bad := strings.Replace(string(fixture(t, "temelfatura-tevkifat.xml")), ">16400.00<", ">18000.00<", 1)
	r2, err := MapXML([]byte(bad), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	hasWarning(t, r2, model.WarnPayableMismatch)
}

// 4. İstisnalı fatura (KDV %0 + istisna kodu).
func TestAcceptance04Istisna(t *testing.T) {
	r := mapFixture(t, "temelfatura-istisna.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if inv.Type != model.TypeIstisna || inv.GlobalTaxExemptionCode != "350" {
		t.Errorf("type %s, global exemption %q", inv.Type, inv.GlobalTaxExemptionCode)
	}
	for _, it := range inv.Items {
		if it.TaxExemptionCode != "350" || !it.TaxRate.IsZero() {
			t.Errorf("item %d: exemption %q rate %s", it.LineNo, it.TaxExemptionCode, it.TaxRate)
		}
	}
	if len(inv.TaxSummaries) != 1 || inv.TaxSummaries[0].TaxExemptionCode != "350" {
		t.Errorf("summary: %+v", inv.TaxSummaries)
	}
	// İhracat profili, GTİP ve döviz aynı yoldan.
	ih := mapFixture(t, "ihracat-istisna.xml", Options{}).Invoice
	if *ih.Profile != model.ProfileIhracat || ih.GlobalTaxExemptionCode != "301" || ih.Items[0].GTIPCode != "610910000011" {
		t.Errorf("ihracat: %v %q %q", *ih.Profile, ih.GlobalTaxExemptionCode, ih.Items[0].GTIPCode)
	}
}

// 5. İade faturası: BillingReference → asıl fatura no/tarih.
func TestAcceptance05Iade(t *testing.T) {
	// The sample is issued by 9205121120 to 1288331521: incoming for us.
	r := mapFixture(t, "temelfatura-iade.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if inv.Type != model.TypeIade || inv.Direction != model.DirectionIncoming {
		t.Errorf("type %s direction %s", inv.Type, inv.Direction)
	}
	if inv.OriginalInvoiceNumber != "GIB2026000000002" || inv.OriginalInvoiceIssueDate == nil || inv.OriginalInvoiceIssueDate.Format("2006-01-02") != "2026-03-01" {
		t.Errorf("billing ref: %q %v", inv.OriginalInvoiceNumber, inv.OriginalInvoiceIssueDate)
	}
	if inv.Counterparty.TaxNumber != "9205121120" {
		t.Errorf("counterparty should be the supplier: %+v", inv.Counterparty)
	}
	// TEVKIFATIADE maps to IADE.
	x := strings.Replace(string(fixture(t, "temelfatura-iade.xml")), ">IADE<", ">TEVKIFATIADE<", 1)
	r2, err := MapXML([]byte(x), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Invoice.Type != model.TypeIade || r2.Invoice.TypeCodeRaw != "TEVKIFATIADE" {
		t.Errorf("TEVKIFATIADE → %s (%s)", r2.Invoice.Type, r2.Invoice.TypeCodeRaw)
	}
}

// 6. Dövizli fatura: CalculationRate, payable_amount_try.
func TestAcceptance06Dovizli(t *testing.T) {
	r := mapFixture(t, "ticarifatura-usd.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if inv.Currency != "USD" || inv.ExchangeRate == nil {
		t.Fatalf("currency %s rate %v", inv.Currency, inv.ExchangeRate)
	}
	eq(t, "exchange_rate", *inv.ExchangeRate, "34.5678")
	eq(t, "payable", inv.Amounts.PayableAmount, "1199.88")
	eq(t, "payable_try", inv.Amounts.PayableAmountTRY, d("1199.88").Mul(d("34.5678")).Round(4).String())
}

// 7. E-arşiv nihai tüketici faturası (TCKN 11111111111).
func TestAcceptance07EArsivNihaiTuketici(t *testing.T) {
	r := mapFixture(t, "earsiv-nihai-tuketici.xml", Options{})
	noWarnings(t, r)
	inv := r.Invoice
	if *inv.Profile != model.ProfileEArsiv || inv.Counterparty.IdentityNumber != DefaultRetailTCKN || !inv.Counterparty.IsDefaultRetail {
		t.Errorf("e-arşiv: %v %+v", *inv.Profile, inv.Counterparty)
	}
}

// 8. ZIP içinde birden çok XML (+ base64, BOM).
func TestAcceptance08ZipCokXML(t *testing.T) {
	names := []string{"temelfatura-satis.xml", "temelfatura-tevkifat.xml", "ticarifatura-usd.xml"}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, _ := zw.Create("faturalar/" + n)
		_, _ = w.Write(fixture(t, n))
	}
	w, _ := zw.Create("__MACOSX/._x.xml")
	_, _ = w.Write([]byte("junk"))
	w, _ = zw.Create("okubeni.txt")
	_, _ = w.Write([]byte("not xml"))
	w, _ = zw.Create("bozuk.xml")
	_, _ = w.Write([]byte("<Invoice><broken"))
	_ = zw.Close()

	for _, payload := range [][]byte{buf.Bytes(), []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))} {
		items, err := Extract("yukleme.zip", payload, DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		var ok, bad int
		seen := map[string]bool{}
		for _, it := range items {
			if it.Err != nil {
				bad++
				continue
			}
			ok++
			if it.Kind != ubltr.KindInvoice {
				t.Errorf("%s kind %s", it.Name, it.Kind)
			}
			r, err := MapXML(it.XML, Options{OwnTaxID: ownVKN})
			if err != nil {
				t.Fatalf("%s: %v", it.Name, err)
			}
			seen[r.Invoice.UUID] = true
		}
		if ok != 3 || bad != 1 || len(seen) != 3 {
			t.Errorf("ok=%d bad=%d uuids=%d", ok, bad, len(seen))
		}
	}
	bom := append([]byte{0xEF, 0xBB, 0xBF}, fixture(t, "temelfatura-satis.xml")...)
	if items, err := Extract("x.xml", bom, DefaultLimits); err != nil || len(items) != 1 {
		t.Errorf("BOM xml: %v %d", err, len(items))
	}
	if _, err := Extract("x.pdf", []byte("%PDF-1.7 ..."), DefaultLimits); !errors.Is(err, ErrNotUBL) {
		t.Errorf("pdf: %v", err)
	}
	if _, err := Extract("x.bin", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, DefaultLimits); !errors.Is(err, ErrUnknownPayload) {
		t.Errorf("unknown: %v", err)
	}
	xxe := []byte(`<?xml version="1.0"?><!DOCTYPE x [<!ENTITY e SYSTEM "file:///etc/passwd">]><Invoice>&e;</Invoice>`)
	if _, err := Extract("x.xml", xxe, DefaultLimits); !errors.Is(err, ErrDTD) {
		t.Errorf("dtd: %v", err)
	}
	if _, err := Extract("x.xml", fixture(t, "temelfatura-satis.xml"), Limits{MaxFileBytes: 100, MaxTotalBytes: 100, MaxEntries: 1}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("size limit: %v", err)
	}
}

// 9. Kendi tarafı bağlanılan VKN ile eşleşmeyen XML reddediliyor.
func TestAcceptance09PartyMismatch(t *testing.T) {
	data := fixture(t, "temelfatura-satis.xml")
	_, err := MapXML(data, Options{OwnTaxID: "1111111111"})
	var pm *PartyMismatchError
	if !errors.As(err, &pm) || pm.Warning().Code != model.WarnPartyMismatch {
		t.Fatalf("err = %v", err)
	}
	// Direction given by the source but our VKN is on the other side.
	if _, err := MapXML(data, Options{OwnTaxID: ownVKN, Direction: model.DirectionIncoming}); !errors.As(err, &pm) {
		t.Fatalf("incoming on an outgoing invoice: %v", err)
	}
	// Customer side: the same file is incoming for the buyer.
	r, err := MapXML(data, Options{OwnTaxID: "12345678901"})
	if err != nil || r.Invoice.Direction != model.DirectionIncoming || r.Invoice.Type != model.TypeAlis || r.Invoice.Counterparty.TaxNumber != ownVKN {
		t.Fatalf("buyer view: %v %+v", err, r)
	}
	if _, err := MapXML(data, Options{}); !errors.Is(err, ErrNoOwnTaxID) {
		t.Fatalf("missing own id: %v", err)
	}
}

// 10. Bilinmeyen InvoiceTypeCode / ProfileID / vergi kodu → kayıt + uyarı.
func TestAcceptance10UnknownCodes(t *testing.T) {
	x := string(fixture(t, "temelfatura-satis.xml"))
	x = strings.Replace(x, ">SATIS<", ">KOMISYONCU<", 1)
	x = strings.Replace(x, ">TEMELFATURA<", ">HKS<", 1)
	// Line 2's VAT code becomes an unknown tax type.
	i := strings.LastIndex(x, ">0015<")
	x = x[:i] + ">9999<" + x[i+len(">0015<"):]
	r, err := MapXML([]byte(x), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	inv := r.Invoice
	if inv.Type != model.TypeSatis || inv.TypeCodeRaw != "KOMISYONCU" || *inv.Profile != model.ProfileOzel || inv.ProfileRaw != "HKS" {
		t.Errorf("fallbacks: %s/%s %s/%s", inv.Type, inv.TypeCodeRaw, *inv.Profile, inv.ProfileRaw)
	}
	hasWarning(t, r, model.WarnUnmappedTypeCode)
	hasWarning(t, r, model.WarnUnmappedProfile)
	w := hasWarning(t, r, model.WarnUnknownTaxCode)
	if w.Params["code"] != "9999" {
		t.Errorf("unknown code params: %v", w.Params)
	}
	// The unknown tax is kept raw on the line.
	found := false
	for _, b := range inv.Items[1].TaxBreakdowns {
		found = found || b.TaxTypeCode == "9999"
	}
	if !found {
		t.Error("unknown tax not kept in tax_breakdowns")
	}
	// Losing that line's VAT makes the totals disagree, which is flagged too.
	hasWarning(t, r, model.WarnTaxTotalMismatch)
}

func TestHeaderEstimateWithoutLines(t *testing.T) {
	x := string(fixture(t, "temelfatura-satis.xml"))
	i := strings.Index(x, "<InvoiceLine")
	j := strings.LastIndex(x, "</Invoice>")
	r, err := MapXML([]byte(x[:i]+x[j:]), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	inv := r.Invoice
	if inv.HasItems || inv.TaxSummarySource != model.TaxSummaryFromHeaderEstimate || len(inv.TaxSummaries) != 2 {
		t.Fatalf("%v %s %+v", inv.HasItems, inv.TaxSummarySource, inv.TaxSummaries)
	}
	hasWarning(t, r, model.WarnHeaderEstimate)
}

func TestLineTaxableMismatch(t *testing.T) {
	x := strings.Replace(string(fixture(t, "temelfatura-satis.xml")), "<TaxableAmount xmlns=\"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2\" currencyID=\"TRY\">120.00</TaxableAmount>\n        <TaxAmount", "<TaxableAmount xmlns=\"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2\" currencyID=\"TRY\">125.00</TaxableAmount>\n        <TaxAmount", 1)
	r, err := MapXML([]byte(x), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	w := hasWarning(t, r, model.WarnLineTaxableMismatch)
	if w.EntityType != model.EntityInvoiceItem || w.LineNo != 2 {
		t.Errorf("warning: %+v", w)
	}
}

func TestAmountsAreLossless(t *testing.T) {
	x := strings.Replace(string(fixture(t, "ticarifatura-usd.xml")), ">34.5678<", ">34.123456789012<", 1)
	r, err := MapXML([]byte(x), Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	if r.Invoice.ExchangeRate.String() != "34.123456789012" {
		t.Errorf("rate lost precision: %s", r.Invoice.ExchangeRate)
	}
	// 0.1 + 0.2 style sums stay exact.
	eq(t, "sum", d("0.1").Add(d("0.2")), "0.3")
}

func TestExtractEnvelope(t *testing.T) {
	a, b := fixture(t, "temelfatura-satis.xml"), fixture(t, "ticarifatura-satis.xml")
	env, err := ubltr.NewEnvelope(ubltr.EnvelopeOptions{
		Sender:   ubltr.EnvelopeParty{Alias: "urn:mail:defaultgb@example.com", VKNTCKN: ownVKN, Name: "AAA"},
		Receiver: ubltr.EnvelopeParty{Alias: "urn:mail:defaultpk@example.com", VKNTCKN: "9205121120"},
	}, a, b)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ubltr.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	items, err := Extract("zarf.xml", data, DefaultLimits)
	if err != nil || len(items) != 2 {
		t.Fatalf("%v %d", err, len(items))
	}
	for _, it := range items {
		if _, err := MapXML(it.XML, Options{OwnTaxID: ownVKN}); err != nil {
			t.Errorf("%s: %v", it.Name, err)
		}
	}
}
