-- Co-pilot: sıradaki iş akışı ve onaylı aksiyon planları (issue #392).

-- tenant-lint: org-wide sıradaki iş listesi; read_all yoksa engagement_assignments ile süzülür
-- name: ListCopilotEngagements :many
SELECT e.id, e.uuid, e.taxpayer_id, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id AS taxpayer_tax_id
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL AND e.status = 'active'
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(search)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(search)::text || '%'
       OR t.tax_id = sqlc.narg(search)::text)
ORDER BY t.title, e.id
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide sıradaki iş listesi sayımı; ListCopilotEngagements ile aynı süzgeç
-- name: CountCopilotEngagements :one
SELECT count(*)::bigint
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL AND e.status = 'active'
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(search)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(search)::text || '%'
       OR t.tax_id = sqlc.narg(search)::text);

-- name: CopilotDocumentTaskCounts :many
-- Dönemin açık (istenmiş, henüz eşleşmemiş) belge talepleri.
SELECT engagement_id,
       count(*) FILTER (WHERE status = 'open')::bigint AS open,
       count(*) FILTER (WHERE status = 'submitted')::bigint AS submitted
FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND period = @period AND status IN ('open', 'submitted')
GROUP BY engagement_id;

-- name: CopilotBankCounts :many
-- Dönemin banka hareketleri, banka eşleştirme ekranındaki durumlarla
-- (CountBankPostings): önerisiz, önerili, onaylı (fişe girmemiş), fişte.
SELECT e.id AS engagement_id,
       count(t.id)::bigint AS total,
       count(t.id) FILTER (WHERE p.id IS NULL)::bigint AS unassigned,
       count(t.id) FILTER (WHERE p.status = 'suggested')::bigint AS suggested,
       count(t.id) FILTER (WHERE p.status = 'approved' AND p.journal_draft_uuid IS NULL)::bigint AS approved,
       count(t.id) FILTER (WHERE p.journal_draft_uuid IS NOT NULL)::bigint AS drafted
FROM engagements e
JOIN bank_transactions t ON t.taxpayer_id = e.taxpayer_id AND NOT t.is_cancelled AND t.period = @period
LEFT JOIN bank_transaction_postings p
       ON p.organization_id = e.organization_id AND p.engagement_id = e.id AND p.bank_transaction_id = t.id
WHERE e.organization_id = @organization_id AND e.id = ANY(@engagement_ids::bigint[])
GROUP BY e.id;

-- name: CopilotSuggestionCounts :many
-- Dönemin bekleyen inceleme önerileri; banka hesabı önerileri banka adımında
-- sayılır.
SELECT engagement_id, count(*)::bigint AS pending
FROM suggestions
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND status = 'pending' AND period = @period AND kind <> 'bank_account'
GROUP BY engagement_id;

-- name: CopilotJournalCounts :many
SELECT engagement_id,
       count(*) FILTER (WHERE status = 'draft')::bigint AS draft,
       count(*) FILTER (WHERE status = 'approved')::bigint AS approved,
       count(*) FILTER (WHERE status = 'exported')::bigint AS exported
FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND period = @period
GROUP BY engagement_id;

