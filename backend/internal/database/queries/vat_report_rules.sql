-- KDV raporu kuralları (#780 B).

-- name: ListVATReportRules :many
SELECT * FROM vat_report_rules WHERE organization_id = @organization_id ORDER BY id;

-- name: CountVATReportRules :one
SELECT COUNT(*)::bigint FROM vat_report_rules WHERE organization_id = @organization_id;

-- name: InsertVATReportRule :one
INSERT INTO vat_report_rules (organization_id, trigger_kind, day, days_before, audience, scope, group_ref,
    engagement_uuids, label, enabled, updated_by)
VALUES (@organization_id, @trigger_kind, sqlc.narg(day), sqlc.narg(days_before), @audience, @scope, sqlc.narg(group_ref),
    @engagement_uuids::uuid[], sqlc.narg(label), @enabled, sqlc.narg(updated_by))
RETURNING *;

-- name: DeleteVATReportRules :exec
DELETE FROM vat_report_rules WHERE organization_id = @organization_id;

-- tenant-lint: cross-org günlük KDV raporu taraması tüm ofislerin etkin kurallarını rls.System içinde okur
-- name: ListActiveVATReportRules :many
-- Raporu açık ofislerin etkin kuralları.
SELECT r.*, o.slug AS organization_slug
FROM vat_report_rules r
INNER JOIN organizations o ON o.id = r.organization_id
LEFT JOIN vat_tracking_settings s ON s.organization_id = r.organization_id
WHERE r.enabled AND COALESCE(s.report_enabled, TRUE)
ORDER BY r.organization_id, r.id;

-- name: ListVATReportTargets :many
-- Bir ofisin etkin mükellef dosyaları (kural kapsamı kodda süzülür).
SELECT e.id AS engagement_id, e.uuid AS engagement_uuid, t.id AS taxpayer_id, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.status = 'active'
ORDER BY t.title, e.id;

-- tenant-lint: cross-org günlük KDV raporu taraması kural tanımlamamış ofisleri varsayılan (ayın report_day'i) kuralla çalıştırır
-- name: ListVATReportDefaultOrgs :many
-- Kural tanımlamamış, raporu açık ve etkin mükellefi olan ofisler.
SELECT o.id AS organization_id, o.slug AS organization_slug, COALESCE(s.report_day, 29)::int AS report_day
FROM organizations o
LEFT JOIN vat_tracking_settings s ON s.organization_id = o.id
WHERE COALESCE(s.report_enabled, TRUE)
  AND NOT EXISTS (SELECT 1 FROM vat_report_rules r WHERE r.organization_id = o.id)
  AND EXISTS (SELECT 1 FROM engagements e WHERE e.organization_id = o.id AND e.status = 'active')
ORDER BY o.id;
