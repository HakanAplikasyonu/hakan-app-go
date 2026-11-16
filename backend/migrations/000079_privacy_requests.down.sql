-- Talep kayıtları ve izinler silinir; yürütülmüş anonimleştirmeler geri gelmez.
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.privacy.request', 'tenant.privacy.approve'));
DELETE FROM permissions WHERE slug IN ('tenant.privacy.request', 'tenant.privacy.approve');
DROP TABLE IF EXISTS privacy_requests;
