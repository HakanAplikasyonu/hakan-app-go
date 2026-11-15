-- Masaüstü yerel ajanı (issue #51).

-- name: UpsertDesktopAgent :one
-- Aynı makine yeniden kayıt olursa satır güncellenir; geri alınmış ajan
-- yeniden kaydolunca tekrar etkinleşir.
INSERT INTO desktop_agents (organization_id, machine_id, name, platform, version, created_by, status, last_seen_at)
VALUES (@organization_id, @machine_id, @name, @platform, @version, sqlc.narg(created_by), 'online', NOW())
ON CONFLICT (organization_id, machine_id) DO UPDATE
SET name = EXCLUDED.name, platform = EXCLUDED.platform, version = EXCLUDED.version,
    created_by = COALESCE(EXCLUDED.created_by, desktop_agents.created_by),
    status = 'online', last_seen_at = NOW(), offline_since = NULL, revoked_at = NULL
RETURNING *;

-- name: HeartbeatDesktopAgent :one
-- Önceki durumu da döner: offline → online geçişi kayda değer.
WITH prev AS (
    SELECT p.id, p.status AS previous_status FROM desktop_agents p
    WHERE p.organization_id = @organization_id AND p.uuid = @uuid AND p.revoked_at IS NULL
    FOR UPDATE
)
UPDATE desktop_agents a
SET last_seen_at = NOW(), status = 'online', offline_since = NULL,
    version = @version, last_heartbeat = @last_heartbeat
FROM prev
WHERE a.organization_id = @organization_id AND a.id = prev.id
RETURNING a.*, prev.previous_status;

-- name: GetDesktopAgent :one
SELECT * FROM desktop_agents WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: ListDesktopAgents :many
SELECT * FROM desktop_agents
WHERE organization_id = @organization_id AND revoked_at IS NULL
ORDER BY name, id;

-- name: RevokeDesktopAgent :execrows
UPDATE desktop_agents SET revoked_at = NOW()
WHERE organization_id = @organization_id AND uuid = @uuid AND revoked_at IS NULL;

-- tenant-lint: cross-org zamanlanmış sağlık taraması bütün ofislerin sessiz ajanlarını işaretler
-- name: MarkSilentDesktopAgentsOffline :many
-- @silent_before'dan beri haber vermeyen çevrimiçi ajanlar offline olur.
-- Dönen satırlar bu çalıştırmada offline'a geçenlerdir (bildirim bir kez).
UPDATE desktop_agents
SET status = 'offline', offline_since = NOW()
WHERE id IN (
    SELECT s.id FROM desktop_agents s
    WHERE s.status = 'online' AND s.revoked_at IS NULL AND s.last_seen_at < @silent_before
    ORDER BY s.last_seen_at
    LIMIT @lim
    FOR UPDATE SKIP LOCKED
)
RETURNING id, uuid, organization_id, name, created_by, last_seen_at;
