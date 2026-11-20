-- Beyan son kontrolü (#669, ofis kararı): ofiste herkes beyanı hazırlar;
-- "kontrol edildi" ve "verildi" işaretini yalnız sorumlu (yetkili rol)
-- koyar. Yeni izin tenant.tax_returns.approve: sahip kendiliğinden alır
-- (trg_permissions_grant_tenant_owner, 000044), yönetici (admin) burada.
-- Kim / ne zaman: checked_by / checked_at (verildi: filed_by / filed_at).
--
-- roles ve role_permissions FORCE altında (000086): sistem kipinde yazılır.
SET LOCAL app.system = 'on';

INSERT INTO permissions (name, slug) VALUES
    ('Check and file tax returns (final check)', 'tenant.tax_returns.approve')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission('admin', 'tenant.tax_returns.approve');

ALTER TABLE tax_returns
    ADD COLUMN checked_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN checked_at TIMESTAMPTZ NULL;
