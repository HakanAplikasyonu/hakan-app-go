-- KDV erken uyarıyı kaldırır. Elle girilen açılış devirleri, bu kodların ofis /
-- mükellef ayarları ve bu kodlarla açılmış uyarılar silinir.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.vat.read', 'tenant.vat.write'));
DELETE FROM permissions WHERE slug IN ('tenant.vat.read', 'tenant.vat.write');
DELETE FROM data_warnings WHERE code IN ('VAT_PAYABLE_THRESHOLD', 'VAT_DEADLINE_NEAR');
DELETE FROM warning_codes WHERE code IN ('VAT_PAYABLE_THRESHOLD', 'VAT_DEADLINE_NEAR');
DROP TABLE IF EXISTS vat_carry_forwards;
