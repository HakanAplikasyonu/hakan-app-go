package privacy

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fixture struct {
	d          *testdb.DB
	svc        *Service
	eng        engagementctx.Scope
	admin      int64
	portalOnly int64 // taxpayer user with no other ties
	shared     int64 // taxpayer user who also works at an office
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "individual", "12345678901")
	engID := d.Engagement(t, org, tp)
	admin := d.User(t)
	d.Member(t, org, admin, "admin")
	portalOnly, shared := d.User(t), d.User(t)
	other := d.Organization(t)
	d.Member(t, other, shared, "member")
	for _, u := range []int64{portalOnly, shared} {
		mustExec(t, d, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, tp, u)
		mustExec(t, d, `INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent) VALUES ($1, $2, NOW() + interval '1 day', 'test-agent')`, u, uuid.NewString())
	}
	mustExec(t, d, `INSERT INTO push_devices (user_id, token, platform) VALUES ($1, $2, 'ios')`, portalOnly, "tok-"+uuid.NewString())
	mustExec(t, d, `INSERT INTO taxpayer_invitations (organization_id, engagement_id, email, role, token_hash, expires_at)
		VALUES ($1, $2, 'davet@example.test', 'employee', $3, NOW() + interval '1 day')`, org, engID, uuid.NewString())
	mustExec(t, d, `INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, type, name)
		VALUES ($1, 'test', 'c1', 'manual', 'customer', 'Karşı Taraf')`, tp)

	var e engagementctx.Scope
	if err := d.Pool.QueryRow(ctx, `SELECT id, uuid, organization_id, taxpayer_id, status, started_at FROM engagements WHERE id = $1`, engID).
		Scan(&e.ID, &e.UUID, &e.OrganizationID, &e.TaxpayerID, &e.Status, &e.StartedAt); err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: New(d.Pool, Config{}), eng: e, admin: admin, portalOnly: portalOnly, shared: shared}
}

func mustExec(t *testing.T, d *testdb.DB, sql string, args ...any) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f fixture) userUUID(t *testing.T, id int64) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	if err := f.d.Pool.QueryRow(context.Background(), `SELECT uuid FROM users WHERE id = $1`, id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

func readZip(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !json.Valid(body) {
			t.Fatalf("%s is not JSON", f.Name)
		}
		out[f.Name] = string(body)
	}
	return out
}

func TestTaxpayerExport(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: KindExport, SubjectType: SubjectTaxpayer, Reason: "md. 11 başvurusu"})
	if err != nil {
		t.Fatal(err)
	}
	// Not downloadable before approval.
	if err := f.svc.WriteExport(ctx, io.Discard, f.eng, req); !errors.Is(err, ErrNotExport) {
		t.Fatalf("pending export: %v", err)
	}
	req, err = f.svc.Approve(ctx, f.eng, f.admin, req.Uuid, "")
	if err != nil || req.Status != StatusCompleted || !req.ExpiresAt.Valid {
		t.Fatalf("approve: %+v %v", req, err)
	}
	var buf bytes.Buffer
	if err := f.svc.WriteExport(ctx, &buf, f.eng, req); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())
	for _, name := range []string{"manifest.json", "taxpayer.json", "users.json", "accounting_record_counts.json", "invitations.json"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s in %v", name, files)
		}
	}
	if !strings.Contains(files["taxpayer.json"], "12345678901") {
		t.Fatal("taxpayer identity missing")
	}
	if strings.Count(files["users.json"], "@example.test") != 2 {
		t.Fatalf("users: %s", files["users.json"])
	}
	if !strings.Contains(files["accounting_record_counts.json"], `"customers": 1`) {
		t.Fatalf("counts: %s", files["accounting_record_counts.json"])
	}
	for name, body := range files {
		if strings.Contains(body, "password_hash") || strings.Contains(body, "token_hash") {
			t.Fatalf("%s leaks a secret column", name)
		}
	}
	// Expired downloads are refused.
	f.svc.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if err := f.svc.WriteExport(ctx, io.Discard, f.eng, req); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
}

func TestUserExport(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: KindExport, SubjectType: SubjectUser, SubjectUser: f.userUUID(t, f.portalOnly)})
	if err != nil {
		t.Fatal(err)
	}
	if req, err = f.svc.Approve(ctx, f.eng, f.admin, req.Uuid, ""); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.svc.WriteExport(ctx, &buf, f.eng, req); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())
	if !strings.Contains(files["sessions.json"], "test-agent") || !strings.Contains(files["devices.json"], "ios") {
		t.Fatalf("sessions/devices: %v", files)
	}
	// A user who is not on this taxpayer cannot be a subject.
	if _, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: KindExport, SubjectType: SubjectUser, SubjectUser: f.userUUID(t, f.admin)}); !errors.Is(err, ErrSubjectUnknown) {
		t.Fatalf("foreign subject: %v", err)
	}
}

