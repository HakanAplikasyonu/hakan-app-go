package lucakoza

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// The client against the fake Koza server (internal/testfakes): no real
// Koza is ever called.

func fakeClient(t *testing.T, f *testfakes.LucaKoza) *Client {
	t.Helper()
	srv := testfakes.Serve(t, f)
	c, err := NewClient(srv.URL+"/LucaKoza/", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func login(t *testing.T, c *Client) {
	t.Helper()
	if err := c.Login(context.Background(), testfakes.LucaKozaOrgCode, testfakes.LucaKozaUser, testfakes.LucaKozaPassword); err != nil {
		t.Fatalf("login: %v", err)
	}
}

func TestLoginOK(t *testing.T) {
	f := testfakes.LucaKozaSample()
	login(t, fakeClient(t, f))
	if f.Logins != 1 {
		t.Fatalf("logins = %d", f.Logins)
	}
}

func TestLoginRefusedShowsKozaMessage(t *testing.T) {
	f := testfakes.LucaKozaSample()
	err := fakeClient(t, f).Login(context.Background(), testfakes.LucaKozaOrgCode, testfakes.LucaKozaUser, "yanlis")
	var kz *KozaError
	if !errors.As(err, &kz) || kz.Step != "login" {
		t.Fatalf("err = %v, want a login KozaError", err)
	}
	if kz.Message != "Kullanıcı adı veya şifre hatalı." {
		t.Fatalf("message = %q", kz.Message)
	}
	if f.Logins != 0 {
		t.Fatal("a refused login opened a session")
	}
}

func TestCompaniesAndSelect(t *testing.T) {
	ctx := context.Background()
	f := testfakes.LucaKozaSample()
	c := fakeClient(t, f)
	login(t, c)
	cos, err := c.Companies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cos) != 2 || cos[0].ID != testfakes.LucaKozaCompanyID || cos[0].Name != "Atlas Gıda - Merkez" {
		t.Fatalf("companies = %+v", cos)
	}
	if err := c.SelectCompany(ctx, testfakes.LucaKozaCompanyID); err != nil {
		t.Fatalf("select: %v", err)
	}
	var kz *KozaError
	if err := c.SelectCompany(ctx, "999"); !errors.As(err, &kz) {
		t.Fatalf("select of a foreign company: %v, want KozaError", err)
	}
}

func TestSessionNeeded(t *testing.T) {
	_, err := fakeClient(t, testfakes.LucaKozaSample()).Companies(context.Background())
	var kz *KozaError
	if !errors.As(err, &kz) {
		t.Fatalf("err = %v, want KozaError without a session", err)
	}
}

func oct() (time.Time, time.Time) {
	return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func eq(t *testing.T, what string, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", what, got, want)
	}
}

func TestReadMapsAllMovementTypes(t *testing.T) {
	ctx := context.Background()
	f := testfakes.LucaKozaSample()
	c := fakeClient(t, f)
	login(t, c)
	if err := c.SelectCompany(ctx, testfakes.LucaKozaCompanyID); err != nil {
		t.Fatal(err)
	}
	from, to := oct()
	b, err := c.Read(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if b.Excluded[SkipUnknownMovement] != 1 {
		t.Errorf("excluded = %v, want one unknown movement type", b.Excluded)
	}
	byID := map[string]model.Invoice{}
	for _, inv := range b.Invoices {
		byID[inv.SourceID] = inv
	}
	if len(byID) != 4 {
		t.Fatalf("invoices = %d, want 4", len(byID))
	}
	want := map[string]struct {
		dir model.Direction
		typ string
	}{
		"1001": {model.DirectionOutgoing, model.TypeSatis},
		"1002": {model.DirectionOutgoing, model.TypeIade},
		"1003": {model.DirectionIncoming, model.TypeAlis},
		"1004": {model.DirectionIncoming, model.TypeIade},
	}
	for id, w := range want {
		inv, ok := byID[id]
		if !ok {
			t.Fatalf("invoice %s missing", id)
		}
		if inv.Direction != w.dir || inv.Type != w.typ || inv.SourceSystem != SourceSystem || inv.Period != "2026-10" {
			t.Errorf("%s: %s %s %s %s", id, inv.Direction, inv.Type, inv.SourceSystem, inv.Period)
		}
	}

	// Satış: "1.234,56" and dotted numbers read exactly; a missing kdvTutar is net × rate.
	s := byID["1001"]
	if s.InvoiceNumber != "KZA2026000000001" || s.Counterparty.TaxNumber != "9100000021" ||
		s.Counterparty.Name != "Yıldız Market Ltd. Şti." || s.Counterparty.TaxOffice != "Kadıköy" {
		t.Errorf("header = %q %+v", s.InvoiceNumber, s.Counterparty)
	}
	eq(t, "line 1 unit", s.Items[0].UnitPrice, d("1234.56"))
	eq(t, "line 2 unit", s.Items[1].UnitPrice, d("10.005"))
	eq(t, "line 2 vat", s.Items[1].TotalTaxAmount, d("2.00"))
	eq(t, "sales base", s.Amounts.TaxExclusiveAmount, d("3723.69"))
	eq(t, "sales vat", s.Amounts.TaxAmount, d("742.74"))
	eq(t, "sales payable", s.Amounts.PayableAmount, d("4466.43"))
	if len(s.TaxSummaries) != 2 {
		t.Fatalf("summaries = %+v", s.TaxSummaries)
	}
	eq(t, "10% base", s.TaxSummaries[0].Base, d("20.01"))
	eq(t, "20% vat", s.TaxSummaries[1].VAT, d("740.74"))

	// Satış iade: a TCKN counterparty.
	if r := byID["1002"]; r.Counterparty.IdentityNumber != "12345678901" {
		t.Errorf("return counterparty = %+v", r.Counterparty)
	}

	// Alım with tevkifat: 5/10 of the VAT is withheld.
	p := byID["1003"]
	eq(t, "withholding", p.Amounts.WithholdingAmount, d("1000.00"))
	eq(t, "withholding rate", p.Items[0].WithholdingRate, d("50"))
	eq(t, "purchase payable", p.Amounts.PayableAmount, d("11000.00"))
	eq(t, "summary withholding", p.TaxSummaries[0].Withholding, d("1000.00"))

	// Alım iade: kdvOran 0.20 is 20 %, "250,5" is 250.5.
	ri := byID["1004"]
	eq(t, "return rate", ri.Items[0].TaxRate, d("20"))
	eq(t, "return unit", ri.Items[0].UnitPrice, d("250.5"))
	eq(t, "return vat", ri.Amounts.TaxAmount, d("100.20"))

	if len(f.Unknown) != 0 {
		t.Fatalf("calls to unknown endpoints: %v", f.Unknown)
	}
}

func TestMapSkipsWithoutCrash(t *testing.T) {
	from, to := oct()
	cases := map[string]obj{
		SkipUnknownMovement: {"ssFaturaBaslikId": "1", "parUstHareketTuru": "abc", "belgeTarihi": "05/10/2026"},
		SkipNoID:            {"belgeTarihi": "05/10/2026"},
		SkipNoDate:          {"ssFaturaBaslikId": "1", "belgeTarihi": "dün"},
		SkipOutsideRange:    {"ssFaturaBaslikId": "1", "belgeTarihi": "05/11/2026"},
		SkipNoAmounts:       {"ssFaturaBaslikId": "1", "belgeTarihi": "05/10/2026", "detayList": "bozuk"},
	}
	for reason, r := range cases {
		if _, _, skip := mapInvoice(MoveSales, r, from, to); skip != reason {
			t.Errorf("%s: skip = %q", reason, skip)
		}
	}
	if _, _, skip := mapInvoice(42, obj{"ssFaturaBaslikId": "1", "belgeTarihi": "05/10/2026"}, from, to); skip != SkipUnknownMovement {
		t.Errorf("movement 42: skip = %q", skip)
	}
}

func TestParseDecimalExact(t *testing.T) {
	raw, err := decode([]byte(`{"a": 12345678901234.123456789, "b": "1.234.567,891", "c": "1,234.5", "d": "  42 ", "e": "x", "f": null}`))
	if err != nil {
		t.Fatal(err)
	}
	o := asObj(raw)
	for k, want := range map[string]string{"a": "12345678901234.123456789", "b": "1234567.891", "c": "1234.5", "d": "42"} {
		got, ok := o.dec(k)
		if !ok || !got.Equal(d(want)) {
			t.Errorf("%s = %s (%v), want %s", k, got, ok, want)
		}
	}
	for _, k := range []string{"e", "f", "missing"} {
		if _, ok := o.dec(k); ok {
			t.Errorf("%s parsed", k)
		}
	}
}

func TestNormalizeBase(t *testing.T) {
	good := map[string]string{
		"http://koza.example.test:8080/LucaKoza/": "http://koza.example.test:8080/LucaKoza",
		" https://koza.example.test ":             "https://koza.example.test",
	}
	for in, want := range good {
		if got, err := NormalizeBase(in); err != nil || got != want {
			t.Errorf("%q = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "koza.example.test", "ftp://koza.example.test", "http://u:p@koza.example.test", "http://koza.example.test/?a=1"} {
		if _, err := NormalizeBase(in); !errors.Is(err, ErrAddress) {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestGuardRefusesInternalAddresses(t *testing.T) {
	srv := testfakes.Serve(t, testfakes.LucaKozaSample())
	c, err := NewClient(srv.URL, GuardedHTTP()) // 127.0.0.1: the server itself
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login(context.Background(), "1", "u", "p"); !errors.Is(err, ErrAddress) {
		t.Fatalf("err = %v, want ErrAddress for a loopback address", err)
	}
}
