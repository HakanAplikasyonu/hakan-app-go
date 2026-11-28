package usecase

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Fiş yapay zekâ kontrolleri (Oğuz'un geri bildirimi): başka biri adına
// kesilmiş fiş, araçlarda olmayan / okunamayan plaka; önizlemede (Checks)
// ve onay ekranı rozetinde (ExpenseView.Checks). Ofis kodu kapatınca çıkmaz.
func TestReceiptChecksAgainstPostgres(t *testing.T) {
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
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rk-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '1'||lpad((floor(random() * 900000000))::bigint::text, 9, '0'), 'Deniz Gıda Ltd. Şti.') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		for _, tbl := range []string{"expenses", "retail_sales", "taxpayer_vehicles", "attachments"} {
			_, _ = pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE taxpayer_id = $1`, tp)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	q := db.New(pool)
	reader := &fakeReader{}
	svc := New(pool, nopStore{}, reader, &recReviewer{}, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	// Başka bir VKN'ye kesilmiş, plakası araçlarda olmayan yakıt fişi.
	reader.answer = `{"kind":"fuel_receipt","fields":{"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.95},"total_amount":{"value":"800,00","confidence":0.95},"vehicle_plate":{"value":"06 XY 42","confidence":0.95},"buyer_tax_number":{"value":"0000000001","confidence":0.9},"buyer_name":{"value":"Yıldız Market A.Ş.","confidence":0.9}}}`
	v, err := svc.Capture(ctx, sc, user, "a.png", png("ka"), extract.KindFuel)
	must(err)
	if v.Draft == nil || v.PlateCheck != "not_registered" || !slices.Equal(v.Checks, []string{CodeBuyerMismatch, CodePlateNotRegistered}) {
		t.Fatalf("capture %+v", v)
	}
	e, err := svc.Expense(ctx, sc, v.Draft.UUID)
	must(err)
	if !slices.Equal(e.Checks, []string{CodePlateNotRegistered, CodeBuyerMismatch}) {
		t.Fatalf("expense checks %v", e.Checks)
	}

	// Kendi adına, plakasız yakıt fişi: plaka okunamadı.
	reader.answer = `{"kind":"fuel_receipt","fields":{"date":{"value":"15.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.95},"total_amount":{"value":"300,00","confidence":0.95},"buyer_name":{"value":"DENİZ GIDA SAN. TİC. LTD. ŞTİ.","confidence":0.9}}}`
	u, err := svc.Capture(ctx, sc, user, "b.png", png("kb"), extract.KindFuel)
	must(err)
	if u.PlateCheck != "unreadable" || !slices.Equal(u.Checks, []string{CodePlateUnreadable}) {
		t.Fatalf("unreadable plate %+v", u)
	}

	// Ofis PLATE_UNREADABLE'ı kapatır: uyarı çıkmaz, rozet boş.
	_, err = pool.Exec(ctx, `INSERT INTO warning_rule_configs (organization_id, code, enabled) VALUES ($1, $2, false)`, org, CodePlateUnreadable)
	must(err)
	reader.answer = `{"kind":"fuel_receipt","fields":{"date":{"value":"16.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.95},"total_amount":{"value":"450,00","confidence":0.95}}}`
	off, err := svc.Capture(ctx, sc, user, "c.png", png("kc"), extract.KindFuel)
	must(err)
	if off.PlateCheck != "unreadable" || len(off.Checks) != 0 {
		t.Fatalf("disabled code shown %+v", off)
	}
	e, err = svc.Expense(ctx, sc, off.Draft.UUID)
	must(err)
	if e.Checks == nil || len(e.Checks) != 0 {
		t.Fatalf("disabled badge %v", e.Checks)
	}
}
