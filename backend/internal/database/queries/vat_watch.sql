-- KDV nöbetçisi (#780 B): günlük tarama ve üretilen olaylar.

-- tenant-lint: cross-org günlük KDV nöbetçisi tüm ofislerin etkin mükelleflerini rls.System içinde tarar
-- name: ListVATWatchTargets :many
-- Nöbetçiyi kapatmamış ofislerin etkin mükellef dosyaları.
SELECT e.organization_id, e.id AS engagement_id, e.uuid AS engagement_uuid,
       t.id AS taxpayer_id, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title,
       o.slug AS organization_slug
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
INNER JOIN organizations o ON o.id = e.organization_id
LEFT JOIN vat_tracking_settings s ON s.organization_id = e.organization_id
WHERE e.status = 'active' AND COALESCE(s.watch_enabled, TRUE)
ORDER BY e.organization_id, e.id;

-- name: LatestVATNotice :one
-- Mükellefe bu dönem için son söylenen.
SELECT uuid, outcome, payable, created_at
FROM vat_notices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: InsertVATWatchEvent :one
-- Olay bir kez yazılır; aynı dedupe_key varsa satır dönmez (pgx.ErrNoRows).
INSERT INTO vat_watch_events (organization_id, engagement_id, period, event, dedupe_key, payload)
VALUES (@organization_id, @engagement_id, @period, @event, @dedupe_key, @payload)
ON CONFLICT (organization_id, dedupe_key) DO NOTHING
RETURNING id, uuid, created_at;

-- name: ListVATWatchEvents :many
-- Bir mükellefin nöbetçi olayları, yeniden eskiye.
SELECT uuid, period, event, payload, created_at
FROM vat_watch_events
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY created_at DESC, id DESC
LIMIT @limit_count;
