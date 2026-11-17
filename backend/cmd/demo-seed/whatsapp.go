package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedWhatsApp (issue #388) remembers the demo taxpayer's WhatsApp chat and
// adds two saves from the extension, so the panel's WhatsApp page has rows
// in the pilot demo. The seed has no object store: the saves carry no file
// (has_file=false); a real save from the extension stores one.
func seedWhatsApp(ctx context.Context, pool *pgxpool.Pool, orgID, engID int64) error {
	owner := demo.Members[0].Email
	return rls.System(ctx, pool, "demo-seed: whatsapp extension rows", func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO whatsapp_chat_links (organization_id, engagement_id, chat_key, label, created_by, updated_by)
			SELECT $1, $2, $3, $4, u.id, u.id FROM users u WHERE u.email = $5
			ON CONFLICT (organization_id, chat_key) DO NOTHING`,
			orgID, engID, demo.WhatsAppChatKey, demo.WhatsAppChatLabel, owner); err != nil {
			return fmt.Errorf("chat link: %w", err)
		}
		for _, c := range []struct{ target, name, ct, sha string }{
			{"document", "kira-sozlesmesi-ekim.pdf", "application/pdf", "d0c0000000000000000000000000000000000000000000000000000000000388"},
			{"expense", "akaryakit-fisi.jpg", "image/jpeg", "e0e0000000000000000000000000000000000000000000000000000000000388"},
		} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO whatsapp_captures (organization_id, engagement_id, taxpayer_id, target, chat_key, file_name,
				                               content_type, size_bytes, sha256, created_by)
				SELECT $1, $2, e.taxpayer_id, $3, $4, $5, $6, 182044, $7, u.id
				FROM engagements e, users u WHERE e.id = $2 AND u.email = $8
				ON CONFLICT (organization_id, engagement_id, target, sha256) DO NOTHING`,
				orgID, engID, c.target, demo.WhatsAppChatKey, c.name, c.ct, c.sha, owner); err != nil {
				return fmt.Errorf("capture %s: %w", c.name, err)
			}
		}
		return nil
	})
}
