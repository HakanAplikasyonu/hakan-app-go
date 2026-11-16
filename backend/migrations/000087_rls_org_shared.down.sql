DO $$
DECLARE t REGCLASS;
BEGIN
    FOREACH t IN ARRAY ARRAY['roles', 'bulk_jobs', 'import_jobs', 'export_jobs', 'excel_import_templates']::regclass[]
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS rls_shared_read ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_shared_write ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_legacy ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', t);
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %s DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END
$$;

DROP FUNCTION IF EXISTS rls_apply_org_shared(REGCLASS);
