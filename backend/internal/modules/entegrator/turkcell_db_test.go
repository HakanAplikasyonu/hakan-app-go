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

type turkcellDoc struct {
	uuid, number string
	created      time.Time
	xml          []byte
}

// fakeTurkcell answers the inbox list (pageIndex / hasNextPage) and UBL
// download of the go-ubltr turkcell module and records every request.
type fakeTurkcell struct {
	mu   sync.Mutex
	docs []turkcellDoc
	reqs []string
}

func (f *fakeTurkcell) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	if r.Header.Get("x-api-key") != "test-key" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "write call", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/v1/inboxinvoice/list" {
		f.list(w, r)
		return
	}
	if rest, ok := strings.CutSuffix(r.URL.Path, "/ubl"); ok && strings.HasPrefix(rest, "/v2/inboxinvoice/") {
		id := strings.TrimPrefix(rest, "/v2/inboxinvoice/")
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				_, _ = w.Write(d.xml)
				return
			}
		}
	}
	http.NotFound(w, r)
}

func (f *fakeTurkcell) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("pageIndex"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	const layout = "2006-01-02 15:04:05"
	from, _ := time.ParseInLocation(layout, q.Get("startDate"), istanbul)
	to, _ := time.ParseInLocation(layout, q.Get("endDate"), istanbul)
	var hit []turkcellDoc
	for _, d := range f.docs {
		c := d.created.Truncate(time.Second)
		if !from.IsZero() && c.Before(from) || !to.IsZero() && c.After(to) {
			continue
		}
		hit = append(hit, d)
	}
	start := min((page-1)*size, len(hit))
	end := min(start+size, len(hit))
	type item struct {
		ID            string  `json:"id"`
		InvoiceNumber string  `json:"invoiceNumber"`
		ExecutionDate string  `json:"executionDate"`
		Type          int     `json:"type"`
		TipType       int     `json:"tipType"`
		TargetTitle   string  `json:"targetTitle"`
		TargetVknTckn string  `json:"targetVknTckn"`
		PayableAmount float64 `json:"payableAmount"`
		Currency      string  `json:"currency"`
		CreatedDate   string  `json:"createdDate"`
	}
	out := struct {
		PageIndex   int    `json:"pageIndex"`
		HasNextPage bool   `json:"hasNextPage"`
		Items       []item `json:"items"`
	}{PageIndex: page, HasNextPage: end < len(hit), Items: []item{}}
	for _, d := range hit[start:end] {
		out.Items = append(out.Items, item{ID: d.uuid, InvoiceNumber: d.number, ExecutionDate: "2026-11-01T00:00:00", Type: 0, TipType: 1,
			TargetTitle: "Karşı Taraf", TargetVknTckn: "4840847211", PayableAmount: 1, Currency: "TRY", CreatedDate: d.created.Format(time.RFC3339)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Turkcell (#59, 30 §9): yalnızca gelen e-fatura listelenir; liste ucu
// olmayan kutular not_listable, istemci Unsupported döner. isNew
// gönderilmez, yalnızca GET çağrılır.
func TestTurkcellSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	RegisterListProfile("turkcell", ListProfile{PageSize: 2, ListGap: turkcellListGap}) // small pages; pacing is off (RequestGap < 0)
	defer RegisterListProfile("turkcell", ListProfile{PageSize: 500, ListGap: turkcellListGap})
	f := &fakeTurkcell{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	for i := range 3 {
		xml, ettn := invoiceXML(t, 800+i, true, "")
		f.docs = append(f.docs, turkcellDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 800+i), created: created.Add(time.Duration(i) * time.Hour), xml: xml})
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	open := func() (*Client, error) {
		return Open("turkcell", provider.Config{APIKey: "test-key", Endpoint: srv.URL})
	}
	c, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.List(ctx, provider.Outbound, ListQuery{Kind: provider.EInvoice}); kindOf(err) != provider.KindUnsupported {
		t.Fatalf("outbox listing: err = %v, want unsupported", err)
	}
	if got := NotListable(c); strings.Join(got, ",") != "e_invoice:outgoing,e_archive:outgoing" {
		t.Fatalf("NotListable = %v", got)
	}

	e.now = mustTime(t, "2026-11-11T10:00:00+03:00")
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) { return open() })
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 3 || rep.Counters.UBLFetched != 3 || rep.Counters.Pages != 2 {
		t.Fatalf("report %+v", rep)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reqs {
		if !strings.HasPrefix(r, "GET ") || strings.Contains(r, "isNew") {
			t.Errorf("unexpected call %s", r)
		}
	}
}

// ListGap iki liste çağrısı arasında beklenir.
func TestListGapPacesListings(t *testing.T) {
	j := &job{s: &Service{cfg: Config{RequestGap: time.Millisecond}}}
	ctx := context.Background()
	start := time.Now()
	for range 3 {
		if err := j.throttleList(ctx, 40*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Fatalf("three listings took %s, want ≥ 80ms", d)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := j.throttleList(cctx, time.Hour); err == nil {
		t.Fatal("cancelled wait returned nil")
	}
}
