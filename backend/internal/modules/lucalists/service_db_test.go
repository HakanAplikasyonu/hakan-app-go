package lucalists

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Luca'ya aktar (Excel) listeleri uçtan uca (uydurma veriler): her liste
// için dosya Luca başlığıyla iner, belge kaydedilir, ikinci çıktı daha önce
// aktarılanı dışarıda bırakır, "yeniden dahil et" onay koduyla yeniden alır.
func TestLucaListsAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	d := testdb.Open(t)
	pool := d.Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	org := d.Organization(t)
	user := d.User(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, org, tp)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	var imp int64
	must(pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('c', 64)) RETURNING id`, org, eng).Scan(&imp))
	_, err := pool.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf) VALUES
		($1, $2, $3, '191.01', 'İNDİRİLECEK KDV %20', '191', 2, true),
		($1, $2, $3, '191.02', 'İNDİRİLECEK KDV %10', '191', 2, true),
		($1, $2, $3, '391.01', 'HESAPLANAN KDV %20', '391', 2, true),
		($1, $2, $3, '391.02', 'HESAPLANAN KDV %10', '391', 2, true),
		($1, $2, $3, '100.01', 'MERKEZ KASA', '100', 2, true),
		($1, $2, $3, '108.01', 'POS KREDİ KARTI', '108', 2, true),
		($1, $2, $3, '600.01', 'YURT İÇİ SATIŞLAR', '600', 2, true)`, org, eng, imp)
	must(err)

	// Gelen fatura: iki oran; ikinci satırın onaylı kodu ve önerisi yok.
	invoice := func(direction, number, typ string, items [][3]string, accounts []string) uuid.UUID {
		t.Helper()
		var id int64
		var u uuid.UUID
		must(pool.QueryRow(ctx, `
			INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
				issue_date, period, currency, tax_summary_source, customer_name, customer_tax_number,
				tax_exclusive_amount, tax_amount, tax_inclusive_amount, payable_amount)
			VALUES ($1, 'upload', $2, 'api', $3, $4, $5, '2026-10-14', '2026-10', 'TRY', 'items',
				'Örnek Karşı Taraf A.Ş.', '1234567890', 1500, 250, 1750, 1750)
			RETURNING id, uuid`, tp, uuid.NewString(), number, direction, typ).Scan(&id, &u))
		for i, it := range items {
			var item int64
			must(pool.QueryRow(ctx, `
				INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no, name, tax_exclusive_amount, line_extension_amount,
					total_tax_amount, tax_rate)
				VALUES ($1, $2, $3, 'Kalem', $4::text::numeric, $4::text::numeric, $5::text::numeric, $6::text::numeric) RETURNING id`,
				tp, id, i+1, it[0], it[1], it[2]).Scan(&item))
			if accounts[i] != "" {
				_, err := pool.Exec(ctx, `
					INSERT INTO account_assignments (organization_id, engagement_id, taxpayer_id, target_type, target_id, field, account_code)
					VALUES ($1, $2, $3, 'invoice_item', $4, 'account_code', $5)`, org, eng, tp, item, accounts[i])
				must(err)
			}
		}
		return u
	}
	items := [][3]string{{"1000", "200", "20"}, {"500", "50", "10"}}
	invoice("incoming", "ALS2026000000001", "ALIS", items, []string{"153.01", ""})
	invoice("outgoing", "SAT2026000000001", "SATIS", items, []string{"600.01", "600.01"})

	// Banka: TL giriş (karşı hesap onaylı) ve dövizli hareket (alınmaz).
	var box int64
	must(pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Örnek Banka TL', 'TRY') RETURNING id`, tp).Scan(&box))
	_, err = pool.Exec(ctx, `INSERT INTO bank_ledger_accounts (organization_id, engagement_id, taxpayer_id, cash_box_id, account_code)
		VALUES ($1, $2, $3, $4, '102.01')`, org, eng, tp, box)
	must(err)
	bank := func(src, currency string) int64 {
		var id int64
		must(pool.QueryRow(ctx, `
			INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
				booked_at, transaction_date, debit_credit, amount, currency, description, period, counterparty_name)
			VALUES ($1, $2, 'bank_test', $3, 'api', '2026-10-05', '2026-10-05', 'A', 1180, $4, 'Tahsilat', '2026-10', 'Örnek Müşteri Ltd.')
			RETURNING id`, tp, box, src, currency).Scan(&id))
		return id
	}
	in := bank("b-1", "TRY")
	bank("b-2", "USD")
	_, err = pool.Exec(ctx, `INSERT INTO bank_transaction_postings (organization_id, engagement_id, taxpayer_id, bank_transaction_id,
		account_code, account_name, status, confidence, source, reason, approved_at) VALUES ($1, $2, $3, $4, '120.01.002', 'Örnek Müşteri', 'approved', 1, 'manual', '', now())`,
		org, eng, tp, in)
	must(err)

	// Z raporu: iki oran, kartla tahsilat.
	var z int64
	must(pool.QueryRow(ctx, `INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
		z_report_number, okc_serial_number, tax_amount, total_amount, card_amount)
		VALUES ($1, 'test', 'z-1', 'manual', '2026-10-09', '2026-10', '0042', 'AB12345678', 150, 1100, 400) RETURNING id`, tp).Scan(&z))
	_, err = pool.Exec(ctx, `INSERT INTO retail_sale_vat_lines (taxpayer_id, retail_sale_id, tax_rate, tax_base, tax_amount)
		VALUES ($1, $2, 20, 500, 100), ($1, $2, 10, 450, 50)`, tp, z)
	must(err)

	svc := New(pool)
	want := map[List]struct{ check, skipped int }{
		IncomingInvoices: {check: 1}, // 153 (satır hesabı yok); cari 320 hafızadan bulunamaz
		OutgoingInvoices: {},
		BankTransactions: {skipped: 1}, // dövizli hareket
		ZReports:         {},
	}
	for _, list := range Lists {
		res, err := svc.Export(ctx, sc, user, list, Input{Period: "2026-10"})
		must(err)
		if res.Report.Exported != 1 || len(res.Report.Skipped) != want[list].skipped {
			t.Fatalf("%s first export %+v", list, res.Report)
		}
		if list == IncomingInvoices && len(res.Report.Check) < want[list].check {
			t.Fatalf("%s check rows %+v", list, res.Report.Check)
		}
		x, err := excelize.OpenReader(bytes.NewReader(res.Data))
		must(err)
		rows, err := x.GetRows(x.GetSheetName(0))
		must(err)
		for i, h := range lucaHeader {
			if rows[0][i] != h {
				t.Fatalf("%s header %d = %q", list, i, rows[0][i])
			}
		}
		// Her fiş dengeli, tek fiş: Fiş No hep 1.
		for _, r := range rows[1:] {
			if r[0] != "1" {
				t.Fatalf("%s: more than one voucher %v", list, rows)
			}
		}

		// İkinci çıktı: aktarılan belge dışarıda, dosya yok.
		res, err = svc.Export(ctx, sc, user, list, Input{Period: "2026-10"})
		if !errors.Is(err, ErrNothingToExport) || len(res.Report.AlreadyExported) != 1 {
			t.Fatalf("%s second export: %v %+v", list, err, res.Report)
		}
		// Yeniden dahil et: onay koduyla kaydedilir.
		res, err = svc.Export(ctx, sc, user, list, Input{Period: "2026-10", IncludeExported: true})
		must(err)
		if res.Report.Exported != 1 || len(res.Report.AlreadyExported) != 0 {
			t.Fatalf("%s re-include %+v", list, res.Report)
		}
		var rows2, confirmed int
		must(pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE confirmed_codes = ARRAY['DOCUMENT_ALREADY_EXPORTED'])
			FROM expense_export_documents WHERE engagement_id = $1 AND source_kind = $2 AND source_uuid IS NOT NULL
			AND expense_uuid IS NULL AND period = '2026-10'
			AND ($2 <> 'invoice' OR document_key IN (SELECT 'inv:' || uuid::text FROM invoices WHERE taxpayer_id = $3 AND direction = $4))`,
			eng, list.SourceKind(), tp, map[List]string{IncomingInvoices: "incoming", OutgoingInvoices: "outgoing"}[list]).Scan(&rows2, &confirmed))
		if rows2 != 2 || confirmed != 1 {
			t.Fatalf("%s recorded rows %d, confirmed %d", list, rows2, confirmed)
		}
	}

	// Bilinmeyen liste ve bozuk dönem.
	if _, err := svc.Export(ctx, sc, user, List("expenses"), Input{Period: "2026-10"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown list: %v", err)
	}
	if _, err := svc.Export(ctx, sc, user, ZReports, Input{Period: "2026-13"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad period: %v", err)
	}
}
