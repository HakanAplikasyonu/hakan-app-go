-- Zorlanan tablolar ve geçiş politikaları geri alınır; şablonlar 000046
-- hâline döner.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT c.oid::regclass AS t FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS rls_legacy ON %s', r.t);
        EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', r.t);
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', r.t);
    END LOOP;
END
$$;

CREATE OR REPLACE FUNCTION rls_apply_org(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_org ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_org ON %s TO hakan_rls_app, hakan_rls_worker
        USING (organization_id = app_current_organization_id())
        WITH CHECK (organization_id = app_current_organization_id())', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_engagement(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_engagement ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_engagement ON %s TO hakan_rls_app, hakan_rls_worker
        USING (app_engagement_visible(organization_id, engagement_id))
        WITH CHECK (app_engagement_visible(organization_id, engagement_id))', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_taxpayer(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_taxpayer ON %s TO hakan_rls_app, hakan_rls_worker
        USING (taxpayer_id = ANY (app_current_taxpayer_ids()))
        WITH CHECK (taxpayer_id = ANY (app_current_taxpayer_ids()))', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_platform(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
END $$;

DROP FUNCTION IF EXISTS rls_enforce(REGCLASS);
DROP FUNCTION IF EXISTS rls_legacy(REGCLASS);
DROP FUNCTION IF EXISTS app_system();

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM hakan_rls_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE USAGE, SELECT, UPDATE ON SEQUENCES FROM hakan_rls_runtime;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM hakan_rls_runtime;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM hakan_rls_runtime;
REVOKE USAGE ON SCHEMA public FROM hakan_rls_runtime;
-- Rol küme genelidir ve başka veritabanlarında kullanılıyor olabilir; silinmez.
