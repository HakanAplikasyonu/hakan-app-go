-- Bağlantı sağlığı uyarıları (#669 K): INT_CONNECTION_FAILED, INT_SYNC_STALE.
-- Üretici control:source_connection; uyarı varlığı source_connection.

-- tenant-lint: cross-org zamanlanmış iş bütün ofislerin bağlantısı olan (ya da açık bağlantı uyarısı kalan) mükelleflerini tarar
-- name: ListConnectionWarningSubjects :many
SELECT e.organization_id, e.id AS engagement_id, e.taxpayer_id
FROM engagements e
WHERE e.status = 'active'
  AND (EXISTS (SELECT 1 FROM source_connections sc WHERE sc.taxpayer_id = e.taxpayer_id)
       OR EXISTS (SELECT 1 FROM data_warnings w
                  WHERE w.organization_id = e.organization_id AND w.engagement_id = e.id
                    AND w.producer = 'control:source_connection' AND w.status = 'open'))
ORDER BY e.organization_id, e.id;

-- name: AutoResolveOtherConnectionWarnings :execrows
-- Bağlantı uyarısı içinde bulunulan aya yazılır. Önceki aylarda açık kalanlar
-- ve artık bulunmayan bağlantılarınkiler kendiliğinden kapanır.
UPDATE data_warnings
SET status = 'auto_resolved', resolved_at = NOW(), resolved_by = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND producer = 'control:source_connection' AND entity_type = 'source_connection'
  AND status = 'open'
  AND NOT (entity_id = ANY(@connection_ids::bigint[]) AND COALESCE(period, '') = @period::text);

-- name: ListOpenConnectionFailedWarnings :many
-- Açık bağlantı hatası uyarıları ve parametreleri (#688): bağlantı düzelince
-- uyarı hemen değil, RecoverHold sonra kapanır; bekleme başlangıcı
-- parametrelerde (recovered_at) durur.
SELECT entity_id, params FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND producer = 'control:source_connection' AND entity_type = 'source_connection'
  AND code = 'INT_CONNECTION_FAILED' AND status = 'open'
  AND COALESCE(period, '') = @period::text;
