package journals

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Sorumlu müşavir son onayı (#566): personel (member) taslak hazırlar ama
// onaylayamaz; sahip, yönetici ve mali müşavir onaylar; stajyer yalnız okur.
// Şablon roller ve bir ofisin kopyaları ayrı ayrı denetlenir.
func TestApprovePermissionByRole(t *testing.T) {
	d := testdb.Open(t)
	org := d.Organization(t)
	count := func(role, perm string) int {
		t.Helper()
		var n int
		if err := d.Pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id JOIN permissions p ON p.id = rp.permission_id
			WHERE r.slug = $1 AND r.scope = 'tenant' AND p.slug = $2
			  AND (r.organization_id IS NULL OR r.organization_id = $3)`,
			role, perm, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	want := func(ok bool) int {
		if ok {
			return 2 // şablon + ofis kopyası
		}
		return 0
	}
	for _, c := range []struct {
		role           string
		write, approve bool
	}{{"owner", true, true}, {"admin", true, true}, {"accountant", true, true}, {"member", true, false}, {"intern", false, false}} {
		if got := count(c.role, "tenant.journals.write"); got != want(c.write) {
			t.Errorf("%s write rows = %d", c.role, got)
		}
		if got := count(c.role, "tenant.journals.approve"); got != want(c.approve) {
			t.Errorf("%s approve rows = %d", c.role, got)
		}
	}
	// Rol uyumu: stajyer fişleri görür; mali müşavir personelin bütün
	// izinlerine sahiptir.
	if got := count("intern", "tenant.journals.read"); got != 2 {
		t.Errorf("intern journals.read rows = %d", got)
	}
	if got := count("accountant", "tenant.review.decide"); got != 2 {
		t.Errorf("accountant review.decide rows = %d", got)
	}
}
