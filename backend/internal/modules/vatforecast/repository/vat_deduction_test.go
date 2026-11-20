package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// KDV indirim dönemi (#640): sonraki aya alınan alış faturasının KDV'si
// belge ayının tahmininden ve KDV1 satırlarından çıkar, indirim ayına girer;
// devir zinciri aynı kuralı izler. Ba/Bs belge ayında kalır.
func TestVATDeductionPeriodMovesInputVAT(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	orgID, tps := loadDemo(t, d)
	s := tps[1] // selin: yalnız alış
	now := time.Date(2026, 12, 15, 10, 0, 0, 0, usecase.Istanbul)
	svc := newService(d.Pool, &now)
	q := db.New(d.Pool)
	const next = "2026-11"

	var invID int64
	var moved string
	if err := d.Pool.QueryRow(ctx, `
		SELECT i.id, SUM(ROUND(s.vat * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::text
		FROM invoices i JOIN invoice_tax_summaries s ON s.invoice_id = i.id
		WHERE i.taxpayer_id = $1 AND i.direction = 'incoming' AND i.period = $2
		  AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL
		GROUP BY i.id HAVING SUM(s.vat) > 0 ORDER BY i.id LIMIT 1`, s.sub.TaxpayerID, demoPeriod).Scan(&invID, &moved); err != nil {
		t.Fatal(err)
	}
	movedVAT := decimal.RequireFromString(moved)

	incomingVAT := func(period string) decimal.Decimal {
		t.Helper()
		rows, err := q.TaxReturnInvoiceLines(ctx, db.TaxReturnInvoiceLinesParams{TaxpayerID: s.sub.TaxpayerID, Period: period})
		if err != nil {
			t.Fatal(err)
		}
		sum := decimal.Zero
		for _, r := range rows {
			if r.Direction == "incoming" {
				sum = sum.Add(invoicerepo.Dec(r.Vat))
			}
		}
		return sum
	}
	babsCount := func(period string) int64 {
		t.Helper()
		rows, err := q.TaxReturnCounterparties(ctx, db.TaxReturnCounterpartiesParams{TaxpayerID: s.sub.TaxpayerID, Period: period})
		if err != nil {
			t.Fatal(err)
		}
		var n int64
		for _, r := range rows {
			n += r.DocumentCount
		}
		return n
	}

	if err := rls.WithScope(ctx, d.Pool, officeScope(t, d.Pool, orgID), func(ctx context.Context, _ pgx.Tx) error {
		octBefore, err := svc.Forecast(ctx, s.sub, demoPeriod)
		if err != nil {
			t.Fatal(err)
		}
		novBefore, err := svc.Forecast(ctx, s.sub, next)
		if err != nil {
			t.Fatal(err)
		}
		retOct, retNov, babsOct := incomingVAT(demoPeriod), incomingVAT(next), babsCount(demoPeriod)

		if _, err := d.Pool.Exec(ctx, `UPDATE invoices SET vat_deduction_period = $2 WHERE id = $1`, invID, next); err != nil {
			t.Fatal(err)
		}
		oct, err := svc.Forecast(ctx, s.sub, demoPeriod)
		if err != nil {
			t.Fatal(err)
		}
		nov, err := svc.Forecast(ctx, s.sub, next)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := oct.Totals.InputVAT.Dec(), octBefore.Totals.InputVAT.Dec().Sub(movedVAT); !got.Equal(want) {
			t.Errorf("october input %s, want %s (moved %s)", got, want, movedVAT)
		}
		if got, want := nov.Totals.InputVAT.Dec(), novBefore.Totals.InputVAT.Dec().Add(movedVAT); !got.Equal(want) {
			t.Errorf("november input %s, want %s", got, want)
		}
		if oct.Totals.InvoiceCount != octBefore.Totals.InvoiceCount-1 || nov.Totals.InvoiceCount != novBefore.Totals.InvoiceCount+1 {
			t.Errorf("invoice counts oct %d->%d nov %d->%d", octBefore.Totals.InvoiceCount, oct.Totals.InvoiceCount,
				novBefore.Totals.InvoiceCount, nov.Totals.InvoiceCount)
		}
		// Devir zinciri: Kasım'a devreden, Ekim'in yeni devredenidir.
		if !nov.Totals.CarriedIn.Dec().Equal(oct.Totals.CarryOut.Dec()) {
			t.Errorf("carry into november %s, october carry out %s", nov.Totals.CarriedIn.Dec(), oct.Totals.CarryOut.Dec())
		}
		// KDV1 satırları da indirim ayını izler; Ba/Bs belge ayında kalır.
		if got := incomingVAT(demoPeriod); !got.Equal(retOct.Sub(movedVAT)) {
			t.Errorf("KDV1 october incoming %s, want %s", got, retOct.Sub(movedVAT))
		}
		if got := incomingVAT(next); !got.Equal(retNov.Add(movedVAT)) {
			t.Errorf("KDV1 november incoming %s, want %s", got, retNov.Add(movedVAT))
		}
		if got := babsCount(demoPeriod); got != babsOct {
			t.Errorf("Ba/Bs october documents %d, want %d", got, babsOct)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
