package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPeriodFilterConsistent (issue #333): every GET under /v1/tenant and
// /v1/taxpayer whose spec declares a `period` query parameter answers a
// malformed period with 400 INVALID_PERIOD and a valid one with 2xx — the
// panel's global period selector relies on the same contract everywhere.
// A list that declares the parameter but ignores or rejects it differently
// fails here.
func TestPeriodFilterConsistent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(backendRoot(), "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Parameters map[string]map[string]any `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	declaresPeriod := func(params any) bool {
		list, _ := params.([]any)
		for _, p := range list {
			m, _ := p.(map[string]any)
			if ref, ok := m["$ref"].(string); ok {
				m = spec.Components.Parameters[strings.TrimPrefix(ref, "#/components/parameters/")]
			}
			if m["name"] == "period" && m["in"] == "query" {
				return true
			}
		}
		return false
	}
	var paths []string
	for path, ops := range spec.Paths {
		if !strings.HasPrefix(path, "/v1/tenant/") && !strings.HasPrefix(path, "/v1/taxpayer/") {
			continue
		}
		get, ok := ops["get"].(map[string]any)
		if !ok {
			continue
		}
		if declaresPeriod(get["parameters"]) || declaresPeriod(ops["parameters"]) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if len(paths) < 15 {
		t.Fatalf("only %d list endpoints declare period: %v", len(paths), paths)
	}

	e := newSweepEnv(t)
	param := regexp.MustCompile(`\{([A-Za-z]+)\}`)
	for _, path := range paths {
		// A resource the demo office has none of (a source connection)
		// answers 404 first; the engagement is the only parameter resolved.
		if strings.Count(path, "{") > 1 {
			continue
		}
		url := param.ReplaceAllStringFunc(path, func(m string) string { return e.param(t, m[1:len(m)-1], path) })
		taxpayer := strings.HasPrefix(path, "/v1/taxpayer/")
		for _, c := range []struct {
			period string
			ok     bool
		}{{"2026-10", true}, {"2026-13", false}, {"10-2026", false}} {
			req := httptest.NewRequest(http.MethodGet, url+"?period="+c.period, nil)
			if taxpayer {
				req.Header.Set("Authorization", "Bearer "+e.tpTok)
				req.Header.Set("X-Taxpayer", e.tpUUID)
			} else {
				req.Header.Set("Authorization", "Bearer "+e.ownerTok)
				req.Header.Set("X-Organization", "demo-smmm")
			}
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, req)
			if c.ok {
				if w.Code >= 300 {
					t.Errorf("%s period=%s = %d %s, want 2xx", path, c.period, w.Code, firstLine(w.Body.String()))
				}
				continue
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if w.Code != http.StatusBadRequest || body.Error.Code != "INVALID_PERIOD" {
				t.Errorf("%s period=%s = %d %s, want 400 INVALID_PERIOD", path, c.period, w.Code, body.Error.Code)
			}
		}
	}
	t.Logf("%d list endpoints checked: %v", len(paths), paths)

	// The filter narrows, not only validates: the review queue by the
	// suggestion's period.
	eng := e.param(t, "engagementUuid", "")
	inv := e.param(t, "invoiceUuid", "")
	if rec := e.do(t, http.MethodPost, "/v1/tenant/engagements/"+eng+"/invoices/"+inv+"/account-suggestions", sweepCase{}, false); rec.Code != http.StatusOK {
		t.Fatalf("account suggestions = %d %s", rec.Code, rec.Body)
	}
	var period string
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT period FROM suggestions WHERE period IS NOT NULL LIMIT 1`).Scan(&period); err != nil {
		t.Fatal(err)
	}
	count := func(p string) float64 {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenant/review/suggestions?status=all&period="+p, nil)
		req.Header.Set("Authorization", "Bearer "+e.ownerTok)
		req.Header.Set("X-Organization", "demo-smmm")
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, req)
		var out struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		for _, k := range []string{"total", "total_count"} {
			if v, ok := out.Data[k].(float64); ok {
				return v
			}
		}
		t.Fatalf("no total in %s", firstLine(w.Body.String()))
		return 0
	}
	if in, out := count(period), count("1999-01"); in == 0 || out != 0 {
		t.Fatalf("suggestions in %s = %v, in 1999-01 = %v; want >0 and 0", period, in, out)
	}
}
