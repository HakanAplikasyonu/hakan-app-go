package usecase_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

// countingInvalidator records cache version bumps per organization.
type countingInvalidator struct{ n atomic.Int64 }

func (c *countingInvalidator) InvalidateOrganization(context.Context, int64) error {
	c.n.Add(1)
	return nil
}

type roleEnv struct {
	d   *testdb.DB
	svc *usecase.Service
	inv *countingInvalidator
}

func newRoleEnv(t *testing.T) roleEnv {
	t.Helper()
	d := testdb.Open(t)
	svc := usecase.New(repository.NewPostgres(d.Pool, db.New(d.Pool)), nil)
	inv := &countingInvalidator{}
	svc.SetInvalidator(inv)
	return roleEnv{d: d, svc: svc, inv: inv}
}

func (e roleEnv) uuids(t *testing.T, orgID, userID int64) (string, uuid.UUID, uuid.UUID) {
	t.Helper()
	var slug string
	var orgUUID, userUUID uuid.UUID
	ctx := context.Background()
	if err := e.d.Pool.QueryRow(ctx, `SELECT slug, uuid FROM organizations WHERE id = $1`, orgID).Scan(&slug, &orgUUID); err != nil {
		t.Fatal(err)
	}
	if err := e.d.Pool.QueryRow(ctx, `SELECT uuid FROM users WHERE id = $1`, userID).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	return slug, orgUUID, userUUID
}

func ptr[T any](v T) *T { return &v }

