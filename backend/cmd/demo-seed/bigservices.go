package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/payroll"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reconciliation"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxreturns"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Bordro, beyanname ve mutabakat uygulamanın kendi servisleriyle kurulur:
// ekranlar, uygulamanın yazacağı anlık görüntüleri (snapshot) okur ve KDV1 /
// MUHSGK tutarları faturalardan, Z raporlarından, fişlerden ve bordrodan
// servisin kendi hesabıyla çıkar. Banka ödemeleri bu tutarlarla üretilir.

// bigWorkers is how many clients run through the services at once.
const bigWorkers = 6

func (c *bigClient) scope(o *bigOffice) engagementctx.Scope {
	return engagementctx.Scope{ID: c.engagementID, OrganizationID: o.orgID, TaxpayerID: c.taxpayerID}
}

// parallelClients runs fn for every model with bigWorkers goroutines and
// returns the first error.
func parallelClients(ms []*clientModel, fn func(k int, m *clientModel) error) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ferr error
		next = make(chan int)
	)
	for i := 0; i < bigWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range next {
				if err := fn(k, ms[k]); err != nil {
					mu.Lock()
					if ferr == nil {
						ferr = fmt.Errorf("%s: %w", ms[k].c.Title, err)
					}
					mu.Unlock()
				}
			}
		}()
	}
	for k := range ms {
		next <- k
	}
	close(next)
	wg.Wait()
	return ferr
}

// trMoney formats kuruş Luca style: 60.000,00.
func trMoney(k kurus) string {
	neg := k < 0
	if neg {
		k = -k
	}
	s := fmt.Sprintf("%d", k/100)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("%s,%02d", b.String(), k%100)
	if neg {
		out = "-" + out
	}
	return out
}

// payrollCSV is the month's Luca bordro dökümü (same columns as the pilot
// demo's payroll.go).
func payrollCSV(title string, pm *payrollMonth) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "LUCA BORDRO DÖKÜMÜ\n%s\nDönem: %s/%s\n\n%s", title, pm.period[:4], pm.period[5:], demoPayrollHeader)
	for _, l := range pm.lines {
		fmt.Fprintf(&b, "%s;%s;%s;%s;%s;%d;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s\n",
			l.emp.name, l.emp.tckn, l.emp.sgkNo, l.emp.job, l.emp.hired.Format("02.01.2006"), l.days,
			trMoney(l.gross), trMoney(l.sgkE), trMoney(l.unempE), trMoney(l.gvEx), trMoney(l.gv), trMoney(l.dvEx),
			trMoney(l.dv), trMoney(l.net), trMoney(l.sgkER), trMoney(l.unempER), trMoney(l.incentive))
	}
	return []byte(b.String())
}

// sgkPaidOn: SGK premiums are paid on the last day of the next month (the
// bank line uses the same day).
func sgkPaidOn(p string) time.Time { return periodStart(p).AddDate(0, 2, -1) }

// bigPayroll imports every month's payroll through the payroll service and
// records the SGK payments of the filed months.
func bigPayroll(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	svc := payroll.New(database.Ambient{Pool: pool}, payrollStore{up: seedStore}, payrollSecret)
	return parallelClients(ms, func(_ int, m *clientModel) error {
		sc := m.c.scope(o)
		for i := range m.payroll {
			pm := &m.payroll[i]
			err := rls.System(ctx, pool, "demo-seed: big payroll", func(ctx context.Context, _ pgx.Tx) error {
				res, err := svc.Import(ctx, sc, o.accountant(), "luca-bordro-"+pm.period+".csv", payrollCSV(m.c.Title, pm), payroll.ImportOptions{})
				if err != nil {
					return fmt.Errorf("payroll %s: %w", pm.period, err)
				}
				if res.Run == nil || res.Run.SGK == nil || !filedByDeadline(w, m.c, pm.period) {
					return nil
				}
				paid := res.Run.SGK.Payable
				on := sgkPaidOn(pm.period)
				if !on.Before(m.today(w)) {
					return nil
				}
				at := on.Format(time.DateOnly)
				_, err = svc.UpdatePremium(ctx, sc.TaxpayerID, res.Run.SGK.UUID, payroll.PremiumInput{Paid: &paid, PaidAt: &at})
				return err
			})
			if err != nil {
				return err
			}
			cnt.add("payroll_runs (servis)", 1)
		}
		return nil
	})
}

