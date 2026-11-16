-- name: InsertAccountPlanImport :one
INSERT INTO account_plan_imports (organization_id, engagement_id, target, source_format, source_kind, file_name, sha256, account_count, skipped_rows, imported_by)
VALUES (@organization_id, @engagement_id, @target, @source_format, @source_kind, @file_name, @sha256, @account_count, @skipped_rows, sqlc.narg(imported_by))
RETURNING *;

-- name: DeleteAccountPlanAccounts :execrows
-- Yeni sürüm gelince etkin plan değişir; eski sürümün özeti imports'ta kalır.
DELETE FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target;

-- name: InsertAccountPlanAccounts :execrows
-- rows: [{"code","name","main_code","level","is_leaf"}, ...]
INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, target, code, name, main_code, level, is_leaf)
SELECT @organization_id, @engagement_id, @import_id, @target, a.code, a.name, a.main_code, a.level, a.is_leaf
FROM jsonb_to_recordset(@rows::jsonb) AS a(code text, name text, main_code text, level smallint, is_leaf boolean);

-- name: ListAccountPlanImports :many
SELECT * FROM account_plan_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY id DESC
LIMIT @lim;

-- name: GetActiveAccountPlanImport :one
SELECT * FROM account_plan_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
ORDER BY id DESC
LIMIT 1;

-- name: ListAccountPlanAccounts :many
SELECT * FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND (sqlc.narg(q)::text IS NULL OR code LIKE sqlc.narg(q) || '%' OR name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY code
LIMIT @lim OFFSET @off;

-- name: CountAccountPlanAccounts :one
SELECT COUNT(*) FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND (sqlc.narg(q)::text IS NULL OR code LIKE sqlc.narg(q) || '%' OR name ILIKE '%' || sqlc.narg(q) || '%');

-- name: FindAccountPlanCodes :many
-- Verilen kodlardan planda olanlar (denetim).
SELECT code, is_leaf FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND code = ANY(@codes::text[]);

-- name: ListAccountPlanCodeSet :many
-- Çıktı (journals, #26) için etkin planın bütün kodları.
SELECT code FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target;

-- name: ListAssignedCodesMissingFromPlan :many
-- "Açılmalı" hesaplar (#319): mükellefe uygulanmış ama etkin planda
-- olmayan hesap kodları; ad, cari hafızasından ya da önerinin önerdiği
-- addan. Eksik hesap şablonunu (Luca) besler.
SELECT a.account_code::text AS code,
       COALESCE(MAX(NULLIF(c.account_name, '')), MAX(s.proposed_value->>'account_name'), '')::text AS name,
       COUNT(*)::bigint AS uses
FROM account_assignments a
LEFT JOIN counterparty_accounts c
       ON c.organization_id = a.organization_id AND c.engagement_id = a.engagement_id AND c.account_code = a.account_code
LEFT JOIN suggestions s ON s.organization_id = a.organization_id AND s.id = a.suggestion_id
WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
  AND NOT EXISTS (SELECT 1 FROM account_plan_accounts p
                  WHERE p.organization_id = a.organization_id AND p.engagement_id = a.engagement_id
                    AND p.target = @target AND p.code = a.account_code)
GROUP BY a.account_code
ORDER BY a.account_code
LIMIT 5000;
