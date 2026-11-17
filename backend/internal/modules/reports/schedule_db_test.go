package reports

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func TestNextRun(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, istanbul) }
	run, p := NextRun(at(2026, 11, 17, 9), 20, nil)
	if p != "2026-10" || run.Format("2006-01-02") != "2026-11-20" {
		t.Fatalf("before the day: %s %s", run, p)
	}
	run, p = NextRun(at(2026, 11, 17, 9), 5, nil)
	if p != "2026-10" || run.Format("2006-01-02") != "2026-11-17" {
		t.Fatalf("due now: %s %s", run, p)
	}
	last := "2026-10"
	run, p = NextRun(at(2026, 11, 17, 9), 5, &last)
	if p != "2026-11" || run.Format("2006-01-02") != "2026-12-05" {
		t.Fatalf("already sent: %s %s", run, p)
	}
}

// TestScheduledReport: an enabled schedule queues last month's report once
// on its day; a taxpayer without an email is marked with the error.
func TestScheduledReport(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	q := db.New(d.Pool)
	mk := func(email string) engagementctx.Scope {
		tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 5300000000+testdb.Next()))
		if email != "" {
			if _, err := d.Pool.Exec(ctx, `UPDATE taxpayers SET contact_email = $2 WHERE id = $1`, tp, email); err != nil {
				t.Fatal(err)
			}
		}
		return engagementctx.Scope{ID: d.Engagement(t, org, tp), UUID: uuid.New(), OrganizationID: org, TaxpayerID: tp}
	}
	withMail, noMail := mk("patron@ornek.com.tr"), mk("")
	svc := NewDeliveryService(q)
	nov3 := time.Date(2026, 11, 3, 10, 0, 0, 0, istanbul)
	svc.now = func() time.Time { return nov3 }

	if _, err := svc.PutSchedule(ctx, q, noMail, 0, ScheduleInput{Enabled: true, DayOfMonth: 1}); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("enable without email: %v", err)
	}
	if _, err := svc.PutSchedule(ctx, q, withMail, 0, ScheduleInput{Enabled: true, DayOfMonth: 40}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("day 40: %v", err)
	}
	v, err := svc.PutSchedule(ctx, q, withMail, 0, ScheduleInput{Enabled: true, DayOfMonth: 2})
	if err != nil || !v.Enabled || v.NextPeriod == nil || *v.NextPeriod != "2026-10" {
		t.Fatalf("enable: %+v %v", v, err)
	}
	// The address goes away after enabling: the run records the error.
	if _, err := svc.PutSchedule(ctx, q, noMail, 0, ScheduleInput{Enabled: true, DayOfMonth: 1, Recipient: "gecici@ornek.com.tr"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE report_schedules SET recipient = 'bozuk' WHERE engagement_id = $1`, noMail.ID); err != nil {
		t.Fatal(err)
	}

	s := &Scheduler{Q: q, Now: func() time.Time { return nov3 }}
	n, err := s.RunDue(ctx)
	if err != nil || n != 1 {
		t.Fatalf("run: n=%d err=%v", n, err)
	}
	// Once per month.
	if n, _ := s.RunDue(ctx); n != 0 {
		t.Fatalf("second run queued %d", n)
	}
	page, err := svc.ListEngagement(ctx, withMail, 10, 0)
	if err != nil || page.Total != 1 || page.Items[0].Origin != OriginScheduled || page.Items[0].Period != "2026-10" {
		t.Fatalf("scheduled delivery: %+v %v", page, err)
	}
	bad, err := svc.GetSchedule(ctx, q, noMail)
	if err != nil || bad.LastError == nil || bad.LastPeriod == nil || *bad.LastPeriod != "2026-10" {
		t.Fatalf("no-email schedule: %+v %v", bad, err)
	}
	// Disabled schedules do not run.
	if _, err := svc.PutSchedule(ctx, q, withMail, 0, ScheduleInput{Enabled: false, DayOfMonth: 2}); err != nil {
		t.Fatal(err)
	}
	dec := &Scheduler{Q: q, Now: func() time.Time { return time.Date(2026, 12, 5, 10, 0, 0, 0, istanbul) }}
	if n, err := dec.RunDue(ctx); err != nil || n != 0 {
		t.Fatalf("disabled run: n=%d err=%v", n, err)
	}
}
