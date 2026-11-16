package usecase

import (
	"context"
	"errors"
	"testing"

	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
)

func TestCheckGrant(t *testing.T) {
	t.Parallel()
	admin := Grantor{Role: "admin", Permissions: []string{"tenant.roles.write", "tenant.settings.read"}}
	cases := []struct {
		name  string
		g     Grantor
		perms []string
		ok    bool
	}{
		{"owner holds everything", OwnerGrantor, []string{"tenant.billing.write"}, true},
		{"subset", admin, []string{"tenant.settings.read"}, true},
		{"nothing added", admin, nil, true},
		{"missing permission", admin, []string{"tenant.settings.read", "tenant.billing.write"}, false},
		{"zero grantor grants nothing", Grantor{}, []string{"tenant.settings.read"}, false},
	}
	for _, c := range cases {
		err := CheckGrant(c.g, c.perms)
		if c.ok != (err == nil) {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if err != nil && !errors.Is(err, ErrPermissionEscalation) {
			t.Errorf("%s: err = %v, want ErrPermissionEscalation", c.name, err)
		}
	}
}

func TestAdded(t *testing.T) {
	t.Parallel()
	got := added([]string{"a", "b"}, []string{"b", "c", "a", "d"})
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Fatalf("added = %v", got)
	}
}

// Davet edilen rol, davet edenin sahip olmadığı izni veremez (#167).
func TestInvitationCannotGrantMorePermissions(t *testing.T) {
	t.Parallel()
	x := newInvFixture(t)
	x.f.perms["member"] = []string{"tenant.settings.read", "tenant.billing.write"}
	in := orgmodel.CreateInvitationInput{
		OrgID: x.org.ID, InviterUserID: x.owner.ID, InviterRoleKey: "admin",
		InviterPermissions: []string{"tenant.settings.read", "tenant.members.write"},
		Email:              "yeni@example.test", RoleKey: "member",
	}
	if _, err := x.svc.CreateInvitation(context.Background(), in); !errors.Is(err, ErrPermissionEscalation) {
		t.Fatalf("err = %v, want ErrPermissionEscalation", err)
	}
	if len(x.notif.sent) != 0 {
		t.Fatal("no email may be sent for a rejected invitation")
	}
	in.InviterPermissions = append(in.InviterPermissions, "tenant.billing.write")
	if _, err := x.svc.CreateInvitation(context.Background(), in); err != nil {
		t.Fatalf("inviter holding every permission: %v", err)
	}
}
