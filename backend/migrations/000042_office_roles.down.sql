-- Ofis rol kopyalarını kaldırır: üyelikler ve davetler şablon rollere geri
-- taşınır, ofislerin özel rolleri (ve onlara bağlı üyelikler) SİLİNİR.
-- SMMM sicil numaraları silinir.
ALTER TABLE users DROP COLUMN IF EXISTS smmm_registry_no;

DROP TRIGGER IF EXISTS trg_permissions_grant_tenant_owner ON permissions;
DROP FUNCTION IF EXISTS permissions_grant_tenant_owner();
DROP TRIGGER IF EXISTS trg_organizations_copy_tenant_roles ON organizations;
DROP FUNCTION IF EXISTS organizations_copy_tenant_roles();
DROP FUNCTION IF EXISTS grant_tenant_role_permission(TEXT, TEXT);
DROP FUNCTION IF EXISTS copy_tenant_template_roles(BIGINT);

UPDATE organization_members om
SET role_id = t.id
FROM roles c, roles t
WHERE om.role_id = c.id
  AND c.scope = 'tenant' AND c.organization_id IS NOT NULL AND c.is_system
  AND t.scope = 'tenant' AND t.organization_id IS NULL AND t.slug = c.slug
  AND t.slug IN ('owner', 'admin', 'member');

UPDATE organization_invitations oi
SET role_id = t.id
FROM roles c, roles t
WHERE oi.role_id = c.id
  AND c.scope = 'tenant' AND c.organization_id IS NOT NULL AND c.is_system
  AND t.scope = 'tenant' AND t.organization_id IS NULL AND t.slug = c.slug
  AND t.slug IN ('owner', 'admin', 'member');

DELETE FROM organization_invitations oi USING roles r
WHERE oi.role_id = r.id AND r.scope = 'tenant' AND r.organization_id IS NOT NULL;
DELETE FROM organization_members om USING roles r
WHERE om.role_id = r.id AND r.scope = 'tenant' AND r.organization_id IS NOT NULL;
DELETE FROM roles WHERE scope = 'tenant' AND organization_id IS NOT NULL;
DELETE FROM organization_members om USING roles r
WHERE om.role_id = r.id AND r.slug IN ('accountant', 'intern') AND r.scope = 'tenant';
DELETE FROM roles WHERE scope = 'tenant' AND organization_id IS NULL AND slug IN ('accountant', 'intern');

UPDATE roles SET name = 'Organization Owner', description = 'Tenant owner template role' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'owner';
UPDATE roles SET name = 'Organization Admin', description = 'Tenant admin template role' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'admin';
UPDATE roles SET name = 'Organization Member', description = 'Tenant member template role' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'member';
