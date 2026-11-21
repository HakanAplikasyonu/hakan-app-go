package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/automation"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedAutomation (#780 C, Otomasyon kuralları): the ready-made rules of the
// demo office, four of them switched on (28'i KDV, son günden 1 gün önce,
// ayın 1'i banka ekstresi, ayın 1'i geçici vergi), a few sent runs (one
// with "e-posta açılmadı, SMS gitti") and two KDV report drafts waiting for
// the office's approval. Runs only when the office has no rules yet, so a
// second run changes nothing and existing rows are never touched.
func seedAutomation(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: automation rules", func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM automation_rules WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
			return fmt.Errorf("count rules: %w", err)
		}
		if n > 0 {
			return nil
		}
		if err := automation.New(pool, nil, nil, "", nil).EnsureTemplates(ctx, orgID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE automation_rules SET active = TRUE, active_since = '2026-10-01T09:00:00+03:00'
			WHERE organization_id = $1 AND template_code IN ('kdv_28', 'kdv_son_gun_1', 'banka_ekstresi_1_5', 'gecici_ay_1')`, orgID); err != nil {
			return fmt.Errorf("activate rules: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO automation_rule_history (organization_id, rule_id, action, snapshot, created_at)
			SELECT organization_id, id, 'enabled', jsonb_build_object('name', name, 'active', TRUE), '2026-10-01T09:00:00+03:00'
			FROM automation_rules WHERE organization_id = $1 AND active`, orgID); err != nil {
			return fmt.Errorf("rule history: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, t.title FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND e.status = 'active' ORDER BY e.id LIMIT 3`, orgID)
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
		const office = "Demo Mali Müşavirlik"
		type run struct {
			code, key, period, status, subject, body, result, followup, at string
			channels                                                       []string
			eng                                                            int64
		}
		var runs []run
		for i, e := range engs {
			runs = append(runs, run{eng: e.id, code: "kdv_28", key: "cal:2026-10:d28", period: "2026-09", status: "sent",
				subject: "Eylül 2026 KDV ödemesi", channels: []string{"sms", "email"},
				body:   fmt.Sprintf("Sayın %s, Eylül 2026 dönemi KDV tahakkukunuz %s. Son ödeme günü 28 Ekim 2026. %s", e.title, []string{"18.450,00 TL", "6.120,00 TL", "2.980,00 TL"}[i], office),
				result: `{"sms":{"status":"dry_run"},"email":{"status":"dry_run"}}`, at: "2026-10-28T09:05:00+03:00"})
			runs = append(runs, run{eng: e.id, code: "banka_ekstresi_1_5", key: "cal:2026-11:d01", period: "2026-10", status: "sent",
				subject: "Ekim 2026 banka ekstreleri", channels: []string{"sms", "inapp"},
				body:   fmt.Sprintf("Sayın %s, Ekim 2026 dönemine ait banka ekstrelerinizi ayın 5'ine kadar yüklemenizi rica ederiz. %s", e.title, office),
				result: `{"sms":{"status":"dry_run"},"inapp":{"status":"sent","count":1}}`, at: "2026-11-01T09:02:00+03:00"})
			// Ödemesi olmayan mükellef "yalnız ödemesi olanlar" ile atlandı.
			status, result := "sent", `{"sms":{"status":"dry_run"}}`
			if i == 2 {
				status, result = "skipped", `{}`
			}
			runs = append(runs, run{eng: e.id, code: "kdv_son_gun_1", key: "due:kdv:2026-09", period: "2026-09", status: status,
				subject: "Eylül 2026 KDV ödemesi yarın", channels: []string{"sms"},
				body:   fmt.Sprintf("Sayın %s, Eylül 2026 dönemi KDV ödemenizin son günü yarın (28 Ekim 2026). Ödediyseniz bu mesajı dikkate almayınız. %s", e.title, office),
				result: result, at: "2026-10-27T10:01:00+03:00"})
			if i < 2 {
				runs = append(runs, run{eng: e.id, code: "kdv_raporu_29", key: "cal:2026-11:d29", period: "2026-11", status: "draft",
					subject: "Kasım 2026 KDV raporu", channels: []string{"sms", "email"},
					body:   fmt.Sprintf("Sayın %s, Kasım 2026 dönemi KDV ön raporunuz hazırlandı. Ayrıntılar ve eksik belgeler için portalı inceleyiniz. %s", e.title, office),
					result: `{}`, at: "2026-11-21T09:00:00+03:00"})
			}
		}
		if len(engs) > 0 {
			// E-posta açılmadı, 24 saat sonra SMS gitti.
			e := engs[0]
			runs = append(runs, run{eng: e.id, code: "gecici_ay_1", key: "cal:2026-11:d01", period: "2026-09", status: "sent",
				subject: "2026 3. dönem geçici vergi", channels: []string{"sms", "email"}, followup: "sent",
				body:   fmt.Sprintf("Sayın %s, 2026 3. dönem geçici vergi beyannamesinin son günü 17 Kasım 2026. Dönem belgelerinizi ayın 10'una kadar iletmenizi rica ederiz. %s", e.title, office),
				result: `{"email":{"status":"dry_run"},"sms":{"status":"dry_run"}}`, at: "2026-11-01T09:03:00+03:00"})
		}
		for _, r := range runs {
			var followAt any
			if r.followup != "" {
				followAt = "2026-11-02T09:03:00+03:00"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO automation_runs (organization_id, rule_id, engagement_id, run_key, period, status, scheduled_for,
					subject, body, channels, result, note, sms_followup_at, sms_followup_status, created_at, sent_at)
				SELECT $1, r.id, $2, $3, $4, $5, $6::timestamptz, $7, $8, $9::text[], $10::jsonb,
					CASE $5 WHEN 'draft' THEN 'Ofis onayı bekliyor.' WHEN 'skipped' THEN 'Ödemesi ya da eksiği yok.' ELSE '' END,
					$11::timestamptz, $12, $6::timestamptz, CASE WHEN $5 = 'sent' THEN $6::timestamptz END
				FROM automation_rules r WHERE r.organization_id = $1 AND r.template_code = $13
				ON CONFLICT (organization_id, rule_id, run_key) DO NOTHING`,
				orgID, r.eng, r.key, r.period, r.status, r.at, r.subject, r.body, r.channels, r.result, followAt, r.followup, r.code); err != nil {
				return fmt.Errorf("automation run: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE automation_rules r SET send_count = s.n, last_run_at = s.last
			FROM (SELECT rule_id, count(*) AS n, max(sent_at) AS last FROM automation_runs
			      WHERE organization_id = $1 AND status = 'sent' GROUP BY rule_id) s
			WHERE r.organization_id = $1 AND r.id = s.rule_id`, orgID); err != nil {
			return fmt.Errorf("send counts: %w", err)
		}
		return nil
	})
}
