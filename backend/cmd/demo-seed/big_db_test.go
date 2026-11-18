package main

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// testScale is the big demo shrunk for the database test.
var testScale = bigScale{Offices: 2, Taxpayers: 4, Months: 3}

// testNow pins the calendar: the big demo is relative to the current month.
var testNow = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func seedBigForTest(t *testing.T, d *testdb.DB) {
	t.Helper()
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	if err := seedBig(ctx, d.Pool, "Demo-Test-1!", testScale, testNow); err != nil {
		t.Fatal(err)
	}
}

// TestBigDemoSeedsAndResets: küçük ölçekli büyük demo ofisleri, mükellefleri
// ve kullanıcıları kurar; -reset hepsini (yalnızca demo verisini) siler.
func TestBigDemoSeedsAndResets(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	// A real office's taxpayer outside the demo prefix range must survive.
	realOrg := d.Organization(t)
	realTP := d.Taxpayer(t, "legal", "1234567890")
	d.Engagement(t, realOrg, realTP)

	seedBigForTest(t, d)
	slugs := demo.BigOfficeSlugs()[:testScale.Offices]
	if n := count(t, d, `SELECT count(*) FROM organizations WHERE slug = ANY($1::text[])`, slugs); n != testScale.Offices {
		t.Fatalf("offices = %d, want %d", n, testScale.Offices)
	}
	for _, slug := range slugs {
		n := count(t, d, `SELECT count(*) FROM engagements e JOIN organizations o ON o.id = e.organization_id
			JOIN taxpayers t ON t.id = e.taxpayer_id WHERE o.slug = $1 AND t.tax_id NOT IN ('9000000017', '10000000146')`, slug)
		if n != testScale.Taxpayers {
			t.Errorf("%s big taxpayers = %d, want %d", slug, n, testScale.Taxpayers)
		}
		if n := count(t, d, `SELECT count(*) FROM engagement_assignments a JOIN organizations o ON o.id = a.organization_id
			WHERE o.slug = $1`, slug); n < testScale.Taxpayers {
			t.Errorf("%s assignments = %d", slug, n)
		}
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayer_members m JOIN users u ON u.id = m.user_id
		WHERE u.email = $1 AND m.role = 'owner'`, demo.BigOffices[1].TaxpayerUser.Email); n != 1 {
		t.Errorf("portal user memberships = %d, want 1", n)
	}

	if err := dropDemo(ctx, d.Pool); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, `SELECT count(*) FROM organizations WHERE slug = ANY($1::text[])`, slugs); n != 0 {
		t.Errorf("offices after reset = %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE tax_id LIKE '93%' OR tax_id LIKE '1000%'`); n != 0 {
		t.Errorf("demo taxpayers after reset = %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM taxpayers WHERE id = $1`, realTP); n != 1 {
		t.Error("reset deleted a real taxpayer")
	}
}
