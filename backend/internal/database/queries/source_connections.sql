-- name: CreateSourceConnection :one
INSERT INTO source_connections (taxpayer_id, source_system, external_company_id, credentials_ref, setup_state, meta, consent_text, consent_at)
VALUES (@taxpayer_id, @source_system, sqlc.narg(external_company_id), sqlc.narg(credentials_ref), @setup_state, @meta, sqlc.narg(consent_text), sqlc.narg(consent_at))
RETURNING *;

-- name: GetSourceConnection :one
SELECT * FROM source_connections
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetSourceConnectionBySystem :one
SELECT * FROM source_connections
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system
ORDER BY id
LIMIT 1;

-- name: ListSourceConnections :many
SELECT * FROM source_connections
WHERE taxpayer_id = @taxpayer_id
ORDER BY source_system, id;

-- name: SetSourceConnectionCredentialsRef :execrows
UPDATE source_connections
SET credentials_ref = sqlc.narg(credentials_ref), setup_state = @setup_state
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: SetSourceConnectionStatus :execrows
UPDATE source_connections
SET status = @status, last_error = sqlc.narg(last_error),
    last_success_at = CASE WHEN @status::text = 'active' THEN NOW() ELSE last_success_at END
WHERE taxpayer_id = @taxpayer_id AND id = @id;
