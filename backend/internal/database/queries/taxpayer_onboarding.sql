-- Mükellef ekleme talepleri (issue #7). Ofis tarafındaki sorgular
-- taxpayers tablosuna dokunmaz (D1b: varlık sızıntısı yasağı).

-- name: CreateOnboardingRequest :one
INSERT INTO taxpayer_onboarding_requests
    (organization_id, kind, tax_id, title, contact_email, token_hash, requested_by, expires_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(tax_id), sqlc.arg(title),
        sqlc.arg(contact_email), sqlc.arg(token_hash), sqlc.narg(requested_by), sqlc.arg(expires_at))
RETURNING *;

-- name: ListOnboardingRequestsByOrg :many
SELECT r.*, e.uuid AS engagement_uuid
FROM taxpayer_onboarding_requests r
LEFT JOIN engagements e ON e.id = r.engagement_id AND e.organization_id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOnboardingRequestsByOrg :one
SELECT COUNT(*)::bigint FROM taxpayer_onboarding_requests WHERE organization_id = sqlc.arg(organization_id);

-- name: CancelOnboardingRequest :execrows
UPDATE taxpayer_onboarding_requests
SET status = 'cancelled', decided_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND status = 'pending';

-- The consent link carries only the token: the mükellef is not an office
-- member, so the lookup cannot be organization-scoped.
-- tenant-lint: cross-org onay bağlantısı, yalnızca token özetiyle tek satır
-- name: GetOnboardingRequestByTokenHash :one
SELECT r.*, o.name AS organization_name, (o.deleted_at IS NOT NULL)::boolean AS organization_deleted
FROM taxpayer_onboarding_requests r
INNER JOIN organizations o ON o.id = r.organization_id
WHERE r.token_hash = sqlc.arg(token_hash);

-- tenant-lint: cross-org onay işlemi, token ile bulunan satır kilitlenir
-- name: LockOnboardingRequest :one
SELECT * FROM taxpayer_onboarding_requests WHERE id = sqlc.arg(id) FOR UPDATE;

-- tenant-lint: cross-org onay sonucu, token ile bulunan satırın id'siyle
-- name: DecideOnboardingRequest :exec
UPDATE taxpayer_onboarding_requests
SET status = sqlc.arg(status), decided_by = sqlc.narg(decided_by), decided_at = NOW(),
    taxpayer_id = sqlc.narg(taxpayer_id), engagement_id = sqlc.narg(engagement_id)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- Mükellef panelinde sahibin göreceği bekleyen talepler (mükellefin kendi
-- VKN'si için). Ofislerin kendi kayıtlarıdır ama yalnızca talebin hedefi
-- olan mükellefe gösterilir.
-- tenant-lint: cross-org mükellef sahibi kendi VKN'sine gelen talepleri görür
-- name: ListPendingOnboardingRequestsForTaxID :many
SELECT r.uuid, r.title, r.tax_id, r.contact_email, r.created_at, r.expires_at, o.name AS organization_name
FROM taxpayer_onboarding_requests r
INNER JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
WHERE r.tax_id = sqlc.arg(tax_id) AND r.status = 'pending' AND r.expires_at > NOW()
ORDER BY r.created_at DESC
LIMIT 50;

-- tenant-lint: cross-org mükellef sahibi talebi uuid ve kendi VKN'siyle bulur
-- name: GetPendingOnboardingRequestForTaxID :one
SELECT r.*, o.name AS organization_name
FROM taxpayer_onboarding_requests r
INNER JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
WHERE r.uuid = sqlc.arg(uuid) AND r.tax_id = sqlc.arg(tax_id);

-- tenant-lint: cross-taxpayer sahip var mı (onay yetkisi)
-- name: ListTaxpayerOwnerUserIDs :many
SELECT user_id FROM taxpayer_members WHERE taxpayer_id = sqlc.arg(taxpayer_id) AND role = 'owner';

-- name: LockTaxpayerByTaxID :one
SELECT * FROM taxpayers WHERE tax_id = sqlc.arg(tax_id) FOR UPDATE;

-- tenant-lint: cross-org onay sırasında mükellefin (hangi ofiste olursa) aktif engagement'ı
-- name: GetActiveEngagementForTaxpayer :one
SELECT * FROM engagements WHERE taxpayer_id = sqlc.arg(taxpayer_id) AND ended_at IS NULL;

-- tenant-lint: cross-org ofis geçişi: önceki ofisin engagement'ı mükellef onayıyla biter
-- name: EndEngagementByID :exec
UPDATE engagements SET status = 'ended', ended_at = NOW() WHERE id = sqlc.arg(id) AND ended_at IS NULL;
