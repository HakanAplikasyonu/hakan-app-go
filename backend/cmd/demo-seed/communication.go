package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedCommunication (#780 A, İletişim merkezi): the demo office's NetGSM
// account in test mode (no password: nothing can leave the demo) and a few
// SMS records, so Ayarlar > SMS shows its history. Only new rows; a second
// run changes nothing.
func seedCommunication(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: communication center", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO office_sms_settings (organization_id, enabled, test_mode, usercode, msgheader, encoding, appname)
			VALUES ($1, TRUE, TRUE, '8500000000', 'OFISDEMO', 'TR', 'Hakan')
			ON CONFLICT (organization_id) DO NOTHING`, orgID); err != nil {
			return fmt.Errorf("sms settings: %w", err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM sms_messages WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
			return fmt.Errorf("count sms: %w", err)
		}
		if n > 0 {
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, t.title FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 ORDER BY e.id LIMIT 3`, orgID)
		if err != nil {
			return fmt.Errorf("engagements: %w", err)
		}
		type eng struct {
			id    int64
			title string
		}
		var engs []eng
		for rows.Next() {
			var e eng
			if err := rows.Scan(&e.id, &e.title); err != nil {
				rows.Close()
				return err
			}
			engs = append(engs, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		samples := []struct {
			phone, body, status, code, errMsg, rule, at string
		}{
			{"5320000001", "Sayın %s, Ekim 2026 KDV beyannameniz verildi. Ödenecek tutar 18.450 TL, son ödeme günü 28 Kasım.\n", "dry_run", "", "Deneme modu, gönderilmedi.", "kdv_tahakkuk", "2026-11-18T09:05:00+03:00"},
			{"5320000002", "Sayın %s, Kasım ayı banka ekstrelerinizi 5 Aralık'a kadar yüklemenizi rica ederiz.\n", "dry_run", "", "Deneme modu, gönderilmedi.", "eksik_ekstre", "2026-11-19T10:30:00+03:00"},
			{"2120000003", "Sayın %s, Ekim 2026 muhtasar beyannameniz verildi.\n", "failed", "phone", "Numara cep telefonu değil ya da hatalı.", "muhtasar_tahakkuk", "2026-11-20T11:15:00+03:00"},
		}
		for i, s := range samples {
			if i >= len(engs) {
				break
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, error_code, error_message, source_rule, created_at)
				VALUES ($1, $2, $3, $4, 'OFISDEMO', $5, $6, $7, $8, $9::timestamptz)`,
				orgID, engs[i].id, s.phone, fmt.Sprintf(s.body, engs[i].title), s.status, s.code, s.errMsg, s.rule, s.at); err != nil {
				return fmt.Errorf("sms sample: %w", err)
			}
		}
		return nil
	})
}
