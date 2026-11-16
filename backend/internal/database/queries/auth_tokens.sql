-- name: CreateRefreshToken :one
-- session_uuid: a new one at sign-in, the session's own on rotation (#323).
INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address, impersonator_user_id, session_uuid)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetValidRefreshTokenByHash :one
SELECT *
FROM refresh_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: RevokeRefreshTokenByHash :execrows
-- Rotation: the token is used up, the session goes on.
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE token_hash = $1
  AND revoked_at IS NULL;

-- name: EndRefreshSessionByHash :execrows
-- Sign-out: the token's whole session ends, its access tokens with it (#323).
UPDATE refresh_tokens
SET revoked_at = COALESCE(revoked_at, NOW()), ended_at = NOW()
WHERE session_uuid = (SELECT r.session_uuid FROM refresh_tokens r WHERE r.token_hash = $1)
  AND ended_at IS NULL;

-- name: RefreshSessionEnded :one
-- Access tokens of an ended (or unknown) session are refused (#323).
SELECT NOT EXISTS (
    SELECT 1 FROM refresh_tokens
    WHERE session_uuid = @session_uuid AND user_id = @user_id AND ended_at IS NULL
)::boolean AS ended;

-- name: RevokeAllRefreshTokensForUser :exec
-- Password change / reset, deactivation: every session ends (#323).
UPDATE refresh_tokens
SET revoked_at = COALESCE(revoked_at, NOW()), ended_at = NOW()
WHERE user_id = $1
  AND ended_at IS NULL;

-- name: ListActiveRefreshTokensByUserID :many
SELECT *
FROM refresh_tokens
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > NOW()
ORDER BY created_at DESC;

-- name: RevokeRefreshTokenByUUIDForUser :execrows
-- $1 is the session (session_uuid): the whole session ends (#323).
UPDATE refresh_tokens
SET revoked_at = COALESCE(revoked_at, NOW()), ended_at = NOW()
WHERE session_uuid = $1
  AND user_id = $2
  AND ended_at IS NULL;

-- name: RevokeOtherRefreshTokensForUser :exec
-- $2 is the caller's session (session_uuid) (#323).
UPDATE refresh_tokens
SET revoked_at = COALESCE(revoked_at, NOW()), ended_at = NOW()
WHERE user_id = $1
  AND session_uuid <> $2
  AND ended_at IS NULL;
