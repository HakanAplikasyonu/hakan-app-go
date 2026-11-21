package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	vatforecastrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/repository"
	vatforecastusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	vatwatch "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/watch"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Demo KDV eşiği (#780 B, demo geri bildirimi): ofisin "ödenecek KDV şu
// tutarı geçerse uyar" eşiği 10.000 TL. Atlas Gıda, Selin Yılmaz ve Deniz
// Market'in bu ayki faturaları eşiği geçer; nöbetçi gerçek akışla çalışır:
// "KDV eşiği aşıldı" olayı (vat_watch_events), Uyarılar sayfasındaki
// VAT_PAYABLE_THRESHOLD uyarısı ve zildeki bildirim oluşur. Olaylar
// dedupe_key ile bir kez yazılır; tekrar çalıştırmada çoğalmaz.
const demoVATThreshold = "10000"

// demoVATThresholdSource marks the extra current-month invoices.
const demoVATThresholdSource = "demo_kdv_esik"

// demoVATThresholdInvoices: this month's invoices that put Selin Yılmaz
// and Deniz Market over the threshold (Atlas is already over with
// salesDemoInvoices).
var demoVATThresholdInvoices = map[string][]monthInvoice{
	"selin": {
		{3, "outgoing", "Boğaziçi Mimarlık Ltd. Şti.", "1840326651", 58000, 20},
		{10, "outgoing", "Kuzey Yapı Denetim A.Ş.", "6011873420", 37000, 20},
		{7, "incoming", "Ofis Kırtasiye ve Bilgisayar Ltd. Şti.", "7350219846", 8000, 20},
	},
	"deniz": {
		{2, "outgoing", "Bornova Toptan Gıda Ltd. Şti.", "2190457713", 120000, 10},
		{9, "outgoing", "İzmir Catering Hizmetleri A.Ş.", "4728806139", 30000, 20},
		{5, "incoming", "Ege Un ve Yem Sanayi A.Ş.", "3305981274", 40000, 1},
	},
}

func seedVATThreshold(ctx context.Context, pool *pgxpool.Pool, orgID int64, now time.Time) error {
	q := database.NewQueries(pool)
	titles := map[string]string{}
	for _, tp := range demo.Taxpayers {
		titles[tp.Title] = tp.Key
	}
	for _, tp := range demo.OperatingTaxpayers {
		titles[tp.Title] = tp.Key
	}
	var targets []db.ListVATWatchTargetsRow
	if err := rls.System(ctx, pool, "demo-seed: VAT threshold targets", func(ctx context.Context, _ pgx.Tx) error {
		all, err := q.ListVATWatchTargets(ctx)
		if err != nil {
			return err
		}
		for _, t := range all {
			if t.OrganizationID == orgID {
				targets = append(targets, t)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("vat threshold targets: %w", err)
	}
	for _, t := range targets {
		if inv, ok := demoVATThresholdInvoices[titles[t.TaxpayerTitle]]; ok {
			if err := seedMonthInvoices(ctx, pool, t.TaxpayerID, now, demoVATThresholdSource, "KDE", inv); err != nil {
				return fmt.Errorf("vat threshold invoices → %s: %w", t.TaxpayerTitle, err)
			}
		}
	}
	cfg := warningsrepository.NewConfig(q)
	vat := vatforecastusecase.New(vatforecastrepository.New(q), cfg, warningsengine.New(warningsrepository.NewWarnings(q), cfg)).
		WithClock(func() time.Time { return now }).WithThresholds(cfg)
	limit := decimal.RequireFromString(demoVATThreshold)
	if err := rls.System(ctx, pool, "demo-seed: office VAT threshold", func(ctx context.Context, _ pgx.Tx) error {
		return vat.SavePayableLimit(ctx, orgID, 0, &limit)
	}); err != nil {
		return fmt.Errorf("vat threshold: %w", err)
	}
	w := &vatwatch.Watcher{Q: q, VAT: vat, Config: cfg, Notifier: demoNotifier{q: q}}
	raised := 0
	for _, t := range targets {
		if err := rls.System(ctx, pool, "demo-seed: VAT watch run", func(ctx context.Context, _ pgx.Tx) error {
			n, err := w.Check(ctx, t)
			raised += n
			return err
		}); err != nil {
			return fmt.Errorf("vat watch → %s: %w", t.TaxpayerTitle, err)
		}
	}
	fmt.Printf("vat threshold %s TL, %d new watch events\n", demoVATThreshold, raised)
	return nil
}

// demoNotifier writes the watch's in-app notifications as delivered rows,
// so the bell shows them without the worker (e-mail is not sent here).
type demoNotifier struct{ q *db.Queries }

func (n demoNotifier) Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, err
	}
	var org, user pgtype.Int8
	if in.OrganizationID != nil {
		org = pgtype.Int8{Int64: *in.OrganizationID, Valid: true}
	}
	if in.UserID != nil {
		user = pgtype.Int8{Int64: *in.UserID, Valid: true}
	}
	var link pgtype.Text
	if in.ActionURL != nil {
		link = pgtype.Text{String: *in.ActionURL, Valid: true}
	}
	for _, ch := range in.Channels {
		if ch != notifmodel.ChannelInapp {
			continue
		}
		if _, err := n.q.CreateNotification(ctx, db.CreateNotificationParams{
			OrganizationID: org, UserID: user, Channel: ch, Status: notifmodel.StatusSent,
			Priority: notifmodel.PriorityNormal, Title: in.Title, Body: in.Body, Payload: payload, ActionUrl: link,
			SourceEvent: pgtype.Text{String: in.SourceEvent, Valid: in.SourceEvent != ""}, MaxAttempts: 5,
		}); err != nil {
			return nil, fmt.Errorf("demo notification: %w", err)
		}
	}
	return nil, nil
}