-- name: CopilotTransferCounts :many
-- Dönemin çıktılarının muhasebe programına aktarımları: her çıktının son
-- aktarımı sayılır. downloaded: Luca / Zirve çıktısı yalnız indirildi
-- (aktarım görevi yok ya da vazgeçildi) ve geri alınmamış fişi var; ofis
-- dosyayı programa yükleyince "Luca'ya yükledim" ile işaretler (#780).
SELECT b.engagement_id,
       count(*) FILTER (WHERE l.status IN ('queued', 'claimed', 'delivered'))::bigint AS open,
       count(*) FILTER (WHERE l.status = 'imported')::bigint AS imported,
       count(*) FILTER (WHERE l.status = 'failed')::bigint AS failed,
       count(*) FILTER (WHERE (l.status IS NULL OR l.status = 'cancelled') AND b.driver IN ('luca', 'zirve')
                          AND EXISTS (
                              SELECT 1 FROM export_batch_items i
                              WHERE i.organization_id = @organization_id AND i.engagement_id = b.engagement_id
                                AND i.export_batch_id = b.id AND i.reversed_at IS NULL))::bigint AS downloaded
FROM export_batches b
LEFT JOIN LATERAL (
    SELECT a.status
    FROM accounting_transfers a
    WHERE a.organization_id = @organization_id AND a.engagement_id = b.engagement_id AND a.export_batch_id = b.id
    ORDER BY a.created_at DESC, a.id DESC
    LIMIT 1
) l ON TRUE
WHERE b.organization_id = @organization_id AND b.engagement_id = ANY(@engagement_ids::bigint[])
  AND b.period = @period
GROUP BY b.engagement_id;

-- name: CopilotReceiptCounts :many
-- Dönemin onay bekleyen masraf fişleri ve Z raporları (#780): onaylanmadan
-- fişe dönmezler, Fişler adımı onları bekler.
SELECT e.id AS engagement_id,
       (SELECT count(*) FROM expenses x
        WHERE x.taxpayer_id = e.taxpayer_id AND x.period = @period AND x.approval_status = 'pending')::bigint AS expenses_pending,
       (SELECT count(*) FROM retail_sales r
        WHERE r.taxpayer_id = e.taxpayer_id AND r.period = @period AND r.approval_status = 'pending')::bigint AS z_reports_pending
FROM engagements e
WHERE e.organization_id = @organization_id AND e.id = ANY(@engagement_ids::bigint[]);

-- name: CopilotLucaImportCounts :many
-- Luca'dan alınanlar (#780): dönemin bordro dökümü (payroll_runs) ve Luca
-- mizanı (trial_balance içe aktarması) yüklendi mi; bordro yalnız çalışanı
-- ya da bordro geçmişi olan mükellefte beklenir. Dönemin son mizan
-- karşılaştırması ve fark sayısı (tutmayan + mizanda olmayan + kaynağı
-- olmayan).
SELECT e.id AS engagement_id,
       EXISTS (SELECT 1 FROM payroll_runs pr
               WHERE pr.taxpayer_id = e.taxpayer_id AND pr.period = @period)::bool AS payroll_imported,
       (EXISTS (SELECT 1 FROM payroll_runs pr
                WHERE pr.taxpayer_id = e.taxpayer_id AND pr.period < @period)
        OR EXISTS (SELECT 1 FROM employees em
                   WHERE em.taxpayer_id = e.taxpayer_id
                     AND (em.exit_date IS NULL OR em.exit_date >= to_date(@period || '-01', 'YYYY-MM-DD'))))::bool AS payroll_expected,
       EXISTS (SELECT 1 FROM account_plan_imports ai
               WHERE ai.organization_id = @organization_id AND ai.engagement_id = e.id
                 AND ai.source_kind = 'trial_balance' AND ai.period = @period)::bool AS trial_balance_imported,
       (rr.id IS NOT NULL)::bool AS reconciled,
       (COALESCE((rr.summary->>'differences')::int, 0)
        + COALESCE((rr.summary->>'missing_in_ledger')::int, 0)
        + COALESCE((rr.summary->>'missing_in_sources')::int, 0))::bigint AS reconciliation_issues
FROM engagements e
LEFT JOIN LATERAL (
    SELECT r.id, r.summary
    FROM reconciliation_runs r
    WHERE r.organization_id = @organization_id AND r.engagement_id = e.id AND r.period = @period
    ORDER BY r.id DESC
    LIMIT 1
) rr ON TRUE
WHERE e.organization_id = @organization_id AND e.id = ANY(@engagement_ids::bigint[]);

-- name: CopilotVATNoticeCounts :many
-- Dönemin mükellefe giden KDV bildirimleri (#780, vat_notices).
SELECT engagement_id, count(*)::bigint AS notices
FROM vat_notices
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND period = @period
GROUP BY engagement_id;

-- name: CopilotReportCounts :many
SELECT engagement_id,
       count(*) FILTER (WHERE status = 'sent')::bigint AS sent,
       count(*) FILTER (WHERE status IN ('queued', 'sending'))::bigint AS pending,
       count(*) FILTER (WHERE status = 'failed')::bigint AS failed
FROM report_deliveries
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND kind = 'month' AND period = @period
GROUP BY engagement_id;

-- name: InsertCopilotPlan :one
INSERT INTO copilot_plans (organization_id, created_by, command, period, planner, model, steps, notes)
VALUES (@organization_id, @created_by, @command, sqlc.narg(period), @planner, sqlc.narg(model), @steps, @notes)
RETURNING *;

-- name: GetCopilotPlan :one
SELECT * FROM copilot_plans
WHERE organization_id = @organization_id AND created_by = @created_by AND uuid = @uuid;

-- name: GetCopilotPlanForUpdate :one
SELECT * FROM copilot_plans
WHERE organization_id = @organization_id AND created_by = @created_by AND uuid = @uuid
FOR UPDATE;

-- name: ListCopilotPlans :many
SELECT * FROM copilot_plans
WHERE organization_id = @organization_id AND created_by = @created_by
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: UpdateCopilotPlan :one
UPDATE copilot_plans SET status = @status, steps = @steps
WHERE organization_id = @organization_id AND created_by = @created_by AND id = @id
RETURNING *;

-- name: GetCopilotEngagement :one
SELECT e.id, e.uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id AS taxpayer_tax_id
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.id = @engagement_id;
