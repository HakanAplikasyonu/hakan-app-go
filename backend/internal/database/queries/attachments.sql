-- name: UpsertAttachment :one
-- Tekil (taxpayer_id, sha256): aynı dosya ikinci kez gelirse mevcut satır döner.
INSERT INTO attachments (taxpayer_id, kind, storage_key, source_url, sha256, size_bytes, content_type, file_name)
VALUES (@taxpayer_id, @kind, @storage_key, sqlc.narg(source_url), @sha256, @size_bytes, sqlc.narg(content_type), sqlc.narg(file_name))
ON CONFLICT (taxpayer_id, sha256) DO UPDATE SET updated_at = NOW()
RETURNING *, (xmax = 0) AS inserted;

-- name: GetAttachmentBySHA256 :one
SELECT * FROM attachments
WHERE taxpayer_id = @taxpayer_id AND sha256 = @sha256;

-- name: SetAttachmentOwner :execrows
UPDATE attachments SET owner_type = @owner_type, owner_id = @owner_id
WHERE taxpayer_id = @taxpayer_id AND id = @id AND owner_id IS NULL;
