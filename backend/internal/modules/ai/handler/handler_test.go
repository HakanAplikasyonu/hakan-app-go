package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

type fakeSvc struct {
	org      int64
	from, to time.Time
	rows     []usecase.ReportRow
}

func (f *fakeSvc) Report(_ context.Context, org int64, from, to time.Time) ([]usecase.ReportRow, error) {
	f.org, f.from, f.to = org, from, to
	return f.rows, nil
}
func (f *fakeSvc) Privacy() usecase.Privacy { return usecase.Privacy{Masking: true} }

func do(h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(orgctx.WithScope(req.Context(), orgctx.Scope{ID: 42}))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestUsageTotalsAndRange(t *testing.T) {
	svc := &fakeSvc{rows: []usecase.ReportRow{
		{Model: "a", Calls: 2, InputTokens: 10, CostUSD: "0.012000"},
		{Model: "b", Calls: 1, UnpricedCalls: 1, CostUSD: "1.5"},
	}}
	h := New(svc)
	rec := do(h.Usage, "/v1/tenant/ai/usage?from=2026-11-01&to=2026-11-30")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Data usageReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if svc.org != 42 {
		t.Fatalf("org = %d", svc.org)
	}
	if got := svc.to.Sub(svc.from); got != 30*24*time.Hour {
		t.Fatalf("range = %v (to must be exclusive, the day after `to`)", got)
	}
	tot := body.Data.Totals
	if tot.Calls != 3 || tot.UnpricedCalls != 1 || tot.CostUSD != "1.512000" || body.Data.To != "2026-11-30" {
		t.Fatalf("report = %+v", body.Data)
	}
}

func TestUsageRejectsBadInput(t *testing.T) {
	h := New(&fakeSvc{})
	for _, q := range []string{"?from=01.11.2026", "?to=x", "?from=2026-11-10&to=2026-11-01", "?from=2024-01-01&to=2026-01-01"} {
		if rec := do(h.Usage, "/v1/tenant/ai/usage"+q); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", q, rec.Code)
		}
	}
}

func TestUsageDefaultsToCurrentMonth(t *testing.T) {
	svc := &fakeSvc{}
	if rec := do(New(svc).Usage, "/v1/tenant/ai/usage"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.from.Day() != 1 || svc.to.Day() != 1 || svc.to.Month() == svc.from.Month() {
		t.Fatalf("default range %v - %v", svc.from, svc.to)
	}
}

func TestParseMicroUSD(t *testing.T) {
	cases := map[string]int64{"0.000000": 0, "1.5": 1_500_000, "0.1234567": 123_456, "x": 0, "12": 12_000_000}
	for in, want := range cases {
		if got := parseMicroUSD(in); got != want {
			t.Errorf("parseMicroUSD(%q) = %d, want %d", in, got, want)
		}
	}
}
