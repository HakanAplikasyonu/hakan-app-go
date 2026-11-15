-- name: InsertAccountRule :one
INSERT INTO account_rules (organization_id, engagement_id, rule_key, version, role, priority, name, conditions,
                           account_code, confidence, origin, copied_from, created_by)
VALUES (@organization_id, sqlc.narg(engagement_id), COALESCE(sqlc.narg(rule_key)::uuid, gen_random_uuid()),
        @version, @role, @priority, @name, @conditions, @account_code, @confidence, @origin,
        sqlc.narg(copied_from), sqlc.narg(created_by))
RETURNING *;

-- name: GetActiveAccountRuleForUpdate :one
SELECT * FROM account_rules
WHERE organization_id = @organization_id
  AND engagement_id IS NOT DISTINCT FROM sqlc.narg(engagement_id)::bigint
  AND rule_key = @rule_key AND status = 'active'
FOR UPDATE;

-- name: RetireAccountRule :execrows
UPDATE account_rules SET status = 'retired', retired_at = NOW()
WHERE organization_id = @organization_id AND id = @id AND status = 'active';

-- name: ListAccountRules :many
-- Bir seviyenin (ofis şablonu: engagement_id NULL) kuralları.
SELECT * FROM account_rules
WHERE organization_id = @organization_id
  AND engagement_id IS NOT DISTINCT FROM sqlc.narg(engagement_id)::bigint
  AND (@include_retired::boolean OR status = 'active')
ORDER BY role, priority, created_at, version;

-- name: ListAccountRuleVersions :many
SELECT * FROM account_rules
WHERE organization_id = @organization_id
  AND engagement_id IS NOT DISTINCT FROM sqlc.narg(engagement_id)::bigint
  AND rule_key = @rule_key
ORDER BY version DESC;

-- name: GetAccountRuleByID :one
SELECT * FROM account_rules WHERE organization_id = @organization_id AND id = @id;

-- name: CopyOfficeAccountRules :one
SELECT copy_office_account_rules(@organization_id::bigint, @engagement_id::bigint)::int AS copied;

-- name: UpsertAccountAssignment :one
-- Önceki değeri döndürür (inceleme kuyruğu geri alma için saklar).
WITH prev AS (
    SELECT account_code FROM account_assignments
    WHERE organization_id = @organization_id AND engagement_id = @engagement_id
      AND target_type = @target_type AND target_id = @target_id AND field = @field
)
INSERT INTO account_assignments (organization_id, engagement_id, taxpayer_id, target_type, target_id, field,
                                 account_code, rule_id, suggestion_id, assigned_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @target_type, @target_id, @field,
        @account_code, sqlc.narg(rule_id), sqlc.narg(suggestion_id), sqlc.narg(assigned_by))
ON CONFLICT (engagement_id, target_type, target_id, field)
DO UPDATE SET account_code = EXCLUDED.account_code, rule_id = EXCLUDED.rule_id,
              suggestion_id = EXCLUDED.suggestion_id, assigned_by = EXCLUDED.assigned_by
RETURNING COALESCE((SELECT account_code FROM prev), '')::text AS previous_code;

-- name: DeleteAccountAssignment :execrows
DELETE FROM account_assignments
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND target_type = @target_type AND target_id = @target_id AND field = @field;

-- name: ListAccountAssignmentsForTargets :many
SELECT * FROM account_assignments
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND target_type = @target_type AND target_id = ANY(@target_ids::bigint[]);

-- name: CounterpartyAccountHistory :many
-- Kural hafızası (docs/01 §3 "carries context forward"): aynı karşı tarafın
-- önceki faturalarında onaylanmış satır hesap kodları, en sık kullanılan önce.
SELECT aa.account_code, COUNT(*)::bigint AS uses, MAX(i.period)::text AS last_period
FROM account_assignments aa
JOIN invoice_items it ON it.id = aa.target_id AND it.taxpayer_id = @taxpayer_id
JOIN invoices i ON i.id = it.invoice_id AND i.taxpayer_id = @taxpayer_id
WHERE aa.organization_id = @organization_id AND aa.engagement_id = @engagement_id
  AND aa.target_type = 'invoice_item' AND aa.field = @field
  AND i.direction = @direction
  AND (i.customer_tax_number = sqlc.narg(tax_number) OR i.customer_identity_number = sqlc.narg(identity_number))
  AND i.id <> @exclude_invoice_id
GROUP BY aa.account_code
ORDER BY uses DESC, last_period DESC
LIMIT 3;

-- name: AccountRuleCandidates :many
-- Kural adayı: müşavirin aynı karşı taraf için aynı hesap koduna verdiği
-- düzeltme / kabul kararları (kural dışı önerilerde). min_count kez
-- tekrarlanınca "kural yapılsın mı?" sorulur.
SELECT (s.inputs->>'counterparty_key')::text AS counterparty_key,
       MAX(s.inputs->>'counterparty_name')::text AS counterparty_name,
       (s.inputs->>'direction')::text AS direction,
       s.target_field,
       (s.applied_value->>'account_code')::text AS account_code,
       COUNT(*)::bigint AS decisions,
       MAX(s.decided_at)::timestamptz AS last_decided_at
FROM suggestions s
WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id
  AND s.kind = 'account_code' AND s.target_type = 'invoice_item' AND s.target_field = 'account_code'
  AND s.status IN ('accepted', 'corrected')
  AND (s.rule_ref IS NULL OR s.rule_ref NOT LIKE 'account_rule:%')
  AND s.inputs ? 'counterparty_key'
  AND s.applied_value ? 'account_code'
GROUP BY 1, 3, 4, 5
HAVING COUNT(*) >= @min_count::bigint
ORDER BY decisions DESC, account_code;

-- name: GetInvoiceForAccounting :one
SELECT i.id, i.uuid, i.taxpayer_id, i.direction, i.type, i.period, i.customer_id,
       i.customer_tax_number, i.customer_identity_number, i.customer_name
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id AND i.uuid = @uuid;

-- name: ListAccountPlanCodesUnder :many
-- Cari hesap açma önerisi: ana hesabın (120 / 320) etkin plandaki alt kodları.
SELECT code, name, level, is_leaf FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND main_code = @main_code
ORDER BY code;
