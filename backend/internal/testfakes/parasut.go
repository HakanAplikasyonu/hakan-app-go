package testfakes

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Paraşüt app credentials and the company the fake serves (docs/kaynak 10).
const (
	ParasutClientID     = "cid"
	ParasutClientSecret = "sec"
	ParasutCompanyID    = "777"
)

// Parasut fakes the Paraşüt API v4: /oauth/token (authorization_code and
// refresh_token grants, checked by client_secret), Bearer tokens, /v4/me
// with the company in included, and JSON:API pages of sales_invoices and
// purchase_bills (page[number], page[size], sorted by -issue_date, relations
// in included).
//
// Resources are any JSON:API resource objects ({id, type, attributes,
// relationships}); the Paraşüt adapter's tests append their own typed ones.
type Parasut struct {
	mu        sync.Mutex
	Sales     []any
	Bills     []any
	Included  []any
	Refreshes int      // tokens handed out
	Requests  []string // path?query of every call
}

// Add appends r to list and its related resources to Included.
func (f *Parasut) Add(list *[]any, r any, inc ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*list = append(*list, r)
	f.Included = append(f.Included, inc...)
}

func (f *Parasut) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, r.URL.Path+"?"+r.URL.RawQuery)
	if r.URL.Path == "/oauth/token" {
		_ = r.ParseForm()
		if r.Form.Get("client_secret") != ParasutClientSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.Refreshes++
		writeJSON(w, map[string]any{"access_token": "acc" + strconv.Itoa(f.Refreshes), "refresh_token": "ref" + strconv.Itoa(f.Refreshes), "expires_in": 7200})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer acc") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var list []any
	switch r.URL.Path {
	case "/v4/me":
		co := map[string]any{"id": ParasutCompanyID, "type": "companies", "attributes": map[string]any{"name": "Atlas", "legal_name": "Atlas Gıda Ticaret A.Ş."}}
		writeJSON(w, map[string]any{"data": map[string]any{"id": "1", "type": "users"}, "included": []any{co}})
		return
	case "/v4/" + ParasutCompanyID + "/sales_invoices":
		list = f.Sales
	case "/v4/" + ParasutCompanyID + "/purchase_bills":
		list = f.Bills
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sorted := append([]any(nil), list...)
	sort.SliceStable(sorted, func(i, j int) bool { return issueDate(sorted[i]) > issueDate(sorted[j]) })
	pg, _ := strconv.Atoi(r.URL.Query().Get("page[number]"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page[size]"))
	lo, hi := min(max(pg-1, 0)*size, len(sorted)), min(max(pg, 1)*size, len(sorted))
	included := f.Included
	if included == nil {
		included = []any{}
	}
	writeJSON(w, map[string]any{"data": sorted[lo:hi], "included": included, "meta": map[string]any{"total_count": len(sorted)}})
}

// issueDate reads attributes.issue_date of a resource of any Go type.
func issueDate(r any) string {
	raw, _ := json.Marshal(r)
	var v struct {
		Attributes struct {
			IssueDate string `json:"issue_date"`
		} `json:"attributes"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.Attributes.IssueDate
}

// ParasutInvoice is a sales invoice or purchase bill resource: one line of
// net at 20 % VAT, contact 1 (Yıldız Market) in included.
func ParasutInvoice(id, typ, date, net string) map[string]any {
	return map[string]any{"id": id, "type": typ,
		"attributes": map[string]any{"issue_date": date, "invoice_no": "PRS" + id, "currency": "TRL", "item_type": "invoice",
			"net_total": net, "gross_total": net, "total_vat": "0.00", "description": "kayıt"},
		"relationships": map[string]any{"contact": map[string]any{"data": map[string]any{"id": "c1", "type": "contacts"}}}}
}

// ParasutSample is a small October data set: two sales invoices and a bill.
func ParasutSample() *Parasut {
	f := &Parasut{}
	contact := map[string]any{"id": "c1", "type": "contacts", "attributes": map[string]any{"name": "Yıldız Market", "tax_number": "9100000021", "contact_type": "company"}}
	f.Included = append(f.Included, contact)
	f.Sales = append(f.Sales, ParasutInvoice("1001", "sales_invoices", "2026-10-04", "100.00"), ParasutInvoice("1002", "sales_invoices", "2026-10-18", "250.00"))
	f.Bills = append(f.Bills, ParasutInvoice("2001", "purchase_bills", "2026-10-11", "80.00"))
	return f
}
