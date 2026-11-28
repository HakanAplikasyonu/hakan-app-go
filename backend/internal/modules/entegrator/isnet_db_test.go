package entegrator

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// isnetDoc is one invoice the fake İşNet service holds.
type isnetDoc struct {
	ettn, number, profile, status string
	dir                           string // Incoming / Outgoing
	created                       time.Time
	xml                           []byte
}

// fakeIsnet answers SearchAllInvoice the way 32 §5 describes (request
// fields in alphabetical order, WCF response). It records every SOAP
// action so the test can prove only read methods were called.
type fakeIsnet struct {
	mu      sync.Mutex
	docs    []isnetDoc
	actions []string
	vkns    []string
}

var (
	reDir     = regexp.MustCompile(`<(?:[a-z]+:)?InvoiceDirection[^>]*>([A-Za-z]+)<`)
	reEttn    = regexp.MustCompile(`<(?:[a-z]+:)?Ettn[^>]*>([^<]+)<`)
	reMin     = regexp.MustCompile(`<(?:[a-z]+:)?MinInvoiceCreationDate[^>]*>([^<]+)<`)
	reMax     = regexp.MustCompile(`<(?:[a-z]+:)?MaxInvoiceCreationDate[^>]*>([^<]+)<`)
	rePage    = regexp.MustCompile(`<(?:[a-z]+:)?PageNumber[^>]*>([0-9]+)<`)
	rePer     = regexp.MustCompile(`<(?:[a-z]+:)?RecordsPerPage[^>]*>([0-9]+)<`)
	reXML     = regexp.MustCompile(`<(?:[a-z]+:)?IsXMLIncluded[^>]*>true<`)
	reVKN     = regexp.MustCompile(`<(?:[a-z]+:)?CompanyTaxCode[^>]*>([0-9]+)<`)
	reArchive = regexp.MustCompile(`<(?:[a-z]+:)?IsArchiveIncluded[^>]*>true<`)
)

const isnetWire = "2006-01-02T15:04:05"

