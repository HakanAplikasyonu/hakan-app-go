package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// İki yazarkasa (Luca uçtan uca denetimi): mükellefin iki ÖKC'si aynı gün
// aynı Z numarasıyla (sayaçlar ayrı) ve aynı toplamla rapor verebilir.
// Telefondan / WhatsApp'tan gelen ikinci yazarkasanın Z raporu mükerrer
// sayılırsa taslağı açılmaz, Z raporları listesine ve Luca dosyasına hiç
// girmez. Aynı yazarkasanın aynı raporu (başka fotoğraf) yine mükerrerdir;
// ÖKC'si okunamayan rapor da her yazarkasayla karşılaştırılır. Uydurma
// veriler.
func TestZDuplicatePerRegister(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'Z') RETURNING id`, "zd-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		for _, tbl := range []string{"expenses", "retail_sales", "attachments"} {
			_, _ = pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE taxpayer_id = $1`, tp)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	reader := &fakeReader{}
	svc := New(pool, nopStore{}, reader, nil, nil, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	z := func(okc string) string {
		f := `"date":{"value":"15.10.2026","confidence":0.95},"z_report_number":{"value":"0042","confidence":0.95},` +
			`"total_amount":{"value":"5.430,00","confidence":0.95},"tax_amount":{"value":"812,00","confidence":0.95}`
		if okc != "" {
			f += `,"okc_serial_number":{"value":"` + okc + `","confidence":0.95}`
		}
		return `{"kind":"z_report","kind_confidence":0.97,"fields":{` + f + `}}`
	}
	capture := func(name, okc string) View {
		t.Helper()
		reader.answer = z(okc)
		v, err := svc.Capture(ctx, sc, user, name+".png", png(name), extract.KindZReport)
		must(err)
		return v
	}

	first := capture("kasa-1", "DM00000001")
	if first.Status != StatusDraft || first.Draft == nil || first.Draft.Type != "retail_sale" {
		t.Fatalf("first register: %+v", first)
	}
	second := capture("kasa-2", "DM00000002")
	if second.Status != StatusDraft || second.Draft == nil {
		t.Fatalf("second register's Z report taken as a duplicate: %+v", second)
	}
	again := capture("kasa-1-tekrar", "DM00000001")
	if again.Status != StatusDuplicate || again.Draft != nil {
		t.Fatalf("same register again: %+v", again)
	}
	noOKC := capture("okc-yok", "")
	if noOKC.Status != StatusDuplicate {
		t.Fatalf("Z report without ÖKC: %+v", noOKC)
	}
	var n int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM retail_sales WHERE taxpayer_id = $1 AND period = '2026-10'`, tp).Scan(&n))
	if n != 2 {
		t.Fatalf("retail sales = %d, want 2", n)
	}
}
