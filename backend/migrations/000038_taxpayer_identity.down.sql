DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('platform.taxpayers.read', 'platform.taxpayers.write'));
DELETE FROM permissions WHERE slug IN ('platform.taxpayers.read', 'platform.taxpayers.write');

-- Kimlik alanlarını kaldırır. VKN/TCKN, unvan ve vergi dairesi verisi silinir.
ALTER TABLE taxpayers
    DROP CONSTRAINT IF EXISTS uq_taxpayers_tax_id,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_title,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_tax_id,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_kind,
    DROP COLUMN IF EXISTS tax_office,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS tax_id,
    DROP COLUMN IF EXISTS kind;
