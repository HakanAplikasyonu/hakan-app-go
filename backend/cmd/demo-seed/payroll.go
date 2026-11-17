package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/payroll"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// payrollSecret keys the national id digest; main sets it from
// APP_ENCRYPTION_KEY so the server recognises the seeded employees.
var payrollSecret string

// payrollStore lets the payroll service upload through seedStore; reads and
// deletes are not needed while seeding.
type payrollStore struct{ up journals.ObjectStore }

func (p payrollStore) Upload(ctx context.Context, f storage.File, path string) error {
	return p.up.Upload(ctx, f, path)
}
func (payrollStore) Download(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("demo-seed: download not supported")
}
func (payrollStore) Delete(context.Context, string) error { return nil }

// Demo bordro (#382): Atlas Gıda'nın dört personeli. TCKN'ler sağlama
// toplamı geçen, kimseye ait olmayan test değerleri. Satırlar 2026 asgari
// ücret istisnalı bordro gibi tutarlı: net = brüt − kesintiler.
const demoPayrollHeader = "Adı Soyadı;TC Kimlik No;SGK Sicil No;Görevi;İşe Giriş Tarihi;Gün;Brüt Ücret;SGK İşçi Payı;" +
	"İşsizlik İşçi Payı;GV İstisnası;Ödenecek Gelir Vergisi;DV İstisnası;Damga Vergisi;Net Ücret;SGK İşveren Payı;" +
	"İşsizlik İşveren Payı;SGK Teşvik\n"

var demoPayrollRows = []string{
	"Deniz Aksoy;10000000078;3400101;Satış Müdürü;01.03.2022;30;60.000,00;8.400,00;600,00;3.315,70;5.684,30;197,38;258,02;45.057,68;12.300,00;1.200,00;3.000,00\n",
	"Elif Kara;10000000214;3400102;Muhasebe Uzmanı;15.06.2023;30;40.000,00;5.600,00;400,00;3.315,70;1.784,30;197,38;106,22;32.109,48;8.200,00;800,00;2.000,00\n",
	"Burak Şen;10000000382;3400103;Depo Sorumlusu;02.01.2024;30;30.000,00;4.200,00;300,00;3.315,70;0,00;197,38;30,32;25.469,68;6.150,00;600,00;1.500,00\n",
	"Zeynep Tan;10000000450;3400104;Satış Temsilcisi;10.02.2025;30;26.005,50;3.640,77;260,06;3.315,70;0,00;197,38;0,00;22.104,67;5.331,13;520,11;1.300,28\n",
}

// demoPDF is a one-page PDF; the seed adds the period so each file differs.
func demoPDF(title string) []byte {
	body := fmt.Sprintf("BT /F1 14 Tf 72 760 Td (%s) Tj ET", title)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := []int{}
	obj := func(s string) {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offs), s)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	return b.Bytes()
}

// seedPayroll gives Atlas the September and October 2026 payrolls (Luca
// format), their SGK accruals (September paid), September's journal draft
// and the payroll PDFs shown in the mükellef panel.
func seedPayroll(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	svc := payroll.New(database.Ambient{Pool: pool}, payrollStore{up: seedStore}, payrollSecret).
		WithJournals(journals.New(pool, seedStore))
	for _, period := range []string{"2026-09", "2026-10"} {
		csv := "LUCA BORDRO DÖKÜMÜ\nAtlas Gıda Ticaret A.Ş.\nDönem: " + period[:4] + "/" + period[5:] + "\n\n" + demoPayrollHeader
		for _, r := range demoPayrollRows {
			csv += r
		}
		res, err := svc.Import(ctx, scope, actor, "luca-bordro-"+period+".csv", []byte(csv), payroll.ImportOptions{})
		if err != nil {
			return fmt.Errorf("import %s: %w", period, err)
		}
		for _, d := range []struct{ kind, title string }{
			{"payroll", period + " bordro dökümü"},
			{"sgk_accrual", period + " SGK tahakkuk fişi"},
		} {
			if _, err := svc.UploadDocument(ctx, scope.TaxpayerID, actor, d.title+".pdf", demoPDF("Atlas Gida - "+d.title),
				payroll.DocumentInput{Period: period, Kind: d.kind, Title: d.title, VisibleToTaxpayer: true}); err != nil {
				return fmt.Errorf("document %s: %w", d.title, err)
			}
		}
		if period == "2026-09" {
			if _, err := svc.CreateJournalDraft(ctx, scope, actor, res.Run.UUID, payroll.Accounts{Expense: "770.03"}); err != nil {
				return fmt.Errorf("journal draft %s: %w", period, err)
			}
			if res.Run.SGK != nil {
				paid := res.Run.SGK.Payable
				at := "2026-10-30"
				if _, err := svc.UpdatePremium(ctx, scope.TaxpayerID, res.Run.SGK.UUID, payroll.PremiumInput{Paid: &paid, PaidAt: &at}); err != nil {
					return fmt.Errorf("sgk payment %s: %w", period, err)
				}
			}
		}
	}
	// Ekim'de işe giren, henüz bordroya girmemiş bir personel (elle eklendi).
	name, job, hire := "Mert Yıldız", "Stajyer", "2026-11-03"
	_, err := svc.CreateEmployee(ctx, scope.TaxpayerID, actor, payroll.EmployeeInput{FullName: &name, JobTitle: &job, HireDate: &hire})
	return err
}
