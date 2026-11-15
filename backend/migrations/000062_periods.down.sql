-- Dönem yönetimini kaldırır: kilitler ve olay günlüğü silinir.
-- change_requests (000037) tablosu ve içindeki talepler kalır.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.periods.read', 'tenant.periods.lock', 'tenant.change_requests.decide'));
DELETE FROM permissions WHERE slug IN
    ('tenant.periods.read', 'tenant.periods.lock', 'tenant.change_requests.decide');
DROP INDEX IF EXISTS idx_change_requests_open_hash;
DROP TABLE IF EXISTS period_lock_events;
DROP TABLE IF EXISTS period_locks;
