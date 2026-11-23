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

-- Mükellef paneli / mobil "Mesajlarım" (#780): şirketin aktif ofisinden
-- kendisine giden SMS, e-posta ve uygulama bildirimleri, yeniden eskiye. Deneme modu (dry_run)
-- kayıtları gerçekte gitmediği için listelenmez. İç alanlar (gönderen
-- kullanıcı, sağlayıcı iş numarası, hata kodu, kaynak kural) seçilmez.
-- Durum kodu: failed, delivered (SMS iletim raporu geldi), sent, pending.

-- name: ListTaxpayerMessages :many
SELECT m.uuid, m.channel, m.subject, m.body, m.address, m.status, m.created_at, m.sent_at, m.delivered_at
FROM (
    SELECT s.uuid, 'sms'::text AS channel, ''::text AS subject, s.body, s.phone AS address,
           (CASE WHEN s.status = 'failed' THEN 'failed'
                 WHEN s.delivered_at IS NOT NULL THEN 'delivered'
                 WHEN s.status = 'sent' THEN 'sent'
                 ELSE 'pending' END)::text AS status,
           s.created_at, s.sent_at, s.delivered_at
    FROM sms_messages s
    WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id AND s.status <> 'dry_run'
    UNION ALL
    SELECT ml.uuid, 'email'::text, ml.subject, ml.body, ml.to_email::text,
           (CASE WHEN ml.status = 'failed' THEN 'failed'
                 WHEN ml.status = 'sent' THEN 'sent'
                 ELSE 'pending' END)::text,
           ml.created_at, ml.sent_at, NULL::timestamptz
    FROM mail_messages ml
    WHERE ml.organization_id = @organization_id AND ml.engagement_id = @engagement_id AND ml.status <> 'dry_run'
    UNION ALL
    SELECT a.uuid, 'push'::text, a.title, a.body, ''::text, 'sent'::text, a.created_at, a.sent_at, NULL::timestamptz
    FROM push_messages a
    WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id AND a.status = 'sent'
) m
ORDER BY m.created_at DESC, m.uuid DESC
LIMIT @lim OFFSET @off;

-- name: CountTaxpayerMessages :one
SELECT ((SELECT count(*) FROM sms_messages s
         WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id AND s.status <> 'dry_run')
      + (SELECT count(*) FROM mail_messages ml
         WHERE ml.organization_id = @organization_id AND ml.engagement_id = @engagement_id AND ml.status <> 'dry_run')
      + (SELECT count(*) FROM push_messages a
         WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id AND a.status = 'sent'))::bigint AS total;

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

-- Uygulama bildirimi kaydı (belge talebi kanalları): iletişim geçmişinde
-- "uygulama" kanalı. Bildirimin kendisi notifications tablosundadır; bu
-- satır gönderim kanıtıdır.

-- name: InsertPushMessage :one
INSERT INTO push_messages (organization_id, engagement_id, title, body, recipients, status, error_code, error_message,
    source_rule, source_ref, sent_by, sent_at)
VALUES (@organization_id, sqlc.narg(engagement_id), @title, @body, @recipients, @status, @error_code, @error_message,
    @source_rule, @source_ref, sqlc.narg(sent_by), sqlc.narg(sent_at))
RETURNING *;

-- name: ListEngagementPushMessages :many
SELECT m.*, COALESCE(NULLIF(TRIM(u.name || ' ' || u.surname), ''), '')::text AS sent_by_name
FROM push_messages m
LEFT JOIN users u ON u.id = m.sent_by
WHERE m.organization_id = @organization_id AND m.engagement_id = @engagement_id
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- Kaynak kayıt başına her kanalın son mesajı (belge talebinin kanal
-- durumu): kaynak kural ve kaynak kayıt (görev uuid'i) ile süzülür.
-- name: LatestMessagesBySource :many
SELECT DISTINCT ON (x.source_ref, x.channel)
       x.source_ref::text AS source_ref, x.source_rule::text AS source_rule, x.channel::text AS channel, x.status::text AS status,
       x.error_code::text AS error_code, x.error_message::text AS error_message,
       x.created_at::timestamptz AS created_at, x.delivered_at::timestamptz AS delivered_at
FROM (
    SELECT s.source_ref, s.source_rule, 'sms'::text AS channel, s.status, s.error_code, s.error_message, s.created_at, s.id,
           s.delivered_at
    FROM sms_messages s
    WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id
      AND s.source_rule = ANY (@source_rules::text[]) AND s.source_ref = ANY (@source_refs::text[])
    UNION ALL
    SELECT ml.source_ref, ml.source_rule, 'email'::text, ml.status, ''::text, ml.error_message, ml.created_at, ml.id, NULL::timestamptz
    FROM mail_messages ml
    WHERE ml.organization_id = @organization_id AND ml.engagement_id = @engagement_id
      AND ml.source_rule = ANY (@source_rules::text[]) AND ml.source_ref = ANY (@source_refs::text[])
    UNION ALL
    SELECT a.source_ref, a.source_rule, 'push'::text, a.status, a.error_code, a.error_message, a.created_at, a.id, NULL::timestamptz
    FROM push_messages a
    WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
      AND a.source_rule = ANY (@source_rules::text[]) AND a.source_ref = ANY (@source_refs::text[])
) x
ORDER BY x.source_ref, x.channel, x.created_at DESC, x.id DESC;
