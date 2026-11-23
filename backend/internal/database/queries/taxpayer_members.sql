-- Mükellef kullanıcıları ve ofis davetleri (issue #4).

-- tenant-lint: cross-taxpayer üyelik çözümü; taxpayer uuid + user_id ile tek satır
-- name: ResolveTaxpayerMembership :one
SELECT t.id, t.uuid, t.kind, t.tax_id, t.title, m.role
FROM taxpayer_members m
INNER JOIN taxpayers t ON t.id = m.taxpayer_id
WHERE t.uuid = sqlc.arg(taxpayer_uuid)
  AND m.user_id = sqlc.arg(user_id);

-- tenant-lint: cross-taxpayer kullanıcının kendi mükellef üyelikleri (/me)
-- name: ListTaxpayerMembershipsByUser :many
SELECT t.uuid, t.kind, t.title, m.role
FROM taxpayer_members m
INNER JOIN taxpayers t ON t.id = m.taxpayer_id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY t.title, t.id;

-- name: AddTaxpayerMember :exec
INSERT INTO taxpayer_members (taxpayer_id, user_id, role)
VALUES (sqlc.arg(taxpayer_id), sqlc.arg(user_id), sqlc.arg(role));

-- name: GetTaxpayerMember :one
SELECT * FROM taxpayer_members
WHERE taxpayer_id = sqlc.arg(taxpayer_id) AND user_id = sqlc.arg(user_id);

-- name: RevokePendingTaxpayerInvitationsForEmail :exec
UPDATE taxpayer_invitations
SET revoked_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND engagement_id = sqlc.arg(engagement_id)
  AND email = sqlc.arg(email)
  AND accepted_at IS NULL
  AND revoked_at IS NULL;

-- name: CreateTaxpayerInvitation :one
INSERT INTO taxpayer_invitations (organization_id, engagement_id, email, role, token_hash, expires_at, invited_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(engagement_id), sqlc.arg(email), sqlc.arg(role),
        sqlc.arg(token_hash), sqlc.arg(expires_at), sqlc.narg(invited_by))
RETURNING *;

-- name: ListTaxpayerInvitationsByEngagement :many
SELECT i.uuid, i.email, i.role, i.expires_at, i.accepted_at, i.revoked_at, i.created_at,
       u.name AS inviter_name, u.surname AS inviter_surname
FROM taxpayer_invitations i
LEFT JOIN users u ON u.id = i.invited_by
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.engagement_id = sqlc.arg(engagement_id)
ORDER BY i.created_at DESC, i.id DESC
LIMIT 100;

-- name: RevokeTaxpayerInvitation :execrows
UPDATE taxpayer_invitations
SET revoked_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND engagement_id = sqlc.arg(engagement_id)
  AND uuid = sqlc.arg(uuid)
  AND accepted_at IS NULL
  AND revoked_at IS NULL;

-- ResendTaxpayerInvitation issues a new link for an open (or expired, never
-- accepted or revoked) invitation: the old token stops working.
-- name: ResendTaxpayerInvitation :one
UPDATE taxpayer_invitations
SET token_hash = sqlc.arg(token_hash), expires_at = sqlc.arg(expires_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND engagement_id = sqlc.arg(engagement_id)
  AND uuid = sqlc.arg(uuid)
  AND accepted_at IS NULL
  AND revoked_at IS NULL
RETURNING *;

-- The invitation link carries only the token: the holder is not an office
-- member, so the lookup cannot be organization-scoped.
-- tenant-lint: cross-org davet bağlantısı, yalnızca token özetiyle tek satır
-- name: GetTaxpayerInvitationByTokenHash :one
SELECT i.*,
       e.taxpayer_id,
       e.ended_at AS engagement_ended_at,
       t.uuid AS taxpayer_uuid,
       t.title AS taxpayer_title,
       o.name AS organization_name,
       (o.deleted_at IS NOT NULL)::boolean AS organization_deleted,
       u.name AS inviter_name,
       u.surname AS inviter_surname
FROM taxpayer_invitations i
INNER JOIN engagements e ON e.id = i.engagement_id AND e.organization_id = i.organization_id
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
INNER JOIN organizations o ON o.id = i.organization_id
LEFT JOIN users u ON u.id = i.invited_by
WHERE i.token_hash = sqlc.arg(token_hash);

-- tenant-lint: cross-org kabul, token özetiyle bulunan satırın id'siyle tek kullanımlık
-- name: AcceptTaxpayerInvitation :one
UPDATE taxpayer_invitations
SET accepted_at = NOW()
WHERE id = sqlc.arg(id)
  AND accepted_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > NOW()
RETURNING id;
