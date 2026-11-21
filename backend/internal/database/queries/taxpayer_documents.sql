-- Belge arşivi ve İSG evrakları (#393). taxpayer_id kapsamlı.

-- name: InsertTaxpayerDocument :one
INSERT INTO taxpayer_documents (taxpayer_id, category, isg_kind, title, note, document_date, valid_until, storage_key,
                                sha256, size_bytes, content_type, file_name, visible_to_taxpayer, created_by, important_kind)
VALUES (@taxpayer_id, @category, sqlc.narg(isg_kind), @title, @note, sqlc.narg(document_date), sqlc.narg(valid_until), @storage_key,
        @sha256, @size_bytes, @content_type, @file_name, @visible_to_taxpayer, sqlc.narg(created_by), sqlc.narg(important_kind))
ON CONFLICT (taxpayer_id, sha256) DO NOTHING
RETURNING *;

-- name: ListTaxpayerDocuments :many
-- only_important: yalnız önemli belgeler (#780 B), bitişi yakın olan önce.
-- Arama başlık, dosya adı ve notta; expires_before verilirse yalnızca o
-- tarihte ya da önce biten evraklar, bitişi yakın olan önce.
SELECT d.*, count(*) OVER ()::bigint AS total_count
FROM taxpayer_documents d
WHERE d.taxpayer_id = @taxpayer_id
  AND (NOT @only_visible::bool OR d.visible_to_taxpayer)
  AND (NOT @only_important::bool OR d.important_kind IS NOT NULL)
  AND (sqlc.narg(category)::text IS NULL OR d.category = sqlc.narg(category)::text)
  AND (sqlc.narg(isg_kind)::text IS NULL OR d.isg_kind = sqlc.narg(isg_kind)::text)
  AND (sqlc.narg(q)::text IS NULL
       OR d.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR d.file_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR d.note ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(expires_before)::date IS NULL OR (d.valid_until IS NOT NULL AND d.valid_until <= sqlc.narg(expires_before)::date))
ORDER BY CASE WHEN sqlc.narg(expires_before)::date IS NOT NULL OR @only_important::bool THEN d.valid_until END ASC NULLS LAST,
         COALESCE(d.document_date, (d.created_at AT TIME ZONE 'Europe/Istanbul')::date) DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: CountTaxpayerDocuments :one
-- Sekme sayaçları ve bitiş uyarısı: kategori başına sayı, süresi dolan ve
-- soon tarihine kadar bitecek evrak (bütün arşiv ve yalnızca İSG).
-- only_visible: mükellef panelinde yalnızca "Mükellef de görsün" işaretliler.
SELECT count(*)::bigint AS total,
       count(*) FILTER (WHERE category = 'isg')::bigint AS isg,
       count(*) FILTER (WHERE valid_until < @today::date)::bigint AS expired,
       count(*) FILTER (WHERE valid_until >= @today::date AND valid_until <= @soon::date)::bigint AS expiring,
       count(*) FILTER (WHERE category = 'isg' AND valid_until < @today::date)::bigint AS isg_expired,
       count(*) FILTER (WHERE category = 'isg' AND valid_until >= @today::date AND valid_until <= @soon::date)::bigint AS isg_expiring,
       count(*) FILTER (WHERE important_kind IS NOT NULL)::bigint AS important,
       count(*) FILTER (WHERE important_kind IS NOT NULL AND valid_until <= @soon::date)::bigint AS important_expiring
FROM taxpayer_documents
WHERE taxpayer_id = @taxpayer_id
  AND (NOT @only_visible::bool OR visible_to_taxpayer);

-- name: GetTaxpayerDocument :one
SELECT * FROM taxpayer_documents WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: DeleteTaxpayerDocument :exec
DELETE FROM taxpayer_documents WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: UpdateTaxpayerDocument :one
-- Ofis düzenlemesi (#780 B): önemli işareti ve türü, "Mükellef de görsün",
-- ad, not ve geçerlilik bitişi. Değişmeyen alan aynen yazılır.
UPDATE taxpayer_documents
SET important_kind      = sqlc.narg(important_kind),
    visible_to_taxpayer = @visible_to_taxpayer,
    title               = @title,
    note                = @note,
    valid_until         = sqlc.narg(valid_until)
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: GetImportantDocumentSettings :one
-- "Önemli belgeler" bölümü ofis ayarı (#780 B); satırı yoksa açık.
SELECT * FROM important_document_settings WHERE organization_id = @organization_id;

-- name: UpsertImportantDocumentSettings :one
INSERT INTO important_document_settings (organization_id, enabled, updated_by, updated_at)
VALUES (@organization_id, @enabled, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
