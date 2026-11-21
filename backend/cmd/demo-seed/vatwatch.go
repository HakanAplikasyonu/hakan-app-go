package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Demo KDV bildirimi ve nöbetçi olayı (#780 B, toplantı örneği): 1 Ekim'de
// mükellefe "Ekim'de ödeme çıkmayacak" denmiş; 3 Ekim'de SSK'ya kesilen
// satış faturasıyla 2.400 TL ödeme çıkmış, nöbetçi "KDV durumu değişti"
// demiş. Rapor sayfasındaki KDV kartında uyarı ve "Mükellefe yeni bilgi
// gönder" görünür.
const (
	demoVATNoticeUUID = "7a7d0c00-0780-4b00-8000-000000000001"
	demoVATPeriod     = "2026-10"
)

func seedVATWatch(ctx context.Context, pool *pgxpool.Pool, orgID, engID, tpID, sentBy int64) error {
	return rls.System(ctx, pool, "demo-seed: VAT notice and watch event", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO vat_notices (uuid, organization_id, engagement_id, taxpayer_id, period, outcome,
			output_vat, input_vat, carried_in, payable, carry_out, due_date, message, sent_by, created_at)
			VALUES ($1::uuid, $2, $3, $4, $5, 'no_payment', 18000, 20000, 0, 0, 2000, '2026-11-26',
			'Bu ay ödemeniz çıkmıyor; 2.000 TL devreden KDV Kasım''a aktarılıyor.', $6, '2026-10-01 10:00:00+03')
			ON CONFLICT (uuid) DO NOTHING`,
			demoVATNoticeUUID, orgID, engID, tpID, demoVATPeriod, sentBy); err != nil {
			return fmt.Errorf("vat notice: %w", err)
		}
		var title, engUUID, tpUUID string
		if err := tx.QueryRow(ctx, `SELECT t.title, e.uuid::text, t.uuid::text FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND e.id = $2`, orgID, engID).Scan(&title, &engUUID, &tpUUID); err != nil {
			return fmt.Errorf("vat watch names: %w", err)
		}
		dedupe := fmt.Sprintf("%s:%s:%s:%s", events.VATStatusChanged, engUUID, demoVATPeriod, demoVATNoticeUUID)
		payload, err := json.Marshal(map[string]any{
			"taxpayer_uuid": tpUUID, "taxpayer_title": title, "engagement_uuid": engUUID, "period": demoVATPeriod,
			"outcome": "payment", "payable": "2400.00", "carry_out": "0.00", "output_vat": "22400.00",
			"input_vat": "20000.00", "carried_in": "0.00", "due_date": "2026-11-26", "dedupe_key": dedupe,
			"previous_outcome": "no_payment", "previous_payable": "0.00", "notice_uuid": demoVATNoticeUUID,
			"notified_at": "2026-10-01T07:00:00Z",
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO vat_watch_events (organization_id, engagement_id, period, event, dedupe_key, payload, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, '2026-10-04 07:00:00+03')
			ON CONFLICT (organization_id, dedupe_key) DO NOTHING`,
			orgID, engID, demoVATPeriod, events.VATStatusChanged, dedupe, payload); err != nil {
			return fmt.Errorf("vat watch event: %w", err)
		}
		return nil
	})
}
