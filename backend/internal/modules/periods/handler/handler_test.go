package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

func req(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: 1})
	ctx = engagementctx.WithScope(ctx, engagementctx.Scope{ID: 2, TaxpayerID: 3})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: 9})
	return r.WithContext(ctx)
}

// Validation fails before the store is reached (nil store).
func TestValidationErrors(t *testing.T) {
	h := New(usecase.New(nil))
	cases := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		r    *http.Request
		path map[string]string
	}{
		{"bad period", h.GetReadiness, req(http.MethodGet, "/x", ""), map[string]string{"period": "2026-1"}},
		{"unlock without reason", h.Unlock, req(http.MethodPost, "/x", `{"reason":" "}`), map[string]string{"period": "2026-01"}},
		{"unknown field", h.Lock, req(http.MethodPost, "/x", `{"nope":1}`), map[string]string{"period": "2026-01"}},
		{"bad change request uuid", h.ApplyChangeRequest, req(http.MethodPost, "/x", ""), map[string]string{"changeRequestUuid": "x"}},
		{"bad status", h.ListChangeRequests, req(http.MethodGet, "/x?status=pending", ""), nil},
		{"bad matrix periods", h.Matrix, req(http.MethodGet, "/x?periods=0", ""), nil},
		{"bad matrix until", h.Matrix, req(http.MethodGet, "/x?until=2026", ""), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.path {
				tc.r.SetPathValue(k, v)
			}
			rec := httptest.NewRecorder()
			tc.call(rec, tc.r)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}
