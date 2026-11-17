package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// seedStore keeps the demo export files: the configured object store when
// main could open it, memory otherwise (tests, no store running).
var seedStore journals.ObjectStore = storage.NewMemory()

// fallbackStore uploads to the object store and, when it is down, keeps the
// file in memory: a missing store must not stop the demo seed.
type fallbackStore struct {
	primary journals.ObjectStore
	memory  *storage.Memory
}

func (f fallbackStore) Upload(ctx context.Context, file storage.File, path string) error {
	data, err := io.ReadAll(file.Body)
	if err != nil {
		return err
	}
	file.Body = bytes.NewReader(data)
	if err := f.primary.Upload(ctx, file, path); err != nil {
		file.Body = bytes.NewReader(data)
		fmt.Printf("object store upload failed (%v); demo export file %s kept in memory\n", err, path)
		return f.memory.Upload(ctx, file, path)
	}
	return nil
}

// DemoAgentMachine is the demo office's desktop agent (machine_id).
const DemoAgentMachine = "demo-office-pc-01"

// seedJournals gives the engagement approved October 2026 vouchers, a Luca
// and a Zirve export, and their transfers to the program (#391): the Luca
// one imported, the Zirve one failed with the agent's folder error, so the
// journals page shows the history and the error screen.
//
// Export files go to the configured object store; without one they are
// kept in memory and the demo rows list them without a downloadable copy.
func seedJournals(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	svc := journals.New(pool, seedStore)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	amt := decimal.RequireFromString
	vouchers := []journals.DraftInput{
		{Date: day(5), Description: "Ekim kira gideri", Lines: []journals.LineInput{
			{Account: "770.01", Description: "Kira", Debit: amt("25000.00")},
			{Account: "320.01", Description: "Kira borcu", Credit: amt("25000.00")},
		}},
		{Date: day(14), Description: "Akaryakıt fişi 0042", Lines: []journals.LineInput{
			{Account: "770.02", Description: "Akaryakıt", Debit: amt("1041.67")},
			{Account: "191.01", Description: "İndirilecek KDV", Debit: amt("208.33")},
			{Account: "100.01", Description: "Kasa", Credit: amt("1250.00")},
		}},
	}
	var ids []uuid.UUID
	for _, v := range vouchers {
		d, err := svc.CreateDraft(ctx, scope, actor, v)
		if err != nil {
			return fmt.Errorf("draft: %w", err)
		}
		if err := svc.Approve(ctx, scope, actor, d.UUID); err != nil {
			return fmt.Errorf("approve: %w", err)
		}
		ids = append(ids, d.UUID)
	}
	luca, err := svc.Export(ctx, scope, actor, journals.ExportInput{Driver: "luca", Drafts: ids[:1]})
	if err != nil {
		return fmt.Errorf("luca export: %w", err)
	}
	zirve, err := svc.Export(ctx, scope, actor, journals.ExportInput{Driver: "zirve", Drafts: ids[1:]})
	if err != nil {
		return fmt.Errorf("zirve export: %w", err)
	}

	var agent uuid.UUID
	err = rls.System(ctx, pool, "demo-seed: desktop agent", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO desktop_agents (organization_id, machine_id, name, platform, created_by)
			VALUES ($1, $2, 'Ofis bilgisayarı', 'windows', $3)
			ON CONFLICT (organization_id, machine_id) DO UPDATE SET name = EXCLUDED.name
			RETURNING uuid`, scope.OrganizationID, DemoAgentMachine, actor).Scan(&agent)
	})
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	t, err := svc.RequestTransfer(ctx, scope, actor, luca.UUID, journals.MethodAgentFolder)
	if err != nil {
		return fmt.Errorf("luca transfer: %w", err)
	}
	if _, err := svc.ClaimTransfer(ctx, scope, t.UUID, agent); err != nil {
		return err
	}
	files := make([]journals.TransferFile, 0, len(t.Files))
	for _, f := range t.Files {
		files = append(files, journals.TransferFile{Name: f.Name, SHA256: f.SHA256, Size: f.Size})
	}
	if _, err := svc.ReportDelivery(ctx, scope, t.UUID, journals.DeliveryInput{
		AgentUUID: agent, Outcome: journals.TransferDelivered, Files: files, FolderLabel: "Luca aktarım",
	}); err != nil {
		return err
	}
	if _, err := svc.CompleteTransfer(ctx, scope, actor, t.UUID, journals.CompleteInput{Outcome: journals.TransferImported}); err != nil {
		return err
	}

	z, err := svc.RequestTransfer(ctx, scope, actor, zirve.UUID, journals.MethodAgentFolder)
	if err != nil {
		return fmt.Errorf("zirve transfer: %w", err)
	}
	if _, err := svc.ClaimTransfer(ctx, scope, z.UUID, agent); err != nil {
		return err
	}
	_, err = svc.ReportDelivery(ctx, scope, z.UUID, journals.DeliveryInput{
		AgentUUID: agent, Outcome: journals.TransferFailed, FolderLabel: "Zirve aktarım",
		ErrorCode: "FOLDER_MISSING", ErrorMessage: "Aktarım klasörü bulunamadı. Klasör taşınmış ya da ağ sürücüsü bağlı değil.",
	})
	return err
}
