-- name: GetExportSettings :one
SELECT * FROM export_settings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;

-- name: UpsertExportSettings :one
-- luca_pull_integrators NULL: ofis ayarı geçerli (#466).
INSERT INTO export_settings (organization_id, engagement_id, invoices_reach_luca_directly, block_on_unknown_account, luca_pull_integrators, updated_by)
VALUES (@organization_id, @engagement_id, @invoices_reach_luca_directly, @block_on_unknown_account, sqlc.narg(luca_pull_integrators)::text[], sqlc.narg(updated_by))
ON CONFLICT (engagement_id) DO UPDATE SET
    invoices_reach_luca_directly = EXCLUDED.invoices_reach_luca_directly,
    block_on_unknown_account = EXCLUDED.block_on_unknown_account,
    luca_pull_integrators = EXCLUDED.luca_pull_integrators,
    updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: NextJournalEntryNo :one
SELECT (COALESCE(MAX(entry_no), 0) + 1)::int AS next_no
FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period;

-- name: InsertJournalDraft :one
INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_type, entry_date, description, document_type, origin, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, @entry_no, @entry_type, @entry_date, @description, sqlc.narg(document_type), @origin, sqlc.narg(created_by))
RETURNING *;

-- name: InsertJournalDraftLine :exec
INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, description, debit, credit,
    quantity, currency, exchange_rate, foreign_amount, document_no, document_date, source_kind, source_id, confidence)
VALUES (@organization_id, @engagement_id, @journal_draft_id, @line_no, @account_code, @description, @debit, @credit,
    sqlc.narg(quantity), sqlc.narg(currency), sqlc.narg(exchange_rate), sqlc.narg(foreign_amount), sqlc.narg(document_no),
    sqlc.narg(document_date), sqlc.narg(source_kind), sqlc.narg(source_id), sqlc.narg(confidence));

-- name: GetJournalDraft :one
SELECT * FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListJournalDraftLines :many
SELECT * FROM journal_draft_lines
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND journal_draft_id = @journal_draft_id
ORDER BY line_no;

-- name: JournalDraftTotals :one
SELECT COALESCE(SUM(debit), 0)::numeric AS debit, COALESCE(SUM(credit), 0)::numeric AS credit, COUNT(*)::int AS lines
FROM journal_draft_lines
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND journal_draft_id = @journal_draft_id;

-- name: ApproveJournalDraft :execrows
UPDATE journal_drafts SET status = 'approved', approved_by = @approved_by, approved_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id AND status = 'draft';

-- name: ListExportableJournalDrafts :many
-- Onaylı ve geri alınmamış bir batch'te olmayan fişler.
SELECT d.* FROM journal_drafts d
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND d.status = 'approved'
  AND (sqlc.narg(period)::text IS NULL OR d.period = sqlc.narg(period))
ORDER BY d.period, d.entry_date, d.entry_no;

-- name: ListJournalDraftsByUUID :many
SELECT * FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = ANY(@uuids::uuid[])
ORDER BY period, entry_date, entry_no;

-- name: InsertExportBatch :one
INSERT INTO export_batches (uuid, organization_id, engagement_id, taxpayer_id, driver, template_version, template_verified, period, files, issues, created_by)
VALUES (@uuid, @organization_id, @engagement_id, @taxpayer_id, @driver, @template_version, @template_verified, sqlc.narg(period), @files, @issues, @created_by)
RETURNING *;

-- name: InsertExportBatchItem :exec
INSERT INTO export_batch_items (organization_id, engagement_id, export_batch_id, journal_draft_id)
VALUES (@organization_id, @engagement_id, @export_batch_id, @journal_draft_id);

-- name: MarkJournalDraftExported :execrows
UPDATE journal_drafts SET status = 'exported'
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id AND status = 'approved';

-- name: GetExportBatch :one
SELECT * FROM export_batches
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListExportBatchItems :many
SELECT i.*, d.uuid AS journal_draft_uuid, d.entry_no, d.period
FROM export_batch_items i
JOIN journal_drafts d ON d.id = i.journal_draft_id AND d.engagement_id = i.engagement_id
WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id AND i.export_batch_id = @export_batch_id
ORDER BY d.period, d.entry_no;

-- name: ReverseExportBatchItem :execrows
-- Geri alma: kim, ne zaman, neden. Fiş tekrar onaylı olur ve yeniden çıktıya alınabilir.
WITH rev AS (
    UPDATE export_batch_items SET reversed_at = NOW(), reversed_by = @reversed_by, reversal_reason = @reason
    WHERE export_batch_items.organization_id = @organization_id AND export_batch_items.engagement_id = @engagement_id
      AND export_batch_items.journal_draft_id = @journal_draft_id AND export_batch_items.reversed_at IS NULL
    RETURNING export_batch_items.journal_draft_id
)
UPDATE journal_drafts SET status = 'approved'
WHERE journal_drafts.organization_id = @organization_id AND journal_drafts.engagement_id = @engagement_id
  AND journal_drafts.id IN (SELECT journal_draft_id FROM rev);
