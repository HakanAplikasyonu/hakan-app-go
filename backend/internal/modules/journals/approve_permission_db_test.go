package journals

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Sorumlu müşavir son onayı (#566): member (stajyer/personel) taslak hazırlar
// ama onaylayamaz; owner ve admin onaylar.
func TestApprovePermissionByRole(t *testing.T) {
	d := testdb.Open(t)
	has := func(role, perm string) bool {
		t.Helper()
		var ok bool
		if err := d.Pool.QueryRow(context.Background(), `
			SELECT EXISTS (SELECT 1 FROM role_permissions rp
				JOIN roles r ON r.id = rp.role_id JOIN permissions p ON p.id = rp.permission_id
				WHERE r.slug = $1 AND r.scope = 'tenant' AND r.organization_id IS NULL AND p.slug = $2)`,
			role, perm).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for _, c := range []struct {
		role           string
		write, approve bool
	}{{"owner", true, true}, {"admin", true, true}, {"member", true, false}} {
		if got := has(c.role, "tenant.journals.write"); got != c.write {
			t.Errorf("%s write = %v", c.role, got)
		}
		if got := has(c.role, "tenant.journals.approve"); got != c.approve {
			t.Errorf("%s approve = %v", c.role, got)
		}
	}
}
