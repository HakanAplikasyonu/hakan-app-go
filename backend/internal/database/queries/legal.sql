-- KVKK hukuki metinleri ve onaylar (issue #57). Platform geneli tablolar;
-- ofis ve mükellef kapsamı onay kaydının kendisindedir.

-- name: GetCurrentLegalDocument :one
SELECT * FROM legal_documents
WHERE kind = @kind AND language = @language AND status = 'published'
ORDER BY version DESC
LIMIT 1;

-- name: GetLegalDocumentVersion :one
SELECT * FROM legal_documents
WHERE kind = @kind AND language = @language AND version = @version AND status <> 'draft';

-- name: ListCurrentLegalDocuments :many
SELECT DISTINCT ON (kind) * FROM legal_documents
WHERE language = @language AND status = 'published'
ORDER BY kind, version DESC;

-- name: ListAllLegalDocuments :many
SELECT * FROM legal_documents ORDER BY kind, language, version DESC;

-- name: GetLegalDocumentByUUID :one
SELECT * FROM legal_documents WHERE uuid = @uuid;

-- name: InsertLegalDocumentDraft :one
-- Sürüm numarası türün ve dilin en büyük sürümünün bir fazlası.
INSERT INTO legal_documents (kind, language, version, title, body, status, legal_review_pending, content_sha256, created_by)
SELECT @kind, @language, COALESCE(MAX(version), 0) + 1, @title, @body, 'draft', TRUE, @content_sha256, sqlc.narg(created_by)
FROM legal_documents WHERE kind = @kind AND language = @language
RETURNING *;

-- name: PublishLegalDocument :one
UPDATE legal_documents
SET status = 'published', published_at = NOW(), legal_review_pending = @legal_review_pending
WHERE uuid = @uuid AND status = 'draft'
RETURNING *;

-- name: RetireOlderLegalDocuments :exec
UPDATE legal_documents SET status = 'retired'
WHERE kind = @kind AND language = @language AND status = 'published' AND version < @version;

-- name: InsertLegalAcceptance :one
INSERT INTO legal_acceptances (document_id, user_id, organization_id, taxpayer_id, context, subject_uuid, content_sha256, ip, user_agent)
VALUES (@document_id, @user_id, sqlc.narg(organization_id), sqlc.narg(taxpayer_id), @context, sqlc.narg(subject_uuid),
        @content_sha256, @ip, @user_agent)
RETURNING *;

-- name: ListUserLegalAcceptances :many
SELECT a.uuid, a.context, a.subject_uuid, a.accepted_at, a.withdrawn_at,
       d.kind, d.language, d.version, d.title, d.uuid AS document_uuid,
       o.name AS organization_name, t.title AS taxpayer_title
FROM legal_acceptances a
JOIN legal_documents d ON d.id = a.document_id
LEFT JOIN organizations o ON o.id = a.organization_id
LEFT JOIN taxpayers t ON t.id = a.taxpayer_id
WHERE a.user_id = @user_id
ORDER BY a.accepted_at DESC
LIMIT 200;

-- name: GetOrganizationLegalAcceptance :one
-- Ofisin bu türdeki en son geçerli onayı.
SELECT a.uuid, a.accepted_at, a.user_id, d.version, d.language, u.email AS accepted_by_email
FROM legal_acceptances a
JOIN legal_documents d ON d.id = a.document_id
JOIN users u ON u.id = a.user_id
WHERE a.organization_id = @organization_id AND a.context = @context AND d.kind = @kind
  AND a.withdrawn_at IS NULL
ORDER BY d.version DESC, a.accepted_at DESC
LIMIT 1;

-- name: WithdrawLegalAcceptancesBySubject :execrows
UPDATE legal_acceptances SET withdrawn_at = NOW()
WHERE subject_uuid = @subject_uuid AND withdrawn_at IS NULL;
