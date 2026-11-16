-- Zorlanmış tablolar (rls_enforce: petitions, vat_carry_forwards,
-- export_settings) FORCE ve rls_system'i korur; diğerleri 000074 durumuna
-- döner.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT c.oid::regclass AS t FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_attribute a ON a.attrelid = c.oid AND NOT a.attisdropped
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relforcerowsecurity
          AND a.attname IN ('organization_id', 'taxpayer_id')
          AND EXISTS (SELECT 1 FROM pg_policies p
                      WHERE p.schemaname = 'public' AND p.tablename = c.relname AND p.policyname = 'rls_legacy')
    LOOP
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', r.t);
        EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', r.t);
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
    PERFORM rls_legacy(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_engagement(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_engagement ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_engagement ON %s TO hakan_rls_app, hakan_rls_worker
        USING (app_engagement_visible(organization_id, engagement_id))
        WITH CHECK (app_engagement_visible(organization_id, engagement_id))', p_table);
    PERFORM rls_legacy(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_taxpayer(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_taxpayer ON %s TO hakan_rls_app, hakan_rls_worker
        USING (taxpayer_id = ANY (app_current_taxpayer_ids()))
        WITH CHECK (taxpayer_id = ANY (app_current_taxpayer_ids()))', p_table);
    PERFORM rls_legacy(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_platform(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    PERFORM rls_legacy(p_table);
END $$;

DROP FUNCTION IF EXISTS rls_owner_force(REGCLASS);
