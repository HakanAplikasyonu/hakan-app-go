package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/engagements"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// İşletme defteri ve serbest meslek demo mükellefleri (#780). Yalnız yeni
// satır eklenir: mevcut demo mükellefleri ve kayıtları değişmez. Kaynak
// anahtarları sabittir (source_system 'demo', source_id 'demo:isletme:...'),
// ikinci çalıştırma aynı satırı tekrar eklemez.

// operatingMonths are the months with records.
var operatingMonths = []string{"2026-08", "2026-09", "2026-10", "2026-11"}

// operatingInvoice is one demo invoice of a month. Amounts are TL, KDV hariç.
type operatingInvoice struct {
	direction, docType, docClass string
	no, counterparty, taxID      string
	base, rate                   int
}

// operatingExpense is one demo receipt of a month.
type operatingExpense struct {
	receiptType, vendor, desc string
	base, rate                int
}

// operatingBook is the monthly pattern of a demo taxpayer; amounts grow a
// little each month so the summary has a trend.
type operatingBook struct {
	invoices []operatingInvoice
	expenses []operatingExpense
	// zBase: daily Z report total (KDV hariç) of the month; 0: none.
	zBase, zRate int
	// opening: 2026 dönem başı mal mevcudu (işletme defteri).
	opening string
}

var operatingBooks = map[string]operatingBook{
	"kaya": {
		invoices: []operatingInvoice{
			{"incoming", "SATIS", "invoice", "KRT", "Anadolu Kağıt Toptan Ltd. Şti.", "9000000104", 42000, 20},
			{"incoming", "SATIS", "invoice", "OFS", "Ofis Malzemeleri Dağıtım A.Ş.", "9000000259", 18500, 20},
			{"incoming", "SATIS", "invoice", "KRA", "Mehmet Öztürk (dükkân kirası)", "", 15000, 20},
			{"outgoing", "SATIS", "invoice", "KYA", "Yıldız Koleji", "9000000370", 26000, 20},
		},
		expenses: []operatingExpense{
			{"okc_receipt", "Enerjisa", "Elektrik faturası", 1900, 20},
			{"fuel_receipt", "Petrol Ofisi", "Akaryakıt", 2400, 20},
			{"bank_fee", "Demo Bank", "POS komisyonu", 650, 0},
		},
		zBase: 2100, zRate: 20, opening: "85000.00",
	},
	"deniz": {
		invoices: []operatingInvoice{
			{"incoming", "SATIS", "invoice", "GDA", "Ege Gıda Toptan A.Ş.", "9000000419", 68000, 1},
			{"incoming", "SATIS", "invoice", "ICK", "Serin İçecek Dağıtım Ltd. Şti.", "9000000530", 24000, 20},
			{"incoming", "SATIS", "invoice", "TMZ", "Parlak Temizlik Ürünleri A.Ş.", "9000000669", 9500, 20},
		},
		expenses: []operatingExpense{
			{"okc_receipt", "Gediz Elektrik", "Elektrik faturası", 3200, 20},
			{"expense_voucher", "Ali Yılmaz", "Hamaliye (gider pusulası)", 1200, 0},
		},
		zBase: 3600, zRate: 1, opening: "140000.00",
	},
	"arslan": {
		invoices: []operatingInvoice{
			{"outgoing", "SATIS", "smm", "SMM", "Marmara Lojistik A.Ş.", "9000000790", 30000, 20},
			{"outgoing", "SATIS", "smm", "SMM", "Kuzey İnşaat Ltd. Şti.", "9000000831", 22000, 20},
			{"incoming", "SATIS", "invoice", "BRO", "Hukuk Yayınevi A.Ş.", "9000000929", 3500, 20},
		},
		expenses: []operatingExpense{
			{"okc_receipt", "Turkcell", "Telefon ve internet", 1100, 20},
			{"other", "Büro kirası", "Büro kirası", 12000, 20},
		},
		opening: "",
	},
	// Serbest meslek mühendis (#780 A1); yabancı hizmet faturaları
	// foreigninvoices.go'da.
	"celik": {
		invoices: []operatingInvoice{
			{"outgoing", "SATIS", "smm", "SMM", "Marmara Otomasyon A.Ş.", "9000001034", 45000, 20},
			{"outgoing", "SATIS", "smm", "SMM", "Ege Makine Sanayi Ltd. Şti.", "9000001041", 28000, 20},
			{"incoming", "SATIS", "invoice", "RBT", "Robot Parça Tedarik A.Ş.", "9000001059", 6500, 20},
		},
		expenses: []operatingExpense{
			{"okc_receipt", "Turkcell", "Telefon ve internet", 900, 20},
			{"other", "Atölye kirası", "Atölye kirası", 15000, 20},
		},
		opening: "",
	},
}

