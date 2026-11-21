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

-- name: ListJournalAccountNames :many
-- Fiş ekranında hesap adları: önce etkin hesap planı (son yüklenen hedef),
-- yoksa cari hafızası. Planda adı olmayan kod listede yoktur.
SELECT DISTINCT ON (code) code::text AS code, name::text AS name FROM (
    SELECT a.code, a.name, 0 AS rank, a.id FROM account_plan_accounts a
    WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
      AND a.code = ANY(@codes::text[]) AND a.name <> ''
    UNION ALL
    SELECT c.account_code, c.account_name, 1 AS rank, c.id FROM counterparty_accounts c
    WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
      AND c.account_code = ANY(@codes::text[]) AND c.account_name <> ''
) n
ORDER BY code, rank, id DESC;

-- name: JournalDraftTotals :one
SELECT COALESCE(SUM(debit), 0)::numeric AS debit, COALESCE(SUM(credit), 0)::numeric AS credit, COUNT(*)::int AS lines
FROM journal_draft_lines
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND journal_draft_id = @journal_draft_id;

-- name: ApproveJournalDraft :execrows
UPDATE journal_drafts SET status = 'approved', approved_by = @approved_by, approved_at = NOW(),
    approval_warnings = @approval_warnings::text[], approval_note = sqlc.narg(approval_note)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id AND status = 'draft';

