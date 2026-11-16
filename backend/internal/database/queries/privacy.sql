-- KVKK veri sahibi talepleri (issue #193).

-- name: InsertPrivacyRequest :one
INSERT INTO privacy_requests (organization_id, engagement_id, kind, subject_type, subject_user_id, reason, requested_by)
VALUES (@organization_id, @engagement_id, @kind, @subject_type, sqlc.narg(subject_user_id), @reason, sqlc.narg(requested_by))
RETURNING *;

-- name: ListPrivacyRequests :many
SELECT * FROM privacy_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: GetPrivacyRequest :one
SELECT * FROM privacy_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: DecidePrivacyRequest :one
-- Only a pending request is decided, once.
UPDATE privacy_requests SET
    status = @status, decided_by = sqlc.narg(decided_by), decided_at = NOW(),
    decision_note = @decision_note, summary = @summary,
    completed_at = CASE WHEN @status::text = 'completed' THEN NOW() END,
    expires_at = sqlc.narg(expires_at)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'pending'
RETURNING *;

-- name: GetPrivacyUser :one
SELECT id, uuid, email, name, surname, status, locale, email_verified_at, last_login_at,
       created_at, deactivated_at, deleted_at, smmm_registry_no
FROM users WHERE id = @id;

-- name: GetPrivacyUserByUUID :one
SELECT id, uuid, email, name, surname, status, locale, email_verified_at, last_login_at,
       created_at, deactivated_at, deleted_at, smmm_registry_no
FROM users WHERE uuid = @uuid;

-- name: ListPrivacyTaxpayerMembers :many
SELECT m.role, m.created_at AS member_since, u.id AS user_id, u.uuid AS user_uuid,
       u.email, u.name, u.surname, u.status
FROM taxpayer_members m
JOIN users u ON u.id = m.user_id
WHERE m.taxpayer_id = @taxpayer_id
ORDER BY m.id;

-- Ties outside this taxpayer: such a user is not anonymised by this office.
-- tenant-lint: cross-org counts a person's memberships in every office; returns numbers only
-- tenant-lint: cross-taxpayer counts a person's memberships in every taxpayer; returns numbers only
-- name: CountPrivacyUserTies :one
SELECT
    (SELECT count(*) FROM organization_members om WHERE om.user_id = sqlc.arg(user_id)::bigint)::bigint AS organizations,
    (SELECT count(*) FROM taxpayer_members tm WHERE tm.user_id = sqlc.arg(user_id)::bigint AND tm.taxpayer_id <> sqlc.arg(taxpayer_id)::bigint)::bigint AS other_taxpayers,
    (SELECT count(*) FROM taxpayer_members tm2 WHERE tm2.user_id = sqlc.arg(user_id)::bigint AND tm2.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS this_taxpayer;

-- name: ListPrivacyAcceptancesForTaxpayer :many
SELECT d.kind, d.version, d.language, a.uuid, a.context, a.content_sha256, a.accepted_at, a.withdrawn_at, a.user_id
FROM legal_acceptances a
JOIN legal_documents d ON d.id = a.document_id
WHERE a.taxpayer_id = @taxpayer_id
ORDER BY a.accepted_at;

-- name: ListPrivacyAcceptancesForUser :many
SELECT d.kind, d.version, d.language, a.uuid, a.context, a.content_sha256, a.accepted_at, a.withdrawn_at, a.ip, a.user_agent
FROM legal_acceptances a
JOIN legal_documents d ON d.id = a.document_id
WHERE a.user_id = @user_id
ORDER BY a.accepted_at;

-- name: ListPrivacyInvitations :many
SELECT uuid, email, role, created_at, expires_at, accepted_at, revoked_at
FROM taxpayer_invitations
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY id;

-- name: ListPrivacyOnboardingRequests :many
SELECT uuid, kind, tax_id, title, contact_email, status, created_at, decided_at
FROM taxpayer_onboarding_requests
WHERE organization_id = @organization_id AND taxpayer_id = @taxpayer_id
ORDER BY id;

-- name: CountPrivacyAccountingRecords :one
-- Accounting records are retained (VUK); the export reports their counts.
SELECT
    (SELECT count(*) FROM invoices i WHERE i.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS invoices,
    (SELECT count(*) FROM bank_transactions b WHERE b.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS bank_transactions,
    (SELECT count(*) FROM expenses e WHERE e.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS expenses,
    (SELECT count(*) FROM retail_sales r WHERE r.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS retail_sales,
    (SELECT count(*) FROM customers c WHERE c.taxpayer_id = sqlc.arg(taxpayer_id)::bigint)::bigint AS customers;

-- name: ListPrivacySessions :many
SELECT created_at, expires_at, revoked_at, ip_address, user_agent
FROM refresh_tokens WHERE user_id = @user_id ORDER BY id;

-- name: ListPrivacyDevices :many
SELECT platform, device_name, app_version, locale, last_seen_at, created_at
FROM push_devices WHERE user_id = @user_id ORDER BY id;

-- name: ListPrivacyPasskeys :many
SELECT name, device_type, last_used_at, created_at
FROM webauthn_credentials WHERE user_id = @user_id ORDER BY id;

-- name: ListPrivacyOAuthProviders :many
SELECT provider, created_at FROM oauth_accounts WHERE user_id = @user_id ORDER BY id;

-- name: CountPrivacyNotifications :one
SELECT count(*)::bigint FROM notifications WHERE user_id = @user_id;

-- name: AnonymizePrivacyUser :exec
-- The row stays (foreign keys, audit trail); every personal field is replaced
-- and the account can never sign in again.
UPDATE users SET
    email = @email, name = 'Silinmiş', surname = 'Kullanıcı', password_hash = '',
    password_set = false, status = 'disabled', email_verified_at = NULL,
    smmm_registry_no = NULL, deactivated_at = COALESCE(deactivated_at, NOW()),
    deleted_at = COALESCE(deleted_at, NOW())
WHERE id = @id;

-- name: DeletePrivacyUserSessions :execrows
DELETE FROM refresh_tokens WHERE user_id = @user_id;

-- name: DeletePrivacyUserDevices :execrows
DELETE FROM push_devices WHERE user_id = @user_id;

-- name: DeletePrivacyUserPushSubscriptions :execrows
DELETE FROM push_subscriptions WHERE user_id = @user_id;

-- name: DeletePrivacyUserPasskeys :execrows
DELETE FROM webauthn_credentials WHERE user_id = @user_id;

-- name: DeletePrivacyUserOAuth :execrows
DELETE FROM oauth_accounts WHERE user_id = @user_id;

-- name: DeletePrivacyUserOTPs :execrows
DELETE FROM otp_codes WHERE user_id = @user_id OR email = @email;

-- name: DeletePrivacyUserNotifications :execrows
DELETE FROM notifications WHERE user_id = @user_id;

-- name: DeletePrivacyTaxpayerMember :execrows
DELETE FROM taxpayer_members WHERE taxpayer_id = @taxpayer_id AND user_id = @user_id;

-- name: MaskPrivacyInvitations :execrows
UPDATE taxpayer_invitations SET email = @email, revoked_at = COALESCE(revoked_at, accepted_at, NOW())
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;

-- name: MaskPrivacyOnboardingContacts :execrows
UPDATE taxpayer_onboarding_requests SET contact_email = @email
WHERE organization_id = @organization_id AND taxpayer_id = @taxpayer_id;
