package rls_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

// fixture: two offices (A, B), each with members, an engagement and a
// taxpayer with a customer (cari) row. Inserted as the table owner, which RLS
// does not restrict.
type fixture struct {
	d                 *testdb.DB
	orgA, orgB        int64
	ownerA, staffA    int64
	assignedA, ownerB int64
	tpA, tpB          int64
	engA, engA2, engB int64
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := testdb.Open(t)
	f := fixture{d: d}
	f.orgA, f.orgB = d.Organization(t), d.Organization(t)
	f.ownerA, f.staffA, f.assignedA, f.ownerB = d.User(t), d.User(t), d.User(t), d.User(t)
	d.Member(t, f.orgA, f.ownerA, "owner")
	d.Member(t, f.orgA, f.staffA, "member")
	d.Member(t, f.orgA, f.assignedA, "member")
	d.Member(t, f.orgB, f.ownerB, "owner")
	f.tpA = d.Taxpayer(t, "legal", "1234567890")
	tpA2 := d.Taxpayer(t, "legal", "0000000001")
	f.tpB = d.Taxpayer(t, "individual", "10000000146")
	f.engA = d.Engagement(t, f.orgA, f.tpA)
	f.engA2 = d.Engagement(t, f.orgA, tpA2)
	f.engB = d.Engagement(t, f.orgB, f.tpB)
	d.Assign(t, f.orgA, f.engA, f.assignedA)
	d.Assign(t, f.orgB, f.engB, f.ownerB)
	for _, tp := range []int64{f.tpA, tpA2, f.tpB} {
		if _, err := d.Pool.Exec(context.Background(), `
			INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name)
			VALUES ($1, 'manual_upload', $2, 'manual', 'Cari')`, tp, "c-"+strconv.FormatInt(tp, 10)); err != nil {
			t.Fatalf("customer: %v", err)
		}
	}
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO secret_records (uuid, purpose, kek_id, wrapped_dek, ciphertext)
		VALUES (gen_random_uuid(), 'test', 'k1', '\x00', '\x00')`); err != nil {
		t.Fatalf("secret: %v", err)
	}
	return f
}

func count(t *testing.T, f fixture, s rls.Scope, query string) int {
	t.Helper()
	var n int
	err := rls.WithScope(context.Background(), f.d.Pool, s, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, query).Scan(&n)
	})
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestRLSHidesOtherOffices is the acceptance check of issue #6: under RLS a
// query with no WHERE clause returns no row of another office or taxpayer.
func TestRLSHidesOtherOffices(t *testing.T) {
	f := setup(t)
	officeA := rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA, EngagementsReadAll: true, TaxpayerIDs: []int64{f.tpA}}
	officeB := rls.Scope{OrganizationID: f.orgB, UserID: f.ownerB, EngagementsReadAll: true, TaxpayerIDs: []int64{f.tpB}}

	cases := []struct {
		query       string
		wantA       int
		wantB       int
		wantAsOwner int
	}{
		{"SELECT count(*) FROM organization_members", 3, 1, 4},
		{"SELECT count(*) FROM engagements", 2, 1, 3},
		{"SELECT count(*) FROM engagement_assignments", 1, 1, 2},
		{"SELECT count(*) FROM customers", 1, 1, 3},
		{"SELECT count(*) FROM taxpayers", 1, 1, 3},
		{"SELECT count(*) FROM secret_records", 0, 0, 1},
	}
	for _, tc := range cases {
		if got := count(t, f, officeA, tc.query); got != tc.wantA {
			t.Errorf("office A: %s = %d, want %d", tc.query, got, tc.wantA)
		}
		if got := count(t, f, officeB, tc.query); got != tc.wantB {
			t.Errorf("office B: %s = %d, want %d", tc.query, got, tc.wantB)
		}
		// Worker role: the same isolation.
		w := officeA
		w.Role = rls.RoleWorker
		if got := count(t, f, w, tc.query); got != tc.wantA {
			t.Errorf("worker A: %s = %d, want %d", tc.query, got, tc.wantA)
		}
		// The table owner (migrations, unscoped code paths) is not restricted.
		var n int
		if err := f.d.Pool.QueryRow(context.Background(), tc.query).Scan(&n); err != nil || n != tc.wantAsOwner {
			t.Errorf("owner: %s = %d (%v), want %d", tc.query, n, err, tc.wantAsOwner)
		}
	}
	// No context: nothing is visible.
	for _, tc := range cases {
		if got := count(t, f, rls.Scope{}, tc.query); got != 0 {
			t.Errorf("empty scope: %s = %d, want 0", tc.query, got)
		}
	}
}

// TestRLSEngagementScope: without read_all a member sees only the engagement
// rows of engagements assigned to them.
func TestRLSEngagementScope(t *testing.T) {
	f := setup(t)
	assigned := rls.Scope{OrganizationID: f.orgA, UserID: f.assignedA}
	unassigned := rls.Scope{OrganizationID: f.orgA, UserID: f.staffA}
	q := "SELECT count(*) FROM engagement_assignments"
	if got := count(t, f, assigned, q); got != 1 {
		t.Errorf("assigned member sees %d assignment rows, want 1", got)
	}
	if got := count(t, f, unassigned, q); got != 0 {
		t.Errorf("unassigned member sees %d assignment rows, want 0", got)
	}
	// An assigned user of A asking with B's organization id sees nothing.
	if got := count(t, f, rls.Scope{OrganizationID: f.orgB, UserID: f.assignedA}, q); got != 0 {
		t.Errorf("cross-office assignment rows = %d, want 0", got)
	}
}

// TestRLSRejectsWritesOutsideScope: WITH CHECK refuses rows of another office
// or taxpayer.
func TestRLSRejectsWritesOutsideScope(t *testing.T) {
	f := setup(t)
	officeA := rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA, EngagementsReadAll: true, TaxpayerIDs: []int64{f.tpA}}
	try := func(name, sql string, args ...any) {
		t.Helper()
		err := rls.WithScope(context.Background(), f.d.Pool, officeA, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
		if err == nil {
			t.Errorf("%s: write outside scope succeeded", name)
		}
	}
	try("customer of office B's taxpayer",
		`INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name) VALUES ($1, 'manual_upload', 'x', 'manual', 'x')`, f.tpB)
	other := f.d.Taxpayer(t, "legal", "1111111114")
	try("engagement for office B",
		`INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2)`, f.orgB, other)
	// Updates on invisible rows touch nothing.
	var n int64
	err := rls.WithScope(context.Background(), f.d.Pool, officeA, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE customers SET name = 'ele geçirildi' WHERE taxpayer_id = $1`, f.tpB)
		n = tag.RowsAffected()
		return err
	})
	if err != nil || n != 0 {
		t.Fatalf("update of office B's customers: %d rows, %v", n, err)
	}
	// Inside the scope writes work.
	err = rls.WithScope(context.Background(), f.d.Pool, officeA, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name) VALUES ($1, 'manual_upload', 'y', 'manual', 'y')`, f.tpA)
		return err
	})
	if err != nil {
		t.Fatalf("write inside scope: %v", err)
	}
}

func TestUnknownRoleRefused(t *testing.T) {
	f := setup(t)
	err := rls.WithScope(context.Background(), f.d.Pool, rls.Scope{Role: "postgres"}, func(context.Context, pgx.Tx) error { return nil })
	if err == nil {
		t.Fatal("arbitrary role accepted")
	}
}
