package entegrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// restDoc is one invoice held by a fake REST integrator.
type restDoc struct {
	uuid, number, profile string
	incoming              bool
	created               time.Time
	xml                   []byte
}

// fakeNilvera answers the Nilvera REST paths the go-ubltr module reads
// (list pages with Page / PageSize / TotalPages, XML download) and refuses
// everything else, recording each request.
type fakeNilvera struct {
	mu   sync.Mutex
	docs []restDoc
	reqs []string
}

func (f *fakeNilvera) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	if r.Header.Get("Authorization") != "Bearer test-key" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "write call", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	switch {
	case p == "/einvoice/Purchase" || p == "/einvoice/Sale" || p == "/earchive/Invoices":
		f.list(w, r, p == "/einvoice/Purchase", p == "/earchive/Invoices")
	case strings.HasSuffix(p, "/xml"):
		id := strings.TrimSuffix(p[strings.LastIndex(strings.TrimSuffix(p, "/xml"), "/")+1:], "/xml")
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write(d.xml)
				return
			}
		}
		http.Error(w, `{"Message":"not found"}`, http.StatusNotFound)
	default:
		http.Error(w, "unexpected path", http.StatusNotFound)
	}
}

func (f *fakeNilvera) list(w http.ResponseWriter, r *http.Request, incoming, archive bool) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("Page"))
	size, _ := strconv.Atoi(q.Get("PageSize"))
	from, _ := time.Parse(time.RFC3339, q.Get("StartDate"))
	to, _ := time.Parse(time.RFC3339, q.Get("EndDate"))
	var hit []restDoc
	for _, d := range f.docs {
		if archive || d.incoming != incoming {
			continue
		}
		c := d.created.Truncate(time.Second)
		if !from.IsZero() && c.Before(from) || !to.IsZero() && c.After(to) {
			continue
		}
		hit = append(hit, d)
	}
	total := (len(hit) + size - 1) / size
	start := min((page-1)*size, len(hit))
	end := min(start+size, len(hit))
	type item struct {
		UUID, InvoiceNumber, InvoiceProfile, InvoiceType, CurrencyCode, IssueDate, CreatedDate, StatusCode string
		TaxNumber, SenderTaxNumber, ReceiverTaxNumber                                                      string
		PayableAmount                                                                                      float64
	}
	out := struct {
		Page, PageSize, TotalCount, TotalPages int
		Content                                []item
	}{Page: page, PageSize: size, TotalCount: len(hit), TotalPages: total, Content: []item{}}
	for _, d := range hit[start:end] {
		out.Content = append(out.Content, item{UUID: d.uuid, InvoiceNumber: d.number, InvoiceProfile: d.profile, InvoiceType: "SATIS",
			CurrencyCode: "TRY", IssueDate: "2026-11-01T00:00:00", CreatedDate: d.created.In(istanbul).Format("2006-01-02T15:04:05"),
			StatusCode: "succeed", TaxNumber: ownVKN, SenderTaxNumber: "4840847211", ReceiverTaxNumber: "4840847211", PayableAmount: 1})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Nilvera (#59, 30 §9): sahte REST sunucusuyla uçtan uca senkron. Sayfalama
// TotalPages ile ilerler, yalnızca GET çağrılır, IsTransfer gönderilmez.
func TestNilveraSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	f := &fakeNilvera{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	for i := range 3 {
		xml, ettn := invoiceXML(t, 300+i, true, "")
		f.docs = append(f.docs, restDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 300+i), profile: "TEMELFATURA", incoming: true,
			created: created.Add(time.Duration(i) * time.Hour), xml: xml})
	}
	xml, out := invoiceXML(t, 400, false, "")
	f.docs = append(f.docs, restDoc{uuid: out, number: "TST2026000000400", profile: "TEMELFATURA", created: created, xml: xml})
	srv := httptest.NewServer(f)
	defer srv.Close()

	e.now = mustTime(t, "2026-11-11T10:00:00+03:00")
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("nilvera", provider.Config{APIKey: "test-key", Endpoint: srv.URL, VKN: own})
	})
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 4 || rep.Counters.UBLFetched != 4 || rep.Counters.Pages < 3 {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 4 {
		t.Fatalf("invoices = %d", n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reqs {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("non-read call %s", r)
		}
		if strings.Contains(r, "IsTransfer") {
			t.Errorf("listing asked for not-transferred only: %s", r)
		}
	}
}
