package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// demoReceiptExpenses is Atlas Gıda's October 2026 receipts as the office would
// upload them from Excel (#384): categories picked by vendor name, one
// with a typed category, a few approved into journal drafts.
const demoReceiptExpenses = "Tarih;Satıcı;VKN / TCKN;Fiş No;Açıklama;Kategori;KDV Oranı;KDV;Toplam;Ödeme;Plaka\n" +
	"02.10.2026;Demirtepe Petrol Ltd. Şti.;9000000033;0018;Dağıtım aracı yakıt;;20;250,00;1.500,00;Kredi kartı;34ABC123\n" +
	"06.10.2026;Köşe Lokantası;;0112;Müşteri yemeği;;10;45,45;500,00;Nakit;\n" +
	"09.10.2026;Hızlı Kargo A.Ş.;9000000041;5521;Numune gönderimi;;20;30,00;180,00;Nakit;\n" +
	"13.10.2026;Kırtasiye Dünyası;;0390;Fatura kağıdı, toner;Kırtasiye ve ofis malzemesi;20;70,00;420,00;Kredi kartı;\n" +
	"20.10.2026;Demirtepe Petrol Ltd. Şti.;9000000033;0044;Dağıtım aracı yakıt;;20;200,00;1.200,00;Kredi kartı;34ABC123\n" +
	"24.10.2026;Hızlı Oto Servis;;0077;Lastik değişimi;;20;400,00;2.400,00;Havale;34ABC123\n" +
	"28.10.2026;Marmara Bankası;;;Hesap işletim ücreti;;;;75,00;Havale;\n"

// demoReceiptZReports are the shop's Z reports of October 2026 on one ÖKC: Z 0105
// is missing (number gap) and 11–12 October has no report (day gap), so the
// Z screen shows both continuity warnings.
const demoReceiptZReports = "Tarih;ÖKC Seri No;Z No;Fiş Sayısı;Nakit;Kart;%1 Matrah;%1 KDV;%10 Matrah;%10 KDV;%20 Matrah;%20 KDV;Toplam\n" +
	"06.10.2026;JH20012345;0101;84;5.100,00;7.000,00;198,02;1,98;1.000,00;100,00;9.000,00;1.800,00;12.100,00\n" +
	"07.10.2026;JH20012345;0102;71;4.200,00;5.700,00;99,01;0,99;900,00;90,00;7.341,67;1.468,33;9.900,00\n" +
	"08.10.2026;JH20012345;0103;66;3.900,00;5.100,00;;;800,00;80,00;6.766,67;1.353,33;9.000,00\n" +
	"09.10.2026;JH20012345;0104;90;6.000,00;8.400,00;297,03;2,97;1.200,00;120,00;10.650,00;2.130,00;14.400,00\n" +
	"10.10.2026;JH20012345;0106;58;3.000,00;4.800,00;;;600,00;60,00;5.950,00;1.190,00;7.800,00\n" +
	"13.10.2026;JH20012345;0107;77;4.500,00;6.600,00;99,01;0,99;1.000,00;100,00;8.250,00;1.650,00;11.100,00\n"

// seedReceipts loads the demo expenses and Z reports through the Excel
// import, approves the first ones (journal drafts with source_kind receipt;
// the expense drafts are exported to Luca, the Z ones wait) and adds one
// office category to the template.
func seedReceipts(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	if err := seedReceiptRows(ctx, pool, scope, actor); err != nil {
		return err
	}
	// Onaylı masraf fişleri Luca'ya aktarılır; Z raporu taslakları bekler
	// (Ekim mutabakatında "aktarılmamış taslak" farkı, ledger.go).
	ids, err := draftsWhere(ctx, pool, scope, `d.origin->>'source' = 'receipt' AND d.origin->>'target_type' = 'expense'`)
	if err != nil {
		return err
	}
	return approveAndExport(ctx, pool, scope, actor, ids)
}

func seedReceiptRows(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.System(ctx, pool, "demo-seed: expenses and Z reports (#384)", func(ctx context.Context, _ pgx.Tx) error {
		q := db.New(pool)
		warn := warningsengine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
		svc := receipts.New(pool, seedStore, nil, nil, warn, nil).WithJournals(journals.New(pool, seedStore))
		if _, err := svc.SaveCategory(ctx, scope.OrganizationID, 0, actor, receipts.CategoryInput{
			Code: "temsil", Name: "Temsil ve ağırlama", AccountCode: "770", Keywords: []string{"lokanta", "restoran", "kafe"},
		}); err != nil {
			return fmt.Errorf("category: %w", err)
		}
		if _, err := svc.ImportExpenses(ctx, scope, actor, "atlas-ekim-masraf.csv", []byte(demoReceiptExpenses)); err != nil {
			return fmt.Errorf("expenses: %w", err)
		}
		if _, err := svc.ImportRetailSales(ctx, scope, "atlas-ekim-z.csv", []byte(demoReceiptZReports)); err != nil {
			return fmt.Errorf("z reports: %w", err)
		}
		exp, err := svc.Expenses(ctx, scope, receipts.ExpenseFilter{Period: "2026-10", Status: "pending", Limit: 100})
		if err != nil {
			return err
		}
		for _, x := range exp.Items {
			if x.CaptureMethod != "excel_import" || x.Date == nil || *x.Date > "2026-10-10" {
				continue
			}
			if _, err := svc.DecideExpense(ctx, scope, x.UUID, actor, true); err != nil {
				return fmt.Errorf("approve expense: %w", err)
			}
		}
		zs, err := svc.RetailSales(ctx, scope, receipts.RetailFilter{Period: "2026-10", Status: "pending", Limit: 100})
		if err != nil {
			return err
		}
		for _, z := range zs.Items {
			if z.Date == nil || *z.Date > "2026-10-08" {
				continue
			}
			if _, err := svc.DecideRetailSale(ctx, scope, z.UUID, actor, true); err != nil {
				return fmt.Errorf("approve z %v: %w", *z.ZNumber, err)
			}
		}
		return nil
	})
}
