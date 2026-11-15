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

type nesDoc struct {
	uuid, number, answer string
	incoming             bool
	created              time.Time
	xml                  []byte
}

// fakeNes answers the NES REST paths the go-ubltr module reads (page /
// pageSize / totalCount paging, XML download) and records every request.
type fakeNes struct {
	mu   sync.Mutex
	docs []nesDoc
	reqs []string
}

func (f *fakeNes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	switch p {
	case "/einvoice/v1/incoming/invoices", "/einvoice/v1/outgoing/invoices", "/earchive/v1/invoices":
		f.list(w, r, p == "/einvoice/v1/incoming/invoices", p == "/earchive/v1/invoices")
		return
	}
	if rest, ok := strings.CutSuffix(p, "/xml"); ok {
		id := rest[strings.LastIndex(rest, "/")+1:]
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write(d.xml)
				return
			}
		}
	}
	http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
}

func (f *fakeNes) list(w http.ResponseWriter, r *http.Request, incoming, archive bool) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	from, _ := time.Parse(time.RFC3339, q.Get("startCreateDate"))
	to, _ := time.Parse(time.RFC3339, q.Get("endCreateDate"))
	var hit []nesDoc
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
	start := min((page-1)*size, len(hit))
	end := min(start+size, len(hit))
	type item struct {
		ID                   string  `json:"id"`
		CreatedAt            string  `json:"createdAt"`
		IssueDate            string  `json:"issueDate"`
		DocumentNumber       string  `json:"documentNumber"`
		ProfileID            string  `json:"profileId"`
		InvoiceTypeCode      string  `json:"invoiceTypeCode"`
		DocumentCurrencyCode string  `json:"documentCurrencyCode"`
		PayableAmount        float64 `json:"payableAmount"`
		DocumentAnswer       string  `json:"documentAnswer"`
		OutgoingStatus       string  `json:"outgoingStatus,omitempty"`
	}
	out := struct {
		Page       int    `json:"page"`
		PageSize   int    `json:"pageSize"`
		TotalCount int    `json:"totalCount"`
		Data       []item `json:"data"`
	}{Page: page, PageSize: size, TotalCount: len(hit), Data: []item{}}
	for _, d := range hit[start:end] {
		it := item{ID: d.uuid, CreatedAt: d.created.In(istanbul).Format(time.RFC3339), IssueDate: "2026-11-01T00:00:00+03:00",
			DocumentNumber: d.number, ProfileID: "TICARIFATURA", InvoiceTypeCode: "SATIS", DocumentCurrencyCode: "TRY", PayableAmount: 1,
			DocumentAnswer: d.answer}
		if !d.incoming {
			it.OutgoingStatus = "EnvelopeHasBeenTransferredToReceiverSuccessfully"
		}
		out.Data = append(out.Data, it)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// NES (#59, 30 §9): sahte REST sunucusuyla uçtan uca senkron. totalCount
// ile sayfalama, yalnızca GET, erpTransferred yok; reddedilen ticari fatura
// cancelled, yanıt bekleyen PENDING_RESPONSE.
func TestNesSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	f := &fakeNes{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	var inbound []string
	for i := range 3 {
		xml, ettn := invoiceXML(t, 500+i, true, "TICARIFATURA")
		f.docs = append(f.docs, nesDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 500+i), answer: "Waiting", incoming: true,
			created: created.Add(time.Duration(i) * time.Hour), xml: xml})
		inbound = append(inbound, ettn)
	}
	xml, out := invoiceXML(t, 600, false, "TICARIFATURA")
	f.docs = append(f.docs, nesDoc{uuid: out, number: "TST2026000000600", answer: "Rejected", created: created, xml: xml})
	srv := httptest.NewServer(f)
	defer srv.Close()

	e.now = mustTime(t, "2026-11-11T10:00:00+03:00")
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("nes", provider.Config{APIKey: "test-key", Endpoint: srv.URL, VKN: own})
	})
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 4 || rep.Counters.UBLFetched != 4 || rep.Counters.Pages < 3 {
		t.Fatalf("report %+v", rep)
	}
	if s := e.invoiceStatus(t, out); s != StatusCancelled {
		t.Fatalf("rejected outgoing invoice = %s", s)
	}
	if d := e.doc(t, inbound[0], "incoming"); len(d.Warnings) != 1 || d.Warnings[0] != WarnPendingResponse {
		t.Fatalf("waiting commercial invoice warnings = %v", d.Warnings)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reqs {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("non-read call %s", r)
		}
		if strings.Contains(r, "erpTransferred") {
			t.Errorf("listing asked for not-transferred only: %s", r)
		}
	}
}
