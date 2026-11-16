-- Müşavir mobil mükellef listesi ve ayrıntısı (issue #238). İstekler RLS
-- kapsamında (issue #166) çalışır; süzgeçler burada da açıkça yazılır.

-- tenant-lint: org-wide ofis mükellef listesi; read_all yoksa engagement_assignments ile süzülür
-- name: ListOfficeClients :many
SELECT e.id AS engagement_id, e.uuid AS engagement_uuid,
       t.uuid AS taxpayer_uuid, t.kind, t.tax_id, t.title, t.tax_office,
       (SELECT count(*) FROM data_warnings w
          WHERE w.organization_id = @organization_id AND w.engagement_id = e.id AND w.status = 'open')::bigint AS open_warnings,
       (SELECT count(*) FROM data_warnings w
          WHERE w.organization_id = @organization_id AND w.engagement_id = e.id AND w.status = 'open'
            AND w.severity = 'blocker')::bigint AS open_blockers,
       (SELECT count(*) FROM document_tasks d
          WHERE d.organization_id = @organization_id AND d.engagement_id = e.id
            AND d.status IN ('open', 'submitted'))::bigint AS open_tasks,
       (e.id IN (SELECT a.engagement_id FROM engagement_assignments a
                 WHERE a.organization_id = @organization_id AND a.user_id = @user_id))::bool AS assigned_to_me
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (NOT @only_mine::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(search)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(search)::text || '%' ESCAPE '\'
       OR t.tax_id LIKE sqlc.narg(search)::text || '%' ESCAPE '\')
  AND (NOT @only_warnings::bool OR EXISTS (
        SELECT 1 FROM data_warnings w
        WHERE w.organization_id = @organization_id AND w.engagement_id = e.id AND w.status = 'open'))
  AND (NOT @only_tasks::bool OR EXISTS (
        SELECT 1 FROM document_tasks d
        WHERE d.organization_id = @organization_id AND d.engagement_id = e.id AND d.status IN ('open', 'submitted')))
  AND (sqlc.narg(cursor_title)::text IS NULL
       OR (t.title, e.id) > (sqlc.narg(cursor_title)::text, sqlc.narg(cursor_id)::bigint))
ORDER BY t.title, e.id
LIMIT @lim;

-- name: GetEngagementTaxpayerContact :one
SELECT t.uuid, t.kind, t.tax_id, t.title, t.tax_office, COALESCE(t.contact_phone, '')::text AS contact_phone, COALESCE(t.contact_email::text, '')::text AS contact_email
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.id = @engagement_id;

-- name: ListEngagementTaxpayerMembers :many
SELECT u.uuid, u.name, u.surname, u.email::text AS email, m.role
FROM engagements e
JOIN taxpayer_members m ON m.taxpayer_id = e.taxpayer_id
JOIN users u ON u.id = m.user_id
WHERE e.organization_id = @organization_id AND e.id = @engagement_id
  AND u.status = 'active' AND u.deactivated_at IS NULL
ORDER BY m.role DESC, u.name, u.id
LIMIT 20;

-- name: GetEngagementReceiptImage :one
SELECT a.storage_key, a.content_type, a.size_bytes
FROM receipt_captures r
JOIN attachments a ON a.taxpayer_id = r.taxpayer_id AND a.id = r.attachment_id
WHERE r.organization_id = @organization_id AND r.engagement_id = @engagement_id AND r.uuid = @receipt_uuid;
