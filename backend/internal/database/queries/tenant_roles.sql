-- Ofise özel roller (issue #5, D16). Her sorgu organization_id ile ofise
-- bağlıdır; şablon roller (organization_id IS NULL) burada listelenmez.

-- name: ListOrganizationRoles :many
SELECT r.id, r.slug, r.name, COALESCE(r.description, '')::text AS description, r.is_system,
       COALESCE((SELECT array_agg(p.slug ORDER BY p.slug)
                 FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
                 WHERE rp.role_id = r.id), '{}')::text[] AS permissions,
       (SELECT COUNT(*) FROM organization_members om
        WHERE om.organization_id = r.organization_id AND om.role_id = r.id)::bigint AS member_count
FROM roles r
WHERE r.scope = 'tenant' AND r.organization_id = sqlc.arg(organization_id)
ORDER BY r.is_system DESC, r.id;

-- name: GetOrganizationRoleByKey :one
SELECT * FROM roles
WHERE scope = 'tenant' AND organization_id = sqlc.arg(organization_id) AND slug = sqlc.arg(role_key);

-- name: ListTenantPermissions :many
SELECT slug, name FROM permissions WHERE slug LIKE 'tenant.%' ORDER BY slug;

-- name: CreateOrganizationRole :one
INSERT INTO roles (name, slug, description, is_system, scope, organization_id)
VALUES (sqlc.arg(name), sqlc.arg(role_key), sqlc.arg(description), false, 'tenant', sqlc.arg(organization_id))
RETURNING *;

-- name: UpdateOrganizationRole :one
UPDATE roles
SET name = COALESCE(sqlc.narg(name), name),
    description = COALESCE(sqlc.narg(description), description)
WHERE scope = 'tenant' AND organization_id = sqlc.arg(organization_id) AND slug = sqlc.arg(role_key)
RETURNING *;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions rp
USING roles r
WHERE rp.role_id = r.id
  AND r.id = sqlc.arg(role_id)
  AND r.scope = 'tenant'
  AND r.organization_id = sqlc.arg(organization_id);

-- name: AddRolePermissionsBySlug :exec
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = ANY(sqlc.arg(permission_slugs)::text[]) AND p.slug LIKE 'tenant.%'
WHERE r.id = sqlc.arg(role_id) AND r.scope = 'tenant' AND r.organization_id = sqlc.arg(organization_id)
ON CONFLICT DO NOTHING;

-- name: CountOrganizationRoleUsage :one
SELECT
    (SELECT COUNT(*) FROM organization_members om
     WHERE om.organization_id = sqlc.arg(organization_id) AND om.role_id = sqlc.arg(role_id))::bigint
  + (SELECT COUNT(*) FROM organization_invitations oi
     WHERE oi.organization_id = sqlc.arg(organization_id) AND oi.role_id = sqlc.arg(role_id)
       AND oi.accepted_at IS NULL AND oi.revoked_at IS NULL AND oi.expires_at > NOW())::bigint
  AS usage;

-- name: DeleteOrganizationRole :execrows
DELETE FROM roles
WHERE scope = 'tenant' AND organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(role_id)
  AND NOT is_system;
