-- RLS'yi uygulamanın bütün yollarına yayma, kademeli (issue #166).
--
-- Sorun: #6'daki politikalar yalnızca rls.WithScope içindeki işlemlerde
-- (SET LOCAL ROLE hakan_rls_app) çalışıyor. Geri kalan her sorgu tablo sahibi
-- olarak, RLS'siz çalışıyor. FORCE ROW LEVEL SECURITY tek başına çözüm değil:
-- yalnızca tablo sahibini etkiler ve süper kullanıcı (resmi postgres imajının
-- POSTGRES_USER'ı, CI'daki postgres) FORCE'u da atlar.
--
-- Çözüm: hakan_rls_runtime. Uygulama havuzu her bağlantıda bu role geçer
-- (database.NewPostgresPool, DB_RLS_RUNTIME). Rol süper kullanıcı değil,
-- BYPASSRLS değil, hiçbir tablonun sahibi değil; RLS ona her zaman uygulanır.
--
-- Kademeli geçiş, tablo başına iki durum:
--   geçişte   rls_legacy politikası (hakan_rls_runtime için USING true):
--             kapsamsız sorgular eskisi gibi çalışır.
--   zorlanmış rls_enforce(tablo): rls_legacy kalkar. Kapsamsız sorgu satır
--             görmez, yazamaz. Yalnızca rls.WithScope (kapsamlı rol) ya da
--             rls.System (açık, gerekçeli atlama) erişir. Ayrıca FORCE ve
--             sahip için app.system politikası: sahibi süper kullanıcı
--             olmayan kurulumlarda da aynı kural geçerli olur.
-- Zorlanmış tablolar internal/database/rls_enforced.txt dosyasında listelenir
-- (TestRLSEnforcement denetler).

DO $$
BEGIN
    BEGIN
        CREATE ROLE hakan_rls_runtime NOLOGIN NOBYPASSRLS;
    EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
    END;
    EXECUTE format('GRANT hakan_rls_runtime TO %I', current_user);
END
$$;

GRANT USAGE ON SCHEMA public TO hakan_rls_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO hakan_rls_runtime;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO hakan_rls_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hakan_rls_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO hakan_rls_runtime;

-- Açık atlama: rls.System bu ayarı işlem başına açar.
CREATE OR REPLACE FUNCTION app_system() RETURNS BOOLEAN
    LANGUAGE sql STABLE AS
$$ SELECT COALESCE(current_setting('app.system', true), '') = 'on' $$;

-- Geçişteki tablo: çalışma zamanı rolü her şeyi görür (eski davranış).
CREATE OR REPLACE FUNCTION rls_legacy(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('DROP POLICY IF EXISTS rls_legacy ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_legacy ON %s TO hakan_rls_runtime USING (true) WITH CHECK (true)', p_table);
END $$;

-- Zorlanan tablo: kapsamsız erişim kapanır.
CREATE OR REPLACE FUNCTION rls_enforce(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('DROP POLICY IF EXISTS rls_legacy ON %s', p_table);
    EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_system ON %s TO CURRENT_USER USING (app_system()) WITH CHECK (app_system())', p_table);
END $$;

-- Şablonlar yeni tabloyu geçiş durumunda açar; zorlamak ayrı ve bilinçli bir
-- adımdır (rls_enforce + rls_enforced.txt + sızıntı testi).
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

-- Bugün RLS'si açık her tablo geçiş durumunda başlar.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT c.oid::regclass AS t FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
    LOOP
        PERFORM rls_legacy(r.t);
    END LOOP;
END
$$;

-- İlk zorlanan grup: dilekçeler. Bütün erişim yolları zaten rls.WithScope
-- içinde (internal/modules/petitions).
SELECT rls_enforce('petitions'::regclass);