// bigTaxReturns prepares and files the KDV1, MUHSGK and Ba/Bs returns
// through the tax return service, month by month (KDV carries forward):
// past months filed; the last closed month prepared or in draft (deadline
// not reached); problem clients late with the month before. The filed
// payables go to the bank model.
func bigTaxReturns(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) ([]taxDue, error) {
	now := w.now
	svc := taxreturns.New(pool).WithClock(func() time.Time { return now })
	out := make([]taxDue, len(ms))
	last := len(w.periods) - 1
	err := parallelClients(ms, func(k int, m *clientModel) error {
		due := taxDue{kdv: map[string]kurus{}, muh: map[string]kurus{}, filed: map[string]bool{}}
		sc := m.c.scope(o)
		for _, p := range m.periods {
			idx := indexOf(w.periods, p)
			if idx >= last {
				break
			}
			filed := filedByDeadline(w, m.c, p)
			kinds := []string{taxreturns.KindKDV1, taxreturns.KindBABS}
			if m.payrollOf(p) != nil {
				kinds = append(kinds, taxreturns.KindMUHSGK)
			}
			// The last closed month: a quarter of the clients still in
			// draft, problem clients not started.
			prepare := true
			if idx == last-1 && !filed {
				if m.c.Problem {
					continue
				}
				prepare = m.c.Index%4 != 1
			}
			err := rls.System(ctx, pool, "demo-seed: big tax returns", func(ctx context.Context, _ pgx.Tx) error {
				for _, kind := range kinds {
					if !prepare {
						note := "Bekleyen masraf fişleri onaylanınca hazırlanacak."
						if _, err := svc.SaveDraft(ctx, sc, o.accountant(), kind, p, taxreturns.DraftInput{Note: &note}); err != nil {
							return fmt.Errorf("%s %s draft: %w", kind, p, err)
						}
						cnt.add("tax_returns (servis)", 1)
						continue
					}
					pv, err := prepareReturn(ctx, svc, sc, o.accountant(), kind, p)
					if err != nil {
						return err
					}
					if filed {
						day := 24
						if kind == taxreturns.KindKDV1 {
							day = 26
						}
						on := periodStart(p).AddDate(0, 1, day-1-k%3)
						accrual := fmt.Sprintf("%s-%s-%05d", strings.ToUpper(kind), p, 1000+k*37+idx)
						if pv, err = svc.File(ctx, sc, o.accountant(), kind, p, taxreturns.FileInput{
							FiledOn: on.Format(time.DateOnly), AccrualNumber: &accrual,
						}); err != nil {
							return fmt.Errorf("%s %s file: %w", kind, p, err)
						}
					}
					switch {
					case pv.KDV1 != nil:
						due.kdv[p] = kr(pv.KDV1.Totals.Payable.Dec().InexactFloat64())
					case pv.MUHSGK != nil:
						due.muh[p] = kr(pv.MUHSGK.Totals.TaxPayable.Dec().InexactFloat64())
					}
					cnt.add("tax_returns (servis)", 1)
				}
				return nil
			})
			if err != nil {
				return err
			}
			due.filed[p] = filed
		}
		// Months without a prepared return (not reached yet): model estimate.
		for _, p := range m.periods {
			if _, ok := due.kdv[p]; !ok {
				due.kdv[p] = m.kdvPayable[p]
			}
			if _, ok := due.muh[p]; !ok {
				if pm := m.payrollOf(p); pm != nil {
					due.muh[p] = pm.taxTotal()
				}
			}
		}
		out[k] = due
		return nil
	})
	return out, err
}

