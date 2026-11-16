package officedash

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
)

// TemplateBlocker is the critical warning notification (issue #241).
const TemplateBlocker = events.WarningsBlocker

// alertWindow: blockers older than this are not announced (a backlog from a
// long outage is not pushed to phones).
const alertWindow = 24 * time.Hour

// alertBatch caps the engagements handled per run.
const alertBatch = 500

// AlertQueries is what the sweep reads and writes.
type AlertQueries interface {
	ListUnalertedBlockerWarnings(ctx context.Context, arg db.ListUnalertedBlockerWarningsParams) ([]db.ListUnalertedBlockerWarningsRow, error)
	ListEngagementAlertRecipients(ctx context.Context, arg db.ListEngagementAlertRecipientsParams) ([]int64, error)
	GetEngagementAlertNames(ctx context.Context, arg db.GetEngagementAlertNamesParams) (db.GetEngagementAlertNamesRow, error)
	InsertWarningOfficeAlerts(ctx context.Context, arg db.InsertWarningOfficeAlertsParams) (int64, error)
}

// Notifier enqueues notifications (notifications usecase.Service).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Alerts tells the office about new blocking warnings: once per warning, one
// notification per engagement and person (the engagement's assignees; the
// office owners when nobody is assigned). In-app rows mirror to mobile push.
// It runs as a scheduled job across offices (rls.System in the worker).
type Alerts struct {
	Q           AlertQueries
	Notifier    Notifier
	FrontendURL string
	Log         *slog.Logger
	Now         func() time.Time
}

// SendBlockerAlerts is the job's entry point; it returns how many
// notifications were enqueued.
func (a *Alerts) SendBlockerAlerts(ctx context.Context) (int, error) {
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	log := a.Log
	if log == nil {
		log = slog.Default()
	}
	rows, err := a.Q.ListUnalertedBlockerWarnings(ctx, db.ListUnalertedBlockerWarningsParams{
		Since: pgtype.Timestamptz{Time: now.Add(-alertWindow), Valid: true}, Lim: alertBatch,
	})
	if err != nil {
		return 0, fmt.Errorf("officedash: unalerted blockers: %w", err)
	}
	sent := 0
	for _, r := range rows {
		users, err := a.Q.ListEngagementAlertRecipients(ctx, db.ListEngagementAlertRecipientsParams{OrganizationID: r.OrganizationID, EngagementID: r.EngagementID})
		if err != nil {
			return sent, fmt.Errorf("officedash: alert recipients: %w", err)
		}
		names, err := a.Q.GetEngagementAlertNames(ctx, db.GetEngagementAlertNamesParams{OrganizationID: r.OrganizationID, EngagementID: r.EngagementID})
		if err != nil {
			return sent, fmt.Errorf("officedash: alert names: %w", err)
		}
		// The office panel's warnings page; the müşavir mobil app maps the
		// path (and the engagement in the query) to its own screen (#241).
		link := strings.TrimRight(a.FrontendURL, "/") + "/t/" + names.OrganizationSlug + "/warnings?engagement=" + names.EngagementUuid.String()
		count := strconv.FormatInt(r.WarningCount, 10)
		n := 0
		if a.Notifier != nil {
			for _, uid := range users {
				org, user := r.OrganizationID, uid
				if _, err := a.Notifier.Enqueue(ctx, notifmodel.EnqueueInput{
					OrganizationID: &org, UserID: &user,
					Channels:     []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
					TemplateCode: TemplateBlocker, SourceEvent: TemplateBlocker, ActionURL: &link,
					TemplateVars: map[string]string{"taxpayer_name": names.TaxpayerTitle, "count": count, "link": link},
					Payload: map[string]any{
						"kind": "warnings_blocker", "engagement_uuid": names.EngagementUuid.String(),
						"organization_slug": names.OrganizationSlug, "count": r.WarningCount,
					},
				}); err != nil {
					log.Warn("blocker_alert_failed", "error", err, "engagement_id", r.EngagementID)
					continue
				}
				n++
			}
		}
		// Recorded even when nobody could be told, so the sweep does not
		// retry the same warnings every run.
		if _, err := a.Q.InsertWarningOfficeAlerts(ctx, db.InsertWarningOfficeAlertsParams{
			WarningIds: r.WarningIds, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, Recipients: int32(n), //nolint:gosec // small
		}); err != nil {
			return sent, fmt.Errorf("officedash: record alerts: %w", err)
		}
		sent += n
	}
	return sent, nil
}
