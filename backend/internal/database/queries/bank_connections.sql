-- Banka web servisi bağlantıları (#49): source_connections üzerinde
-- (source_system = 'bank_<id>'), mükellef kapsamında.

-- name: ListBankConnections :many
SELECT * FROM source_connections
WHERE taxpayer_id = @taxpayer_id AND source_system LIKE 'bank\_%' AND source_system <> 'bank_file'
ORDER BY id;

-- name: UpdateBankConnectionSetup :one
UPDATE source_connections
SET setup_state = @setup_state,
    status      = @status,
    last_error  = sqlc.narg(last_error),
    meta        = meta || @meta_patch::jsonb
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: MarkBankConnectionSynced :exec
UPDATE source_connections
SET status = 'active', last_error = NULL, last_success_at = NOW(),
    meta = meta || @meta_patch::jsonb
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: LinkCashBoxToBankConnection :one
UPDATE cash_boxes
SET bank_connection_id  = @bank_connection_id,
    bank_id             = COALESCE(bank_id, sqlc.narg(bank_id)),
    bank_account_number = COALESCE(bank_account_number, sqlc.narg(bank_account_number)),
    bank_branch_code    = COALESCE(bank_branch_code, sqlc.narg(bank_branch_code))
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;
