-- AI dönem özetlerini kaldırır (bütün taslak ve onaylı özetler silinir).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.summaries.read', 'tenant.summaries.write', 'tenant.summaries.approve'));
DELETE FROM permissions WHERE slug IN ('tenant.summaries.read', 'tenant.summaries.write', 'tenant.summaries.approve');
DROP TABLE IF EXISTS period_summaries;
