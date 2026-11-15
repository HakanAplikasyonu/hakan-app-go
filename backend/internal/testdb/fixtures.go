package testdb

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

var seq atomic.Int64

// Next returns a process-unique number for fixture names.
func Next() int64 { return seq.Add(1) }

// User inserts an active user and returns its id.
func (d *DB) User(t testing.TB) int64 {
	t.Helper()
	n := Next()
	var id int64
	err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, name, surname, status)
		VALUES ($1, 'x', 'Test', $2, 'active') RETURNING id`,
		fmt.Sprintf("u%d@example.test", n), fmt.Sprintf("U%d", n)).Scan(&id)
	if err != nil {
		t.Fatalf("testdb: user: %v", err)
	}
	return id
}

// Organization inserts an active organization and returns its id.
func (d *DB) Organization(t testing.TB) int64 {
	t.Helper()
	n := Next()
	var id int64
	err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("ofis-%d", n), fmt.Sprintf("Ofis %d", n)).Scan(&id)
	if err != nil {
		t.Fatalf("testdb: organization: %v", err)
	}
	return id
}

// Member adds userID to orgID with a tenant role key: the office's own copy
// when it has one (issue #5), else the template.
func (d *DB) Member(t testing.TB, orgID, userID int64, roleKey string) {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), `
		INSERT INTO organization_members (organization_id, user_id, role_id)
		SELECT $1, $2, r.id FROM roles r
		WHERE r.scope = 'tenant' AND (r.organization_id = $1 OR r.organization_id IS NULL) AND r.slug = $3
		ORDER BY r.organization_id NULLS LAST
		LIMIT 1`,
		orgID, userID, roleKey)
	if err != nil {
		t.Fatalf("testdb: member: %v", err)
	}
}

// Taxpayer inserts a taxpayer with the given tax id and returns its id.
func (d *DB) Taxpayer(t testing.TB, kind, taxID string) int64 {
	t.Helper()
	var id int64
	err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO taxpayers (kind, tax_id, title) VALUES ($1, $2, $3) RETURNING id`,
		kind, taxID, "Mükellef "+taxID).Scan(&id)
	if err != nil {
		t.Fatalf("testdb: taxpayer: %v", err)
	}
	return id
}

// Engagement opens an active engagement and returns its id.
func (d *DB) Engagement(t testing.TB, orgID, taxpayerID int64) int64 {
	t.Helper()
	var id int64
	err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`,
		orgID, taxpayerID).Scan(&id)
	if err != nil {
		t.Fatalf("testdb: engagement: %v", err)
	}
	return id
}

// Assign assigns a member to an engagement.
func (d *DB) Assign(t testing.TB, orgID, engagementID, userID int64) {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), `
		INSERT INTO engagement_assignments (organization_id, engagement_id, user_id) VALUES ($1, $2, $3)`,
		orgID, engagementID, userID)
	if err != nil {
		t.Fatalf("testdb: assign: %v", err)
	}
}

// EndEngagement ends an engagement.
func (d *DB) EndEngagement(t testing.TB, engagementID int64) {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), `
		UPDATE engagements SET status = 'ended', ended_at = NOW() WHERE id = $1`, engagementID)
	if err != nil {
		t.Fatalf("testdb: end engagement: %v", err)
	}
}