func (f *fakeIsnet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b := string(body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, strings.Trim(r.Header.Get("SOAPAction"), `"`))
	if m := reVKN.FindStringSubmatch(b); m != nil {
		f.vkns = append(f.vkns, m[1])
	}
	if strings.Contains(b, "SearchArchiveInvoice") {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>
<SearchArchiveInvoiceResponse xmlns="http://tempuri.org/"><SearchArchiveInvoiceResult xmlns:a="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
<a:ArchiveInvoices/><a:ErrorMessage i:nil="true"/><a:PagingResponse><a:PageNumber>1</a:PageNumber><a:RecordsPerPage>2</a:RecordsPerPage><a:TotalNumberOfPages>0</a:TotalNumberOfPages><a:TotalNumberOfRecords>0</a:TotalNumberOfRecords></a:PagingResponse>
<a:Result>Success</a:Result></SearchArchiveInvoiceResult></SearchArchiveInvoiceResponse></s:Body></s:Envelope>`)
		return
	}
	if !strings.Contains(b, "SearchAllInvoice") {
		http.Error(w, "unexpected call", http.StatusInternalServerError)
		return
	}
	if !reArchive.MatchString(b) {
		http.Error(w, "IsArchiveIncluded must be true (32 §5)", http.StatusBadRequest)
		return
	}
	dir := "Incoming"
	if m := reDir.FindStringSubmatch(b); m != nil {
		dir = m[1]
	}
	trt := time.FixedZone("TRT", 3*3600)
	parse := func(re *regexp.Regexp) time.Time {
		if m := re.FindStringSubmatch(b); m != nil {
			t, _ := time.ParseInLocation(isnetWire, m[1], trt)
			return t
		}
		return time.Time{}
	}
	from, to := parse(reMin), parse(reMax)
	ettn := ""
	if m := reEttn.FindStringSubmatch(b); m != nil {
		ettn = strings.ToLower(m[1])
	}
	var hit []isnetDoc
	for _, d := range f.docs {
		if d.dir != dir || (ettn != "" && strings.ToLower(d.ettn) != ettn) {
			continue
		}
		// The real service keeps creation times with their fraction and
		// compares them to the whole-second bounds it receives.
		c := d.created
		if !from.IsZero() && c.Before(from) || !to.IsZero() && c.After(to) {
			continue
		}
		hit = append(hit, d)
	}
	page, per := 1, 20
	_, _ = fmt.Sscan(firstSub(rePage, b, "1"), &page)
	_, _ = fmt.Sscan(firstSub(rePer, b, "20"), &per)
	pages := (len(hit) + per - 1) / per
	start := min((page-1)*per, len(hit))
	end := min(start+per, len(hit))
	var items strings.Builder
	for _, d := range hit[start:end] {
		xml := ""
		if reXML.MatchString(b) {
			xml = "<a:InvoiceXml>" + base64.StdEncoding.EncodeToString(d.xml) + "</a:InvoiceXml>"
		}
		fmt.Fprintf(&items, `<a:Invoice><a:CurrencyCode>TRY</a:CurrencyCode><a:DetailStatus>Basariyla_Tamamlandi</a:DetailStatus>
<a:ETTN>%s</a:ETTN><a:InvoiceCreationDate>%s</a:InvoiceCreationDate><a:InvoiceDate>2026-11-01T00:00:00</a:InvoiceDate>
<a:InvoiceNumber>%s</a:InvoiceNumber><a:InvoiceType>SATIS</a:InvoiceType>%s<a:IsArchived>false</a:IsArchived>
<a:Receiver><a:ReceiverName>Karşı Taraf</a:ReceiverName><a:ReceiverTaxCode>4840847211</a:ReceiverTaxCode></a:Receiver>
<a:ScenarioType>%s</a:ScenarioType><a:Status>%s</a:Status><a:TotalPayableAmount>1.00</a:TotalPayableAmount></a:Invoice>`,
			d.ettn, d.created.In(trt).Format(isnetWire+".000"), d.number, xml, d.profile, d.status)
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>
<SearchAllInvoiceResponse xmlns="http://tempuri.org/"><SearchAllInvoiceResult xmlns:a="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
<a:ErrorMessage i:nil="true"/><a:Invoices>%s</a:Invoices>
<a:PagingResponse><a:PageNumber>%d</a:PageNumber><a:RecordsPerPage>%d</a:RecordsPerPage><a:TotalNumberOfPages>%d</a:TotalNumberOfPages><a:TotalNumberOfRecords>%d</a:TotalNumberOfRecords></a:PagingResponse>
<a:Result>Success</a:Result></SearchAllInvoiceResult></SearchAllInvoiceResponse></s:Body></s:Envelope>`,
		items.String(), page, per, pages, len(hit))
}

func firstSub(re *regexp.Regexp, s, def string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return def
}

