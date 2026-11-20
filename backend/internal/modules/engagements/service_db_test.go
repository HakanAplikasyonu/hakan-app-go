package engagements

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #206: hizmet ilişkisi yanıtı mükellef adını taşır; masaüstü ajanı
// mükellefleri UUID yerine adlarıyla listeler. Ad ofis kapsamındadır:
// başka ofisin ilişkisi (ve mükellefinin adı) hiçbir yoldan görünmez.
func TestEngagementCarriesTaxpayerTitleWithinOffice(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org, other := d.Organization(t), d.Organization(t)
	user := d.User(t)
	d.Member(t, org, user, "member")

	mine := d.Taxpayer(t, "legal", "1234567801")
	theirs := d.Taxpayer(t, "legal", "1234567802")
	e1 := d.Engagement(t, org, mine)
	e2 := d.Engagement(t, other, theirs)
	d.Assign(t, org, e1, user)

	var e1UUID, e2UUID uuid.UUID
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, e1).Scan(&e1UUID); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, e2).Scan(&e2UUID); err != nil {
		t.Fatal(err)
	}

	svc := New(NewPostgres(db.New(d.Pool)), nil)
	want := "Mükellef 1234567801"

	owner := orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll}}
	items, total, err := svc.List(ctx, owner, user, "active", 100, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("read_all list = %+v %d %v", items, total, err)
	}
	if items[0].UUID != e1UUID || items[0].TaxpayerTitle != want {
		t.Fatalf("read_all item = %+v, want title %q", items[0], want)
	}

	member := orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead}}
	items, _, err = svc.List(ctx, member, user, "", 100, 0)
	if err != nil || len(items) != 1 || items[0].TaxpayerTitle != want {
		t.Fatalf("assigned list = %+v %v", items, err)
	}

	got, err := svc.Get(ctx, org, e1UUID)
	if err != nil || got.TaxpayerTitle != want {
		t.Fatalf("get = %+v %v", got, err)
	}
	raw, _ := json.Marshal(got)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["taxpayer_title"] != want {
		t.Fatalf("json taxpayer_title = %v", m["taxpayer_title"])
	}

	// Başka ofisin ilişkisi: ne tek tek ne listede.
	if _, err := svc.Get(ctx, org, e2UUID); err != ErrNotFound {
		t.Fatalf("other office get err = %v, want ErrNotFound", err)
	}
	otherOwner := orgctx.Scope{ID: other, Permissions: owner.Permissions}
	items, _, err = svc.List(ctx, otherOwner, user, "", 100, 0)
	if err != nil || len(items) != 1 || items[0].TaxpayerTitle != "Mükellef 1234567802" {
		t.Fatalf("other office list = %+v %v", items, err)
	}
}

// #639: ofis geneli atama listesi tek istekte gelir; ofis kapsamındadır,
// read_all yoksa yalnız çağıranın atandığı ilişkiler görünür.
func TestListOrgAssignmentsScopedToOfficeAndCaller(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org, other := d.Organization(t), d.Organization(t)
	me, colleague := d.User(t), d.User(t)
	d.Member(t, org, me, "member")
	d.Member(t, org, colleague, "member")
	d.Member(t, other, me, "member")

	e1 := d.Engagement(t, org, d.Taxpayer(t, "legal", "1234567811"))
	e2 := d.Engagement(t, org, d.Taxpayer(t, "legal", "1234567812"))
	_ = d.Engagement(t, org, d.Taxpayer(t, "legal", "1234567813")) // atanmamış
	e4 := d.Engagement(t, other, d.Taxpayer(t, "legal", "1234567814"))
	d.Assign(t, org, e1, me)
	d.Assign(t, org, e1, colleague)
	d.Assign(t, org, e2, colleague)
	d.Assign(t, other, e4, me)

	uuidOf := func(id int64) uuid.UUID {
		var u uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	svc := New(NewPostgres(db.New(d.Pool)), nil)

	owner := orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll}}
	got, err := svc.ListOrgAssignments(ctx, owner, me, "")
	if err != nil || len(got) != 2 {
		t.Fatalf("read_all = %+v %v", got, err)
	}
	if got[0].EngagementUUID != uuidOf(e1) || len(got[0].Items) != 2 || got[1].EngagementUUID != uuidOf(e2) || len(got[1].Items) != 1 {
		t.Fatalf("read_all groups = %+v", got)
	}

	member := orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead}}
	got, err = svc.ListOrgAssignments(ctx, member, me, "active")
	if err != nil || len(got) != 1 || got[0].EngagementUUID != uuidOf(e1) || len(got[0].Items) != 2 {
		t.Fatalf("assigned only = %+v %v", got, err)
	}

	if _, err := svc.ListOrgAssignments(ctx, member, me, "bogus"); err == nil {
		t.Fatal("invalid status accepted")
	}
}
