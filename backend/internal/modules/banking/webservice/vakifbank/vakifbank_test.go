package vakifbank

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
)

var creds = webservice.Credentials{"customer_no": "123456789", "username": "atlas_ws", "password": "p<&>1"}

func window() webservice.Window {
	return webservice.Window{
		Start: time.Date(2026, 11, 3, 0, 0, 0, 0, istanbul),
		End:   time.Date(2026, 11, 3, 23, 59, 0, 0, istanbul),
	}
}

func serve(t *testing.T, status int, body string, seen *string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if seen != nil {
			*seen = r.Header.Get("Content-Type") + "\n" + string(b)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client())
}

func TestFetchMapsAccountsAndMovements(t *testing.T) {
	fixture, err := os.ReadFile("testdata/getirhareket.xml")
	if err != nil {
		t.Fatal(err)
	}
	var req string
	c := serve(t, 200, string(fixture), &req)
	res, err := c.Fetch(context.Background(), creds, window())
	if err != nil {
		t.Fatal(err)
	}

	// Request: SOAP 1.2 with WS-Addressing (22 §1), escaped credentials, wall-clock dates.
	for _, want := range []string{
		`application/soap+xml`, `action="Peak.Integration.ExternalInbound.Ekstre/ISOnlineEkstreServis/GetirHareket"`,
		`<wsa:Action>Peak.Integration.ExternalInbound.Ekstre/ISOnlineEkstreServis/GetirHareket</wsa:Action>`,
		`<wsa:To>` + c.Endpoint + `</wsa:To>`,
		`<dto:Sifre>p&lt;&amp;&gt;1</dto:Sifre>`,
		`<dto:SorguBaslangicTarihi>2026-11-03 00:00</dto:SorguBaslangicTarihi>`,
		`<dto:SorguBitisTarihi>2026-11-03 23:59</dto:SorguBitisTarihi>`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("request lacks %q", want)
		}
	}
	if strings.Contains(req, "HesapNo") {
		t.Error("no account filter expected")
	}

	if len(res.Accounts) != 1 {
		t.Fatalf("accounts: %d", len(res.Accounts))
	}
	a := res.Accounts[0]
	if a.IBAN != "TR590001500158007301234567" || a.Currency != "TRY" || a.Kind != "demand" || !a.Active ||
		a.OwnerTaxID != "9000000017" || *a.Balance != "21485.50" || *a.Opening != "10000.00" {
		t.Fatalf("account: %+v", a)
	}
	if len(a.Movements) != 2 {
		t.Fatalf("movements: %d", len(a.Movements))
	}
	in, out := a.Movements[0], a.Movements[1]
	if in.SourceID != "2026110300001" || in.Direction != banking.DirIn || in.Amount != "12000.00" ||
		in.CounterpartyName != "YILDIZ MARKET LTD STI" || in.CounterpartyTaxID != "9100000021" ||
		in.CounterpartyIBAN != "TR320010009999901234567890" || in.BankReference != "REF-1" ||
		*in.BalanceAfter != "22000.00" || in.SourceSystem != "bank_vakifbank" || in.CaptureMethod != banking.CaptureAPI {
		t.Fatalf("incoming: %+v", in)
	}
	if in.BookedAt.Nanosecond() != 137_000_000 {
		t.Errorf("precise time not used: %v", in.BookedAt)
	}
	if out.Direction != banking.DirOut || out.CounterpartyName != "KALEM KIRTASIYE" || out.CounterpartyTaxID != "9200000035" ||
		out.PaymentPurpose != "M01" || *out.BalanceAfter != "21485.50" {
		t.Fatalf("outgoing: %+v", out)
	}
	// 20 §9 madde 11: 23:59 stays on its own day.
	if got := out.TransactionDate.Format("2006-01-02"); got != "2026-11-03" {
		t.Errorf("late movement on %s", got)
	}
}

func TestBankCodesMapToReasons(t *testing.T) {
	answer := func(code, msg string) string {
		return `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetirHareketResponse><GetirHareketResult>` +
			`<IslemKodu>` + code + `</IslemKodu><IslemAciklamasi>` + msg + `</IslemAciklamasi></GetirHareketResult></GetirHareketResponse></s:Body></s:Envelope>`
	}
	cases := map[string]webservice.Reason{
		"VBH0009": webservice.ReasonIPRejected,
		"VBH0012": webservice.ReasonBadCredentials,
		"VBH0011": webservice.ReasonInstructionMissing,
		"VBH0014": webservice.ReasonRateLimited,
		"VBH9999": webservice.ReasonBankError, // unknown code: bank_error, raw code kept
	}
	for code, want := range cases {
		_, err := serve(t, 200, answer(code, "x"), nil).Fetch(context.Background(), creds, window())
		be := webservice.AsError(err)
		if be.Reason != want || be.Code != code {
			t.Errorf("%s: got %+v", code, be)
		}
	}
	_, err := serve(t, 200, answer("VBH0010", "365 gün"), nil).Fetch(context.Background(), creds, window())
	if !errors.Is(err, ErrTooOld) {
		t.Errorf("VBH0010: %v", err)
	}
}

func TestFirewallRejection(t *testing.T) {
	_, err := serve(t, 403, "<html>The requested URL was rejected.</html>", nil).Fetch(context.Background(), creds, window())
	if be := webservice.AsError(err); be.Reason != webservice.ReasonIPRejected {
		t.Fatalf("got %+v", be)
	}
}

func TestInputChecks(t *testing.T) {
	c := New("http://127.0.0.1:1", nil)
	if _, err := c.Fetch(context.Background(), webservice.Credentials{"customer_no": "1"}, window()); webservice.AsError(err).Reason != webservice.ReasonBadCredentials {
		t.Error("missing credentials must not call the bank")
	}
	w := window()
	w.Account = "TR33"
	if _, err := c.Fetch(context.Background(), creds, w); webservice.AsError(err).Reason != webservice.ReasonInvalidRequest {
		t.Error("account number pattern")
	}
}
