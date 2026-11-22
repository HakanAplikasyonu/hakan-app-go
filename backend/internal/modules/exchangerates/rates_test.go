package exchangerates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func dp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestParseToday(t *testing.T) {
	data, err := os.ReadFile("testdata/today.xml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Date.Equal(date("2026-11-20")) {
		t.Fatalf("date = %s, want 2026-11-20", b.Date)
	}
	if len(b.Rates) != 5 {
		t.Fatalf("rates = %d, want 5", len(b.Rates))
	}
	by := map[string]Rate{}
	for _, r := range b.Rates {
		by[r.Currency] = r
	}
	usd := by["USD"]
	if usd.ForexBuying.String() != "34.5012" || usd.ForexSelling.String() != "34.5634" ||
		usd.BanknoteBuying.String() != "34.477" || usd.BanknoteSelling.String() != "34.6153" || usd.Unit != 1 {
		t.Fatalf("USD = %+v", usd)
	}
	if v, ok := by["JPY"].Value(KindForexBuying); !ok || v.String() != "0.22315" {
		t.Fatalf("JPY per unit = %s %v, want 0.22315 (100 units)", v, ok)
	}
	xdr := by["XDR"]
	if xdr.ForexSelling != nil || xdr.BanknoteBuying != nil {
		t.Fatalf("XDR empty values must be nil: %+v", xdr)
	}
	if _, ok := xdr.Value(KindBanknoteSelling); ok {
		t.Fatal("XDR has no banknote selling rate")
	}
}

func TestParseRejects(t *testing.T) {
	for name, src := range map[string]string{
		"not xml":  "<html>bakım</html",
		"no date":  `<Tarih_Date><Currency CurrencyCode="USD"><Unit>1</Unit><ForexBuying>1</ForexBuying></Currency></Tarih_Date>`,
		"no rates": `<Tarih_Date Tarih="20.11.2026" Date="11/20/2026"></Tarih_Date>`,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseLatin5Archive(t *testing.T) {
	// Older archive files declare ISO-8859-9; İ is 0xDD there.
	src := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-9\"?>\n" +
		"<Tarih_Date Tarih=\"02.01.2024\" Date=\"01/02/2024\"><Currency CurrencyCode=\"GBP\"><Unit>1</Unit>" +
		"<Isim>\xddNG\xddL\xddZ STERL\xdd</Isim><ForexBuying>37.5</ForexBuying></Currency></Tarih_Date>")
	b, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Date.Equal(date("2024-01-02")) || b.Rates[0].Currency != "GBP" {
		t.Fatalf("got %+v", b)
	}
}

func TestPickRateRule(t *testing.T) {
	rates := []Rate{
		{Date: date("2026-11-13"), Currency: "USD", Unit: 1, ForexBuying: dp("34.10"), ForexSelling: dp("34.20")},
		{Date: date("2026-11-19"), Currency: "USD", Unit: 1, ForexBuying: dp("34.40")},
		{Date: date("2026-11-20"), Currency: "USD", Unit: 1, ForexBuying: dp("34.50"), ForexSelling: dp("34.56")},
	}
	cases := []struct {
		name, invoice, kind, want, wantDay string
		ok                                 bool
	}{
		// Friday 15:30 bulletin is the rate of the weekend and Monday.
		{"monday takes friday", "2026-11-23", KindForexBuying, "34.5", "2026-11-20", true},
		{"saturday takes friday", "2026-11-21", KindForexBuying, "34.5", "2026-11-20", true},
		// Strictly before: an invoice of the bulletin day takes the day before.
		{"same day takes previous", "2026-11-20", KindForexBuying, "34.4", "2026-11-19", true},
		// Holiday gap (no bulletin 14-18): the last published one.
		{"after holiday", "2026-11-18", KindForexBuying, "34.1", "2026-11-13", true},
		{"selling kind", "2026-11-21", KindForexSelling, "34.56", "2026-11-20", true},
		// The 19th has no selling value: falls back to the 13th.
		{"kind missing that day", "2026-11-20", KindForexSelling, "34.2", "2026-11-13", true},
		{"too old", "2026-12-10", KindForexBuying, "", "", false},
		{"before any bulletin", "2026-11-13", KindForexBuying, "", "", false},
	}
	for _, c := range cases {
		r, v, ok := PickRate(rates, date(c.invoice), c.kind)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if v.String() != c.want || r.Date.Format("2006-01-02") != c.wantDay {
			t.Errorf("%s: got %s on %s, want %s on %s", c.name, v, r.Date.Format("2006-01-02"), c.want, c.wantDay)
		}
	}
}

func TestMayAutoFill(t *testing.T) {
	if !MayAutoFill(false, "") {
		t.Error("a missing rate is filled")
	}
	if !MayAutoFill(true, SourceTCMB) {
		t.Error("an automatic rate follows the table and the setting")
	}
	if MayAutoFill(true, SourceManual) {
		t.Error("a rate entered by hand is never overwritten")
	}
	if MayAutoFill(true, "") {
		t.Error("a rate from the source document is kept")
	}
}

func TestMissingDaysSkipsWeekendsAndKnown(t *testing.T) {
	have := map[string]bool{"2026-11-18": true}
	got := MissingDays(date("2026-11-14"), date("2026-11-20"), have)
	var s []string
	for _, d := range got {
		s = append(s, d.Format("2006-01-02"))
	}
	want := "2026-11-16,2026-11-17,2026-11-19,2026-11-20"
	if strings.Join(s, ",") != want {
		t.Fatalf("missing = %v, want %s", s, want)
	}
}

func TestArchiveURL(t *testing.T) {
	got := ArchiveURL(DefaultBaseURL, date("2026-11-03"))
	if got != "https://www.tcmb.gov.tr/kurlar/202611/03112026.xml" {
		t.Fatalf("url = %s", got)
	}
}

func TestClientHolidayIs404(t *testing.T) {
	data, err := os.ReadFile("testdata/today.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/kurlar/today.xml", "/kurlar/202611/20112026.xml":
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := Client{HTTP: srv.Client(), BaseURL: srv.URL + "/kurlar"}
	ctx := context.Background()
	if b, err := c.Today(ctx); err != nil || len(b.Rates) != 5 {
		t.Fatalf("today: %v %d", err, len(b.Rates))
	}
	if _, err := c.Day(ctx, date("2026-11-20")); err != nil {
		t.Fatalf("archive day: %v", err)
	}
	if _, err := c.Day(ctx, date("2026-10-29")); !errors.Is(err, ErrNotPublished) {
		t.Fatalf("holiday: err = %v, want ErrNotPublished", err)
	}
}

func TestClientNetworkDown(t *testing.T) {
	c := Client{HTTP: &http.Client{Timeout: time.Second}, BaseURL: "http://127.0.0.1:1/kurlar"}
	if _, err := c.Today(context.Background()); err == nil || errors.Is(err, ErrNotPublished) {
		t.Fatalf("err = %v, want a network error", err)
	}
}
