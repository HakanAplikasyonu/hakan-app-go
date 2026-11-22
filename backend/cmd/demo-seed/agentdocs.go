package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Ajandan gelen dosyalar (#780 e): Yerel ajanlar sayfasındaki "Ajandan
// gelen dosyalar" listesi boş görünmesin diye ofis bilgisayarının izlediği
// klasörlerden gelmiş, mükellefi anlaşılamamış iki dosya: Luca bordro
// dökümü (dönem ve VKN yok) ve yabancı fatura PDF'i. Dosyalar depoya da
// yazılır; "Eşleştir" gerçekten içe aktarır. Tekrar çalıştırmada çoğalmaz.
var demoAgentDocs = []struct {
	folder, name, kind, contentType, reason, body string
}{
	{
		folder: `C:\Users\Ofis\Desktop\Luca\Bordro`, name: "Luca_bordro_dokumu.csv", kind: "payroll", contentType: "text/csv",
		reason: "Mükellef dosyadan ya da klasör adından bulunamadı; mükellefi seçin.",
		body: "LUCA BORDRO DÖKÜMÜ\n\n" + demoPayrollHeader +
			"Deniz Kaya;10000000146;3400999;Muhasebe;2025-03-01;30;40.000,00;5.600,00;400,00;3.315,70;2.034,30;197,38;106,22;31.859,48;8.200,00;800,00;2.000,00\n",
	},
	{
		folder: `C:\Users\Ofis\Desktop\Yabancı faturalar`, name: "AWS_fatura.pdf", kind: "foreign_invoice", contentType: "application/pdf",
		reason: "Mükellef dosyadan ya da klasör adından bulunamadı; mükellefi seçin.",
		body:   "%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n",
	},
}

// seedAgentDocuments writes the two waiting files of the demo office.
func seedAgentDocuments(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: agent documents", func(ctx context.Context, tx pgx.Tx) error {
		var agentID int64
		if err := tx.QueryRow(ctx, `INSERT INTO desktop_agents (organization_id, machine_id, name, platform)
			VALUES ($1, $2, 'Ofis bilgisayarı', 'windows')
			ON CONFLICT (organization_id, machine_id) DO UPDATE SET name = desktop_agents.name
			RETURNING id`, orgID, DemoAgentMachine).Scan(&agentID); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
		for _, d := range demoAgentDocs {
			sum := sha256.Sum256([]byte(d.body))
			sha := hex.EncodeToString(sum[:])
			key := fmt.Sprintf("agent-documents/%d/%s%s", orgID, sha, path.Ext(d.name))
			if seedStore != nil {
				if err := seedStore.Upload(ctx, storage.File{
					Body: bytes.NewReader([]byte(d.body)), Size: int64(len(d.body)), ContentType: d.contentType, Filename: d.name,
				}, key); err != nil {
					return fmt.Errorf("agent document %s: %w", d.name, err)
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO agent_documents (organization_id, agent_id, folder, file_name, sha256,
					size_bytes, content_type, storage_key, kind, status, reason, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'needs_match', $10, NOW() - interval '2 hours')
				ON CONFLICT (organization_id, sha256) DO NOTHING`,
				orgID, agentID, d.folder, d.name, sha, len(d.body), d.contentType, key, d.kind, d.reason); err != nil {
				return fmt.Errorf("agent document %s: %w", d.name, err)
			}
		}
		return nil
	})
}