// TestNewOfficeGetsTemplateRoleCopies: ofis açılınca beş şablon rol ofise
// kopyalanır (D16) ve üyelikler ofis kopyasına bağlanır.
func TestNewOfficeGetsTemplateRoleCopies(t *testing.T) {
	e := newRoleEnv(t)
	ctx := context.Background()
	owner := e.d.User(t)
	var ownerUUID uuid.UUID
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM users WHERE id = $1`, owner).Scan(&ownerUUID)
	org, err := e.svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Yeni Ofis", OwnerUserUUID: ownerUUID})
	if err != nil {
		t.Fatal(err)
	}
	var orgID int64
	_ = e.d.Pool.QueryRow(ctx, `SELECT id FROM organizations WHERE uuid = $1`, org.UUID).Scan(&orgID)
	roles, err := e.svc.ListRoles(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, r := range roles {
		keys[r.Key] = r.IsSystem
	}
	for _, k := range []string{"owner", "admin", "accountant", "member", "intern"} {
		if !keys[k] {
			t.Errorf("template role %s missing or not system: %v", k, keys)
		}
	}
	var heldOrg *int64
	if err := e.d.Pool.QueryRow(ctx, `SELECT r.organization_id FROM organization_members m JOIN roles r ON r.id = m.role_id WHERE m.organization_id = $1`, orgID).Scan(&heldOrg); err != nil {
		t.Fatal(err)
	}
	if heldOrg == nil || *heldOrg != orgID {
		t.Fatalf("owner membership points at %v, want office copy of %d", heldOrg, orgID)
	}
}

// TestCustomRoleAppliesOnlyInItsOffice: issue #5 kabul kriterleri 1-3.
func TestCustomRoleAppliesOnlyInItsOffice(t *testing.T) {
	e := newRoleEnv(t)
	ctx := context.Background()
	orgA, orgB := e.d.Organization(t), e.d.Organization(t)
	owner, u := e.d.User(t), e.d.User(t)
	e.d.Member(t, orgA, owner, "owner")
	e.d.Member(t, orgA, u, "member")
	e.d.Member(t, orgB, u, "member")
	slugA, orgAUUID, uUUID := e.uuids(t, orgA, u)
	slugB, orgBUUID, _ := e.uuids(t, orgB, u)

	before := e.inv.n.Load()
	role, err := e.svc.CreateRole(ctx, orgA, usecase.RoleInput{
		Key: "kidemli", Name: ptr("Kıdemli müşavir"),
		Permissions: &[]string{"tenant.engagements.read", "tenant.engagements.read_all", "tenant.engagements.read"},
		Grantor:     usecase.OwnerGrantor,
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.IsSystem || len(role.Permissions) != 2 {
		t.Fatalf("role = %+v", role)
	}
	if e.inv.n.Load() <= before {
		t.Fatal("creating a role did not bump the cache version")
	}

	before = e.inv.n.Load()
	if _, err := e.svc.ChangeMemberRole(ctx, orgAUUID, uUUID, "kidemli", usecase.OwnerGrantor); err != nil {
		t.Fatalf("assign custom role: %v", err)
	}
	if e.inv.n.Load() <= before {
		t.Fatal("role change did not bump the cache version")
	}

	scopeA, err := e.svc.Resolve(ctx, slugA, u)
	if err != nil {
		t.Fatal(err)
	}
	if scopeA.MemberRole != "kidemli" || !scopeA.Has("tenant.engagements.read_all") {
		t.Fatalf("office A scope = %+v", scopeA)
	}
	scopeB, err := e.svc.Resolve(ctx, slugB, u)
	if err != nil {
		t.Fatal(err)
	}
	if scopeB.MemberRole != "member" || scopeB.Has("tenant.engagements.read_all") {
		t.Fatalf("office B scope leaked office A's role: %+v", scopeB)
	}

	// Başka ofisin rolü atanamıyor: B'de "kidemli" yok.
	if _, err := e.svc.ChangeMemberRole(ctx, orgBUUID, uUUID, "kidemli", usecase.OwnerGrantor); !errors.Is(err, usecase.ErrRoleNotFound) {
		t.Fatalf("assigning office A's role in office B: %v, want ErrRoleNotFound", err)
	}
	// Veritabanı da reddeder (doğrudan yazım).
	_, err = e.d.Pool.Exec(ctx, `UPDATE organization_members SET role_id = (SELECT id FROM roles WHERE organization_id = $1 AND slug = 'kidemli') WHERE organization_id = $2 AND user_id = $3`, orgA, orgB, u)
	if err == nil {
		t.Fatal("database accepted another office's role")
	}
	rolesB, _ := e.svc.ListRoles(ctx, orgB)
	for _, r := range rolesB {
		if r.Key == "kidemli" {
			t.Fatal("office A's custom role is listed in office B")
		}
	}

	// İzin değişikliği o ofiste hemen geçerli; önbellek sayacı artar.
	before = e.inv.n.Load()
	if _, err := e.svc.UpdateRole(ctx, orgA, usecase.RoleInput{Key: "kidemli", Permissions: &[]string{"tenant.engagements.read"}, Grantor: usecase.OwnerGrantor}); err != nil {
		t.Fatal(err)
	}
	if e.inv.n.Load() <= before {
		t.Fatal("updating a role did not bump the cache version")
	}
	scopeA, _ = e.svc.Resolve(ctx, slugA, u)
	if scopeA.Has("tenant.engagements.read_all") {
		t.Fatal("removed permission still granted")
	}

	// Kullanımdaki rol silinemez; serbest kalınca silinir.
	if err := e.svc.DeleteRole(ctx, orgA, "kidemli"); !errors.Is(err, usecase.ErrRoleInUse) {
		t.Fatalf("delete in-use role: %v", err)
	}
	if _, err := e.svc.ChangeMemberRole(ctx, orgAUUID, uUUID, "member", usecase.OwnerGrantor); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteRole(ctx, orgA, "kidemli"); err != nil {
		t.Fatalf("delete free role: %v", err)
	}
	_ = owner
}

func TestSystemRolesAndOwnerProtection(t *testing.T) {
	e := newRoleEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	owner := e.d.User(t)
	e.d.Member(t, org, owner, "owner")
	_, orgUUID, ownerUUID := e.uuids(t, org, owner)

	for _, k := range []string{"owner", "admin", "accountant", "member", "intern"} {
		if err := e.svc.DeleteRole(ctx, org, k); !errors.Is(err, usecase.ErrRoleIsSystem) {
			t.Errorf("delete system role %s: %v", k, err)
		}
	}
	if _, err := e.svc.UpdateRole(ctx, org, usecase.RoleInput{Key: "owner", Permissions: &[]string{}, Grantor: usecase.OwnerGrantor}); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("editing owner permissions: %v", err)
	}
	// Sistem rolünün adı ve izinleri değişebilir.
	if _, err := e.svc.UpdateRole(ctx, org, usecase.RoleInput{Key: "intern", Name: ptr("Aday"), Permissions: &[]string{"tenant.settings.read"}, Grantor: usecase.OwnerGrantor}); err != nil {
		t.Fatalf("edit system role: %v", err)
	}
	if _, err := e.svc.CreateRole(ctx, org, usecase.RoleInput{Key: "admin", Name: ptr("x"), Permissions: &[]string{}, Grantor: usecase.OwnerGrantor}); !errors.Is(err, usecase.ErrRoleKeyTaken) {
		t.Fatalf("duplicate key: %v", err)
	}
	for _, bad := range []usecase.RoleInput{
		{Key: "A", Name: ptr("x"), Permissions: &[]string{}},
		{Key: "ok_key", Name: ptr("x"), Permissions: &[]string{"platform.users.read"}},
		{Key: "ok_key", Name: ptr(""), Permissions: &[]string{}},
	} {
		if _, err := e.svc.CreateRole(ctx, org, bad); !errors.Is(err, usecase.ErrInvalidRequest) {
			t.Errorf("create %+v: %v, want ErrInvalidRequest", bad, err)
		}
	}
	// Son sahip koruması korunur.
	if _, err := e.svc.ChangeMemberRole(ctx, orgUUID, ownerUUID, "admin", usecase.OwnerGrantor); !errors.Is(err, usecase.ErrLastOrganizationOwner) {
		t.Fatalf("demoting last owner: %v", err)
	}
}

// TestRolePermissionsCannotEscalate: rol yazma izni olan kullanıcı yalnızca
// kendisinde olan izinleri bir role koyabilir ya da rolle verebilir (#167).
func TestRolePermissionsCannotEscalate(t *testing.T) {
	e := newRoleEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	owner, staff := e.d.User(t), e.d.User(t)
	e.d.Member(t, org, owner, "owner")
	e.d.Member(t, org, staff, "member")
	_, orgUUID, staffUUID := e.uuids(t, org, staff)
	lead := usecase.Grantor{Role: "ekip_lideri", Permissions: []string{
		"tenant.roles.write", "tenant.members.write", "tenant.engagements.read",
	}}

	// Kendisinde olmayan izinle yeni rol açılamaz.
	_, err := e.svc.CreateRole(ctx, org, usecase.RoleInput{
		Key: "genis", Name: ptr("Geniş"), Grantor: lead,
		Permissions: &[]string{"tenant.engagements.read", "tenant.engagements.read_all"},
	})
	if !errors.Is(err, usecase.ErrPermissionEscalation) {
		t.Fatalf("create with foreign permission: %v", err)
	}
	if _, err := e.svc.CreateRole(ctx, org, usecase.RoleInput{
		Key: "dar", Name: ptr("Dar"), Grantor: lead, Permissions: &[]string{"tenant.engagements.read"},
	}); err != nil {
		t.Fatalf("create within own permissions: %v", err)
	}

	// Var olan role yeni izin eklenemez; rolde zaten olan izin kalabilir,
	// izin çıkarılabilir.
	if _, err := e.svc.CreateRole(ctx, org, usecase.RoleInput{
		Key: "genis", Name: ptr("Geniş"), Grantor: usecase.OwnerGrantor,
		Permissions: &[]string{"tenant.engagements.read", "tenant.engagements.read_all"},
	}); err != nil {
		t.Fatalf("owner create: %v", err)
	}
	if _, err := e.svc.UpdateRole(ctx, org, usecase.RoleInput{
		Key: "dar", Grantor: lead, Permissions: &[]string{"tenant.engagements.read", "tenant.settings.write"},
	}); !errors.Is(err, usecase.ErrPermissionEscalation) {
		t.Fatalf("adding foreign permission: %v", err)
	}
	if _, err := e.svc.UpdateRole(ctx, org, usecase.RoleInput{
		Key: "genis", Name: ptr("Geniş ekip"), Grantor: lead,
		Permissions: &[]string{"tenant.engagements.read_all"},
	}); err != nil {
		t.Fatalf("keeping existing and removing permissions: %v", err)
	}
	roles, err := e.svc.ListRoles(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Key == "dar" && (len(r.Permissions) != 1 || r.Permissions[0] != "tenant.engagements.read") {
			t.Fatalf("rejected update changed the role: %v", r.Permissions)
		}
	}

	// Kendisinde olmayan izni taşıyan rol üyeye verilemez.
	if _, err := e.svc.ChangeMemberRole(ctx, orgUUID, staffUUID, "genis", lead); !errors.Is(err, usecase.ErrPermissionEscalation) {
		t.Fatalf("assigning broader role: %v", err)
	}
	if _, err := e.svc.ChangeMemberRole(ctx, orgUUID, staffUUID, "dar", lead); err != nil {
		t.Fatalf("assigning role within own permissions: %v", err)
	}
	if _, err := e.svc.ChangeMemberRole(ctx, orgUUID, staffUUID, "genis", usecase.OwnerGrantor); err != nil {
		t.Fatalf("owner assigning: %v", err)
	}
}

// TestNewTenantPermissionReachesEveryOwner: sonraki migration'ların eklediği
// tenant.* izni bütün ofislerin sahip rolüne kendiliğinden verilir.
func TestNewTenantPermissionReachesEveryOwner(t *testing.T) {
	e := newRoleEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	owner := e.d.User(t)
	e.d.Member(t, org, owner, "owner")
	slug, _, _ := e.uuids(t, org, owner)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO permissions (name, slug) VALUES ('x', 'tenant.future.read')`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Pool.Exec(ctx, `SELECT grant_tenant_role_permission('accountant', 'tenant.future.read')`); err != nil {
		t.Fatal(err)
	}
	scope, err := e.svc.Resolve(ctx, slug, owner)
	if err != nil || !scope.Has("tenant.future.read") {
		t.Fatalf("owner scope = %+v, %v", scope, err)
	}
	roles, _ := e.svc.ListRoles(ctx, org)
	for _, r := range roles {
		has := false
		for _, p := range r.Permissions {
			has = has || p == "tenant.future.read"
		}
		if (r.Key == "owner" || r.Key == "accountant") != has {
			t.Errorf("role %s has tenant.future.read = %v", r.Key, has)
		}
	}
	_ = orgctx.RoleOwner
}
