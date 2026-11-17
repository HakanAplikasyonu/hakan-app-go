-- Belge arşivi ve İSG evrakları (#393). taxpayer_id kapsamlı.

-- name: InsertTaxpayerDocument :one
INSERT INTO taxpayer_documents (taxpayer_id, category, isg_kind, title, note, document_date, valid_until, storage_key,
                                sha256, size_bytes, content_type, file_name, visible_to_taxpayer, created_by)
VALUES (@taxpayer_id, @category, sqlc.narg(isg_kind), @title, @note, sqlc.narg(document_date), sqlc.narg(valid_until), @storage_key,
        @sha256, @size_bytes, @content_type, @file_name, @visible_to_taxpayer, sqlc.narg(created_by))
ON CONFLICT (taxpayer_id, sha256) DO NOTHING
RETURNING *;

-- name: ListTaxpayerDocuments :many
-- Arama başlık, dosya adı ve notta; expires_before verilirse yalnızca o
-- tarihte ya da önce biten evraklar, bitişi yakın olan önce.
SELECT d.*, count(*) OVER ()::bigint AS total_count
FROM taxpayer_documents d
WHERE d.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(category)::text IS NULL OR d.category = sqlc.narg(category)::text)
  AND (sqlc.narg(isg_kind)::text IS NULL OR d.isg_kind = sqlc.narg(isg_kind)::text)
  AND (sqlc.narg(q)::text IS NULL
       OR d.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR d.file_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR d.note ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(expires_before)::date IS NULL OR (d.valid_until IS NOT NULL AND d.valid_until <= sqlc.narg(expires_before)::date))
ORDER BY CASE WHEN sqlc.narg(expires_before)::date IS NOT NULL THEN d.valid_until END ASC NULLS LAST,
         COALESCE(d.document_date, (d.created_at AT TIME ZONE 'Europe/Istanbul')::date) DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: CountTaxpayerDocuments :one
-- Sekme sayaçları ve bitiş uyarısı: kategori başına sayı, süresi dolan ve
-- soon tarihine kadar bitecek evrak (bütün arşiv ve yalnızca İSG).
SELECT count(*)::bigint AS total,
       count(*) FILTER (WHERE category = 'isg')::bigint AS isg,
       count(*) FILTER (WHERE valid_until < @today::date)::bigint AS expired,
       count(*) FILTER (WHERE valid_until >= @today::date AND valid_until <= @soon::date)::bigint AS expiring,
       count(*) FILTER (WHERE category = 'isg' AND valid_until < @today::date)::bigint AS isg_expired,
       count(*) FILTER (WHERE category = 'isg' AND valid_until >= @today::date AND valid_until <= @soon::date)::bigint AS isg_expiring
FROM taxpayer_documents
WHERE taxpayer_id = @taxpayer_id;

-- name: GetTaxpayerDocument :one
SELECT * FROM taxpayer_documents WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: DeleteTaxpayerDocument :exec
DELETE FROM taxpayer_documents WHERE taxpayer_id = @taxpayer_id AND id = @id;