func prepareReturn(ctx context.Context, svc *taxreturns.Service, sc engagementctx.Scope, actor int64, kind, p string) (taxreturns.Preview, error) {
	pv, err := svc.Preview(ctx, sc, kind, p)
	if err != nil {
		return pv, fmt.Errorf("%s %s preview: %w", kind, p, err)
	}
	acks := pv.OpenWarnings
	for try := 0; try < 2; try++ {
		pv, err = svc.Prepare(ctx, sc, actor, kind, p, acks)
		var un *taxreturns.UnacknowledgedError
		if errors.As(err, &un) {
			acks = append(acks, un.Codes...)
			continue
		}
		break
	}
	if err != nil {
		return pv, fmt.Errorf("%s %s prepare: %w", kind, p, err)
	}
	return pv, nil
}

// bigReconciliation runs the month-end reconciliation of the last closed
// month for every client and sends counterparty letters: one agreed, one
// disputed and one still waiting.
func bigReconciliation(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	review := reviewusecase.New(reviewrepository.New(pool, db.New(pool))).Register(reconciliation.SuggestionKind, reconciliation.Applier{})
	svc := reconciliation.New(database.Ambient{Pool: pool}, review).
		WithLetters(reconciliation.LetterConfig{Secret: payrollSecret, FrontendURL: "http://localhost:3000"})
	p := w.periods[len(w.periods)-2]
	return parallelClients(ms, func(k int, m *clientModel) error {
		if indexOf(m.periods, p) < 0 {
			return nil
		}
		sc := m.c.scope(o)
		return rls.System(ctx, pool, "demo-seed: big reconciliation", func(ctx context.Context, _ pgx.Tx) error {
			det, err := svc.Run(ctx, sc, o.accountant(), reconciliation.RunInput{Period: p})
			if err != nil {
				return fmt.Errorf("run %s: %w", p, err)
			}
			cnt.add("reconciliation_runs (servis)", 1)
			for _, it := range det.Items {
				if it.Result == reconciliation.ResultMatched || !m.c.Problem || it.MainCode != "120" {
					continue
				}
				note := "Kur farkı fişi Luca'da elle girildi; portalda karşılığı yok. Müşteriden teyit bekleniyor."
				if _, err := svc.Decide(ctx, sc, it.UUID, o.accountant(), reconciliation.DecisionInput{Status: reconciliation.StatusExplained, Note: &note}); err != nil {
					return fmt.Errorf("decide: %w", err)
				}
			}
			page, err := svc.Counterparties(ctx, sc, det.Run.UUID.String(), "")
			if err != nil {
				return fmt.Errorf("counterparties: %w", err)
			}
			for i, c := range page.Items {
				if i >= 3 {
					break
				}
				email := fmt.Sprintf("mutabakat%d@%s.example.com", i+1, strings.ToLower(numberPrefix(c.Name)))
				l, err := svc.CreateLetter(ctx, sc, o.accountant(), reconciliation.LetterInput{CounterpartyUUID: c.UUID, Email: &email})
				if err != nil {
					return fmt.Errorf("letter: %w", err)
				}
				cnt.add("reconciliation_letters (servis)", 1)
				name := "Muhasebe Sorumlusu"
				switch (i + k) % 3 {
				case 0:
					if _, err := svc.RecordResponse(ctx, sc, l.UUID, reconciliation.ResponseInput{Response: "agreed", ResponderName: &name}); err != nil {
						return fmt.Errorf("agreed: %w", err)
					}
				case 1:
					bal := decimal.NewFromInt(int64(1250 + 375*(k%9))).StringFixed(2)
					note := "Bizim kayıtlarımızda son fatura henüz işlenmedi; bakiye farklı."
					if _, err := svc.RecordResponse(ctx, sc, l.UUID, reconciliation.ResponseInput{Response: "disputed", Balance: &bal, Note: &note, ResponderName: &name}); err != nil {
						return fmt.Errorf("disputed: %w", err)
					}
				}
			}
			return nil
		})
	})
}
