package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/shopspring/decimal"
)

func req(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: 1})
	ctx = engagementctx.WithScope(ctx, engagementctx.Scope{ID: 2, TaxpayerID: 3})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: 9})
	return r.WithContext(ctx)
}

func seed(t *testing.T) (*Handler, usecase.Suggestion) {
	t.Helper()
	svc := usecase.New(usecase.NewMemoryStore())
	id := int64(5)
	s, err := svc.Submit(context.Background(), usecase.Subject{OrganizationID: 1, EngagementID: 2, TaxpayerID: 3}, usecase.Input{
		Kind: "account_code", TargetType: "invoice_item", TargetID: &id, Value: json.RawMessage(`"760"`),
		Confidence: decimal.RequireFromString("0.4"), Rationale: "r", Provenance: usecase.Provenance{Origin: usecase.OriginInput},
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(svc, nil), s
}

func TestListDefaultsToPending(t *testing.T) {
	h, s := seed(t)
	rec := httptest.NewRecorder()
	h.ListEngagement(rec, req(http.MethodGet, "/x", ""))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), s.UUID.String()) || !strings.Contains(rec.Body.String(), `"confidence":"0.4000"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ListEngagement(rec, req(http.MethodGet, "/x?status=rejected", ""))
	if !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("filter ignored: %s", rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ListEngagement(rec, req(http.MethodGet, "/x?rule_candidate=maybe", ""))
	if rec.Code != 400 {
		t.Fatalf("bad bool = %d", rec.Code)
	}
}

func TestDecideAndGet(t *testing.T) {
	h, s := seed(t)
	r := req(http.MethodPost, "/x", `{"reason":""}`)
	r.SetPathValue("suggestionUuid", s.UUID.String())
	rec := httptest.NewRecorder()
	h.Reject(rec, r)
	if rec.Code != 400 {
		t.Fatalf("reject without reason = %d", rec.Code)
	}
	r = req(http.MethodPost, "/x", `{}`)
	r.SetPathValue("suggestionUuid", s.UUID.String())
	rec = httptest.NewRecorder()
	h.Accept(rec, r)
	if rec.Code != 409 {
		t.Fatalf("accept without applier = %d %s", rec.Code, rec.Body)
	}
	r = req(http.MethodPost, "/x", `{"reason":"ilgisiz"}`)
	r.SetPathValue("suggestionUuid", s.UUID.String())
	rec = httptest.NewRecorder()
	h.Reject(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"rejected"`) {
		t.Fatalf("reject = %d %s", rec.Code, rec.Body)
	}
	r = req(http.MethodGet, "/x", "")
	r.SetPathValue("suggestionUuid", s.UUID.String())
	rec = httptest.NewRecorder()
	h.Get(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"action":"reject"`) || !strings.Contains(rec.Body.String(), `"reason":"ilgisiz"`) {
		t.Fatalf("get = %d %s", rec.Code, rec.Body)
	}
}

func TestSettings(t *testing.T) {
	h, _ := seed(t)
	rec := httptest.NewRecorder()
	h.PutOfficeSetting(rec, req(http.MethodPut, "/x", `{"kind":"*","threshold":"1.5"}`))
	if rec.Code != 400 {
		t.Fatalf("bad threshold = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.PutEngagementSetting(rec, req(http.MethodPut, "/x", `{"kind":"account_code","threshold":"0.8","auto_accept":true}`))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"level":"engagement"`) || !strings.Contains(rec.Body.String(), `"account_code":{"auto_accept":true,"threshold":"0.8"}`) {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	r := req(http.MethodDelete, "/x", "")
	r.SetPathValue("kind", "nope")
	rec = httptest.NewRecorder()
	h.DeleteOfficeSetting(rec, r)
	if rec.Code != 404 {
		t.Fatalf("delete missing = %d", rec.Code)
	}
}
