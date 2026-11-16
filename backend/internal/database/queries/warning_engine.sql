-- Uyarı motoru (issue #13) sorguları. Tablo #9'da (data_warnings).

-- name: GetLatestDataWarning :one
-- Aynı anahtarın (05 §1: engagement, varlık, kod, dönem) en son kaydı.
SELECT * FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND entity_type = @entity_type
  AND entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::bigint
  AND code = @code AND COALESCE(period, '') = @period::text
ORDER BY id DESC
LIMIT 1;

-- name: AutoResolveDataWarnings :execrows
-- Koşul ortadan kalktı: üreticinin bu varlıktaki, artık bulunmayan kodlu açık uyarıları.
UPDATE data_warnings
SET status = 'auto_resolved', resolved_at = NOW(), resolved_by = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND producer = @producer AND entity_type = @entity_type
  AND entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::bigint
  AND COALESCE(period, '') = @period::text
  AND status = 'open'
  AND code = ANY(@codes::text[])
  AND NOT (code = ANY(@keep::text[]));

-- tenant-lint: org-wide ofis uyarı listesi; read_all yoksa engagement_assignments ile süzülür
-- name: ListTenantDataWarnings :many
SELECT w.id, w.uuid, w.engagement_id, e.uuid AS engagement_uuid, w.period, w.entity_type, w.entity_id,
       inv.uuid AS invoice_uuid, cust.uuid AS customer_uuid,
       w.code, w.severity, w.params, w.producer, w.status, w.resolved_at, w.created_at, w.updated_at
FROM data_warnings w
JOIN engagements e ON e.organization_id = w.organization_id AND e.id = w.engagement_id
LEFT JOIN invoices inv ON w.entity_type = 'invoice' AND inv.id = w.entity_id AND inv.taxpayer_id = w.taxpayer_id
LEFT JOIN customers cust ON w.entity_type = 'customer' AND cust.id = w.entity_id AND cust.taxpayer_id = w.taxpayer_id
WHERE w.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR w.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR w.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(period)::text IS NULL OR w.period = sqlc.narg(period)::text)
  AND (sqlc.narg(code)::text IS NULL OR w.code = sqlc.narg(code)::text)
  AND (sqlc.narg(severity)::text IS NULL OR w.severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(status)::text IS NULL OR w.status = sqlc.narg(status)::text)
  AND (sqlc.narg(entity_type)::text IS NULL OR w.entity_type = sqlc.narg(entity_type)::text)
ORDER BY CASE w.severity WHEN 'blocker' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
         w.updated_at DESC, w.id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofis uyarı listesi sayımı; ListTenantDataWarnings ile aynı süzgeç
-- name: CountTenantDataWarnings :one
SELECT count(*)::bigint
FROM data_warnings w
WHERE w.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR w.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR w.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(period)::text IS NULL OR w.period = sqlc.narg(period)::text)
  AND (sqlc.narg(code)::text IS NULL OR w.code = sqlc.narg(code)::text)
  AND (sqlc.narg(severity)::text IS NULL OR w.severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(status)::text IS NULL OR w.status = sqlc.narg(status)::text)
  AND (sqlc.narg(entity_type)::text IS NULL OR w.entity_type = sqlc.narg(entity_type)::text);

-- tenant-lint: org-wide ofis uyarı grupları (#333); ListTenantDataWarnings ile aynı görünürlük süzgeci
-- name: GroupTenantDataWarnings :many
-- Kod başına sayım: en ağır seviye, uyarı ve engagement sayısı, son güncelleme.
SELECT w.code,
       (CASE max(CASE w.severity WHEN 'blocker' THEN 3 WHEN 'warning' THEN 2 ELSE 1 END)
            WHEN 3 THEN 'blocker' WHEN 2 THEN 'warning' ELSE 'info' END)::text AS severity,
       count(*)::bigint AS warning_count,
       count(DISTINCT w.engagement_id)::bigint AS engagement_count,
       max(w.updated_at)::timestamptz AS latest_at
FROM data_warnings w
WHERE w.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR w.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR w.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(period)::text IS NULL OR w.period = sqlc.narg(period)::text)
  AND (sqlc.narg(severity)::text IS NULL OR w.severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(status)::text IS NULL OR w.status = sqlc.narg(status)::text)
GROUP BY w.code
ORDER BY max(CASE w.severity WHEN 'blocker' THEN 3 WHEN 'warning' THEN 2 ELSE 1 END) DESC,
         count(*) DESC, w.code;

-- tenant-lint: org-wide toplu "görüldü" (#333); read_all yoksa engagement_assignments ile süzülür, yalnızca aktif engagement
-- name: AcknowledgeTenantDataWarningsByCode :execrows
-- DecideDataWarning'in toplu hâli: kapsamdaki bu kodlu bütün açık uyarılar.
UPDATE data_warnings w
SET status = 'acknowledged', resolved_by = sqlc.narg(resolved_by), resolved_at = NOW()
WHERE w.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR w.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR w.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND w.engagement_id IN (
        SELECT e.id FROM engagements e
        WHERE e.organization_id = @organization_id AND e.status = 'active')
  AND w.code = @code AND w.status = 'open'
  AND (sqlc.narg(period)::text IS NULL OR w.period = sqlc.narg(period)::text);
