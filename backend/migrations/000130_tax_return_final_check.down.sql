SET LOCAL app.system = 'on';

ALTER TABLE tax_returns
    DROP COLUMN IF EXISTS checked_at,
    DROP COLUMN IF EXISTS checked_by;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.tax_returns.approve');
DELETE FROM permissions WHERE slug = 'tenant.tax_returns.approve';
