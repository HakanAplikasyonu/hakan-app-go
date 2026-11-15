-- Faturalama (issue #52). Planlar platform düzeyinde; ofis satırları
-- organization_id ile.

-- name: ListBillingPlans :many
SELECT * FROM billing_plans ORDER BY active DESC, code;

-- name: GetBillingPlanByCode :one
SELECT * FROM billing_plans WHERE code = @code;

-- name: GetBillingPlan :one
SELECT * FROM billing_plans WHERE id = @id;

-- name: UpsertBillingPlan :one
INSERT INTO billing_plans (code, name, currency, base_fee, per_taxpayer_fee, included_taxpayers, vat_rate, active)
VALUES (@code, @name, @currency, @base_fee, @per_taxpayer_fee, @included_taxpayers, @vat_rate, @active)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name, currency = EXCLUDED.currency, base_fee = EXCLUDED.base_fee,
    per_taxpayer_fee = EXCLUDED.per_taxpayer_fee, included_taxpayers = EXCLUDED.included_taxpayers,
    vat_rate = EXCLUDED.vat_rate, active = EXCLUDED.active
RETURNING *;

-- name: GetOrganizationBilling :one
SELECT * FROM organization_billing WHERE organization_id = @organization_id;

-- name: UpsertOrganizationBilling :one
INSERT INTO organization_billing (organization_id, plan_id, read_only, read_only_reason, updated_by)
VALUES (@organization_id, @plan_id, @read_only, @read_only_reason, sqlc.narg(updated_by))
ON CONFLICT (organization_id) DO UPDATE
SET plan_id = EXCLUDED.plan_id, read_only = EXCLUDED.read_only,
    read_only_reason = EXCLUDED.read_only_reason, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: OrganizationReadOnly :one
SELECT COALESCE((SELECT read_only FROM organization_billing WHERE organization_id = @organization_id), FALSE)::bool;

-- Aktif mükellef (D9): ayın herhangi bir gününde ofisle aktif bir
-- engagement'ı olan mükellef. Aynı mükellef ay içinde ayrılıp dönse de 1 sayılır.
-- name: ListActiveTaxpayersInPeriod :many
SELECT t.id, t.uuid, t.title, t.tax_id,
       min(e.started_at)::timestamptz AS first_started_at,
       max(COALESCE(e.ended_at, 'infinity'::timestamptz))::timestamptz AS last_ended_at
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id
  AND e.started_at < @period_end::timestamptz
  AND (e.ended_at IS NULL OR e.ended_at >= @period_start::timestamptz)
GROUP BY t.id, t.uuid, t.title, t.tax_id
ORDER BY t.title, t.id;

-- name: UpsertBillingStatement :one
INSERT INTO billing_statements (
    organization_id, period, plan_code, plan_snapshot, active_taxpayers, billable_taxpayers, currency,
    base_fee, taxpayer_fee_total, subtotal, vat_rate, vat_amount, total, lines, taxpayers, generated_at
) VALUES (
    @organization_id, @period, @plan_code, @plan_snapshot, @active_taxpayers, @billable_taxpayers, @currency,
    @base_fee, @taxpayer_fee_total, @subtotal, @vat_rate, @vat_amount, @total, @lines, @taxpayers, NOW()
)
ON CONFLICT (organization_id, period) DO UPDATE
SET plan_code = EXCLUDED.plan_code, plan_snapshot = EXCLUDED.plan_snapshot,
    active_taxpayers = EXCLUDED.active_taxpayers, billable_taxpayers = EXCLUDED.billable_taxpayers,
    currency = EXCLUDED.currency, base_fee = EXCLUDED.base_fee, taxpayer_fee_total = EXCLUDED.taxpayer_fee_total,
    subtotal = EXCLUDED.subtotal, vat_rate = EXCLUDED.vat_rate, vat_amount = EXCLUDED.vat_amount,
    total = EXCLUDED.total, lines = EXCLUDED.lines, taxpayers = EXCLUDED.taxpayers, generated_at = NOW()
WHERE billing_statements.organization_id = @organization_id AND billing_statements.status = 'draft'
RETURNING *;

-- name: GetBillingStatement :one
SELECT * FROM billing_statements WHERE organization_id = @organization_id AND period = @period;

-- name: ListBillingStatements :many
SELECT * FROM billing_statements WHERE organization_id = @organization_id ORDER BY period DESC LIMIT 24;

-- name: SetBillingStatementStatus :one
UPDATE billing_statements SET status = @status
WHERE organization_id = @organization_id AND period = @period AND status = 'draft'
RETURNING *;
