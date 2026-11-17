package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedCopilot (issue #392) leaves the office owner's command history in the
// co-pilot: the October report was sent from the command bar (it matches
// the manual October delivery of seedReport) and a plan for the October
// close waits for approval, so "Son komutlarınız" has rows in the pilot
// demo. The next-work list itself needs no seed: it is computed from the
// bank, review, journal and report rows the other seeders write.
func seedCopilot(ctx context.Context, pool *pgxpool.Pool, orgID, engID int64) error {
	owner := demo.Members[0].Email
	return rls.System(ctx, pool, "demo-seed: co-pilot plans", func(ctx context.Context, tx pgx.Tx) error {
		var engUUID, title string
		if err := tx.QueryRow(ctx, `SELECT e.uuid::text, t.title FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND e.id = $2`, orgID, engID).Scan(&engUUID, &title); err != nil {
			return fmt.Errorf("engagement: %w", err)
		}
		step := func(i int, action, status, finished string) string {
			s := fmt.Sprintf(`{"index":%d,"action":%q,"engagement_uuid":%q,"taxpayer_title":%q,"period":"2026-10","status":%q`,
				i, action, engUUID, title, status)
			if finished != "" {
				s += fmt.Sprintf(`,"finished_at":%q`, finished)
			}
			return s + "}"
		}
		for _, p := range []struct {
			command, status, steps, notes, created string
		}{
			{"Atlas Gıda ekim raporunu gönder", "done",
				"[" + step(0, "report_send", "done", "2026-11-03T10:30:00Z") + "]", "[]", "2026-11-03T10:29:40Z"},
			{"Atlas Gıda ekim kapanışı", "proposed",
				"[" + step(0, "bank_approve", "pending", "") + "," + step(1, "bank_drafts", "pending", "") + "," +
					step(2, "journals_approve", "pending", "") + "," + step(3, "export_transfer", "pending", "") + "]",
				"[]", "2026-11-17T08:15:00Z"},
		} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO copilot_plans (organization_id, created_by, command, period, status, planner, steps, notes, created_at, updated_at)
				SELECT $1, u.id, $2, '2026-10', $3, 'rules', $4::jsonb, $5::jsonb, $6::timestamptz, $6::timestamptz
				FROM users u WHERE u.email = $7`,
				orgID, p.command, p.status, p.steps, p.notes, p.created, owner); err != nil {
				return fmt.Errorf("plan %q: %w", p.command, err)
			}
		}
		return nil
	})
}
