-- RLS değişmezi (issue #302): organization_id ya da taxpayer_id taşıyan her
-- tabloda RLS açık, FORCE açık ve en az bir politika var
-- (internal/database/rls_invariant_db_test.go denetler; bilinçli istisnalar
-- orada gerekçeleriyle listelenir).
--
-- FORCE yalnızca tablo sahibini bağlar. Uygulama hakan_rls_runtime ile
-- çalışır (000074): sahip değil, RLS ona her zaman uygulanır; kapsamsız
-- sorgular rls_legacy ile eskisi gibi sürer. Bu migration o yolu
-- değiştirmez. Değişen, sahibin kendisi: süper kullanıcı olmayan bir sahip
-- (yönetilen Postgres) artık bu tablolarda yalnızca rls.System içinde
-- (app.system) satır görür ve yazar; başka bir sahip oturumu hiçbir şey
-- görmez. Süper kullanıcı FORCE'u yine atlar.
--
-- Sahip için rls_system politikası (000074'teki rls_enforce ile aynı) her
-- tabloya eklenir; böylece rls.System çalışmaya devam eder.

CREATE OR REPLACE FUNCTION rls_owner_force(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_system ON %s TO CURRENT_USER USING (app_system()) WITH CHECK (app_system())', p_table);
END $$;

-- Şablonlar yeni tabloyu da FORCE ile açar.
CREATE OR REPLACE FUNCTION rls_apply_org(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_org ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_org ON %s TO hakan_rls_app, hakan_rls_worker
        USING (organization_id = app_current_organization_id())
        WITH CHECK (organization_id = app_current_organization_id())', p_table);
    PERFORM rls_legacy(p_table);
    PERFORM rls_owner_force(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_engagement(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_engagement ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_engagement ON %s TO hakan_rls_app, hakan_rls_worker
        USING (app_engagement_visible(organization_id, engagement_id))
        WITH CHECK (app_engagement_visible(organization_id, engagement_id))', p_table);
    PERFORM rls_legacy(p_table);
    PERFORM rls_owner_force(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_taxpayer(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_taxpayer ON %s TO hakan_rls_app, hakan_rls_worker
        USING (taxpayer_id = ANY (app_current_taxpayer_ids()))
        WITH CHECK (taxpayer_id = ANY (app_current_taxpayer_ids()))', p_table);
    PERFORM rls_legacy(p_table);
    PERFORM rls_owner_force(p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_platform(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    PERFORM rls_legacy(p_table);
    PERFORM rls_owner_force(p_table);
END $$;

-- Bugün RLS'si açık, ofis ya da mükellef sütunu taşıyan her tablo.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT DISTINCT c.oid::regclass AS t FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_attribute a ON a.attrelid = c.oid AND NOT a.attisdropped
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relrowsecurity
          AND a.attname IN ('organization_id', 'taxpayer_id')
    LOOP
        PERFORM rls_owner_force(r.t);
    END LOOP;
END
$$;