-- name: RevertJournalDraftApproval :execrows
-- Onayı geri al (#566): yalnız onaylı (aktarılmamış) fiş taslağa döner;
-- kim, ne zaman, neden fişte kalır.
UPDATE journal_drafts
SET status = 'draft', approved_by = NULL, approved_at = NULL,
    approval_reverted_by = @reverted_by, approval_reverted_at = NOW(), approval_revert_reason = @reason
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id AND status = 'approved';

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

-- name: JournalDraftIssues :many
-- Taslak fişin onayını engelleyen sorunlar (#566): borç-alacak tutmuyor ya
-- da satır yok, hesap planında olmayan kod, ana (alt hesabı olan) hesaba
-- kayıt, kilitli dönem. Planı yüklenmemiş mükellefte kod denetlenmez.
WITH d AS (
    SELECT jd.id, jd.period FROM journal_drafts jd
    WHERE jd.organization_id = @organization_id AND jd.engagement_id = @engagement_id
      AND jd.id = ANY(@draft_ids::bigint[])
), has_plan AS (
    SELECT EXISTS (
        SELECT 1 FROM account_plan_accounts a
        WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
    ) AS yes
)
SELECT d.id AS draft_id, 'unbalanced'::text AS code, 0::int AS line_no, ''::text AS account_code,
       (COALESCE(SUM(l.debit), 0) - COALESCE(SUM(l.credit), 0))::numeric AS difference
FROM d
LEFT JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = d.id
GROUP BY d.id
HAVING COUNT(l.id) = 0 OR COALESCE(SUM(l.debit), 0) <> COALESCE(SUM(l.credit), 0)
UNION ALL
SELECT d.id, 'unknown_account', l.line_no, l.account_code, 0::numeric
FROM d
JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = d.id
WHERE (SELECT yes FROM has_plan)
  AND NOT EXISTS (
      SELECT 1 FROM account_plan_accounts a
      WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id AND a.code = l.account_code
  )
UNION ALL
SELECT d.id, 'parent_account', l.line_no, l.account_code, 0::numeric
FROM d
JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = d.id
JOIN account_plan_accounts a
  ON a.organization_id = @organization_id AND a.engagement_id = @engagement_id AND a.code = l.account_code AND NOT a.is_leaf
UNION ALL
SELECT d.id, 'period_locked', 0, '', 0::numeric
FROM d
JOIN period_locks pl
  ON pl.organization_id = @organization_id AND pl.engagement_id = @engagement_id AND pl.period = d.period
UNION ALL
-- Eksik belge (uyarı): fişteki bir banka hareketi hiçbir belgeye (fatura,
-- fiş satırı) eşleşmemiş ve satırında belge numarası yok.
SELECT DISTINCT d.id, 'missing_document', 0, '', 0::numeric
FROM d
JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id
 AND l.journal_draft_id = d.id AND l.source_kind = 'bank_transaction' AND COALESCE(l.document_no, '') = ''
JOIN engagements e ON e.organization_id = @organization_id AND e.id = @engagement_id
WHERE NOT EXISTS (
    SELECT 1 FROM bank_transaction_matches m
    WHERE m.taxpayer_id = e.taxpayer_id AND m.bank_transaction_id::text = l.source_id
  )
UNION ALL
-- Mükerrer olabilir (uyarı): aynı kaynak belge (fatura, fiş, banka hareketi)
-- başka bir fişte de var.
SELECT DISTINCT d.id, 'possible_duplicate', 0, '', 0::numeric
FROM d
JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = d.id
 AND l.source_kind IN ('invoice', 'invoice_item', 'receipt', 'bank_transaction')
JOIN journal_draft_lines o
  ON o.organization_id = @organization_id AND o.engagement_id = @engagement_id
 AND o.source_kind = l.source_kind AND o.source_id = l.source_id AND o.journal_draft_id <> d.id
ORDER BY 1, 3;

-- name: SetJournalDraftLineAccount :execrows
-- Kontrol ekranında hatalı hesap kodu düzeltilir (#566); yalnız taslakta.
UPDATE journal_draft_lines l SET account_code = @account_code
FROM journal_drafts d
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id AND d.id = @draft_id
  AND d.status = 'draft'
  AND l.organization_id = d.organization_id AND l.engagement_id = d.engagement_id
  AND l.journal_draft_id = d.id AND l.line_no = @line_no;

-- name: SetJournalDraftLineAmounts :execrows
-- Kontrol ekranında borç-alacak farkı düzeltilir (#566): yalnız taslakta ve
-- kaynağı olmayan (elle / Excel'den girilen) satırda; banka, fatura, bordro
-- satırının tutarı kaynağından gelir.
UPDATE journal_draft_lines l SET debit = @debit, credit = @credit
FROM journal_drafts d
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id AND d.id = @draft_id
  AND d.status = 'draft'
  AND l.organization_id = d.organization_id AND l.engagement_id = d.engagement_id
  AND l.journal_draft_id = d.id AND l.line_no = @line_no
  AND l.source_kind IS NULL;

-- name: JournalDraftExports :many
-- Fişin geri alınmamış çıktısı ve o çıktının son aktarım durumu (#574).
SELECT i.journal_draft_id, b.uuid AS batch_uuid, b.driver, b.created_at AS exported_at,
       COALESCE(t.status, '')::text AS transfer_status, COALESCE(t.error_message, '')::text AS transfer_error
FROM export_batch_items i
JOIN export_batches b ON b.id = i.export_batch_id AND b.engagement_id = i.engagement_id
LEFT JOIN LATERAL (
    SELECT x.status, x.error_message FROM accounting_transfers x
    WHERE x.export_batch_id = b.id AND x.engagement_id = b.engagement_id
    ORDER BY x.created_at DESC, x.id DESC LIMIT 1
) t ON TRUE
WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id
  AND i.journal_draft_id = ANY(@ids::bigint[]) AND i.reversed_at IS NULL;

-- name: AddJournalDraftManualLink :execrows
-- Fişe elle bağlanan kayıt (#710): fatura ya da banka hareketi, fişin
-- origin.manual_links dizisinde {kind, id}. Satırlara, tutarlara ve aktarıma
-- dokunmaz; yalnız ilişkili kayıtlarda görünür. Aynı bağ ikinci kez eklenmez.
UPDATE journal_drafts SET origin = jsonb_set(origin, '{manual_links}',
    (CASE WHEN jsonb_typeof(origin -> 'manual_links') = 'array' THEN origin -> 'manual_links' ELSE '[]'::jsonb END)
    || jsonb_build_array(jsonb_build_object('kind', @kind::text, 'id', @target_id::bigint)))
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @draft_id
  AND NOT (CASE WHEN jsonb_typeof(origin -> 'manual_links') = 'array' THEN origin -> 'manual_links' ELSE '[]'::jsonb END)
      @> jsonb_build_array(jsonb_build_object('kind', @kind::text, 'id', @target_id::bigint));

-- name: RemoveJournalDraftManualLink :execrows
-- Elle bağlanan kaydın bağı kaldırılır; bağ yoksa satır etkilenmez.
UPDATE journal_drafts SET origin = jsonb_set(origin, '{manual_links}', COALESCE((
    SELECT jsonb_agg(x) FROM jsonb_array_elements(origin -> 'manual_links') x
    WHERE x <> jsonb_build_object('kind', @kind::text, 'id', @target_id::bigint)), '[]'::jsonb))
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @draft_id
  AND jsonb_typeof(origin -> 'manual_links') = 'array'
  AND origin -> 'manual_links' @> jsonb_build_array(jsonb_build_object('kind', @kind::text, 'id', @target_id::bigint));

-- name: ListEngagementCustomers :many
-- Fişe bağlanabilecek cari kartları (#710): mükellefin müşteri ve
-- tedarikçileri, unvana göre; q unvanda aranır (VKN ile arama yok).
SELECT c.uuid, c.name, c.type FROM customers c
WHERE c.taxpayer_id = @taxpayer_id AND c.type <> 'employee' AND c.deleted_at_source IS NULL
  AND (@q::text = '' OR c.name ILIKE '%' || @q::text || '%')
ORDER BY c.name, c.id
LIMIT @lim;
