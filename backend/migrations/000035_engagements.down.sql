DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('tenant.engagements.read', 'tenant.engagements.write', 'tenant.engagements.read_all')
);
DELETE FROM permissions
WHERE slug IN ('tenant.engagements.read', 'tenant.engagements.write', 'tenant.engagements.read_all');

DROP TABLE IF EXISTS engagement_assignments;
DROP TABLE IF EXISTS engagements;
DROP TABLE IF EXISTS taxpayers;
