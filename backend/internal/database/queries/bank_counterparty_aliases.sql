-- Banka karşı taraf takma adları (issue #383).

-- name: ListBankCounterpartyAliases :many
SELECT * FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY updated_at DESC, id DESC;

-- name: ListBankCounterpartyAliasesPage :many
-- Liste ucu (issue #453): sayfalı, toplamla.
SELECT sqlc.embed(a), COUNT(*) OVER () AS total_count FROM bank_counterparty_aliases a
WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status)::text)
ORDER BY a.updated_at DESC, a.id DESC
LIMIT @lim OFFSET @off;

-- name: FindBankCounterpartyAliases :many
-- Bir hareketin anahtarlarına uyan takma adlar: kural önce, sonra IBAN >
-- VKN/TCKN > ad, yön tutan önce.
SELECT * FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (debit_credit IS NULL OR debit_credit = @debit_credit::text)
  AND ((match_type = 'iban' AND match_value = @iban::text)
    OR (match_type = 'tax_id' AND match_value = @tax_id::text)
    OR (match_type = 'name' AND match_value = @name::text))
ORDER BY (status = 'rule') DESC, CASE match_type WHEN 'iban' THEN 0 WHEN 'tax_id' THEN 1 ELSE 2 END,
         (debit_credit IS NULL), updated_at DESC;

-- name: GetBankCounterpartyAlias :one
SELECT * FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: GetBankCounterpartyAliasByKey :one
SELECT * FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND match_type = @match_type AND match_value = @match_value
  AND debit_credit IS NOT DISTINCT FROM sqlc.narg(debit_credit)::char(1);

-- name: InsertBankCounterpartyAlias :one
INSERT INTO bank_counterparty_aliases (organization_id, engagement_id, taxpayer_id, match_type, match_value,
                                       display_name, debit_credit, account_code, account_name, status, origin,
                                       accepts, corrections, suggestion_id, decided_by, last_decided_at)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @match_type, @match_value, @display_name,
        sqlc.narg(debit_credit), @account_code, @account_name, @status, @origin, @accepts, @corrections,
        sqlc.narg(suggestion_id), sqlc.narg(decided_by), sqlc.narg(last_decided_at))
RETURNING *;

-- name: UpdateBankCounterpartyAlias :one
UPDATE bank_counterparty_aliases
SET display_name = @display_name, account_code = @account_code, account_name = @account_name,
    status = @status, origin = @origin, accepts = @accepts, corrections = @corrections,
    suggestion_id = sqlc.narg(suggestion_id), decided_by = sqlc.narg(decided_by),
    last_decided_at = sqlc.narg(last_decided_at)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: IncrementBankCounterpartyAliasHits :exec
UPDATE bank_counterparty_aliases SET hits = hits + 1
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: DeleteBankCounterpartyAlias :execrows
DELETE FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListBankCounterpartyAliasesBySuggestion :many
SELECT * FROM bank_counterparty_aliases
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND suggestion_id = @suggestion_id;
