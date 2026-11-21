package communication

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication/netgsm"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Delivery report polling (#780 D): NetGSM accepted the message (status
// sent, job id written); the report endpoint later says whether the
// operator delivered it. The sweep writes delivered_at, or marks the record
// failed with the operator's reason.
const (
	// DeliveryWindow: messages older than this are no longer polled.
	DeliveryWindow = 3 * 24 * time.Hour
	// deliveryBatch caps the records one sweep looks at.
	deliveryBatch = 2000
	// deliveryRequestsPerOffice caps report calls per office per sweep, so
	// the sweep never eats the office's 40 requests a minute.
	deliveryRequestsPerOffice = 10
)

// DeliveryErrorPrefix starts error_code of a record the operator failed to
// deliver ("dlr_3" = wrong number).
const DeliveryErrorPrefix = "dlr_"

// SweepDeliveryReports polls NetGSM for the delivery state of the recent
// sent messages of every office and returns how many records changed. It
// does nothing when the server does not send for real (NETGSM_SEND_ENABLED
// off) and skips offices in test mode. Each office's calls go through the
// same 40-a-minute limiter as sending.
func (s *Service) SweepDeliveryReports(ctx context.Context, log *slog.Logger) (int, error) {
	if !s.live {
		return 0, nil
	}
	var pending []db.ListSmsAwaitingDeliveryRow
	err := rls.System(ctx, s.pool, "communication: sms delivery reports across offices", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		pending, err = s.q.ListSmsAwaitingDelivery(ctx, db.ListSmsAwaitingDeliveryParams{
			Since: pgtype.Timestamptz{Time: s.now().Add(-DeliveryWindow), Valid: true}, Lim: deliveryBatch,
		})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("list sms awaiting delivery: %w", err)
	}
	if len(pending) == 0 {
		return 0, nil
	}
	byOrg := map[int64][]db.ListSmsAwaitingDeliveryRow{}
	var orgs []int64
	for _, p := range pending {
		if _, ok := byOrg[p.OrganizationID]; !ok {
			orgs = append(orgs, p.OrganizationID)
		}
		byOrg[p.OrganizationID] = append(byOrg[p.OrganizationID], p)
	}
	changed := 0
	var firstErr error
	for _, org := range orgs {
		n, err := s.sweepOffice(ctx, org, byOrg[org])
		changed += n
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return changed, err
			}
			if log != nil {
				log.Warn("sms_delivery_report_failed", "organization_id", org, "error", err)
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return changed, firstErr
}

// sweepOffice polls one office's records.
func (s *Service) sweepOffice(ctx context.Context, org int64, rows []db.ListSmsAwaitingDeliveryRow) (int, error) {
	var settings *db.OfficeSmsSetting
	err := rls.System(ctx, s.pool, "communication: office sms account for delivery reports", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		settings, err = s.load(ctx, org)
		return err
	})
	if err != nil {
		return 0, err
	}
	// Test mode or a switched-off account: nothing was really sent.
	if settings == nil || !settings.Enabled || settings.TestMode {
		return 0, nil
	}
	cred, err := s.credentials(ctx, settings)
	if err != nil {
		return 0, err
	}
	byJob := map[string][]db.ListSmsAwaitingDeliveryRow{}
	var jobs []string
	for _, r := range rows {
		if _, ok := byJob[r.ProviderJobID]; !ok {
			jobs = append(jobs, r.ProviderJobID)
		}
		byJob[r.ProviderJobID] = append(byJob[r.ProviderJobID], r)
	}
	changed := 0
	for i, calls := 0, 0; i < len(jobs) && calls < deliveryRequestsPerOffice; i, calls = i+netgsm.MaxReportJobs, calls+1 {
		end := min(i+netgsm.MaxReportJobs, len(jobs))
		if err := s.limit.wait(ctx, org); err != nil {
			return changed, err
		}
		items, err := s.client.Report(ctx, cred, jobs[i:end])
		if err != nil {
			return changed, err
		}
		n, err := s.applyReport(ctx, org, byJob, items)
		changed += n
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}

// applyReport writes the delivered / failed outcomes of one report answer.
func (s *Service) applyReport(ctx context.Context, org int64, byJob map[string][]db.ListSmsAwaitingDeliveryRow, items []netgsm.ReportItem) (int, error) {
	type update struct {
		id   int64
		item netgsm.ReportItem
	}
	var ups []update
	perJob := map[string]int{}
	for _, it := range items {
		perJob[it.JobID]++
	}
	for _, it := range items {
		if it.Delivery == netgsm.DeliveryWaiting {
			continue
		}
		for _, r := range byJob[it.JobID] {
			// A job of several numbers: match the number; a one-number job
			// (every send of this service) needs no match.
			if perJob[it.JobID] > 1 && !samePhone(r.Phone, it.Phone) {
				continue
			}
			ups = append(ups, update{id: r.ID, item: it})
		}
	}
	if len(ups) == 0 {
		return 0, nil
	}
	changed := 0
	err := rls.System(ctx, s.pool, "communication: write sms delivery reports", func(ctx context.Context, _ pgx.Tx) error {
		for _, u := range ups {
			var n int64
			var err error
			if u.item.Delivery == netgsm.DeliveryDelivered {
				at := s.now()
				if u.item.DeliveredAt != nil && !u.item.DeliveredAt.After(at) {
					at = *u.item.DeliveredAt
				}
				n, err = s.q.MarkSmsDelivered(ctx, db.MarkSmsDeliveredParams{OrganizationID: org, ID: u.id,
					DeliveredAt: pgtype.Timestamptz{Time: at, Valid: true}})
			} else {
				n, err = s.q.MarkSmsDeliveryFailed(ctx, db.MarkSmsDeliveryFailedParams{OrganizationID: org, ID: u.id,
					ErrorCode:    DeliveryErrorPrefix + u.item.Status,
					ErrorMessage: "Operatör iletemedi: " + netgsm.StatusMessage(u.item.Status)})
			}
			if err != nil {
				return err
			}
			changed += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("write sms delivery: %w", err)
	}
	return changed, nil
}

// samePhone compares the last ten digits of two numbers.
func samePhone(a, b string) bool {
	return lastDigits(a, 10) != "" && lastDigits(a, 10) == lastDigits(b, 10)
}

func lastDigits(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) > n {
		d = d[len(d)-n:]
	}
	return d
}
