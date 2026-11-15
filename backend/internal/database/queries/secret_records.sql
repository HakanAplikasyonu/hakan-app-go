-- name: InsertSecretRecord :exec
INSERT INTO secret_records (uuid, purpose, alg, kek_id, wrapped_dek, ciphertext)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetSecretRecord :one
SELECT uuid, purpose, alg, kek_id, wrapped_dek, ciphertext
FROM secret_records
WHERE uuid = $1;

-- name: ReplaceSecretRecord :execrows
UPDATE secret_records
SET kek_id = $2, wrapped_dek = $3, ciphertext = $4, rotated_at = NULL
WHERE uuid = $1 AND purpose = $5;

-- name: RewrapSecretRecord :execrows
UPDATE secret_records
SET kek_id = @new_kek_id, wrapped_dek = @wrapped_dek, rotated_at = NOW()
WHERE uuid = @uuid AND kek_id = @old_kek_id;

-- name: ListSecretRecordsNotOnKEK :many
SELECT uuid, purpose, alg, kek_id, wrapped_dek, ciphertext
FROM secret_records
WHERE kek_id <> @active_kek_id
ORDER BY id
LIMIT @lim;

-- name: CountSecretRecordsByKEK :many
SELECT kek_id, COUNT(*)::bigint AS n
FROM secret_records
GROUP BY kek_id
ORDER BY kek_id;

-- name: DeleteSecretRecord :execrows
DELETE FROM secret_records WHERE uuid = $1;
