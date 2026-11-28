package edm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

func open(t *testing.T, f *testfakes.EDM, user, pass string) *Provider {
	t.Helper()
	srv := testfakes.Serve(t, f)
	p, err := New(Config{Username: user, Password: pass, Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListInboxAndOutbox(t *testing.T) {
	f := testfakes.EDMSample()
	p := open(t, f, testfakes.EDMUsername, testfakes.EDMPassword)
	ctx := context.Background()
	q := provider.ListQuery{Kind: provider.EInvoice, From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC), Limit: 10}
	in, err := p.ListInbox(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Items) != 1 || in.Next != "" {
		t.Fatalf("inbox: %+v", in)
	}
	d := in.Items[0]
	if d.UUID != "5f0c7a52-3a51-4c43-9c3b-6c1b0f0e0a01" || d.ID != "EDM2026000000001" || d.Direction != provider.Inbound ||
		d.SenderVKN != "1234567801" || d.SenderName != "Tedarikçi A.Ş." || d.Payable != "1180.00" || d.Currency != "TRY" ||
		d.Profile != "TEMELFATURA" || d.Type != "SATIS" || d.Status != provider.StatusReceived || d.GIBCode != 1300 || d.CreatedAt.IsZero() {
		t.Fatalf("inbox item: %+v", d)
	}
	out, err := p.ListOutbox(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Status != provider.StatusDelivered || out.Items[0].Direction != provider.Outbound {
		t.Fatalf("outbox: %+v", out)
	}
	if f.Logins != 1 {
		t.Fatalf("one session for both listings, got %d logins", f.Logins)
	}
	for _, want := range []string{"<DIRECTION>OUT</DIRECTION>", "<HEADER_ONLY>Y</HEADER_ONLY>", "<INVOICE_CONTENT_TYPE>XML</INVOICE_CONTENT_TYPE>", "<CR_START_DATE>2026-11-01T03:00:00+03:00</CR_START_DATE>", "tem:GetInvoiceRequest"} {
		if !strings.Contains(f.LastBody, want) {
			t.Errorf("request lacks %s:\n%s", want, f.LastBody)
		}
	}
	if err := p.Logout(ctx); err != nil || f.Logouts != 1 {
		t.Fatalf("logout: %v (%d)", err, f.Logouts)
	}
}

func TestListPagesWithOffset(t *testing.T) {
	f := testfakes.EDMSample()
	f.Invoices[1].Direction = "IN"
	p := open(t, f, testfakes.EDMUsername, testfakes.EDMPassword)
	page, err := p.ListInbox(context.Background(), provider.ListQuery{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Next != "1" {
		t.Fatalf("first page: %+v", page)
	}
	page, err = p.ListInbox(context.Background(), provider.ListQuery{Limit: 1, Cursor: page.Next})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "ATL2026000000007" {
		t.Fatalf("second page: %+v %v", page, err)
	}
}

func TestFetchUBL(t *testing.T) {
	f := testfakes.EDMSample()
	p := open(t, f, testfakes.EDMUsername, testfakes.EDMPassword)
	res, err := p.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Outbound, UUID: "5f0c7a52-3a51-4c43-9c3b-6c1b0f0e0a02"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ContentType != "application/xml" || !strings.Contains(string(res.Data), "ATL2026000000007") {
		t.Fatalf("fetch: %+v", res)
	}
	if _, err := p.Fetch(context.Background(), provider.FetchQuery{UUID: "00000000-0000-0000-0000-000000000000"}); provider.KindOf(err) != provider.KindNotFound {
		t.Fatalf("missing document: %v", err)
	}
}

func TestWrongPasswordIsAuth(t *testing.T) {
	f := testfakes.EDMSample()
	p := open(t, f, testfakes.EDMUsername, "wrong")
	err := p.Login(context.Background())
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.KindAuth || pe.Code != "2001" || !strings.Contains(pe.Message, "şifre") {
		t.Fatalf("got %#v", err)
	}
}

func TestServiceDownIsUnavailable(t *testing.T) {
	f := testfakes.EDMSample()
	f.Down = true
	p := open(t, f, testfakes.EDMUsername, testfakes.EDMPassword)
	if err := p.Login(context.Background()); provider.KindOf(err) != provider.KindUnavailable {
		t.Fatalf("got %v", err)
	}
}

func TestSessionExpiredLogsInAgain(t *testing.T) {
	f := testfakes.EDMSample()
	p := open(t, f, testfakes.EDMUsername, testfakes.EDMPassword)
	p.session, p.gen = "stale", 1
	if _, err := p.ListInbox(context.Background(), provider.ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if f.Logins != 1 {
		t.Fatalf("logins %d", f.Logins)
	}
}

type timeoutDoer struct{}

type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func (timeoutDoer) Do(*http.Request) (*http.Response, error) { return nil, netTimeout{} }

func TestTimeoutIsTimeout(t *testing.T) {
	p, err := New(Config{Username: "u", Password: "p", Endpoint: "https://edm.invalid/svc", HTTP: timeoutDoer{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Login(context.Background()); provider.KindOf(err) != provider.KindTimeout {
		t.Fatalf("got %v", err)
	}
}

func TestFaultKinds(t *testing.T) {
	cases := []struct {
		op, msg string
		want    provider.ErrorKind
	}{
		{"login", "Kullanıcı adı veya şifre hatalı", provider.KindAuth},
		{"login", "Bu kullanıcının web servis yetkisi bulunmamaktadır", provider.KindPermission},
		{"login", "", provider.KindAuth},
		{"list-inbox", "Aktif session bulunamadı", provider.KindAuth},
		{"fetch", "Fatura bulunamadı", provider.KindNotFound},
		{"list-inbox", "İşlem zaman aşımına uğradı", provider.KindTimeout},
		{"list-inbox", "beklenmeyen", provider.KindUnknown},
	}
	for _, c := range cases {
		if got := faultKind(c.op, c.msg); got != c.want {
			t.Errorf("%s %q: %s, want %s", c.op, c.msg, got, c.want)
		}
	}
}

func TestParseFaultDetail(t *testing.T) {
	re := parseFault(`<RequestFault xmlns="http://tempuri.org/"><INTL_TXN_ID xmlns="">7</INTL_TXN_ID><ERROR_CODE xmlns="">2001</ERROR_CODE>` +
		`<ERROR_SHORT_DES xmlns="">Kullanıcı adı veya şifre hatalı</ERROR_SHORT_DES><ERROR_LONG_DES xmlns="">Giriş başarısız</ERROR_LONG_DES></RequestFault>`)
	if re == nil || re.Code != "2001" || re.message() != "Kullanıcı adı veya şifre hatalı (Giriş başarısız)" {
		t.Fatalf("%+v", re)
	}
	if parseFault("") != nil || parseFault("<x/>") != nil {
		t.Fatal("empty detail")
	}
	e := classify("login", &provider.Error{Kind: provider.KindTransport, Err: &soap.Fault{Reason: "x", Detail: `<RequestFault><ERROR_CODE>9</ERROR_CODE></RequestFault>`}})
	if e.Kind != provider.KindAuth || e.Code != "9" {
		t.Fatalf("%+v", e)
	}
}

func TestDocumentStatus(t *testing.T) {
	if s := documentStatus("SEND - SUCCEED", "", 1300); s != provider.StatusDelivered {
		t.Fatal(s)
	}
	if s := documentStatus("RECEIVE - SUCCEED", "RED", 0); s != provider.StatusRejected {
		t.Fatal(s)
	}
	if d := documentInfo(invoice{Header: invoiceHeader{EArchive: "true"}}, provider.Outbound); d.Kind != provider.EArchive {
		t.Fatal(d.Kind)
	}
}

// 35 §4: gönderilemeyen paket teslim hatasıdır; gelen faturaya verilen
// yanıt sürerken ya da başarısız olduğunda fatura "alındı" kalır.
func TestDocumentStatusTable(t *testing.T) {
	cases := []struct {
		status string
		gib    int
		want   provider.Status
	}{
		{"PACKAGE - FAILED", 0, provider.StatusFailed},
		{"PACKAGE - FAIL", 0, provider.StatusFailed},
		{"SIGN - FAILED", 0, provider.StatusFailed},
		{"SEND - FAILED", 0, provider.StatusFailed},
		{"PACKAGE - PROCESSING", 0, provider.StatusProcessing},
		{"SEND - WAIT_GIB_RESPONSE", 0, provider.StatusProcessing},
		{"RECEIVE - WAIT_APPLICATION_RESPONSE", 1300, provider.StatusReceived},
		{"ACCEPT - PROCESSING", 1300, provider.StatusReceived},
		{"ACCEPT - FAILED", 1300, provider.StatusReceived},
		{"REJECT - FAILED", 1300, provider.StatusReceived},
		{"REJECT - WAIT_SYSTEM_RESPONSE", 1300, provider.StatusReceived},
		// The failing code of the answer's envelope does not fail the invoice.
		{"ACCEPT - FAILED", 1160, provider.StatusReceived},
		{"ACCEPT - SUCCEED", 1300, provider.StatusAccepted},
		{"REJECT - SUCCEED", 1300, provider.StatusRejected},
		// A failing GİB code still wins over an in-progress send.
		{"SEND - PROCESSING", 1160, provider.StatusFailed},
	}
	for _, c := range cases {
		if got := documentStatus(c.status, "", c.gib); got != c.want {
			t.Errorf("%q gib=%d: %s, want %s", c.status, c.gib, got, c.want)
		}
	}
}
