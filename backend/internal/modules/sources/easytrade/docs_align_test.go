package easytrade

// Alignment with Easytrade's API documentation (#830 follow-up): the
// documented invoice detail (customer.tax_number, details.direction /
// invoice_type / invoice_date, lines[]), the list paging meta (per_page ≤
// 100, meta.last_page, X-Company-ID) and Z reports, which the documentation
// does not offer: a 404 is skipped, not a failure. Fake answers only.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

func TestMapInvoiceTaxNumberFirst(t *testing.T) {
	for name, c := range map[string]struct {
		customer string
		vkn      string
	}{
		"tax_number first":            {`{"name":"A Ltd.","tax_number":"9100000021","tax_or_identity_number":"1234567890"}`, "9100000021"},
		"tax_or_identity fallback":    {`{"name":"A Ltd.","tax_or_identity_number":"9100000021"}`, "9100000021"},
		"empty tax_number falls back": {`{"name":"A Ltd.","tax_number":"","tax_or_identity_number":"9100000021"}`, "9100000021"},
	} {
		inv, _, skip := mapInvoice(payload(t, `{"id":1,"issue_date":"2026-10-03","customer":`+c.customer+`,"items":[]}`), model.DirectionOutgoing)
		if skip != "" || inv.Counterparty.TaxNumber != c.vkn {
			t.Errorf("%s: %q skip=%q", name, inv.Counterparty.TaxNumber, skip)
		}
	}
}

// The documented detail: header under details, lines[] (not items).
func TestMapInvoiceDocumentedDetails(t *testing.T) {
	inv, _, skip := mapInvoice(payload(t, `{
	  "id": 7,
	  "details": {"direction": "incoming", "invoice_type": "IADE", "profile": "TICARIFATURA",
	              "invoice_date": "2026-10-12", "currency": "USD", "exchange_rate": 41.5},
	  "customer": {"id": 3, "name": "Kuzey A.Ş.", "tax_number": "9100000021", "customer_type": "company", "is_e_invoice_user": true},
	  "meta": {"global_tax_exemption_code": "", "footer_withholding": null},
	  "lines": [{"name": "Ürün", "quantity": 1, "unit_code": "C62", "unit_price": 100, "tax_rate": 20, "tax_exclusive_amount": 100}]
	}`), model.DirectionOutgoing)
	if skip != "" {
		t.Fatalf("skip %q", skip)
	}
	if inv.Direction != model.DirectionIncoming || inv.Type != model.TypeIade || inv.ProfileRaw != "TICARIFATURA" ||
		inv.Period != "2026-10" || inv.Currency != "USD" || inv.ExchangeRate == nil || inv.ExchangeRate.String() != "41.5" ||
		len(inv.Items) != 1 || inv.Counterparty.TaxNumber != "9100000021" {
		t.Fatalf("invoice %+v", inv)
	}
}

// pagedFake lists 250 rows in pages, reports meta.last_page and checks
// the headers and per_page.
type pagedFake struct {
	mu       sync.Mutex
	rows     int
	pages    []int
	badQuery string
}

func (f *pagedFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	per, _ := strconv.Atoi(q.Get("per_page"))
	page, _ := strconv.Atoi(q.Get("page"))
	if per < 1 || per > 100 || r.Header.Get("X-Company-ID") != "42" {
		f.badQuery = r.URL.RawQuery + " company=" + r.Header.Get("X-Company-ID")
	}
	f.pages = append(f.pages, page)
	last := (f.rows + per - 1) / per
	var data []map[string]any
	for i := (page - 1) * per; i < min(page*per, f.rows); i++ {
		data = append(data, map[string]any{"id": i + 1, "transaction_date": "2026-10-15"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"current_page": page, "last_page": last, "per_page": per, "total": f.rows}})
}

func TestListPagingMeta(t *testing.T) {
	f := &pagedFake{rows: 250}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client(), "42", Token{Value: "tok"})
	c.Interval = 0
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	ids, err := c.listIDs(context.Background(), "/company/financial-transactions", "transaction_date", from, to)
	if err != nil || len(ids) != 250 {
		t.Fatalf("ids %d %v", len(ids), err)
	}
	if f.badQuery != "" {
		t.Fatalf("bad request: %s", f.badQuery)
	}
	if len(f.pages) != 3 || f.pages[2] != 3 {
		t.Fatalf("pages %v (meta.last_page is 3)", f.pages)
	}
	if PerPage > 100 {
		t.Fatalf("PerPage %d > 100", PerPage)
	}
}

// Z reports are not in Easytrade's documentation: a 404 is "not offered",
// so the item is skipped and the run does not fail.
func TestZReports404Skipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"message":"Not Found","data":null}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client(), "42", Token{Value: "tok"})
	c.Interval = 0
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	b, err := c.ReadItem(context.Background(), ItemZReports, []Branch{{ID: "1"}}, from, to)
	if !errors.Is(err, ErrNotSupported) || len(b.ZReports) != 0 {
		t.Fatalf("z 404: %v %d", err, len(b.ZReports))
	}
}
