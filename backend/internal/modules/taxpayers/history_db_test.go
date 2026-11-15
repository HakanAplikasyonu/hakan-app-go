package taxpayers

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/google/uuid"
)

type transferEnv struct {
	onboardEnv
	hist       *History
	orgA, orgB int64
	tp         int64
	tpUUID     uuid.UUID
	owner      int64
	staffA     int64
	staffB     int64
	engB       int64
	engA       int64
	ownerScope taxpayerctx.Scope
	today      string
	beforeB    string // before office B started
	duringB    string // while with office B
}

func insertInvoice(t *testing.T, e onboardEnv, tp int64, number, date string) {
	t.Helper()
	_, err := e.d.Pool.Exec(context.Background(), `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
		                      direction, type, issue_date, period, currency, tax_summary_source)
		VALUES ($1, 'manual_upload', $2, 'manual', $3, 'outgoing', 'SATIS', $4::date,
		        $5, 'TRY', 'items')`, tp, "src-"+number, number, date, date[:7])
	if err != nil {
		t.Fatalf("invoice %s: %v", number, err)
	}
}

// setupTransfer: the company was with office B since January; it moves to
// office A today through an owner-approved request (issue #7 flow).
func setupTransfer(t *testing.T, archiveDaysB int) transferEnv {
	t.Helper()
	e := transferEnv{onboardEnv: newOnboardEnv(t, time.Millisecond)}
	ctx := context.Background()
	e.hist = NewHistory(e.d.Pool)
	e.orgA, e.orgB = e.d.Organization(t), e.d.Organization(t)
	if _, err := e.d.Pool.Exec(ctx, `UPDATE organizations SET settings = jsonb_build_object('engagement_archive_days', $2::int) WHERE id = $1`, e.orgB, archiveDaysB); err != nil {
		t.Fatal(err)
	}
	e.staffA, e.staffB, e.owner = e.d.User(t), e.d.User(t), e.d.User(t)
	e.d.Member(t, e.orgA, e.staffA, "owner")
	e.d.Member(t, e.orgB, e.staffB, "owner")
	e.tp = e.d.Taxpayer(t, KindLegal, "1234567890")
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM taxpayers WHERE id = $1`, e.tp).Scan(&e.tpUUID)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, e.tp, e.owner); err != nil {
		t.Fatal(err)
	}
	if err := e.d.Pool.QueryRow(ctx, `
		INSERT INTO engagements (organization_id, taxpayer_id, started_at)
		VALUES ($1, $2, date_trunc('year', NOW()) - interval '1 year') RETURNING id`, e.orgB, e.tp).Scan(&e.engB); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	e.today = now.Format(time.DateOnly)
	e.beforeB = time.Date(now.Year()-2, 6, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	e.duringB = time.Date(now.Year()-1, 3, 10, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	insertInvoice(t, e.onboardEnv, e.tp, "ESKI-0", e.beforeB)
	insertInvoice(t, e.onboardEnv, e.tp, "ESKI-1", e.duringB)

	e.ownerScope = taxpayerctx.Scope{ID: e.tp, UUID: e.tpUUID, TaxID: "1234567890", Role: taxpayerctx.RoleOwner}
	e.request(t, e.orgA, "1234567890", "muhasebe@alfa.test")
	pending, err := e.flow.PendingForTaxpayer(ctx, e.ownerScope)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %v %v", pending, err)
	}
	if out, err := e.flow.ApproveAsOwner(ctx, e.ownerScope, pending[0].UUID, e.owner); err != nil || out.Status != OutcomeApproved {
		t.Fatalf("transfer approval: %+v %v", out, err)
	}
	if err := e.d.Pool.QueryRow(ctx, `SELECT id FROM engagements WHERE organization_id = $1 AND taxpayer_id = $2 AND ended_at IS NULL`, e.orgA, e.tp).Scan(&e.engA); err != nil {
		t.Fatal(err)
	}
	insertInvoice(t, e.onboardEnv, e.tp, "YENI-1", e.today)
	return e
}

// officeCtx is what the middleware chain would put on an office request.
func (e transferEnv) officeCtx(org, user, eng int64) context.Context {
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll}})
	return engagementctx.WithScope(ctx, engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: e.tp})
}

func (e transferEnv) docs(t *testing.T, org, user, eng int64) []string {
	t.Helper()
	items, err := e.hist.OfficeDocuments(e.officeCtx(org, user, eng), engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: e.tp}, 100, 0)
	if err != nil {
		t.Fatalf("documents: %v", err)
	}
	out := make([]string, 0, len(items))
	for _, d := range items {
		out = append(out, d.InvoiceNumber)
	}
	sort.Strings(out)
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestNewOfficeSeesHistoryOnlyWithConsent: kabul kriteri 1 ve 3.
func TestNewOfficeSeesHistoryOnlyWithConsent(t *testing.T) {
	e := setupTransfer(t, 0)
	ctx := context.Background()

	if got := e.docs(t, e.orgA, e.staffA, e.engA); !eq(got, "YENI-1") {
		t.Fatalf("new office without consent sees %v, want only YENI-1", got)
	}

	// Range consent: only the period with office B.
	from := e.duringB[:4] + "-01-01"
	share, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{FromDate: from})
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if got := e.docs(t, e.orgA, e.staffA, e.engA); !eq(got, "ESKI-1", "YENI-1") {
		t.Fatalf("with range consent sees %v, want ESKI-1 YENI-1", got)
	}
	// The office can see what it was given.
	shares, _ := e.hist.OfficeShares(ctx, engagementctx.Scope{ID: e.engA, OrganizationID: e.orgA, TaxpayerID: e.tp})
	if len(shares) != 1 || !shares[0].Active || shares[0].FromDate == nil || *shares[0].FromDate != from {
		t.Fatalf("office shares: %+v", shares)
	}

	// Revoking takes it back at once; the record stays.
	if err := e.hist.Revoke(ctx, e.ownerScope, e.owner, share.UUID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := e.docs(t, e.orgA, e.staffA, e.engA); !eq(got, "YENI-1") {
		t.Fatalf("after revoke sees %v, want only YENI-1", got)
	}
	list, _ := e.hist.List(ctx, e.ownerScope)
	if len(list) != 1 || list[0].RevokedAt == nil || list[0].Active {
		t.Fatalf("consent record after revoke: %+v", list)
	}
	if err := e.hist.Revoke(ctx, e.ownerScope, e.owner, share.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}

	// Open-ended consent: everything.
	if _, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{}); err != nil {
		t.Fatal(err)
	}
	if got := e.docs(t, e.orgA, e.staffA, e.engA); !eq(got, "ESKI-0", "ESKI-1", "YENI-1") {
		t.Fatalf("with full consent sees %v", got)
	}

	// Only owners decide; bad ranges are refused.
	emp := e.ownerScope
	emp.Role = taxpayerctx.RoleEmployee
	if _, err := e.hist.Share(ctx, emp, e.owner, ShareInput{}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("employee share: %v", err)
	}
	if _, err := e.hist.Share(ctx, e.ownerScope, e.owner, ShareInput{FromDate: "2026-05-01", ToDate: "2026-01-01"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("reversed range: %v", err)
	}
}

// TestOldOfficeLosesNewDocuments: kabul kriteri 2. Eski ofis geçişten sonra
// yeni belgeleri görmez; arşiv süresinde yalnızca kendi dönemini salt okur.
func TestOldOfficeLosesNewDocuments(t *testing.T) {
	withArchive := setupTransfer(t, 30)
	if got := withArchive.docs(t, withArchive.orgB, withArchive.staffB, withArchive.engB); !eq(got, "ESKI-0", "ESKI-1") {
		t.Fatalf("old office in archive window sees %v, want its own period only (no YENI-1)", got)
	}
	var until *time.Time
	_ = withArchive.d.Pool.QueryRow(context.Background(), `SELECT archive_until FROM engagements WHERE id = $1`, withArchive.engB).Scan(&until)
	if until == nil || time.Until(*until) < 29*24*time.Hour {
		t.Fatalf("archive_until = %v, want ~30 days ahead", until)
	}

	noArchive := setupTransfer(t, 0)
	if got := noArchive.docs(t, noArchive.orgB, noArchive.staffB, noArchive.engB); len(got) != 0 {
		t.Fatalf("old office without archive window sees %v, want nothing", got)
	}
	// A share for the new office never opens anything to the old one.
	if _, err := noArchive.hist.Share(context.Background(), noArchive.ownerScope, noArchive.owner, ShareInput{}); err != nil {
		t.Fatal(err)
	}
	if got := noArchive.docs(t, noArchive.orgB, noArchive.staffB, noArchive.engB); len(got) != 0 {
		t.Fatalf("old office after share to new office sees %v", got)
	}
}

// TestFirstOfficeSeesOwnHistory: a company that never had another office
// brings its history to its first office without a separate consent.
func TestFirstOfficeSeesOwnHistory(t *testing.T) {
	e := newOnboardEnv(t, time.Millisecond)
	hist := NewHistory(e.d.Pool)
	org := e.d.Organization(t)
	staff := e.d.User(t)
	e.d.Member(t, org, staff, "owner")
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	insertInvoice(t, e, tp, "ONCE", "2024-02-01")
	eng := e.d.Engagement(t, org, tp)
	te := transferEnv{onboardEnv: e, hist: hist, tp: tp}
	if got := te.docs(t, org, staff, eng); !eq(got, "ONCE") {
		t.Fatalf("first office sees %v, want ONCE", got)
	}
}
