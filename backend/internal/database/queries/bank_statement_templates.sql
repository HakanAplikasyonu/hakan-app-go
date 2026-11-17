-- Banka ekstresi şablonları (issue #383): ofisin kaydettiği başlık
-- düzenleri. Başlangıç şablonları kodda (banking/statement/templates.go).

-- name: ListBankStatementTemplates :many
SELECT * FROM bank_statement_templates
WHERE organization_id = @organization_id
ORDER BY updated_at DESC, id DESC;

-- name: UpsertBankStatementTemplate :one
INSERT INTO bank_statement_templates (organization_id, bank_id, name, format, fingerprint, columns, created_by)
VALUES (@organization_id, sqlc.narg(bank_id), @name, @format, @fingerprint, @columns, sqlc.narg(created_by))
ON CONFLICT (organization_id, format, fingerprint)
DO UPDATE SET bank_id = EXCLUDED.bank_id, name = EXCLUDED.name, columns = EXCLUDED.columns,
              created_by = EXCLUDED.created_by
RETURNING *;

-- name: TouchBankStatementTemplate :exec
UPDATE bank_statement_templates
SET hits = hits + 1, last_used_at = NOW()
WHERE organization_id = @organization_id AND id = @id;

-- name: DeleteBankStatementTemplate :execrows
DELETE FROM bank_statement_templates
WHERE organization_id = @organization_id AND uuid = @uuid;
