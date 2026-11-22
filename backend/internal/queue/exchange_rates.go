package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskExchangeRates fetches the TCMB daily rates and fills the invoice
// rates (#780).
const TaskExchangeRates = "app:exchange_rates:fetch"

// exchangeRatesCron: business days at 15:40 and 17:40 Istanbul. TCMB
// publishes at 15:30; the second run catches a late bulletin. Rates are
// upserted, so a rerun changes nothing.
const exchangeRatesCron = "CRON_TZ=Europe/Istanbul 40 15,17 * * 1-5"

// NewExchangeRatesTask builds the fetch.
func NewExchangeRatesTask() *asynq.Task {
	return asynq.NewTask(TaskExchangeRates, []byte("{}"))
}

// WithExchangeRates registers the fetch (no database transaction: HTTP
// calls) and the fill (the caller wraps it in rls.System). A network
// failure (no access to www.tcmb.gov.tr) is logged and the task ends
// without a retry: the next run tries again and the app keeps working.
// The fill runs either way, for bulletins stored earlier.
func (w *Worker) WithExchangeRates(fetch, fill func(ctx context.Context) (int, error)) *Worker {
	w.mux.HandleFunc(TaskExchangeRates, func(ctx context.Context, _ *asynq.Task) error {
		if fetch != nil {
			n, err := fetch(ctx)
			if err != nil {
				w.log.Warn("exchange_rates_fetch_failed", "stored", n, "error", err)
			} else if n > 0 {
				w.log.Info("exchange_rates_fetched", "bulletins", n)
			}
		}
		if fill == nil {
			return nil
		}
		n, err := fill(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			w.log.Info("exchange_rates_filled", "invoices", n)
		}
		return nil
	})
	return w
}

// RegisterExchangeRatesSchedule adds the fetch to the scheduler.
func RegisterExchangeRatesSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		exchangeRatesCron, NewExchangeRatesTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(15*time.Minute), asynq.Unique(time.Hour),
	); err != nil {
		return fmt.Errorf("queue: register exchange rates: %w", err)
	}
	return nil
}
