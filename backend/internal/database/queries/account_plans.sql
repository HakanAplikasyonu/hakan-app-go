-- name: InsertAccountPlanImport :one
INSERT INTO account_plan_imports (organization_id, engagement_id, target, source_format, source_kind, file_name, sha256, account_count, skipped_rows, imported_by, period)
VALUES (@organization_id, @engagement_id, @target, @source_format, @source_kind, @file_name, @sha256, @account_count, @skipped_rows, sqlc.narg(imported_by), sqlc.narg(period))
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

-- name: InsertTrialBalanceLines :execrows
-- Mizan satırları (#393). rows: [{"code","name","main_code","level","is_leaf","debit","credit","balance"}, ...]
INSERT INTO trial_balance_lines (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf, debit, credit, balance)
SELECT @organization_id, @engagement_id, @import_id, a.code, a.name, a.main_code, a.level, a.is_leaf, a.debit, a.credit, a.balance
FROM jsonb_to_recordset(@rows::jsonb) AS a(code text, name text, main_code text, level smallint, is_leaf boolean,
                                           debit numeric, credit numeric, balance numeric);

-- name: ListTrialBalanceImports :many
-- Tutarlı mizan içe aktarmaları (#393), yeni önce; toplamlar alt (yaprak)
-- hesaplardan, ana hesaplar iki kez sayılmasın.
SELECT i.*,
       t.debit_total::numeric AS debit_total,
       t.credit_total::numeric AS credit_total
FROM account_plan_imports i
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(l.debit) FILTER (WHERE l.is_leaf), 0) AS debit_total,
           COALESCE(SUM(l.credit) FILTER (WHERE l.is_leaf), 0) AS credit_total,
           COUNT(*) AS line_count
    FROM trial_balance_lines l
    WHERE l.organization_id = i.organization_id AND l.engagement_id = i.engagement_id AND l.import_id = i.id
) t
WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id
  AND i.source_kind = 'trial_balance' AND t.line_count > 0
ORDER BY i.id DESC
LIMIT @lim;

-- name: ListTrialBalanceLines :many
-- Bir mizanın satırları; max_level verilirse o kırılıma kadar.
SELECT code, name, main_code, level, is_leaf, debit, credit, balance
FROM trial_balance_lines
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id
  AND (sqlc.narg(max_level)::smallint IS NULL OR level <= sqlc.narg(max_level))
ORDER BY code;

-- name: LiveTrialBalanceTotals :many
-- Canlı mizan (#669 E): ofisin kendi fişlerinden hesap bazında borç /
-- alacak toplamı, [from_period, to_period] aralığında. Varsayılan onaylı ve
-- aktarılmış fişler; include_drafts taslakları da katar.
SELECT l.account_code::text AS account_code,
       SUM(l.debit)::numeric AS debit,
       SUM(l.credit)::numeric AS credit
FROM journal_draft_lines l
JOIN journal_drafts d ON d.id = l.journal_draft_id AND d.engagement_id = l.engagement_id
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND d.period >= @from_period::text AND d.period <= @to_period::text
  AND (d.status IN ('approved', 'exported') OR @include_drafts::bool)
GROUP BY l.account_code
ORDER BY l.account_code;

-- name: AccountPlanNames :many
-- Etkin plandaki hesap adları (canlı mizanın adları, #669 E).
SELECT code::text AS code, name FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND code = ANY(@codes::text[]);

-- name: CariAccountInfo :many
-- Cari mizanı (#780): cari alt hesapların adı ve VKN'si; etkin plan, üretilen
-- cari alt hesaplar ve cari hafızası. Kod başına birden çok satır gelebilir,
-- öncelik rank'ta (0 plan).
SELECT code::text AS code, name::text AS name, tax_id::text AS tax_id, rank::int AS rank FROM (
    SELECT a.code, a.name, '' AS tax_id, 0 AS rank, a.id FROM account_plan_accounts a
    WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
      AND a.code = ANY(@codes::text[])
    UNION ALL
    SELECT g.code, g.name, CASE WHEN g.counterparty_key ~ '^[0-9]{10,11}$' THEN g.counterparty_key ELSE '' END, 1, g.id
    FROM generated_subaccounts g
    WHERE g.organization_id = @organization_id AND g.engagement_id = @engagement_id
      AND g.code = ANY(@codes::text[])
    UNION ALL
    SELECT c.account_code, c.account_name, c.counterparty_tax_id, 2, c.id FROM counterparty_accounts c
    WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
      AND c.account_code = ANY(@codes::text[])
) n
ORDER BY code, rank, id DESC;