// İşNet (32 §12, 30 §9): sahte SOAP servisiyle uçtan uca senkron. VKN her
// çağrıda bağlantı kaydındaki mükellef VKN'si; yalnızca SearchAllInvoice
// çağrılır; sayfalama TotalNumberOfPages ile ilerler.
func TestIsnetSyncAgainstFakeService(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	fake := &fakeIsnet{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	for i := range 3 {
		xml, ettn := invoiceXML(t, 100+i, true, "")
		fake.docs = append(fake.docs, isnetDoc{ettn: strings.ToUpper(ettn), number: fmt.Sprintf("TST2026%09d", 100+i), profile: "TEMELFATURA",
			status: "Basariyla_Tamamlandi", dir: "Incoming", created: created.Add(time.Duration(i) * time.Hour), xml: xml})
	}
	xml, ettn := invoiceXML(t, 200, false, "")
	fake.docs = append(fake.docs, isnetDoc{ettn: ettn, number: "TST2026000000200", profile: "TICARIFATURA",
		status: "Alici_Reddetti", dir: "Outgoing", created: created, xml: xml})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	e.now = mustTime(t, "2026-11-11T10:00:00+03:00")
	e.svc.WithOpener(func(_ context.Context, c db.SourceConnection, own string) (*Client, error) {
		name, _ := ProviderName(c.SourceSystem)
		if name != "fake" {
			t.Fatalf("unexpected source %s", c.SourceSystem)
		}
		// The VKN comes from the connection's taxpayer, never from input (32 §2).
		return Open("isnet", provider.Config{VKN: own, Endpoint: srv.URL, Options: map[string]string{"addressbook_endpoint": srv.URL}})
	})
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.New != 4 || rep.Counters.UBLFetched != 4 || rep.Status != "success" {
		t.Fatalf("report %+v", rep)
	}
	if rep.Counters.Pages < 3 {
		t.Fatalf("paging not used: %+v", rep.Counters)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 4 {
		t.Fatalf("invoices = %d", n)
	}
	d := e.doc(t, strings.ToLower(ettn), "outgoing")
	if d.StatusNormalized != StatusCancelled {
		t.Fatalf("rejected outgoing invoice = %s (%s)", d.StatusNormalized, d.StatusRaw)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, a := range fake.actions {
		if !strings.HasSuffix(a, "/SearchAllInvoice") && !strings.HasSuffix(a, "/SearchArchiveInvoice") {
			t.Errorf("non-read call %q", a)
		}
	}
	for _, v := range fake.vkns {
		if v != ownVKN {
			t.Errorf("request VKN %s, want the connection's %s", v, ownVKN)
		}
	}
	if len(fake.vkns) == 0 {
		t.Error("no CompanyTaxCode sent")
	}
}

// Denetim logu: her entegratör çağrısı bir satır; gövde ve kimlik bilgisi yok.
func TestAuditDoerLogsCalls(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	d := auditDoer{next: srv.Client(), log: log, system: "entegrator_isnet", vkn: "1234567890"}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/InvoiceService/InvoiceWCFService.svc", strings.NewReader("<secret>1234567890</secret>"))
	req.Header.Set("SOAPAction", `"http://tempuri.org/IInvoiceService/SearchAllInvoice"`)
	req.SetBasicAuth("user", "pass")
	resp, err := d.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	out := buf.String()
	for _, want := range []string{"entegrator_call", "SearchAllInvoice", "status=200", "entegrator_isnet", "vkn=12******90"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q: %s", want, out)
		}
	}
	for _, leak := range []string{"secret", "1234567890", "pass", "Basic"} {
		if strings.Contains(out, leak) {
			t.Errorf("log leaks %q: %s", leak, out)
		}
	}
}

// Ay sınırı (32 §3): İşNet sınırları saniye hassasiyetinde alır, oluşturma
// zamanını kesirli tutar. Pencerenin son saniyesinde (23:59:59.5) oluşan
// fatura kaçmaz: bitiş sonraki pencerenin başlangıcına yuvarlanır.
func TestIsnetLastSecondOfWindow(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	fake := &fakeIsnet{}
	xml, ettn := invoiceXML(t, 300, true, "")
	fake.docs = append(fake.docs, isnetDoc{ettn: ettn, number: "TST2026000000300", profile: "TEMELFATURA",
		status: "Basariyla_Tamamlandi", dir: "Incoming", created: mustTime(t, "2026-10-31T23:59:59.5+03:00"), xml: xml})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	// Incremental window [2026-10-29 00:00, 2026-11-01 00:00).
	e.now = mustTime(t, "2026-11-01T00:00:40+03:00")
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("isnet", provider.Config{VKN: own, Endpoint: srv.URL, Options: map[string]string{"addressbook_endpoint": srv.URL}})
	})
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.New != 1 || rep.Counters.UBLFetched != 1 {
		t.Fatalf("last-second invoice missed: %+v", rep.Counters)
	}
	_ = e.doc(t, strings.ToLower(ettn), "incoming")
}
