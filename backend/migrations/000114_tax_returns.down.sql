-- Beyan kayıtları (düzeltmeler, hazırlanan görüntüler, verildi bilgisi)
-- silinir; önizleme kaynak veriden yeniden hesaplanabilir.
SET LOCAL app.system = 'on';

DROP TABLE IF EXISTS tax_returns;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.tax_returns.read', 'tenant.tax_returns.write'));
DELETE FROM permissions WHERE slug IN ('tenant.tax_returns.read', 'tenant.tax_returns.write');
