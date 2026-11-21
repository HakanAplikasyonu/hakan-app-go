-- İletişim merkezi (#780 A): ofis NetGSM ayarı ve giden SMS / e-posta kaydı.

-- name: GetOfficeSmsSettings :one
SELECT * FROM office_sms_settings WHERE organization_id = @organization_id;

-- name: UpsertOfficeSmsSettings :one
INSERT INTO office_sms_settings (organization_id, enabled, test_mode, usercode, password_ref, msgheader, encoding, appname, updated_by, updated_at)
VALUES (@organization_id, @enabled, @test_mode, @usercode, sqlc.narg(password_ref), @msgheader, @encoding, @appname, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled, test_mode = EXCLUDED.test_mode, usercode = EXCLUDED.usercode,
    password_ref = EXCLUDED.password_ref, msgheader = EXCLUDED.msgheader, encoding = EXCLUDED.encoding,
    appname = EXCLUDED.appname, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;

-- name: SetOfficeSmsTestResult :one
UPDATE office_sms_settings
SET last_test_at = NOW(), last_test_ok = @ok, last_test_code = @code, last_test_headers = @headers::text[]
WHERE organization_id = @organization_id
RETURNING *;

-- name: InsertSmsMessage :one
INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, error_code, error_message,
    source_rule, source_ref, sent_by, bulk_id)
VALUES (@organization_id, sqlc.narg(engagement_id), @phone, @body, @msgheader, @status, @error_code, @error_message,
    @source_rule, @source_ref, sqlc.narg(sent_by), sqlc.narg(bulk_id))
RETURNING *;

-- name: FinishSmsMessage :one
UPDATE sms_messages
SET status = @status, provider_job_id = @provider_job_id, error_code = @error_code, error_message = @error_message,
    sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: ListOfficeSmsMessages :many
SELECT m.*, COALESCE(t.title, '')::text AS taxpayer_title, COALESCE(e.uuid::text, '')::text AS engagement_uuid
FROM sms_messages m
LEFT JOIN engagements e ON e.organization_id = m.organization_id AND e.id = m.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE m.organization_id = @organization_id
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: CountOfficeSmsMessagesSince :one
SELECT count(*)::bigint FROM sms_messages
WHERE organization_id = @organization_id AND created_at >= @since AND status IN ('sent', 'uncertain');

-- name: InsertMailMessage :one
INSERT INTO mail_messages (organization_id, engagement_id, to_email, subject, body, status, error_message,
    source_rule, source_ref, sent_by, bulk_id)
VALUES (@organization_id, sqlc.narg(engagement_id), @to_email, @subject, @body, @status, @error_message,
    @source_rule, @source_ref, sqlc.narg(sent_by), sqlc.narg(bulk_id))
RETURNING *;

-- name: FinishMailMessage :one
UPDATE mail_messages
SET status = @status, error_message = @error_message, sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: GetEngagementContactForMessage :one
SELECT e.id AS engagement_id, t.title, COALESCE(t.contact_phone, '')::text AS contact_phone,
    COALESCE(t.contact_email::text, '')::text AS contact_email
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.uuid = @engagement_uuid;

-- İletişim geçmişi (#780 D): mükellefe giden SMS ve e-postalar, yeniden eskiye.

-- name: ListEngagementSmsMessages :many
SELECT m.*, COALESCE(NULLIF(TRIM(u.name || ' ' || u.surname), ''), '')::text AS sent_by_name
FROM sms_messages m
LEFT JOIN users u ON u.id = m.sent_by
WHERE m.organization_id = @organization_id AND m.engagement_id = @engagement_id
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: ListEngagementMailMessages :many
SELECT m.*, COALESCE(NULLIF(TRIM(u.name || ' ' || u.surname), ''), '')::text AS sent_by_name
FROM mail_messages m
LEFT JOIN users u ON u.id = m.sent_by
WHERE m.organization_id = @organization_id AND m.engagement_id = @engagement_id
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- İletim raporu taraması (#780 D): NetGSM'in kabul ettiği, henüz iletim
-- bilgisi gelmemiş son günlerin SMS'leri. Bütün ofisler (rls.System içinde).
-- tenant-lint: cross-org NetGSM iletim raporu taraması, bütün ofisler
-- name: ListSmsAwaitingDelivery :many
SELECT id, organization_id, provider_job_id, phone
FROM sms_messages
WHERE status = 'sent' AND delivered_at IS NULL AND provider_job_id <> '' AND sent_at >= @since
ORDER BY organization_id, sent_at, id
LIMIT @lim;

-- name: MarkSmsDelivered :execrows
UPDATE sms_messages SET delivered_at = @delivered_at
WHERE organization_id = @organization_id AND id = @id AND delivered_at IS NULL;

-- name: MarkSmsDeliveryFailed :execrows
UPDATE sms_messages SET status = 'failed', error_code = @error_code, error_message = @error_message
WHERE organization_id = @organization_id AND id = @id AND status = 'sent' AND delivered_at IS NULL;

-- E-posta izleme (#780 D): piksel ve bağlantı oturum açmadan gelir; anahtar
-- tahmin edilemez track_token'dır. Zaman yalnız ilk seferde yazılır.
-- tenant-lint: cross-org izleme pikseli, oturumsuz, track_token benzersiz
-- name: MarkMailOpenedByToken :execrows
UPDATE mail_messages SET opened_at = NOW()
WHERE track_token = @track_token AND opened_at IS NULL;

-- tenant-lint: cross-org ek bağlantısı, oturumsuz, track_token benzersiz
-- name: MarkMailDownloadedByToken :execrows
UPDATE mail_messages
SET downloaded_at = COALESCE(downloaded_at, NOW()), opened_at = COALESCE(opened_at, NOW())
WHERE track_token = @track_token AND (downloaded_at IS NULL OR opened_at IS NULL);
