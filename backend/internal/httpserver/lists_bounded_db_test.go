package httpserver

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// List bounds (issue #453). Every office / taxpayer list either pages
// (limit / offset, a hard maximum) or is bounded by what it lists: a
// catalog, a configuration, or one run / period. A list that ignores
// ?limit=1 must be named in boundedLists with the reason; a new list that
// grows with the data and returns everything fails here.

// boundedLists ignore limit; each is bounded by its domain.
var boundedLists = map[string]string{
	"GET /v1/tenant/engagements/{engagementUuid}/late-documents":              "late documents of one taxpayer (#780 C), capped in SQL",
	"GET /v1/tenant/late-documents/summary":                                   "one row per taxpayer with late documents (#780 C), capped in SQL",
	"GET /v1/tenant/vat-report-rules":                                         "office VAT report rules (at most 10)",
	"GET /v1/tenant/roles":                                                    "office roles (system + custom, a handful)",
	"GET /v1/tenant/role-permissions":                                         "permission catalog",
	"GET /v1/tenant/einvoice-integrators":                                     "e-invoice integrator catalog (go-ubltr registry)",
	"GET /v1/tenant/petition-templates":                                       "petition template catalog",
	"GET /v1/tenant/warning-rules":                                            "warning rule catalog",
	"GET /v1/tenant/engagements/{engagementUuid}/warning-rules":               "warning rule catalog",
	"GET /v1/tenant/warnings/groups":                                          "one row per warning code",
	"GET /v1/tenant/engagements/{engagementUuid}/warnings/groups":             "one row per warning code",
	"GET /v1/tenant/excel-import-data-types":                                  "data type catalog",
	"GET /v1/tenant/expense-categories":                                       "expense category catalog (system + office)",
	"GET /v1/tenant/engagements/{engagementUuid}/expense-categories":          "expense category catalog (system + office)",
	"GET /v1/tenant/automations":                                              "office automation rules (7 ready-made + a handful the office adds)",
	"GET /v1/tenant/export-drivers":                                           "accounting export driver catalog",
	"GET /v1/tenant/bank-statement-templates":                                 "bank statement template catalog",
	"GET /v1/tenant/engagements/{engagementUuid}/reconciliation/letters":      "server-side cap (LIMIT)",
	"GET /v1/tenant/engagements/{engagementUuid}/account-plan/imports":        "server-side cap (LIMIT)",
	"GET /v1/tenant/engagements/{engagementUuid}/account-plan/trial-balances": "server-side cap (LIMIT)",
	"GET /v1/tenant/engagements/{engagementUuid}/tax-returns":                 "three returns a month, filtered by year",
	"GET /v1/taxpayer/important-documents":                                    "server-side cap (LIMIT 200): a company's important documents (#780 B)",
	"GET /v1/tenant/engagement-assignments":                                   "server-side cap (LIMIT): assignees of the office's engagements, one request instead of one per row (#639)",
	"GET /v1/tenant/communication/groups":                                     "office taxpayer groups (a handful per office, unique by name) (#780)",
}

var fileListRe = regexp.MustCompile(`/(export|pdf|file)$`)

// maxPage is the largest page any list may answer with.
const maxPage = 500

// TestListsBounded: as office A's owner (office lists) and taxpayer X
// (taxpayer panel lists), every list GET without a record id
//   - honours ?limit=1, or is in boundedLists;
//   - never answers more than maxPage items, even for ?limit=1000000;
//   - answers a bad limit / offset with 4xx, not 5xx.
func TestListsBounded(t *testing.T) {
	e := newAuthzEnv(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	taxX := matrixCaller{"taxX", e.tpTok, map[string]string{"X-Taxpayer": e.tpUUID}}
	seen := map[string]bool{}
	checked := 0
	for _, op := range specOps(t) {
		if op.method != http.MethodGet || identifying(op.path) || fileListRe.MatchString(op.path) {
			continue
		}
		var c matrixCaller
		switch {
		case strings.HasPrefix(op.path, "/v1/tenant/"):
			c = ownA
		case strings.HasPrefix(op.path, "/v1/taxpayer/"):
			c = taxX
		default:
			continue
		}
		key := op.method + " " + op.path
		seen[key] = true
		path := e.fill(t, op.path)
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		items := func(q string) (int, int, bool) {
			rec := e.request(t, c, op.method, path+sep+q, sweepCase{})
			var out struct {
				Data struct {
					Items *[]json.RawMessage `json:"items"`
				} `json:"data"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			if out.Data.Items == nil {
				return rec.Code, 0, false
			}
			return rec.Code, len(*out.Data.Items), true
		}
		code, n, isList := items("limit=1")
		if !isList {
			continue
		}
		checked++
		if code == http.StatusOK && n > 1 {
			if _, ok := boundedLists[key]; !ok {
				t.Errorf("%s ignores limit (%d items for limit=1): page it or name it in boundedLists with the reason", key, n)
			}
		}
		if code, n, _ := items("limit=1000000"); code >= 500 || n > maxPage {
			t.Errorf("%s?limit=1000000 = %d with %d items (max %d)", key, code, n, maxPage)
		}
		for _, q := range []string{"limit=abc", "limit=-1", "offset=-1", "offset=99999999999"} {
			if code, _, _ := items(q); code >= 500 {
				t.Errorf("%s?%s = %d, want 4xx or 200", key, q, code)
			}
		}
	}
	for key := range boundedLists {
		if !seen[key] {
			t.Errorf("boundedLists names %s, which is not a list in openapi.yaml any more", key)
		}
	}
	if checked < 60 {
		t.Fatalf("only %d lists checked", checked)
	}
	t.Logf("%d lists checked", checked)
}