// seedOperatingTaxpayers adds the işletme defteri / serbest meslek demo
// taxpayers to the demo office: engagement, assignees, defter türü and a
// few months of approved records.
func seedOperatingTaxpayers(ctx context.Context, pool *pgxpool.Pool, orgID int64, tpSvc *taxpayers.Service,
	engSvc *engagements.Service, store *engagements.Postgres, byEmail map[string]authmodel.User,
) error {
	for _, tp := range demo.OperatingTaxpayers {
		identity, err := ensureTaxpayer(ctx, tpSvc, tp.Taxpayer)
		if err != nil {
			return fmt.Errorf("taxpayer %s: %w", tp.Key, err)
		}
		created, err := engSvc.Create(ctx, orgID, identity.UUID)
		if errors.Is(err, engagements.ErrAlreadyActive) {
			// A previous run without -reset left it.
			fmt.Printf("taxpayer %-6s %s already seeded\n", tp.Key, identity.TaxID)
			continue
		}
		if err != nil {
			return fmt.Errorf("engagement %s: %w", tp.Key, err)
		}
		eng, err := store.GetEngagement(ctx, orgID, created.UUID)
		if err != nil {
			return fmt.Errorf("engagement %s: %w", tp.Key, err)
		}
		if err := seedReportContact(ctx, pool, eng.TaxpayerID, tp.Key); err != nil {
			return fmt.Errorf("report contact → %s: %w", tp.Key, err)
		}
		for _, email := range tp.Assignees {
			if err := engSvc.Assign(ctx, orgID, eng.ID, byEmail[email].UUID); err != nil {
				return fmt.Errorf("assign %s → %s: %w", email, tp.Key, err)
			}
		}
		book := operatingBooks[tp.Key]
		if err := rls.System(ctx, pool, "demo-seed: operating account taxpayer", func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE engagements SET ledger_type = $3 WHERE organization_id = $1 AND id = $2`,
				orgID, eng.ID, tp.LedgerType); err != nil {
				return fmt.Errorf("ledger type: %w", err)
			}
			if book.opening != "" {
				if _, err := tx.Exec(ctx, `INSERT INTO operating_account_inventories (organization_id, engagement_id, year, opening_inventory)
					VALUES ($1, $2, 2026, $3::numeric) ON CONFLICT (organization_id, engagement_id, year) DO NOTHING`,
					orgID, eng.ID, book.opening); err != nil {
					return fmt.Errorf("inventory: %w", err)
				}
			}
			return seedOperatingRecords(ctx, tx, eng.TaxpayerID, tp.Key, book)
		}); err != nil {
			return fmt.Errorf("operating account → %s: %w", tp.Key, err)
		}
		fmt.Printf("taxpayer %-6s %s engagement %s (%s)\n", tp.Key, identity.TaxID, created.UUID, tp.LedgerType)
	}
	return nil
}

// seedOperatingRecords inserts the months of invoices (with their VAT
// summary, so KDV1 reads them), approved expenses and approved Z reports
// (with their VAT breakdown).
func seedOperatingRecords(ctx context.Context, tx pgx.Tx, tpID int64, key string, book operatingBook) error {
	for mi, m := range operatingMonths {
		grow := 100 + mi*6 // percent
		for ii, inv := range book.invoices {
			base := inv.base * grow / 100
			vat := base * inv.rate / 100
			day := fmt.Sprintf("%s-%02d", m, 5+ii*6)
			no := fmt.Sprintf("%s2026%09d", inv.no, (mi+1)*100+ii+1)
			src := fmt.Sprintf("demo:isletme:%s:inv:%s:%d", key, m, ii)
			var id int64
			err := tx.QueryRow(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
					direction, type, document_class, issue_date, period, customer_name, customer_tax_number, currency,
					line_extension_amount, tax_exclusive_amount, tax_amount, tax_inclusive_amount, payable_amount,
					payable_amount_try, tax_summary_source)
				VALUES ($1, 'demo', $2, 'manual', $3, $4, $5, $6, $7::date, $8, $9, NULLIF($10, ''), 'TRY',
					$11::numeric, $11::numeric, $12::numeric, $11::numeric + $12::numeric, $11::numeric + $12::numeric,
					$11::numeric + $12::numeric, 'source_summary')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING
				RETURNING id`,
				tpID, src, no, inv.direction, inv.docType, inv.docClass, day, m, inv.counterparty, inv.taxID, base, vat).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("invoice %s: %w", src, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO invoice_tax_summaries (taxpayer_id, invoice_id, tax_rate, base, vat)
				VALUES ($1, $2, $3::numeric, $4::numeric, $5::numeric)`, tpID, id, inv.rate, base, vat); err != nil {
				return fmt.Errorf("invoice summary %s: %w", src, err)
			}
		}
		for ei, e := range book.expenses {
			base := e.base * grow / 100
			vat := base * e.rate / 100
			day := fmt.Sprintf("%s-%02d", m, 10+ei*4)
			src := fmt.Sprintf("demo:isletme:%s:exp:%s:%d", key, m, ei)
			if _, err := tx.Exec(ctx, `INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period,
					amount, tax_rate, tax_exclusive_amount, tax_amount, description, receipt_type, vendor_name, receipt_number,
					approval_status, approved_at)
				VALUES ($1, 'demo', $2, 'manual', $3::date, $4, $5::numeric + $6::numeric, $7::numeric, $5::numeric, $6::numeric,
					$8, $9, $10, $11, 'approved', NOW())
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`,
				tpID, src, day, m, base, vat, e.rate, e.desc, e.receiptType, e.vendor, fmt.Sprintf("F-%s-%d", m, ei+1)); err != nil {
				return fmt.Errorf("expense %s: %w", src, err)
			}
		}
		if book.zBase == 0 {
			continue
		}
		// Ayın son günü tek Z raporu: aylık perakende satış (gün × ortalama).
		base := book.zBase * 26 * grow / 100
		vat := base * book.zRate / 100
		src := fmt.Sprintf("demo:isletme:%s:z:%s", key, m)
		var id int64
		err := tx.QueryRow(ctx, `INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
				z_report_number, okc_serial_number, subtotal_amount, tax_amount, total_amount, cash_amount, card_amount,
				approval_status, approved_at)
			VALUES ($1, 'demo', $2, 'manual', $3::date, $4, $5, $6, $7::numeric, $8::numeric, $7::numeric + $8::numeric,
				($7::numeric + $8::numeric) * 0.4, ($7::numeric + $8::numeric) * 0.6, 'approved', NOW())
			ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING
			RETURNING id`,
			tpID, src, m+"-28", m, fmt.Sprintf("Z-%04d", 200+mi), "DM"+key, base, vat).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("z report %s: %w", src, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO retail_sale_vat_lines (taxpayer_id, retail_sale_id, tax_rate, tax_base, tax_amount)
			VALUES ($1, $2, $3::numeric, $4::numeric, $5::numeric)`, tpID, id, book.zRate, base, vat); err != nil {
			return fmt.Errorf("z breakdown %s: %w", src, err)
		}
	}
	return nil
}
