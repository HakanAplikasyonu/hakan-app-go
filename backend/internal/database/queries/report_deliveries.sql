-- Mükellef raporu gönderimleri (issue #386).

-- name: InsertReportDelivery :one
INSERT INTO report_deliveries (organization_id, engagement_id, taxpayer_id, kind, period, recipient, origin, report, office_name, requested_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @kind, @period, @recipient, @origin, @report, @office_name, sqlc.narg(requested_by))
RETURNING *;

-- name: ListEngagementReportDeliveries :many
SELECT * FROM report_deliveries
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: CountEngagementReportDeliveries :one
SELECT count(*)::bigint FROM report_deliveries
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;

-- tenant-lint: org-wide ofis gönderim geçmişi; read_all yoksa engagement_assignments ile süzülür
-- name: ListOfficeReportDeliveries :many
SELECT d.*, e.uuid AS engagement_uuid, t.title AS taxpayer_title
FROM report_deliveries d
JOIN engagements e ON e.organization_id = d.organization_id AND e.id = d.engagement_id
JOIN taxpayers t ON t.id = d.taxpayer_id
WHERE d.organization_id = @organization_id
  AND (@all_engagements::bool OR d.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
ORDER BY d.created_at DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofis gönderim geçmişi sayımı; ListOfficeReportDeliveries ile aynı süzgeç
-- name: CountOfficeReportDeliveries :one
SELECT count(*)::bigint
FROM report_deliveries d
WHERE d.organization_id = @organization_id
  AND (@all_engagements::bool OR d.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id));

-- tenant-lint: cross-org worker taraması (rls.System); sırası gelen gönderimleri sahiplenir
-- name: ClaimReportDeliveries :many
-- Sırası gelen ya da 10 dakikadır "sending"de takılı kalan gönderimler.
WITH picked AS (
    SELECT id FROM report_deliveries
    WHERE (status = 'queued' AND next_attempt_at <= NOW())
       OR (status = 'sending' AND updated_at < NOW() - INTERVAL '10 minutes')
    ORDER BY next_attempt_at, id
    LIMIT @batch_limit
    FOR UPDATE SKIP LOCKED
)
UPDATE report_deliveries d
SET status = 'sending', attempts = d.attempts + 1
FROM picked
WHERE d.id = picked.id
RETURNING d.*;

-- tenant-lint: cross-org worker taraması; satır ClaimReportDeliveries ile sahiplenildi
-- name: MarkReportDeliverySent :exec
UPDATE report_deliveries SET status = 'sent', sent_at = NOW(), last_error = NULL
WHERE id = @id AND status = 'sending';

-- tenant-lint: cross-org worker taraması; satır ClaimReportDeliveries ile sahiplenildi
-- name: MarkReportDeliveryRetry :exec
UPDATE report_deliveries SET status = 'queued', last_error = @last_error, next_attempt_at = @next_attempt_at
WHERE id = @id AND status = 'sending';

-- tenant-lint: cross-org worker taraması; satır ClaimReportDeliveries ile sahiplenildi
-- name: MarkReportDeliveryFailed :exec
UPDATE report_deliveries SET status = 'failed', last_error = @last_error
WHERE id = @id AND status = 'sending';
