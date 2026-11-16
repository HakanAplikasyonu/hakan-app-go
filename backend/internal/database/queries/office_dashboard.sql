-- Müşavir mobil ana panosu (issue #237). Ofis çapında sayılar; read_all
-- izni olmayan üye yalnızca atandığı engagement'ları görür. İstek RLS
-- kapsamında (issue #166) çalışır, süzgeç burada da açıkça yazılır.

-- tenant-lint: org-wide ofis panosu sayıları; read_all yoksa engagement_assignments ile süzülür
-- name: OfficeDashboardCounts :one
WITH visible AS (
    SELECT e.id FROM engagements e
    WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
      AND (@all_engagements::bool OR e.id IN (
            SELECT a.engagement_id FROM engagement_assignments a
            WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
)
SELECT
    (SELECT count(*) FROM suggestions s
       WHERE s.organization_id = @organization_id AND s.engagement_id IN (SELECT id FROM visible)
         AND s.status = 'pending')::bigint AS review_pending,
    (SELECT count(*) FROM document_tasks d
       WHERE d.organization_id = @organization_id AND d.engagement_id IN (SELECT id FROM visible)
         AND d.status = 'open')::bigint AS doc_tasks_open,
    (SELECT count(*) FROM document_tasks d
       WHERE d.organization_id = @organization_id AND d.engagement_id IN (SELECT id FROM visible)
         AND d.status = 'submitted')::bigint AS doc_tasks_submitted,
    (SELECT count(*) FROM document_tasks d
       WHERE d.organization_id = @organization_id AND d.engagement_id IN (SELECT id FROM visible)
         AND d.status = 'open' AND d.due_at < @now::timestamptz)::bigint AS doc_tasks_overdue,
    (SELECT count(*) FROM data_warnings w
       WHERE w.organization_id = @organization_id AND w.engagement_id IN (SELECT id FROM visible)
         AND w.status = 'open' AND w.severity = 'blocker')::bigint AS warnings_blocker,
    (SELECT count(*) FROM data_warnings w
       WHERE w.organization_id = @organization_id AND w.engagement_id IN (SELECT id FROM visible)
         AND w.status = 'open' AND w.severity = 'warning')::bigint AS warnings_warning,
    (SELECT count(*) FROM data_warnings w
       WHERE w.organization_id = @organization_id AND w.engagement_id IN (SELECT id FROM visible)
         AND w.status = 'open' AND w.severity = 'info')::bigint AS warnings_info,
    (SELECT count(*) FROM receipt_captures r
       WHERE r.organization_id = @organization_id AND r.engagement_id IN (SELECT id FROM visible)
         AND r.created_at >= @since::timestamptz)::bigint AS uploads_receipts,
    (SELECT count(*) FROM ubl_upload_jobs u
       WHERE u.organization_id = @organization_id AND u.engagement_id IN (SELECT id FROM visible)
         AND u.created_at >= @since::timestamptz)::bigint AS uploads_invoices,
    (SELECT count(*) FROM document_task_uploads t
       WHERE t.organization_id = @organization_id AND t.engagement_id IN (SELECT id FROM visible)
         AND t.created_at >= @since::timestamptz)::bigint AS uploads_documents;

-- tenant-lint: org-wide ofis panosu son yüklemeler; read_all yoksa engagement_assignments ile süzülür
-- name: OfficeDashboardRecentUploads :many
WITH visible AS (
    SELECT e.id, e.uuid, e.taxpayer_id FROM engagements e
    WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
      AND (@all_engagements::bool OR e.id IN (
            SELECT a.engagement_id FROM engagement_assignments a
            WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
), uploads AS (
    SELECT 'receipt'::text AS kind, r.uuid, r.engagement_id, r.status, NULL::uuid AS task_uuid, r.created_at
    FROM receipt_captures r
    WHERE r.organization_id = @organization_id AND r.engagement_id IN (SELECT id FROM visible)
      AND r.created_at >= @since::timestamptz
    UNION ALL
    SELECT 'invoice'::text, u.uuid, u.engagement_id, u.status, NULL::uuid, u.created_at
    FROM ubl_upload_jobs u
    WHERE u.organization_id = @organization_id AND u.engagement_id IN (SELECT id FROM visible)
      AND u.created_at >= @since::timestamptz
    UNION ALL
    SELECT 'document'::text, t.uuid, t.engagement_id, d.status, d.uuid, t.created_at
    FROM document_task_uploads t
    JOIN document_tasks d ON d.organization_id = t.organization_id AND d.id = t.task_id
    WHERE t.organization_id = @organization_id AND t.engagement_id IN (SELECT id FROM visible)
      AND t.created_at >= @since::timestamptz
)
SELECT x.kind, x.uuid, v.uuid AS engagement_uuid, tp.uuid AS taxpayer_uuid, tp.title AS taxpayer_title,
       x.status, x.task_uuid, x.created_at
FROM uploads x
JOIN visible v ON v.id = x.engagement_id
JOIN taxpayers tp ON tp.id = v.taxpayer_id
ORDER BY x.created_at DESC, x.uuid
LIMIT @lim;
