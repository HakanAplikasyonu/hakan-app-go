-- Zamanlanmış kaynak senkronu (issue #385).

-- tenant-lint: cross-taxpayer worker taraması (rls.System) zamanı gelmiş bağlantıları ofisler arası seçer
-- name: ListDueSourceConnections :many
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = @source_system AND setup_state = 'active' AND status IN ('active', 'rate_limited')
  AND sync_interval_minutes IS NOT NULL AND next_sync_at IS NOT NULL AND next_sync_at <= @now
ORDER BY next_sync_at
LIMIT @lim;

-- name: SetSourceConnectionSchedule :one
UPDATE source_connections
SET sync_interval_minutes = sqlc.narg(sync_interval_minutes), next_sync_at = sqlc.narg(next_sync_at), sync_failures = @sync_failures
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: RescheduleSourceConnection :exec
-- Bir senkron bittiğinde sıradaki zamanı yazar; aralığa dokunmaz. Senkron
-- sürerken zamanlama kapatıldıysa (aralık NULL) satır değişmez: kapalı kalır.
UPDATE source_connections
SET next_sync_at = @next_sync_at, sync_failures = @sync_failures
WHERE taxpayer_id = @taxpayer_id AND id = @id AND sync_interval_minutes IS NOT NULL;

-- tenant-lint: cross-taxpayer Easytrade webhook'u şirket kimliğiyle gelir, bağlantının mükellefi bilinmez (rls.System)
-- name: ListSourceConnectionsByExternalCompany :many
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = @source_system AND external_company_id = @external_company_id
  AND setup_state = 'active' AND status IN ('active', 'rate_limited')
ORDER BY id;

-- name: PullSourceConnectionNow :one
-- Webhook (issue #385): zamanlanmış bağlantının sıradaki senkronunu şimdiye
-- çeker ve son bildirim zamanını yazar.
UPDATE source_connections
SET next_sync_at = CASE WHEN sync_interval_minutes IS NULL THEN next_sync_at
                        ELSE LEAST(COALESCE(next_sync_at, @now::timestamptz), @now::timestamptz) END,
    meta = meta || jsonb_build_object('last_webhook_at', @now::timestamptz)
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid
RETURNING id;

-- Düzenli entegratör senkronu (çapraz denetim A1 "Kalan"): İşNet ve EDM
-- bağlantıları saatlik artımlı çekimle ve gece taramasıyla çalışır. Aynı
-- kolonlar (000094) kullanılır: next_sync_at sıradaki artımlı çekim,
-- sync_failures ardışık başarısız çalışma sayısı.

-- tenant-lint: cross-taxpayer worker taraması (rls.System) zamanı gelmiş entegratör bağlantılarını ofisler arası seçer
-- name: ListDueEntegratorConnections :many
-- Kurulumu bitmiş (ilk çekimi başarılı) ve zamanı gelmiş bağlantılar. Henüz
-- zamanlanmamış (next_sync_at NULL: zamanlayıcıdan önce kurulmuş) bağlantı
-- da hemen seçilir; ilk çalışması zamanlamayı yazar.
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = ANY(@source_systems::text[]) AND setup_state <> 'draft' AND status IN ('active', 'rate_limited')
  AND (next_sync_at IS NULL OR next_sync_at <= @now)
ORDER BY next_sync_at NULLS FIRST, id
LIMIT @lim;

-- tenant-lint: cross-taxpayer gece taraması (rls.System) bütün ofislerin etkin entegratör bağlantılarını seçer
-- name: ListScheduledEntegratorConnections :many
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = ANY(@source_systems::text[]) AND setup_state <> 'draft' AND status IN ('active', 'rate_limited')
ORDER BY id;

-- name: SetEntegratorSyncSchedule :exec
-- Bir entegratör çalışması bitince sıradaki artımlı çekimi ve ardışık hata
-- sayısını yazar. Başarılı çalışma kurulumu bitirir (setup_state active);
-- geçici hata durumu değiştirmez, yalnızca son hatayı yazar.
UPDATE source_connections
SET sync_interval_minutes = @sync_interval_minutes, next_sync_at = @next_sync_at, sync_failures = @sync_failures,
    setup_state = CASE WHEN @succeeded::boolean THEN 'active' ELSE setup_state END,
    last_error = COALESCE(sqlc.narg(last_error)::jsonb, last_error)
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: LastSuccessfulSyncRunStartedAt :one
-- Artımlı pencerenin başı: son başarılı (ya da kısmi; başarısız pencereleri
-- sonraki çalışmalar sürdürür) çalışmanın başlangıcı.
SELECT MAX(started_at)::timestamptz AS started_at FROM sync_runs
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND status IN ('success', 'partial');

-- name: LastSuccessfulSyncRunFinishedAt :one
-- Ekrandaki "son çekim": belge getiren (başarılı ya da kısmi) son çalışmanın bitişi.
SELECT MAX(finished_at)::timestamptz AS finished_at FROM sync_runs
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND status IN ('success', 'partial');
