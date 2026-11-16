-- Ofissiz sistem satırı taşıyan tablolar için RLS (issue #307, #302'nin
-- istisna listesi). Bu tablolarda organization_id NULL satırlar platformun
-- ortak kayıtlarıdır: sistem rolleri, global Excel şablonları, ofissiz
-- açılmış IO motoru işleri. Yalnız ofis politikası (rls_apply_org) bu
-- satırları her kapsamda gizlerdi.
--
-- rls_apply_org_shared:
--   okuma  kapsamın ofisi ve organization_id NULL satırlar
--   yazma  yalnızca kapsamın ofisi; NULL satır yalnızca rls.System'den
--          (ya da geçişteki kapsamsız sorgudan, rls_legacy) yazılır.
-- Mükellef kapsamı (ofis yok) yalnızca NULL satırları görür, yazamaz.
-- Kapsamsız sorgular rls_legacy ile eskisi gibi çalışır; FORCE ve sahibin
-- rls_system politikası rls_owner_force ile (000086).
CREATE OR REPLACE FUNCTION rls_apply_org_shared(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_shared_read ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_shared_read ON %s FOR SELECT TO hakan_rls_app, hakan_rls_worker
        USING (organization_id IS NULL OR organization_id = app_current_organization_id())', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_shared_write ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_shared_write ON %s TO hakan_rls_app, hakan_rls_worker
        USING (organization_id = app_current_organization_id())
        WITH CHECK (organization_id = app_current_organization_id())', p_table);
    PERFORM rls_legacy(p_table);
    PERFORM rls_owner_force(p_table);
END $$;

SELECT rls_apply_org_shared(t) FROM unnest(ARRAY[
    'roles', 'bulk_jobs', 'import_jobs', 'export_jobs', 'excel_import_templates'
]::regclass[]) AS t;
