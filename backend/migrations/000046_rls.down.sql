-- RLS politikalarını, yardımcı fonksiyonları ve kapsamlı rollerin izinlerini
-- kaldırır. Roller kümeye ait olduğu için silinmez (başka veritabanları
-- kullanıyor olabilir).
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN SELECT schemaname, tablename, policyname FROM pg_policies
             WHERE schemaname = 'public' AND policyname LIKE 'rls\_%'
    LOOP
        EXECUTE format('DROP POLICY %I ON %I.%I', r.policyname, r.schemaname, r.tablename);
    END LOOP;
    FOR r IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
             WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
    LOOP
        EXECUTE format('ALTER TABLE public.%I DISABLE ROW LEVEL SECURITY', r.relname);
    END LOOP;
END
$$;

DROP FUNCTION IF EXISTS rls_apply_platform(REGCLASS);
DROP FUNCTION IF EXISTS rls_apply_taxpayer(REGCLASS);
DROP FUNCTION IF EXISTS rls_apply_engagement(REGCLASS);
DROP FUNCTION IF EXISTS rls_apply_org(REGCLASS);
DROP FUNCTION IF EXISTS app_engagement_visible(BIGINT, BIGINT);
DROP FUNCTION IF EXISTS app_engagements_read_all();
DROP FUNCTION IF EXISTS app_current_taxpayer_ids();
DROP FUNCTION IF EXISTS app_current_user_id();
DROP FUNCTION IF EXISTS app_current_organization_id();

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM hakan_rls_app, hakan_rls_worker;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE USAGE, SELECT ON SEQUENCES FROM hakan_rls_app, hakan_rls_worker;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM hakan_rls_app, hakan_rls_worker;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM hakan_rls_app, hakan_rls_worker;
REVOKE USAGE ON SCHEMA public FROM hakan_rls_app, hakan_rls_worker;
