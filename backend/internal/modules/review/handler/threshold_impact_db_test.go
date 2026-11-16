package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/shopspring/decimal"
)

type impactDTO struct {
	Threshold                    string
	Months                       int
	Kind                         *string
	CurrentThreshold             string `json:"current_threshold"`
	CurrentAutoAccept            bool   `json:"current_auto_accept"`
	Decided                      int64
	WouldAutoAccept              int64 `json:"would_auto_accept"`
	WouldAutoAcceptThenCorrected int64 `json:"would_auto_accept_then_corrected"`
	CurrentlyAutoAccepted        int64 `json:"currently_auto_accepted"`
	WouldGoToQueue               int64 `json:"would_go_to_queue"`
}

// TestThresholdImpact is issue #333's threshold part: one aggregate over the
// office's decided suggestions, against a candidate threshold.
func TestThresholdImpact(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org, other := d.Organization(t), d.Organization(t)
	user := d.User(t)
	d.Member(t, org, user, "accountant")
	tp1, tp2, tp3 := d.Taxpayer(t, "legal", "9000000201"), d.Taxpayer(t, "legal", "9000000202"), d.Taxpayer(t, "legal", "9000000203")
	eng1, eng2, engOther := d.Engagement(t, org, tp1), d.Engagement(t, org, tp2), d.Engagement(t, other, tp3)

	ins := func(o, eng, tp int64, kind, conf, status, ago string, by bool) {
		t.Helper()
		var actor any
		if by {
			actor = user
		}
		_, err := d.Pool.Exec(ctx, `
			INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, proposed_value,
				confidence, rationale, origin, status, decided_by, decided_at)
			VALUES ($1, $2, $3, $4, 'invoice', '"x"', $5, 'test', 'input', $6, $7,
				CASE WHEN $8 = '' THEN NULL ELSE NOW() - $8::interval END)`,
			o, eng, tp, kind, conf, status, actor, ago)
		if err != nil {
			t.Fatal(err)
		}
	}
	ins(org, eng1, tp1, "receipt_field", "0.95", "auto_accepted", "1 day", false)
	ins(org, eng1, tp1, "receipt_field", "0.92", "corrected", "10 days", true)
	ins(org, eng1, tp1, "receipt_field", "0.91", "rejected", "20 days", true)
	ins(org, eng1, tp1, "receipt_field", "0.90", "accepted", "30 days", true)
	ins(org, eng1, tp1, "receipt_field", "0.85", "corrected", "40 days", true)
	ins(org, eng1, tp1, "receipt_field", "0.50", "rejected", "50 days", true)
	ins(org, eng1, tp1, "receipt_field", "0.99", "pending", "", false)
	ins(org, eng1, tp1, "receipt_field", "0.99", "superseded", "2 days", false)
	ins(org, eng1, tp1, "receipt_field", "0.97", "accepted", "8 months", true)
	ins(org, eng2, tp2, "account_code", "0.93", "corrected", "3 days", true)
	ins(other, engOther, tp3, "receipt_field", "0.99", "accepted", "1 day", true)

	svc := usecase.New(repository.New(d.Pool, database.NewQueries(d.Pool)))
	h := New(svc, nil)
	caller, perms := user, []string{rbac.PermTenantReviewRead, rbac.PermTenantEngagementsReadAll}
	do := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, target, nil)
		c := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: caller})
		c = orgctx.WithScope(c, orgctx.Scope{ID: org, Permissions: perms})
		rec := httptest.NewRecorder()
		rls.Middleware(d.Pool, nil, nil)(http.HandlerFunc(h.ThresholdImpact)).ServeHTTP(rec, r.WithContext(c))
		return rec
	}
	get := func(target string) impactDTO {
		t.Helper()
		rec := do(target)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body)
		}
		var env struct{ Data impactDTO }
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Data
	}
	check := func(got impactDTO, decided, auto, corrected, current, queue int64) {
		t.Helper()
		if got.Decided != decided || got.WouldAutoAccept != auto || got.WouldAutoAcceptThenCorrected != corrected ||
			got.CurrentlyAutoAccepted != current || got.WouldGoToQueue != queue {
			t.Fatalf("impact = %+v, want decided=%d auto=%d corrected=%d current=%d queue=%d", got, decided, auto, corrected, current, queue)
		}
	}

	got := get("/v1/tenant/review/threshold-impact?threshold=0.90")
	check(got, 7, 5, 3, 1, 2)
	if got.Threshold != "0.90" || got.Months != 6 || got.Kind != nil || got.CurrentThreshold != "0.9500" || !got.CurrentAutoAccept {
		t.Fatalf("header = %+v", got)
	}
	check(get("/v1/tenant/review/threshold-impact?threshold=0.9&kind=receipt_field"), 6, 4, 2, 1, 2)
	check(get("/v1/tenant/review/threshold-impact?threshold=0.90&months=12"), 8, 6, 3, 1, 2)
	check(get("/v1/tenant/review/threshold-impact?threshold=0.95"), 7, 1, 0, 1, 6)
	check(get("/v1/tenant/review/threshold-impact?threshold=1.00"), 7, 0, 0, 1, 7)
	check(get("/v1/tenant/review/threshold-impact?threshold=0.50&months=1"), 5, 5, 3, 1, 0)

	// Without read_all: only assigned engagements (eng2).
	member := d.User(t)
	d.Member(t, org, member, "member")
	d.Assign(t, org, eng2, member)
	caller, perms = member, []string{rbac.PermTenantReviewRead}
	check(get("/v1/tenant/review/threshold-impact?threshold=0.90"), 1, 1, 1, 0, 0)
	caller, perms = user, []string{rbac.PermTenantReviewRead, rbac.PermTenantEngagementsReadAll}

	// current_threshold follows the office's settings for the kind.
	thr := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	off := false
	if err := svc.SetSetting(ctx, org, 0, usecase.SettingRow{Kind: "*", Threshold: thr("0.97")}, user); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSetting(ctx, org, 0, usecase.SettingRow{Kind: "receipt_field", Threshold: thr("0.8"), AutoAccept: &off}, user); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSetting(ctx, org, eng1, usecase.SettingRow{Kind: "receipt_field", Threshold: thr("0.6")}, user); err != nil {
		t.Fatal(err)
	}
	if got := get("/v1/tenant/review/threshold-impact?threshold=0.90"); got.CurrentThreshold != "0.9700" || !got.CurrentAutoAccept {
		t.Fatalf("office * policy = %+v", got)
	}
	if got := get("/v1/tenant/review/threshold-impact?threshold=0.90&kind=receipt_field"); got.CurrentThreshold != "0.8000" || got.CurrentAutoAccept || *got.Kind != "receipt_field" {
		t.Fatalf("office kind policy = %+v", got)
	}

	for _, q := range []string{
		"", "?threshold=", "?threshold=abc", "?threshold=0.49", "?threshold=1.01", "?threshold=0.905",
		"?threshold=0.9&months=0", "?threshold=0.9&months=25", "?threshold=0.9&months=x", "?threshold=0.9&kind=Bad-Kind",
	} {
		if rec := do("/v1/tenant/review/threshold-impact" + q); rec.Code != http.StatusBadRequest {
			t.Fatalf("%q = %d %s", q, rec.Code, rec.Body)
		}
	}
}
