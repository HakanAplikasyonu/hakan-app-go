package identity

import (
	"slices"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

func TestWithSessionBaseline(t *testing.T) {
	base := []string{rbac.PermAuthSession, rbac.PermNotificationsRead}
	if got := withSessionBaseline(nil); !slices.Equal(got, base) {
		t.Fatalf("no roles: %v", got)
	}
	in := []string{rbac.PermNotificationsRead}
	got := withSessionBaseline(in)
	if !slices.Contains(got, rbac.PermAuthSession) || !slices.Contains(got, rbac.PermNotificationsRead) || len(got) != 2 {
		t.Fatalf("with roles: %v", got)
	}
	if len(in) != 1 {
		t.Fatal("input slice was modified")
	}
	already := []string{rbac.PermAuthSession, rbac.PermNotificationsRead, "x"}
	if got := withSessionBaseline(already); len(got) != 3 {
		t.Fatalf("duplicate added: %v", got)
	}
}
