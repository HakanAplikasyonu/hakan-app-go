package accountrules

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
)

// Enqueuer is the Asynq client subset.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// RunAutoSuggest runs usecase.AutoSuggest for one queue payload (#319). In
// a worker it opens the job's row level security scope (office + taxpayer);
// inside a scoped request it joins that scope.
func RunAutoSuggest(pool rls.Beginner, svc *usecase.Service, log *slog.Logger) func(ctx context.Context, p queue.AccountSuggestPayload) error {
	return func(ctx context.Context, p queue.AccountSuggestPayload) error {
		sub := usecase.Subject{OrganizationID: p.OrganizationID, EngagementID: p.EngagementID, TaxpayerID: p.TaxpayerID}
		run := func(ctx context.Context) error {
			res, err := svc.AutoSuggest(ctx, sub, p.Invoices)
			if err != nil {
				return err
			}
			log.Info("account_auto_suggest", "engagement", p.EngagementID, "invoices", res.Invoices,
				"suggestions", res.Suggestions, "applied", res.Applied, "skipped", res.Skipped)
			return nil
		}
		if rls.InScope(ctx) {
			return run(ctx)
		}
		return rls.WithScope(ctx, pool, rls.Job(p.OrganizationID, p.TaxpayerID), func(ctx context.Context, _ pgx.Tx) error {
			return run(ctx)
		})
	}
}

// AfterUpload is the UBL upload's after-process hook (#319): the invoices a
// job added get account code suggestions. With a queue the work is a
// TaskAccountSuggest job (enqueued after the job's transaction commits, when
// there is one); without one it runs inline. A failure is logged: the
// upload itself has succeeded, and "Hesap kodu öner" can re-run it.
func AfterUpload(enq Enqueuer, run func(ctx context.Context, p queue.AccountSuggestPayload) error, log *slog.Logger) func(ctx context.Context, job db.UblUploadJob, added []uuid.UUID) {
	return func(ctx context.Context, job db.UblUploadJob, added []uuid.UUID) {
		p := queue.AccountSuggestPayload{OrganizationID: job.OrganizationID, EngagementID: job.EngagementID, TaxpayerID: job.TaxpayerID, Invoices: added}
		if enq == nil {
			if err := run(ctx, p); err != nil {
				log.Warn("account_auto_suggest_failed", "engagement", job.EngagementID, "error", err)
			}
			return
		}
		enqueue := func(context.Context) {
			task, err := queue.NewAccountSuggestTask(p)
			if err == nil {
				_, err = enq.Enqueue(task, asynq.Queue(queue.QueueImports), asynq.MaxRetry(3))
			}
			if err != nil {
				log.Warn("account_auto_suggest_enqueue_failed", "engagement", job.EngagementID, "error", err)
			}
		}
		if !dbtx.AfterCommit(ctx, enqueue) {
			enqueue(ctx)
		}
	}
}
