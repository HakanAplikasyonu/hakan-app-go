-- Mükellef defterleri (#780): işletme defteri ve e-defter dosyaları.

-- name: ListLedgerFiles :many
-- Bir işin yüklenen defterleri, yeni dönem önce. Özet JSON'u yok (tek
-- dosya ucunda).
SELECT id, uuid, kind, period, file_name, content_type, size_bytes, readable, uploaded_by, created_at,
       COALESCE((SELECT btrim(u.name || ' ' || u.surname) FROM users u WHERE u.id = f.uploaded_by), '')::text AS uploaded_by_name
FROM ledger_files f
WHERE f.organization_id = @organization_id AND f.engagement_id = @engagement_id
ORDER BY f.period DESC, f.created_at DESC, f.id DESC
LIMIT 500;

-- name: GetLedgerFile :one
SELECT * FROM ledger_files
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: GetLedgerFileBySha :one
SELECT * FROM ledger_files
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND sha256 = @sha256;

-- name: InsertLedgerFile :one
-- Aynı dosya (sha256) ikinci kez gelirse satır açılmaz (boş sonuç).
INSERT INTO ledger_files (
    organization_id, engagement_id, kind, period, file_name, content_type, storage_key,
    sha256, size_bytes, readable, summary, uploaded_by
) VALUES (
    @organization_id, @engagement_id, @kind, @period, @file_name, @content_type, @storage_key,
    @sha256, @size_bytes, @readable, @summary, sqlc.narg(uploaded_by)::bigint
)
ON CONFLICT (engagement_id, sha256) DO NOTHING
RETURNING *;

-- name: DeleteLedgerFile :one
DELETE FROM ledger_files
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
RETURNING storage_key;

-- name: GetLedgerSettings :one
SELECT period_kind FROM ledger_settings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;

-- name: UpsertLedgerSettings :one
INSERT INTO ledger_settings (organization_id, engagement_id, period_kind, updated_by, updated_at)
VALUES (@organization_id, @engagement_id, @period_kind, sqlc.narg(updated_by)::bigint, NOW())
ON CONFLICT (engagement_id) DO UPDATE
SET period_kind = EXCLUDED.period_kind, updated_by = EXCLUDED.updated_by, updated_at = NOW()
WHERE ledger_settings.organization_id = EXCLUDED.organization_id
RETURNING period_kind;
