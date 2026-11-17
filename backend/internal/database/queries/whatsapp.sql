-- WhatsApp Web uzantısı (issue #388): sohbet → mükellef eşlemesi ve uzantı
-- kayıt izi. İstekler RLS kapsamında (whatsapp_* zorlanmış, issue #166)
-- çalışır; ofis ve engagement süzgeçleri burada da açıkça yazılır.

-- tenant-lint: org-wide sohbet anahtarı ofis içinde tekil; görünmeyen engagement'ın satırı RLS ile düşer
-- name: GetWhatsAppChatLink :one
SELECT l.uuid, l.chat_key, l.label, l.engagement_id, l.updated_at,
       e.uuid AS engagement_uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id
FROM whatsapp_chat_links l
JOIN engagements e ON e.organization_id = l.organization_id AND e.id = l.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE l.organization_id = @organization_id AND l.chat_key = @chat_key AND e.ended_at IS NULL;

-- name: UpsertWhatsAppChatLink :one
INSERT INTO whatsapp_chat_links (organization_id, engagement_id, chat_key, label, created_by, updated_by)
VALUES (@organization_id, @engagement_id, @chat_key, @label, sqlc.narg(user_id), sqlc.narg(user_id))
ON CONFLICT (organization_id, chat_key) DO UPDATE
    SET engagement_id = EXCLUDED.engagement_id, label = EXCLUDED.label, updated_by = EXCLUDED.updated_by
RETURNING uuid;

-- tenant-lint: org-wide ofisin eşleme listesi; görünmeyen engagement'ların satırı RLS ile düşer
-- name: ListWhatsAppChatLinks :many
SELECT l.uuid, l.chat_key, l.label, l.updated_at,
       e.uuid AS engagement_uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id,
       count(*) OVER ()::bigint AS total
FROM whatsapp_chat_links l
JOIN engagements e ON e.organization_id = l.organization_id AND e.id = l.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE l.organization_id = @organization_id
ORDER BY l.updated_at DESC, l.id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide eşleme uuid ile silinir; görünmeyen engagement'ın satırı RLS ile düşer
-- name: DeleteWhatsAppChatLink :execrows
DELETE FROM whatsapp_chat_links WHERE organization_id = @organization_id AND uuid = @uuid;

-- tenant-lint: org-wide ilk eşlemede öneri: iletişim telefonunun son 10 hanesi sohbet numarasıyla aynı mükellefler
-- name: SuggestWhatsAppTaxpayers :many
SELECT e.uuid AS engagement_uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
  AND t.contact_phone IS NOT NULL
  AND right(regexp_replace(t.contact_phone, '[^0-9]', '', 'g'), 10) = right(@phone::text, 10)
  AND length(regexp_replace(t.contact_phone, '[^0-9]', '', 'g')) >= 10
ORDER BY t.title
LIMIT 5;

-- name: InsertWhatsAppCapture :one
INSERT INTO whatsapp_captures (
    organization_id, engagement_id, taxpayer_id, target, chat_key, file_name, content_type,
    size_bytes, sha256, attachment_id, receipt_capture_id, document_task_id, created_by
) VALUES (
    @organization_id, @engagement_id, @taxpayer_id, @target, sqlc.narg(chat_key), @file_name, @content_type,
    @size_bytes, @sha256,
    (SELECT a.id FROM attachments a WHERE a.taxpayer_id = @taxpayer_id AND a.uuid = sqlc.narg(attachment_uuid)::uuid),
    (SELECT rc.id FROM receipt_captures rc
       WHERE rc.organization_id = @organization_id AND rc.engagement_id = @engagement_id AND rc.uuid = sqlc.narg(receipt_uuid)::uuid),
    (SELECT dt.id FROM document_tasks dt
       WHERE dt.organization_id = @organization_id AND dt.engagement_id = @engagement_id AND dt.uuid = sqlc.narg(task_uuid)::uuid),
    sqlc.narg(created_by)
)
ON CONFLICT (organization_id, engagement_id, target, sha256) DO NOTHING
RETURNING uuid;

-- name: GetWhatsAppCaptureByFile :one
SELECT uuid
FROM whatsapp_captures
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target AND sha256 = @sha256;

-- tenant-lint: org-wide ofisin son uzantı kayıtları; engagement süzgeci isteğe bağlı, görünmeyenler RLS ile düşer
-- name: ListWhatsAppCaptures :many
SELECT c.uuid, c.target, c.file_name, c.content_type, c.size_bytes, c.created_at,
       e.uuid AS engagement_uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title,
       COALESCE(u.name, '')::text AS created_by_name,
       rc.uuid AS receipt_uuid, rc.status AS receipt_status,
       dt.uuid AS task_uuid, dt.title AS task_title,
       a.uuid AS attachment_uuid,
       count(*) OVER ()::bigint AS total
FROM whatsapp_captures c
JOIN engagements e ON e.organization_id = c.organization_id AND e.id = c.engagement_id
JOIN taxpayers t ON t.id = c.taxpayer_id
LEFT JOIN users u ON u.id = c.created_by
LEFT JOIN receipt_captures rc ON rc.id = c.receipt_capture_id
LEFT JOIN document_tasks dt ON dt.id = c.document_task_id
LEFT JOIN attachments a ON a.taxpayer_id = c.taxpayer_id AND a.id = c.attachment_id
WHERE c.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR c.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (sqlc.narg(target)::text IS NULL OR c.target = sqlc.narg(target)::text)
ORDER BY c.created_at DESC, c.id DESC
LIMIT @lim OFFSET @off;

-- name: GetWhatsAppCapture :one
SELECT c.uuid, c.target, c.file_name, c.content_type, c.size_bytes, c.created_at,
       e.uuid AS engagement_uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title,
       COALESCE(u.name, '')::text AS created_by_name,
       rc.uuid AS receipt_uuid, rc.status AS receipt_status,
       dt.uuid AS task_uuid, dt.title AS task_title,
       a.uuid AS attachment_uuid
FROM whatsapp_captures c
JOIN engagements e ON e.organization_id = c.organization_id AND e.id = c.engagement_id
JOIN taxpayers t ON t.id = c.taxpayer_id
LEFT JOIN users u ON u.id = c.created_by
LEFT JOIN receipt_captures rc ON rc.id = c.receipt_capture_id
LEFT JOIN document_tasks dt ON dt.id = c.document_task_id
LEFT JOIN attachments a ON a.taxpayer_id = c.taxpayer_id AND a.id = c.attachment_id
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id AND c.uuid = @uuid;

-- name: GetWhatsAppCaptureFile :one
SELECT a.storage_key, a.content_type, a.file_name, a.size_bytes
FROM whatsapp_captures c
JOIN attachments a ON a.taxpayer_id = c.taxpayer_id AND a.id = c.attachment_id
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id AND c.uuid = @uuid;
