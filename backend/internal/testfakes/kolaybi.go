package testfakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// KolayBi credentials the fake accepts (docs/kaynak 11).
const (
	KolayBiChannel   = "HAKAN"
	KolayBiAPIKey    = "key-1"
	KolayBiCompanyID = "5"
)

// KolayBi fakes the KolayBi Ofis API: api_key → access token, Bearer +
// Channel headers, /companies, paged /invoices by type and issue date (even
// ids listed without lines, so the adapter fetches their detail) and
// /invoices/{id}.
type KolayBi struct {
	mu sync.Mutex
	// Docs are the records by document type (sale_invoice, purchase_invoice,
	// sale_return_invoice, general_expense).
	Docs map[string][]map[string]any
	// Tokens counts the access tokens handed out.
	Tokens int
	// Expired makes the next authenticated call answer 401 once.
	Expired bool
}

func (f *KolayBi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Channel") != KolayBiChannel {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.Path == "/kolaybi/v1/access_token" {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["api_key"] != KolayBiAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.Tokens++
		writeJSON(w, map[string]any{"data": map[string]any{"access_token": "t" + strconv.Itoa(f.Tokens)}})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer t") || f.Expired {
		f.Expired = false
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/kolaybi/v1/companies":
		writeJSON(w, map[string]any{"data": []map[string]any{{"id": 5, "title": "Atlas Gıda"}, {"id": 6, "title": "Atlas Lojistik"}}})
	case r.URL.Path == "/kolaybi/v1/invoices":
		q := r.URL.Query()
		if q.Get("company_id") != KolayBiCompanyID {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		var rows []map[string]any
		for _, d := range f.Docs[q.Get("type")] {
			if d["order_date"].(string) >= q.Get("min_issue_date") && d["order_date"].(string) <= q.Get("max_issue_date") {
				row := map[string]any{}
				for k, v := range d {
					if k == "items" && d["id"].(int)%2 == 0 {
						continue // even ids: list without lines → detail call
					}
					row[k] = v
				}
				rows = append(rows, row)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["id"].(int) < rows[j]["id"].(int) })
		page, _ := strconv.Atoi(q.Get("page"))
		const per = 15
		last := (len(rows) + per - 1) / per
		lo, hi := min((page-1)*per, len(rows)), min(page*per, len(rows))
		writeJSON(w, map[string]any{"data": map[string]any{"data": rows[lo:hi], "current_page": page, "last_page": last}})
	case strings.HasPrefix(r.URL.Path, "/kolaybi/v1/invoices/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/kolaybi/v1/invoices/"))
		for _, list := range f.Docs {
			for _, d := range list {
				if d["id"].(int) == id {
					writeJSON(w, map[string]any{"data": d})
					return
				}
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// KolayBiDoc is one KolayBi record as the API returns it: quantity × 12.50
// at 20 % VAT, to Yıldız Market.
func KolayBiDoc(id int, date string, qty int) map[string]any {
	net := float64(qty) * 12.5
	vat := net * 0.2
	return map[string]any{"id": id, "serial_no": fmt.Sprintf("KB%07d", id), "order_date": date, "currency": "TRY",
		"document_type": "SATIS", "document_scenario": "EARSIVFATURA", "description": "kayıt",
		"associate":   map[string]any{"name": "Yıldız Market", "identity_no": "9100000021", "is_corporate": true},
		"items":       []map[string]any{{"description": "Ürün", "quantity": strconv.Itoa(qty), "unit_price": "12.50", "vat_rate": 20}},
		"grand_total": fmt.Sprintf("%.2f", net+vat), "total_vat": fmt.Sprintf("%.2f", vat)}
}

// KolayBiSample is a small October data set: three sales and one purchase.
func KolayBiSample() *KolayBi {
	return &KolayBi{Docs: map[string][]map[string]any{
		"sale_invoice":     {KolayBiDoc(1, "2026-10-03", 2), KolayBiDoc(2, "2026-10-09", 4), KolayBiDoc(3, "2026-10-20", 1)},
		"purchase_invoice": {KolayBiDoc(4, "2026-10-12", 3)},
	}}
}
