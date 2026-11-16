DO $$
DECLARE t REGCLASS;
BEGIN
    FOREACH t IN ARRAY ARRAY['notifications', 'activity_events']::regclass[]
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS rls_user_read ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_user_write ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_legacy ON %s', t);
        EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', t);
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %s DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END
$$;
