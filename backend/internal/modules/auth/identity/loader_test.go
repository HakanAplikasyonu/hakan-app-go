package identity

import (
	"slices"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

func TestWithSessionBaseline(t *testing.T) {
	if got := withSessionBaseline(nil); !slices.Equal(got, []string{rbac.PermAuthSession}) {
		t.Fatalf("no roles: %v", got)
	}
	in := []string{"notifications.read"}
	got := withSessionBaseline(in)
	if !slices.Contains(got, rbac.PermAuthSession) || !slices.Contains(got, "notifications.read") || len(got) != 2 {
		t.Fatalf("with roles: %v", got)
	}
	if len(in) != 1 {
		t.Fatal("input slice was modified")
	}
	already := []string{rbac.PermAuthSession, "x"}
	if got := withSessionBaseline(already); len(got) != 2 {
		t.Fatalf("duplicate added: %v", got)
	}
}
