-- Tenant-core faz 6d (issue #5): ofise özel roller (D16).
--
-- 1. Şablon roller: sahip (owner), yönetici (admin), mali müşavir
--    (accountant), personel (member), stajyer (intern). Şablonlar
--    organization_id IS NULL satırlarıdır.
-- 2. Her ofis açılınca şablonlar izinleriyle birlikte ofise kopyalanır
--    (trg_organizations_copy_tenant_roles). Ofis kopyalarını değiştirebilir,
--    yeni rol ekleyebilir. Kopyalar is_system = true: silinemez.
-- 3. Var olan ofisler için kopyalar burada oluşturulur ve üyelikler ile açık
--    davetler şablondan ofis kopyasına taşınır.
--
-- Sonraki migration'lar için kural (DATABASE_RULES.md):
--   * Yeni bir tenant.* izni eklendiğinde sahip rolleri (şablon + bütün ofis
--     kopyaları) onu kendiliğinden alır (trg_permissions_grant_tenant_owner).
--   * Diğer rollere izin vermek için grant_tenant_role_permission('admin',
--     'tenant.x.read') kullanın: şablona ve ofis kopyalarına birlikte verir.
--     Yalnızca şablona yazılan izin var olan ofislere ulaşmaz.

INSERT INTO roles (name, slug, description, is_system, scope, organization_id) VALUES
    ('Mali Müşavir', 'accountant', 'Tenant accountant template role', true, 'tenant', NULL),
    ('Stajyer', 'intern', 'Tenant intern template role', true, 'tenant', NULL)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

UPDATE roles SET name = 'Sahip' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'owner';
UPDATE roles SET name = 'Yönetici' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'admin';
UPDATE roles SET name = 'Personel' WHERE scope = 'tenant' AND organization_id IS NULL AND slug = 'member';

-- Mali müşavir: personelin izinleri + engagement yönetimi (atama hariç
-- read_all yok: yalnızca atandığı mükellefleri görür).
INSERT INTO role_permissions (role_id, permission_id)
SELECT acc.id, rp.permission_id
FROM roles acc
JOIN roles mem ON mem.scope = 'tenant' AND mem.organization_id IS NULL AND mem.slug = 'member'
JOIN role_permissions rp ON rp.role_id = mem.id
WHERE acc.scope = 'tenant' AND acc.organization_id IS NULL AND acc.slug = 'accountant'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT acc.id, p.id
FROM roles acc
JOIN permissions p ON p.slug = 'tenant.engagements.write'
WHERE acc.scope = 'tenant' AND acc.organization_id IS NULL AND acc.slug = 'accountant'
ON CONFLICT DO NOTHING;

-- Stajyer: personelin yalnızca okuma izinleri.
INSERT INTO role_permissions (role_id, permission_id)
SELECT intern.id, rp.permission_id
FROM roles intern
JOIN roles mem ON mem.scope = 'tenant' AND mem.organization_id IS NULL AND mem.slug = 'member'
JOIN role_permissions rp ON rp.role_id = mem.id
JOIN permissions p ON p.id = rp.permission_id AND p.slug LIKE '%.read'
WHERE intern.scope = 'tenant' AND intern.organization_id IS NULL AND intern.slug = 'intern'
ON CONFLICT DO NOTHING;

-- Şablonları (izinleriyle) bir ofise kopyalar. Ofiste aynı anahtarlı rol
-- varsa o şablon atlanır.
CREATE OR REPLACE FUNCTION copy_tenant_template_roles(p_organization_id BIGINT)
RETURNS void AS $$
BEGIN
    INSERT INTO roles (name, slug, description, is_system, scope, organization_id)
    SELECT t.name, t.slug, t.description, true, 'tenant', p_organization_id
    FROM roles t
    WHERE t.scope = 'tenant' AND t.organization_id IS NULL
    ON CONFLICT (organization_id, slug) WHERE scope = 'tenant' AND organization_id IS NOT NULL DO NOTHING;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT c.id, rp.permission_id
    FROM roles t
    JOIN roles c ON c.scope = 'tenant' AND c.organization_id = p_organization_id AND c.slug = t.slug AND c.is_system
    JOIN role_permissions rp ON rp.role_id = t.id
    WHERE t.scope = 'tenant' AND t.organization_id IS NULL
    ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION organizations_copy_tenant_roles()
RETURNS trigger AS $$
BEGIN
    PERFORM copy_tenant_template_roles(NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organizations_copy_tenant_roles
    AFTER INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION organizations_copy_tenant_roles();

-- Bir izni şablon role ve bütün ofis kopyalarına (aynı anahtarlı sistem
-- rolleri) verir. Sonraki migration'lar bunu kullanmalı.
CREATE OR REPLACE FUNCTION grant_tenant_role_permission(p_role_slug TEXT, p_permission_slug TEXT)
RETURNS void AS $$
BEGIN
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT r.id, p.id
    FROM roles r
    JOIN permissions p ON p.slug = p_permission_slug
    WHERE r.scope = 'tenant'
      AND r.slug = p_role_slug
      AND (r.organization_id IS NULL OR r.is_system)
    ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- Yeni bir tenant.* izni sahip rollerine (şablon + kopyalar) kendiliğinden
-- verilir: sahip her zaman ofisteki bütün izinlere sahiptir.
CREATE OR REPLACE FUNCTION permissions_grant_tenant_owner()
RETURNS trigger AS $$
BEGIN
    IF NEW.slug LIKE 'tenant.%' THEN
        INSERT INTO role_permissions (role_id, permission_id)
        SELECT r.id, NEW.id
        FROM roles r
        WHERE r.scope = 'tenant' AND r.slug = 'owner' AND (r.organization_id IS NULL OR r.is_system)
        ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_permissions_grant_tenant_owner
    AFTER INSERT ON permissions
    FOR EACH ROW
    EXECUTE FUNCTION permissions_grant_tenant_owner();

-- Var olan ofisler: kopyala ve üyelik / açık davetleri ofis kopyasına taşı.
SELECT copy_tenant_template_roles(o.id) FROM organizations o;

UPDATE organization_members om
SET role_id = c.id
FROM roles t, roles c
WHERE om.role_id = t.id
  AND t.scope = 'tenant' AND t.organization_id IS NULL
  AND c.scope = 'tenant' AND c.organization_id = om.organization_id AND c.slug = t.slug;

UPDATE organization_invitations oi
SET role_id = c.id
FROM roles t, roles c
WHERE oi.role_id = t.id
  AND t.scope = 'tenant' AND t.organization_id IS NULL
  AND c.scope = 'tenant' AND c.organization_id = oi.organization_id AND c.slug = t.slug;

-- SMMM oda sicil no (D16): profil alanı, doğrulanmaz.
ALTER TABLE users ADD COLUMN smmm_registry_no VARCHAR(32) NULL;
