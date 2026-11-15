package entegrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

type trendyolDoc struct {
	uuid, number, reply string
	created             time.Time
	xml                 []byte
}

// fakeTrendyol answers sign-in, incoming e-Fatura search and the two-step
// download (temporary link, then the file) the go-ubltr module uses.
type fakeTrendyol struct {
	mu   sync.Mutex
	url  string
	docs []trendyolDoc
	reqs []string
}

var trendyolReadPaths = map[string]bool{
	"POST /api/auth/signin":                                true,
	"POST /api/invoice/documents/incoming-einvoice/search": true,
	"POST /api/invoice/documents/download":                 true,
}

func (f *fakeTrendyol) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	f.reqs = append(f.reqs, key)
	body, _ := io.ReadAll(r.Body)
	switch {
	case key == "POST /api/auth/signin":
		w.Header().Set("x-access-token", "tok")
		_, _ = io.WriteString(w, "{}")
		return
	case r.Header.Get("x-access-token") != "tok" && !strings.HasPrefix(r.URL.Path, "/dl/"):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	case key == "POST /api/invoice/documents/incoming-einvoice/search":
		f.search(w, body)
	case key == "POST /api/invoice/documents/download":
		var req struct {
			DocumentType string `json:"documentType"`
			InvoiceUUID  string `json:"invoiceUuid"`
		}
		_ = json.Unmarshal(body, &req)
		if req.DocumentType != "INCOMING_EINVOICE" {
			http.Error(w, "unexpected type", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, f.url+"/dl/"+req.InvoiceUUID)
	case strings.HasPrefix(key, "GET /dl/"):
		id := strings.TrimPrefix(r.URL.Path, "/dl/")
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				_, _ = w.Write(d.xml)
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.Error(w, "unexpected call", http.StatusMethodNotAllowed)
	}
}

func (f *fakeTrendyol) search(w http.ResponseWriter, body []byte) {
	var req struct {
		StartDate  string                   `json:"startDate"`
		EndDate    string                   `json:"endDate"`
		Pagination struct{ Page, Size int } `json:"pagination"`
	}
	_ = json.Unmarshal(body, &req)
	from, _ := time.ParseInLocation(time.DateOnly, req.StartDate, istanbul)
	to, _ := time.ParseInLocation(time.DateOnly, req.EndDate, istanbul)
	var hit []trendyolDoc
	for _, d := range f.docs {
		day := d.created.In(istanbul).Truncate(24 * time.Hour)
		if !from.IsZero() && d.created.Before(from) || !to.IsZero() && day.After(to.Add(24*time.Hour)) {
			continue
		}
		hit = append(hit, d)
	}
	size := max(req.Pagination.Size, 1)
	start := min(req.Pagination.Page*size, len(hit))
	end := min(start+size, len(hit))
	type item struct {
		InvoiceUUID   string `json:"invoiceUuid"`
		InvoiceID     string `json:"invoiceId"`
		Status        string `json:"status"`
		Currency      string `json:"currency"`
		PayableAmount int64  `json:"payableAmount"`
		Scenario      string `json:"scenario"`
		TypeCode      string `json:"invoiceTypeCode"`
		IssuedAt      string `json:"issuedAt"`
		CreatedAt     string `json:"createdAt"`
		SenderTaxID   string `json:"senderTaxId"`
		SenderTitle   string `json:"senderTitle"`
		ReplyType     string `json:"replyType,omitempty"`
		GibStatusCode int    `json:"gibStatusCode"`
	}
	out := struct {
		PageSize    int    `json:"pageSize"`
		CurrentPage int    `json:"currentPage"`
		Content     []item `json:"content"`
	}{PageSize: size, CurrentPage: req.Pagination.Page, Content: []item{}}
	for _, d := range hit[start:end] {
		out.Content = append(out.Content, item{InvoiceUUID: d.uuid, InvoiceID: d.number, Status: "SUCCESS", Currency: "TRY", PayableAmount: 100,
			Scenario: "TICARIFATURA", TypeCode: "SATIS", IssuedAt: "2026-11-01T00:00:00Z", CreatedAt: d.created.UTC().Format(time.RFC3339),
			SenderTaxID: "4840847211", SenderTitle: "Karşı Taraf", ReplyType: d.reply, GibStatusCode: 1300})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Trendyol (#59, 30 §9): yalnızca gelen e-fatura listelenebilir. Giden ve
// e-arşiv için liste ucu yok: istemci "desteklenmiyor" der, bağlantı
// kaydına not_listable yazılır; sessiz boş liste dönmez.
func TestTrendyolSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	f := &fakeTrendyol{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	var ettns []string
	for i, reply := range []string{"", "DENIED", ""} {
		xml, ettn := invoiceXML(t, 700+i, true, "TICARIFATURA")
		f.docs = append(f.docs, trendyolDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 700+i), reply: reply,
			created: created.Add(time.Duration(i) * time.Hour), xml: xml})
		ettns = append(ettns, ettn)
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	f.url = srv.URL

	open := func() (*Client, error) {
		return Open("trendyol", provider.Config{Username: "api@example.invalid", Password: "test", Endpoint: srv.URL,
			Options: map[string]string{"user_id": "1", "company_id": "2"}})
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
	if rep.Status != "success" || rep.Counters.New != 3 || rep.Counters.UBLFetched != 3 || rep.Counters.Pages < 2 {
		t.Fatalf("report %+v", rep)
	}
	if s := e.invoiceStatus(t, ettns[1]); s != StatusCancelled {
		t.Fatalf("denied invoice = %s", s)
	}
	var caps string
	_ = e.pool.QueryRow(ctx, `SELECT capabilities->>'not_listable' FROM source_connections WHERE id = $1`, e.conn.ID).Scan(&caps)
	if !strings.Contains(caps, "e_invoice:outgoing") || !strings.Contains(caps, "e_archive:outgoing") {
		t.Fatalf("capabilities.not_listable = %s", caps)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reqs {
		if !trendyolReadPaths[r] && !strings.HasPrefix(r, "GET /dl/") {
			t.Errorf("unexpected call %s", r)
		}
	}
}