func TestTaxpayerErasure(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: KindErasure, SubjectType: SubjectTaxpayer})
	if err != nil {
		t.Fatal(err)
	}
	if req, err = f.svc.Approve(ctx, f.eng, f.admin, req.Uuid, "onaylandı"); err != nil {
		t.Fatal(err)
	}
	var sum map[string]any
	_ = json.Unmarshal(req.Summary, &sum)
	if sum["users_anonymised"] != float64(1) || sum["users_kept_shared"] != float64(1) || sum["users_unlinked"] != float64(2) {
		t.Fatalf("summary: %v", sum)
	}
	retained, _ := sum["retained"].(map[string]any)
	if retained["retention_years"] != float64(5) {
		t.Fatalf("retained: %v", retained)
	}

	var email, name, status string
	var hash string
	if err := f.d.Pool.QueryRow(ctx, `SELECT email, name, status, password_hash FROM users WHERE id = $1`, f.portalOnly).
		Scan(&email, &name, &status, &hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(email, "@anonymized.invalid") || name != "Silinmiş" || status != "disabled" || hash != "" {
		t.Fatalf("not anonymised: %s %s %s", email, name, status)
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := f.d.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM refresh_tokens WHERE user_id = $1`, f.portalOnly); n != 0 {
		t.Fatalf("sessions left: %d", n)
	}
	if n := count(`SELECT count(*) FROM push_devices WHERE user_id = $1`, f.portalOnly); n != 0 {
		t.Fatalf("devices left: %d", n)
	}
	// The shared user keeps the account and sessions, only the link goes.
	if n := count(`SELECT count(*) FROM refresh_tokens WHERE user_id = $1`, f.shared); n != 1 {
		t.Fatalf("shared user's sessions touched: %d", n)
	}
	if n := count(`SELECT count(*) FROM users WHERE id = $1 AND email LIKE '%@example.test'`, f.shared); n != 1 {
		t.Fatal("shared user anonymised")
	}
	if n := count(`SELECT count(*) FROM taxpayer_members WHERE taxpayer_id = $1`, f.eng.TaxpayerID); n != 0 {
		t.Fatalf("members left: %d", n)
	}
	if n := count(`SELECT count(*) FROM taxpayer_invitations WHERE engagement_id = $1 AND email = 'davet@example.test'`, f.eng.ID); n != 0 {
		t.Fatal("invitation email kept")
	}
	// Accounting data and the taxpayer identity stay.
	if n := count(`SELECT count(*) FROM customers WHERE taxpayer_id = $1`, f.eng.TaxpayerID); n != 1 {
		t.Fatal("accounting record deleted")
	}
	if n := count(`SELECT count(*) FROM taxpayers WHERE id = $1 AND tax_id = '12345678901'`, f.eng.TaxpayerID); n != 1 {
		t.Fatal("taxpayer identity changed")
	}
	// Decided once.
	if _, err := f.svc.Approve(ctx, f.eng, f.admin, req.Uuid, ""); !errors.Is(err, ErrNotPending) {
		t.Fatalf("second approve: %v", err)
	}
}

func TestRejectAndScope(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: KindErasure, SubjectType: SubjectTaxpayer})
	if err != nil {
		t.Fatal(err)
	}
	if req, err = f.svc.Reject(ctx, f.eng, f.admin, req.Uuid, "yasal saklama süresi dolmadı"); err != nil || req.Status != StatusRejected {
		t.Fatalf("reject: %+v %v", req, err)
	}
	if _, err := f.svc.Approve(ctx, f.eng, f.admin, req.Uuid, ""); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approve after reject: %v", err)
	}
	// Nothing was erased.
	var n int
	_ = f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM taxpayer_members WHERE taxpayer_id = $1`, f.eng.TaxpayerID).Scan(&n)
	if n != 2 {
		t.Fatalf("members after reject: %d", n)
	}
	// Another engagement cannot see the request.
	other := f.eng
	other.ID, other.OrganizationID = other.ID+999, other.OrganizationID+999
	if _, err := f.svc.Get(ctx, other, req.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-engagement get: %v", err)
	}
	if _, err := f.svc.Create(ctx, f.eng, f.admin, CreateInput{Kind: "delete-everything", SubjectType: SubjectTaxpayer}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad kind: %v", err)
	}
}

// Approval is for office owners and admins only (migration 000077).
func TestPermissionSeeds(t *testing.T) {
	d := testdb.Open(t)
	rows, err := d.Pool.Query(context.Background(), `
		SELECT r.slug, p.slug FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id JOIN permissions p ON p.id = rp.permission_id
		WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND p.slug LIKE 'tenant.privacy.%'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for rows.Next() {
		var role, perm string
		if err := rows.Scan(&role, &perm); err != nil {
			t.Fatal(err)
		}
		got[role+" "+perm] = true
	}
	for _, want := range []string{"owner tenant.privacy.approve", "admin tenant.privacy.approve", "member tenant.privacy.request"} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
	if got["member tenant.privacy.approve"] {
		t.Error("members must not approve")
	}
}
