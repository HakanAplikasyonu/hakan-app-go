-- AI dönem özeti (issue #35): olgu sorguları ve taslaklar.

-- name: SummaryInvoiceTotals :many
-- Dönemin aktif faturaları, yöne göre.
SELECT direction, count(*)::bigint AS invoices,
       coalesce(sum(tax_exclusive_amount), 0)::numeric AS base,
       coalesce(sum(tax_amount), 0)::numeric AS vat,
       coalesce(sum(withholding_amount), 0)::numeric AS withholding,
       coalesce(sum(payable_amount), 0)::numeric AS payable
FROM invoices
WHERE taxpayer_id = @taxpayer_id AND period = @period AND status_normalized = 'active'
GROUP BY direction
ORDER BY direction;

-- name: SummaryExpenseTotals :one
-- Onaylı masraflar (beyana yalnızca onaylı masraf girer).
SELECT count(*)::bigint AS expenses, coalesce(sum(amount), 0)::numeric AS amount
FROM expenses
WHERE taxpayer_id = @taxpayer_id AND period = @period AND approval_status = 'approved';

-- name: SummaryOpenWarnings :many
SELECT severity, count(*)::bigint AS warnings
FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND period = @period AND status = 'open'
GROUP BY severity
ORDER BY severity;

-- name: InsertPeriodSummary :one
INSERT INTO period_summaries (organization_id, engagement_id, taxpayer_id, period, facts, template, body, model, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, @facts, @template, @body, sqlc.narg(model), sqlc.narg(created_by))
RETURNING *;

-- name: SupersedePeriodSummaryDrafts :exec
UPDATE period_summaries SET status = 'superseded'
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period AND status = 'draft';

-- name: GetPeriodSummary :one
SELECT * FROM period_summaries
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListPeriodSummaries :many
SELECT * FROM period_summaries
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period)::text)
ORDER BY period DESC, created_at DESC
LIMIT 50;

-- name: UpdatePeriodSummaryBody :one
UPDATE period_summaries SET body = @body, edited = true
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'draft'
RETURNING *;

-- name: ApprovePeriodSummary :one
UPDATE period_summaries SET status = 'approved', approved_by = sqlc.narg(approved_by), approved_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'draft'
RETURNING *;
