-- Yabancı hizmet faturasından fiş taslağı (#780 KDV2 B).

-- name: ListForeignInvoiceJournals :many
-- Bir dönemin faturalarına bağlı fişler (liste ekranında fiş durumu).
SELECT l.foreign_invoice_id, d.uuid AS journal_uuid, d.entry_no, d.period AS journal_period, d.status AS journal_status
FROM foreign_invoice_journal_links l
JOIN foreign_service_invoices f ON f.id = l.foreign_invoice_id AND f.engagement_id = l.engagement_id
JOIN journal_drafts d ON d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND f.period = @period
LIMIT 500;

-- name: GetForeignInvoiceJournal :one
SELECT l.journal_draft_id, d.uuid AS journal_uuid, d.entry_no, d.period AS journal_period, d.status AS journal_status,
       EXISTS (SELECT 1 FROM export_batch_items i WHERE i.engagement_id = l.engagement_id AND i.journal_draft_id = l.journal_draft_id)::boolean AS ever_exported
FROM foreign_invoice_journal_links l
JOIN journal_drafts d ON d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.foreign_invoice_id = @foreign_invoice_id;

-- name: InsertForeignInvoiceJournalLink :execrows
-- Bir iş bir yabancı faturadan tek fiş üretir; ikinci istek 0 satır döner.
INSERT INTO foreign_invoice_journal_links (organization_id, engagement_id, foreign_invoice_id, journal_draft_id, created_by)
VALUES (@organization_id, @engagement_id, @foreign_invoice_id, @journal_draft_id, sqlc.narg(created_by)::bigint)
ON CONFLICT (engagement_id, foreign_invoice_id) DO NOTHING;

-- name: DeleteForeignInvoiceJournalDraft :execrows
-- Faturanın fişini siler: yalnız taslak durumundaki, hiç aktarılmamış fiş.
-- Satırlar ve bağ fişle birlikte gider (ON DELETE CASCADE).
DELETE FROM journal_drafts d
USING foreign_invoice_journal_links l
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.foreign_invoice_id = @foreign_invoice_id
  AND d.organization_id = l.organization_id AND d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
  AND d.status = 'draft'
  AND NOT EXISTS (SELECT 1 FROM export_batch_items i WHERE i.engagement_id = d.engagement_id AND i.journal_draft_id = d.id)
  AND NOT EXISTS (SELECT 1 FROM export_batch_documents b WHERE b.engagement_id = d.engagement_id AND b.journal_draft_id = d.id);

-- name: UpsertForeignInvoiceAccounts :exec
-- Fiş hesap kodları; boş (NULL) kod hazır varsayılanı kullanır.
INSERT INTO foreign_invoice_settings (
    organization_id, kdv2_account, stopaj_account, deductible_vat_account, counter_account,
    rounding_income_account, rounding_expense_account, updated_by, updated_at
) VALUES (
    @organization_id, sqlc.narg(kdv2_account)::text, sqlc.narg(stopaj_account)::text, sqlc.narg(deductible_vat_account)::text,
    sqlc.narg(counter_account)::text, sqlc.narg(rounding_income_account)::text, sqlc.narg(rounding_expense_account)::text,
    sqlc.narg(updated_by)::bigint, NOW()
)
ON CONFLICT (organization_id) DO UPDATE SET
    kdv2_account = EXCLUDED.kdv2_account, stopaj_account = EXCLUDED.stopaj_account,
    deductible_vat_account = EXCLUDED.deductible_vat_account, counter_account = EXCLUDED.counter_account,
    rounding_income_account = EXCLUDED.rounding_income_account, rounding_expense_account = EXCLUDED.rounding_expense_account,
    updated_by = EXCLUDED.updated_by, updated_at = NOW();

-- name: SetForeignServiceKindExpenseAccount :exec
-- İşlem türünün gider hesabı; tür satırı yoksa hazır varsayılanlarla açılır.
INSERT INTO foreign_service_kind_settings (
    organization_id, service_kind, treatment, stopaj_rate, gross_up, vat_rate, enabled, expense_account, updated_by, updated_at
) VALUES (
    @organization_id, @service_kind, @treatment, @stopaj_rate, @gross_up, @vat_rate, TRUE,
    sqlc.narg(expense_account)::text, sqlc.narg(updated_by)::bigint, NOW()
)
ON CONFLICT (organization_id, service_kind) DO UPDATE SET
    expense_account = EXCLUDED.expense_account, updated_by = EXCLUDED.updated_by, updated_at = NOW();
