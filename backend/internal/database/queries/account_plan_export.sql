-- Luca hesap planı ve cari alt hesap çıktısı (#464).

-- name: ListGeneratedSubaccounts :many
SELECT * FROM generated_subaccounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
ORDER BY code;

-- name: GetGeneratedSubaccount :one
SELECT * FROM generated_subaccounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND main_code = @main_code AND counterparty_key = @counterparty_key;

-- name: InsertGeneratedSubaccount :one
-- Aynı cari ya da aynı kod için ikinci satır yazılmaz (eşzamanlı istekte
-- kaybeden ON CONFLICT ile boş döner, çağıran yeniden okur).
INSERT INTO generated_subaccounts (organization_id, engagement_id, taxpayer_id, target, main_code, code, parent_code,
    counterparty_key, customer_uuid, name, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @target, @main_code, @code, @parent_code,
    @counterparty_key, sqlc.narg(customer_uuid), @name, sqlc.narg(created_by))
ON CONFLICT DO NOTHING
RETURNING *;

-- name: ListPlanAccountsForExport :many
-- Etkin planın hesapları (tüm hesap planı dosyası ve kod üretimi).
SELECT code, name, main_code, level, is_leaf FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
ORDER BY code;

-- name: LatestAccountPlanImport :one
-- Karşılaştırılan sürüm: mükellefin son Luca hesap planı / mizan dökümü.
SELECT id, uuid, source_kind, file_name, created_at FROM account_plan_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
ORDER BY id DESC
LIMIT 1;

-- name: ListEngagementCounterpartyAccounts :many
-- Cari hafızası (#319): incelemede kararlaştırılmış cari hesaplar.
SELECT counterparty_tax_id, main_code, account_code, account_name FROM counterparty_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY account_code;

-- name: RememberGeneratedCounterpartyAccount :exec
-- Üretilen kod cari hafızasına da yazılır; incelemede verilmiş bir karar
-- varsa ona dokunulmaz.
INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code,
                                   account_code, account_name, decided_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @counterparty_tax_id, @main_code,
        @account_code, @account_name, sqlc.narg(decided_by))
ON CONFLICT (engagement_id, counterparty_tax_id, main_code) DO NOTHING;

-- name: ListSubaccountCandidates :many
-- Alt hesap açılabilecek cariler: perakende varsayılan müşterisi ve
-- kaynakta silinenler hariç.
SELECT uuid, type, name, tax_number, identity_number, tax_office, address, district, city, email
FROM customers
WHERE taxpayer_id = @taxpayer_id
  AND NOT is_default_retail_customer
  AND deleted_at_source IS NULL
  AND type IN ('customer', 'supplier', 'both')
  AND btrim(name) <> ''
ORDER BY name, id
LIMIT 5000;

-- name: GetCustomerForSubaccount :one
SELECT uuid, type, name, tax_number, identity_number, tax_office, address, district, city, email
FROM customers
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: ListCustomersByTaxIDs :many
-- Dosyadaki vergi bilgileri: VKN / TCKN ile cari kartı.
SELECT DISTINCT ON (COALESCE(tax_number, identity_number))
       uuid, name, tax_number, identity_number, tax_office, address, district, city, email
FROM customers
WHERE taxpayer_id = @taxpayer_id AND deleted_at_source IS NULL
  AND (tax_number = ANY(@tax_ids::text[]) OR identity_number = ANY(@tax_ids::text[]))
ORDER BY COALESCE(tax_number, identity_number), updated_at DESC;

-- name: InsertAccountPlanExport :one
INSERT INTO account_plan_exports (organization_id, engagement_id, target, kind, template_version, template_verified,
    compared_import_id, file_name, sha256, size, account_count, created_by)
VALUES (@organization_id, @engagement_id, @target, @kind, @template_version, @template_verified,
    sqlc.narg(compared_import_id), @file_name, @sha256, @size, @account_count, sqlc.narg(created_by))
RETURNING *;

-- name: ListAccountPlanExports :many
SELECT e.*, i.uuid AS compared_import_uuid, i.file_name AS compared_file_name
FROM account_plan_exports e
LEFT JOIN account_plan_imports i ON i.organization_id = e.organization_id AND i.id = e.compared_import_id
WHERE e.organization_id = @organization_id AND e.engagement_id = @engagement_id
ORDER BY e.created_at DESC, e.id DESC
LIMIT @lim OFFSET @off;
