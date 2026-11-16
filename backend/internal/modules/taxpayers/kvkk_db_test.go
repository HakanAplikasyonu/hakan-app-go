package taxpayers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	legalusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
)

// Kabul (#57): geçmiş veri paylaşımı açık rıza metnine bağlı. Paylaşım
// onaylanan sürümle birlikte kaydedilir, eski sürümle yapılamaz, geri
// alınınca rıza kaydı da geri alınmış olur.
func TestHistoryShareRequiresExplicitConsent(t *testing.T) {
	e := setupTransfer(t, 0)
	ctx := context.Background()
	legal := legalusecase.NewPostgres(e.d.Pool)
	e.hist.WithConsents(legal)

	if _, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("rıza sürümü olmadan: %v", err)
	}
	if _, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{ConsentVersion: 9}); !errors.Is(err, legalusecase.ErrOutdated) {
		t.Fatalf("eski/yanlış sürüm: %v", err)
	}
	var shares int
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM taxpayer_history_shares WHERE taxpayer_id = $1`, e.tp).Scan(&shares)
	if shares != 0 {
		t.Fatalf("reddedilen paylaşım yazılmamalı (işlem geri alınmalı): %d", shares)
	}

	sh, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{ConsentVersion: 1, IP: "10.1.2.3", UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var (
		kind, ctxName, ip string
		version           int
		org, tp           int64
	)
	if err := e.d.Pool.QueryRow(ctx, `
		SELECT d.kind, d.version, a.context, a.ip, a.organization_id, a.taxpayer_id
		FROM legal_acceptances a JOIN legal_documents d ON d.id = a.document_id
		WHERE a.subject_uuid = $1 AND a.withdrawn_at IS NULL`, sh.UUID).Scan(&kind, &version, &ctxName, &ip, &org, &tp); err != nil {
		t.Fatalf("rıza kaydı: %v", err)
	}
	if kind != legalusecase.KindHistoryConsent || version != 1 || ctxName != legalusecase.ContextHistory ||
		ip != "10.1.2.3" || org != e.orgA || tp != e.tp {
		t.Fatalf("rıza kaydı: %s v%d %s %s org=%d tp=%d", kind, version, ctxName, ip, org, tp)
	}

	if err := e.hist.Revoke(ctx, e.ownerScope, e.owner, sh.UUID); err != nil {
		t.Fatal(err)
	}
	var withdrawn bool
	_ = e.d.Pool.QueryRow(ctx, `SELECT withdrawn_at IS NOT NULL FROM legal_acceptances WHERE subject_uuid = $1`, sh.UUID).Scan(&withdrawn)
	if !withdrawn {
		t.Fatal("rıza geri alınmış olarak işaretlenmedi")
	}
}

// Ofis ekleme onayı aydınlatma metninin güncel sürümünü ister ve kaydeder.
func TestOnboardingApprovalRequiresPrivacyNotice(t *testing.T) {
	e := newOnboardEnv(t, 0)
	ctx := context.Background()
	h := NewOnboardingHandler(e.flow, nil).WithNotice(legalusecase.NewPostgres(e.d.Pool))
	org, user := e.d.Organization(t), e.d.User(t)
	e.d.Member(t, org, e.d.User(t), "owner")
	token := e.request(t, org, "1234567890", "sahip@alfa.test")
	var email string
	_ = e.d.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, user).Scan(&email)
	if _, err := e.d.Pool.Exec(ctx, `UPDATE users SET email = 'sahip@alfa.test' WHERE id = $1`, user); err != nil {
		t.Fatal(err)
	}

	approve := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/taxpayer-consents/approve", bytes.NewBufferString(body))
		req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: user}))
		rec := httptest.NewRecorder()
		h.approveConsent(rec, req)
		return rec.Code
	}
	if c := approve(`{"token":"` + token + `"}`); c != http.StatusBadRequest {
		t.Fatalf("aydınlatma sürümü olmadan: %d", c)
	}
	if c := approve(`{"token":"` + token + `","privacy_notice_version":7}`); c != http.StatusConflict {
		t.Fatalf("eski sürüm: %d", c)
	}
	if c := approve(`{"token":"` + token + `","privacy_notice_version":1}`); c != http.StatusOK {
		t.Fatalf("onay: %d", c)
	}
	var n int
	_ = e.d.Pool.QueryRow(ctx, `
		SELECT count(*) FROM legal_acceptances a JOIN legal_documents d ON d.id = a.document_id
		WHERE a.user_id = $1 AND d.kind = 'privacy_notice' AND a.context = 'onboarding_consent'`, user).Scan(&n)
	if n != 1 {
		t.Fatalf("aydınlatma kaydı: %d", n)
	}
}
