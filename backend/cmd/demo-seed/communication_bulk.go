package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedCommunicationBulk (#780 B, İletişim sayfası): two taxpayer groups and
// one finished bulk SMS in test mode (its records carry the bulk id), so
// "Toplu gönderim" and "Gruplar" open with something to show. Only new rows;
// a second run changes nothing.
func seedCommunicationBulk(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: communication bulk sends", func(ctx context.Context, tx pgx.Tx) error {
		var owner *int64
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = 'sahip@demo.hakan.local'`).Scan(&id); err == nil {
			owner = &id
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, t.title FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND e.status = 'active' ORDER BY e.id LIMIT 6`, orgID)
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
		if len(engs) == 0 {
			return nil
		}
		groups := []struct {
			name, desc string
			from, to   int
		}{
			{"KDV'ye hassas mükellefler", "KDV iadesi ya da devreden KDV'si yüksek, ay sonunda yakından takip edilenler.", 0, 3},
			{"Bordrolu işverenler", "Muhtasar ve SGK bildirimi olan, personel bilgisi her ay istenen mükellefler.", 1, 6},
		}
		for _, g := range groups {
			var gid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO taxpayer_groups (organization_id, name, description, created_by)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (organization_id, lower(name)) DO NOTHING
				RETURNING id`, orgID, g.name, g.desc, owner).Scan(&gid)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already seeded
			}
			if err != nil {
				return fmt.Errorf("group %s: %w", g.name, err)
			}
			for i := g.from; i < g.to && i < len(engs); i++ {
				if _, err := tx.Exec(ctx, `
					INSERT INTO taxpayer_group_members (organization_id, group_id, engagement_id) VALUES ($1, $2, $3)
					ON CONFLICT DO NOTHING`, orgID, gid, engs[i].id); err != nil {
					return fmt.Errorf("group member: %w", err)
				}
			}
		}

		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bulk_sends WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
			return fmt.Errorf("count bulk sends: %w", err)
		}
		if n > 0 || len(engs) < 3 {
			return nil
		}
		// Bitmiş bir toplu SMS: iki mükellefe gitti (deneme modu), biri sabit
		// hat olduğu için başarısız; bir tekrar elendi, bir mükellefin
		// numarası yok.
		body := "Sayın :mukellef, :donem dönemi belgelerinizi ayın 5'ine kadar iletmenizi rica ederiz. Demo SMMM"
		ids := []int64{engs[0].id, engs[1].id, engs[2].id}
		skipped := "[]"
		if len(engs) > 3 {
			ids = append(ids, engs[3].id)
			skipped = fmt.Sprintf(`[{"engagement_uuid":"","title":%q,"reason":"no_contact"}]`, engs[3].title)
		}
		var bulkUUID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO bulk_sends (organization_id, channel, body, target_kind, target_label, engagement_ids, status,
				total, processed, sent, failed, duplicates, skipped, created_by, created_at, updated_at, finished_at)
			VALUES ($1, 'sms', $2, 'group', 'Grup: KDV''ye hassas mükellefler', $3, 'done', 3, 3, 2, 1, 1, $4::jsonb, $5,
				'2026-11-17T16:00:00+03:00', '2026-11-17T16:00:09+03:00', '2026-11-17T16:00:09+03:00')
			RETURNING uuid::text`, orgID, body, ids, skipped, owner).Scan(&bulkUUID); err != nil {
			return fmt.Errorf("bulk send: %w", err)
		}
		samples := []struct {
			phone, status, code, errMsg string
		}{
			{"5320000011", "dry_run", "", "Deneme modu, gönderilmedi."},
			{"5320000012", "dry_run", "", "Deneme modu, gönderilmedi."},
			{"2120000013", "failed", "phone", "Numara cep telefonu değil ya da hatalı."},
		}
		for i, s := range samples {
			text := fmt.Sprintf("Sayın %s, Ekim 2026 dönemi belgelerinizi ayın 5'ine kadar iletmenizi rica ederiz. Demo SMMM\n", engs[i].title)
			if _, err := tx.Exec(ctx, `
				INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, error_code, error_message,
					source_rule, source_ref, sent_by, bulk_id, created_at)
				VALUES ($1, $2, $3, $4, 'OFISDEMO', $5, $6, $7, 'bulk', $8, $9, $10::uuid, '2026-11-17T16:00:03+03:00')`,
				orgID, engs[i].id, s.phone, text, s.status, s.code, s.errMsg, bulkUUID, owner, bulkUUID); err != nil {
				return fmt.Errorf("bulk sms: %w", err)
			}
		}
		return nil
	})
}
