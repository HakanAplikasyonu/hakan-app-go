package handler

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

func TestTaxpayerPanelEnabled(t *testing.T) {
	cases := map[string]bool{
		"":                                   true,
		"{}":                                 true,
		"not json":                           true,
		`{"taxpayer_vat_sales_panel":true}`:  true,
		`{"taxpayer_vat_sales_panel":"no"}`:  true,
		`{"taxpayer_vat_sales_panel":false}`: false,
		`{"other":false}`:                    true,
	}
	for in, want := range cases {
		if got := TaxpayerPanelEnabled([]byte(in)); got != want {
			t.Errorf("TaxpayerPanelEnabled(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTrendPeriods(t *testing.T) {
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got := TrendPeriods(start, 4)
	want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TrendPeriods = %v, want %v", got, want)
	}
	if got := TrendPeriods(start, 0); !reflect.DeepEqual(got, []string{"2026-02"}) {
		t.Fatalf("TrendPeriods(n=0) = %v", got)
	}
}

func TestTaxpayerWithoutPortalIsNotFound(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/taxpayer/vat-sales", nil)
	req = req.WithContext(taxpayerctx.WithScope(req.Context(), taxpayerctx.Scope{ID: 1}))
	rec := httptest.NewRecorder()
	h.Taxpayer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
