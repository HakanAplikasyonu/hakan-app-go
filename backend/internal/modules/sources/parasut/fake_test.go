package parasut

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeParasut serves a JSON:API data set the way 10 describes it: pages of
// page[size], sorted by -issue_date, relations in included.
type fakeParasut struct {
	mu       sync.Mutex
	sales    []resource
	bills    []resource
	included []resource
	refreshs int
	requests []string
}

func (f *fakeParasut) add(list *[]resource, r resource, inc ...resource) {
	*list = append(*list, r)
	f.included = append(f.included, inc...)
}

func (f *fakeParasut) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv
}

// handler is the fake's HTTP handler (contract tests wrap it with faults).
func (f *fakeParasut) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Path == "/oauth/token" {
			_ = r.ParseForm()
			if r.Form.Get("client_secret") != "sec" {
				w.WriteHeader(401)
				return
			}
			f.refreshs++
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "acc" + strconv.Itoa(f.refreshs), "refresh_token": "ref" + strconv.Itoa(f.refreshs), "expires_in": 7200})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer acc") {
			w.WriteHeader(401)
			return
		}
		var list []resource
		switch r.URL.Path {
		case "/v4/me":
			co := resource{ID: "777", Type: "companies", Attributes: attrs(map[string]any{"name": "Atlas", "legal_name": "Atlas Gıda Ticaret A.Ş."})}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "1", "type": "users"}, "included": []resource{co}})
			return
		case "/v4/777/sales_invoices":
			list = f.sales
		case "/v4/777/purchase_bills":
			list = f.bills
		default:
			w.WriteHeader(404)
			return
		}
		sorted := append([]resource(nil), list...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].str("issue_date") > sorted[j].str("issue_date") })
		page, _ := strconv.Atoi(r.URL.Query().Get("page[number]"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page[size]"))
		lo, hi := (page-1)*size, page*size
		if lo > len(sorted) {
			lo = len(sorted)
		}
		if hi > len(sorted) {
			hi = len(sorted)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": sorted[lo:hi], "included": f.included, "meta": map[string]any{"total_count": len(sorted)}})
	})
}

func attrs(m map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func relOne(typ, id string) rel {
	if id == "" {
		return rel{Data: json.RawMessage("null")}
	}
	b, _ := json.Marshal(ref{ID: id, Type: typ})
	return rel{Data: b}
}

func relMany(typ string, ids ...string) rel {
	refs := make([]ref, len(ids))
	for i, id := range ids {
		refs[i] = ref{ID: id, Type: typ}
	}
	b, _ := json.Marshal(refs)
	return rel{Data: b}
}

// money formats like Paraşüt's string decimals.
func money(v float64) string { return fmt.Sprintf("%.2f", v) }
