-- Uyarı motoru yapılandırmasını kaldırır (ofis/mükellef ayarları silinir).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.warnings.read', 'tenant.warnings.write', 'tenant.warning_rules.write'));
DELETE FROM permissions WHERE slug IN
    ('tenant.warnings.read', 'tenant.warnings.write', 'tenant.warning_rules.write');
DROP TABLE IF EXISTS warning_rule_configs;
DROP TABLE IF EXISTS warning_codes;
