-- Mutabakat verisi (çalıştırmalar, fark kalemleri, cari mutabakat, BA/BS
-- beyan satırları, mutabakat mektupları ve yanıtları) silinir. Mizan ve
-- kaynak kayıtlar kalır. İnceleme kuyruğundaki mutabakat önerileri silinir.
-- Sahip FORCE altında: veri silen adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DELETE FROM suggestions WHERE kind = 'reconciliation';

DROP TABLE IF EXISTS reconciliation_letters;
DROP TABLE IF EXISTS babs_declarations;
DROP TABLE IF EXISTS counterparty_reconciliations;
DROP TABLE IF EXISTS reconciliation_items;
DROP TABLE IF EXISTS reconciliation_runs;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.reconciliation.read', 'tenant.reconciliation.write'));
DELETE FROM permissions WHERE slug IN ('tenant.reconciliation.read', 'tenant.reconciliation.write');
