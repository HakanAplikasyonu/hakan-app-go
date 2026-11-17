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
-- aktarımı sayılır.
SELECT x.engagement_id,
       count(*) FILTER (WHERE x.status IN ('queued', 'claimed', 'delivered'))::bigint AS open,
       count(*) FILTER (WHERE x.status = 'imported')::bigint AS imported,
       count(*) FILTER (WHERE x.status = 'failed')::bigint AS failed
FROM (
    SELECT DISTINCT ON (a.export_batch_id) a.engagement_id, a.status
    FROM accounting_transfers a
    JOIN export_batches b ON b.engagement_id = a.engagement_id AND b.id = a.export_batch_id
    WHERE a.organization_id = @organization_id AND a.engagement_id = ANY(@engagement_ids::bigint[])
      AND b.organization_id = @organization_id AND b.period = @period
    ORDER BY a.export_batch_id, a.created_at DESC, a.id DESC
) x
GROUP BY x.engagement_id;

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
