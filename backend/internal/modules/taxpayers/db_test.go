package taxpayers

import (
	"context"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func newDBService(t *testing.T) (*Service, *testdb.DB) {
	t.Helper()
	d := testdb.Open(t)
	return New(NewPostgres(db.New(d.Pool))), d
}

func TestCreateRejectsSecondTaxpayerWithSameTaxID(t *testing.T) {
	svc, _ := newDBService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, CreateInput{Kind: KindLegal, TaxID: "1234567890", Title: "Örnek A.Ş."}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Aynı VKN, farklı yazımla ve farklı unvanla.
	_, err := svc.Create(ctx, CreateInput{Kind: KindLegal, TaxID: "123 456 78-90", Title: "Başka Unvan"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second create err = %v, want ErrAlreadyExists", err)
	}
	// Aynı TCKN iki kez.
	if _, err := svc.Create(ctx, CreateInput{Kind: KindIndividual, TaxID: "10000000146", Title: "Ayşe Yılmaz"}); err != nil {
		t.Fatalf("tckn create: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Kind: KindIndividual, TaxID: "10000000146", Title: "Ayşe Yılmaz"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("tckn second create err = %v", err)
	}
	_, total, err := svc.List(ctx, "", 50, 0)
	if err != nil || total != 2 {
		t.Fatalf("list total = %d err = %v, want 2", total, err)
	}
}

func TestDatabaseRejectsMalformedTaxID(t *testing.T) {
	_, d := newDBService(t)
	for _, tc := range []struct{ kind, taxID string }{
		{"legal", "10000000146"}, // tüzel kişi TCKN ile
		{"legal", "12345"},
		{"individual", "01234567890"},
		{"other", "1234567890"},
	} {
		_, err := d.Pool.Exec(context.Background(),
			`INSERT INTO taxpayers (kind, tax_id, title) VALUES ($1, $2, 'x')`, tc.kind, tc.taxID)
		if err == nil {
			t.Errorf("insert kind=%s tax_id=%s succeeded, want check violation", tc.kind, tc.taxID)
		}
	}
}

func TestFindByTaxID(t *testing.T) {
	svc, _ := newDBService(t)
	ctx := context.Background()
	if _, ok, err := svc.FindByTaxID(ctx, "1234567890"); err != nil || ok {
		t.Fatalf("before create: ok=%v err=%v", ok, err)
	}
	created, err := svc.Create(ctx, CreateInput{Kind: KindLegal, TaxID: "1234567890", Title: "Örnek A.Ş."})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := svc.FindByTaxID(ctx, "123-456-7890")
	if err != nil || !ok || got.UUID != created.UUID {
		t.Fatalf("after create: %+v ok=%v err=%v", got, ok, err)
	}
}

// TestOfficeAccessFollowsActiveAssignedEngagement is the acceptance check of
// issue #3: a taxpayer-scoped read returns nothing to an office user once the
// engagement has ended, and never to an unassigned member or another office.
func TestOfficeAccessFollowsActiveAssignedEngagement(t *testing.T) {
	svc, d := newDBService(t)
	ctx := context.Background()

	orgA, orgB := d.Organization(t), d.Organization(t)
	owner, assigned, unassigned, other := d.User(t), d.User(t), d.User(t), d.User(t)
	d.Member(t, orgA, owner, "owner")
	d.Member(t, orgA, assigned, "member")
	d.Member(t, orgA, unassigned, "member")
	d.Member(t, orgB, other, "owner")

	tp := d.Taxpayer(t, KindLegal, "1234567890")
	eng := d.Engagement(t, orgA, tp)
	d.Assign(t, orgA, eng, assigned)

	type probe struct {
		name    string
		org     int64
		user    int64
		readAll bool
	}
	check := func(p probe, want bool) {
		t.Helper()
		_, err := svc.ForOfficeMember(ctx, p.org, p.user, p.readAll, tp)
		got := err == nil
		if err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: unexpected error %v", p.name, err)
		}
		if got != want {
			t.Errorf("%s: access = %v, want %v", p.name, got, want)
		}
	}

	ownerRA := probe{"owner (read_all)", orgA, owner, true}
	assignedP := probe{"assigned member", orgA, assigned, false}
	unassignedP := probe{"unassigned member", orgA, unassigned, false}
	otherOffice := probe{"other office with read_all", orgB, other, true}
	foreignUser := probe{"member of A asking as office B", orgB, assigned, false}

	check(ownerRA, true)
	check(assignedP, true)
	check(unassignedP, false)
	check(otherOffice, false)
	check(foreignUser, false)

	d.EndEngagement(t, eng)

	check(ownerRA, false)
	check(assignedP, false)
	check(unassignedP, false)
	check(otherOffice, false)

	// Office B opens the next engagement: A stays locked out, B's owner gets in.
	d.Engagement(t, orgB, tp)
	check(ownerRA, false)
	check(assignedP, false)
	check(otherOffice, true)
}
