package reports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeMail struct {
	sent []mail.Message
	err  error
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

// TestDeliveryQueueSendRetry: one-click queue, history, the sweep sends the
// PDF, a failing mail server is retried and finally marked failed.
func TestDeliveryQueueSendRetry(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 5200000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	if _, err := d.Pool.Exec(ctx, `UPDATE taxpayers SET contact_email = 'muhasebe@ornek.com.tr' WHERE id = $1`, tp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
		direction, type, issue_date, period, currency, tax_summary_source, tax_exclusive_amount, tax_amount, payable_amount)
		VALUES ($1, 'test', 'd1', 'manual', 'd1', 'outgoing', 'SATIS', '2026-10-05', '2026-10', 'TRY', 'items', 1000, 200, 1200)`, tp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO period_summaries (organization_id, engagement_id, taxpayer_id, period, facts, template, body, status)
		VALUES ($1, $2, $3, '2026-10', '[]', 't', 'Gizli taslak', 'draft')`, org, eng, tp); err != nil {
		t.Fatal(err)
	}
	q := db.New(d.Pool)
	svc := NewDeliveryService(q)
	sc := engagementctx.Scope{ID: eng, UUID: uuid.New(), OrganizationID: org, TaxpayerID: tp}
	rg, _ := ParseRange("2026-10", "")

	if _, err := svc.Queue(ctx, sc, QueueInput{Range: rg, Recipient: "not-an-email"}); !errors.Is(err, ErrBadRecipient) {
		t.Fatalf("bad recipient: %v", err)
	}
	dv, err := svc.Queue(ctx, sc, QueueInput{Range: rg, OfficeName: "Demo Müşavirlik", Origin: OriginManual})
	if err != nil {
		t.Fatal(err)
	}
	if dv.Status != "queued" || dv.Recipient != "muhasebe@ornek.com.tr" || dv.Label != "Ekim 2026" {
		t.Fatalf("queued: %+v", dv)
	}
	page, err := svc.ListEngagement(ctx, sc, 50, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("history: %+v %v", page, err)
	}
	office, err := svc.ListOffice(ctx, org, 0, true, 50, 0)
	if err != nil || office.Total != 1 || office.Items[0].TaxpayerTitle == "" {
		t.Fatalf("office history: %+v %v", office, err)
	}

	fm := &fakeMail{}
	n, err := (&Sender{Q: q, Mail: fm}).SendDue(ctx)
	if err != nil || n != 1 || len(fm.sent) != 1 {
		t.Fatalf("send: n=%d err=%v sent=%d", n, err, len(fm.sent))
	}
	m := fm.sent[0]
	if m.To[0] != "muhasebe@ornek.com.tr" || !strings.Contains(m.Subject, "Ekim 2026") || !strings.Contains(m.Body, "₺1.000,00") {
		t.Fatalf("mail: %+v", m)
	}
	if m.HTML == "" || !strings.Contains(m.HTML, "₺1.000,00") {
		t.Fatal("report email without the HTML alternative")
	}
	if strings.Contains(m.Body+m.HTML, "Gizli taslak") {
		t.Fatal("a draft comment must not be emailed")
	}
	if len(m.Attachments) != 1 || !strings.HasPrefix(string(m.Attachments[0].Data), "%PDF") || !strings.HasSuffix(m.Attachments[0].Name, ".pdf") {
		t.Fatalf("attachment: %+v", m.Attachments)
	}
	page, _ = svc.ListEngagement(ctx, sc, 50, 0)
	if page.Items[0].Status != "sent" || page.Items[0].SentAt == nil {
		t.Fatalf("after send: %+v", page.Items[0])
	}

	// A failing server: retried with a delay, failed after MaxAttempts.
	if _, err := svc.Queue(ctx, sc, QueueInput{Range: rg, Recipient: "ikinci@ornek.com.tr"}); err != nil {
		t.Fatal(err)
	}
	bad := &Sender{Q: q, Mail: &fakeMail{err: errors.New("smtp down")}}
	for i := 0; i < MaxAttempts; i++ {
		if _, err := bad.SendDue(ctx); err != nil {
			t.Fatal(err)
		}
		// Make the retry due now.
		if _, err := d.Pool.Exec(ctx, `UPDATE report_deliveries SET next_attempt_at = NOW() - INTERVAL '1 second' WHERE status = 'queued'`); err != nil {
			t.Fatal(err)
		}
	}
	page, _ = svc.ListEngagement(ctx, sc, 50, 0)
	last := page.Items[0]
	if last.Status != "failed" || last.Attempts != MaxAttempts || last.LastError == nil || !strings.Contains(*last.LastError, "smtp down") {
		t.Fatalf("failed delivery: %+v", last)
	}
	_ = time.Now
}
