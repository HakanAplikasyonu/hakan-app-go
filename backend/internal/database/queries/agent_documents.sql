-- Yerel ajandan gelen dosyalar (#780 e): Luca bordro dökümü, mizan Excel'i
-- ve yabancı hizmet faturası PDF'i; eşleştirilemeyenler ofise "eşleştir"
-- kaydı olarak düşer.

-- name: GetAgentDocumentBySha :one
SELECT * FROM agent_documents WHERE organization_id = @organization_id AND sha256 = @sha256;

-- name: CreateAgentDocument :one
-- Aynı içerik ikinci kez gelirse satır açılmaz (ErrNoRows: ilk satır okunur).
INSERT INTO agent_documents (organization_id, agent_id, folder, file_name, sha256, size_bytes,
                             content_type, storage_key, kind, period, created_by)
VALUES (@organization_id, sqlc.narg(agent_id), @folder, @file_name, @sha256, @size_bytes,
        @content_type, @storage_key, @kind, sqlc.narg(period), sqlc.narg(created_by))
ON CONFLICT (organization_id, sha256) DO NOTHING
RETURNING *;

-- name: SetAgentDocumentOutcome :one
UPDATE agent_documents
SET engagement_id = sqlc.narg(engagement_id), kind = @kind, period = sqlc.narg(period),
    match_source = @match_source, status = @status, reason = @reason,
    result_uuid = sqlc.narg(result_uuid), resolved_by = sqlc.narg(resolved_by),
    resolved_at = CASE WHEN @status::text IN ('imported', 'dismissed') THEN NOW() ELSE NULL END
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: GetAgentDocument :one
SELECT * FROM agent_documents WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: GetAgentDocumentView :one
SELECT d.*,
       COALESCE(a.name, '')::text AS agent_name,
       e.uuid AS engagement_uuid,
       COALESCE(t.title, '')::text AS taxpayer_title
FROM agent_documents d
LEFT JOIN desktop_agents a ON a.organization_id = d.organization_id AND a.id = d.agent_id
LEFT JOIN engagements e ON e.organization_id = d.organization_id AND e.id = d.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE d.organization_id = @organization_id AND d.id = @id;

-- name: ListAgentDocuments :many
-- Ofisin ajandan gelen dosyaları, en yeni başta; statuses boşsa hepsi.
SELECT d.*,
       COALESCE(a.name, '')::text AS agent_name,
       e.uuid AS engagement_uuid,
       COALESCE(t.title, '')::text AS taxpayer_title
FROM agent_documents d
LEFT JOIN desktop_agents a ON a.organization_id = d.organization_id AND a.id = d.agent_id
LEFT JOIN engagements e ON e.organization_id = d.organization_id AND e.id = d.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE d.organization_id = @organization_id
  AND (cardinality(@statuses::text[]) = 0 OR d.status = ANY(@statuses::text[]))
ORDER BY d.id DESC
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountAgentDocuments :one
SELECT count(*)::bigint FROM agent_documents
WHERE organization_id = @organization_id
  AND (cardinality(@statuses::text[]) = 0 OR status = ANY(@statuses::text[]));

-- name: ListAgentDocumentCandidates :many
-- Eşleştirme adayları: ofisin etkin hizmet ilişkileri, mükellefin VKN /
-- TCKN'si ve unvanı (dosyadaki VKN ya da klasör adı bunlarla karşılaştırılır).
SELECT e.id AS engagement_id, e.uuid AS engagement_uuid, e.taxpayer_id,
       t.uuid AS taxpayer_uuid, t.tax_id, t.title
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.status = 'active'
ORDER BY e.id
LIMIT 5000;
