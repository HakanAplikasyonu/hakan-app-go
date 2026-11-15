-- İnceleme kuyruğunu kaldırır: bütün öneriler, karar günlüğü ve eşik ayarları silinir.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.review.read', 'tenant.review.decide', 'tenant.review.settings.write'));
DELETE FROM permissions WHERE slug IN
    ('tenant.review.read', 'tenant.review.decide', 'tenant.review.settings.write');
DROP TABLE IF EXISTS review_settings;
DROP TABLE IF EXISTS suggestion_decisions;
DROP TABLE IF EXISTS suggestions;
