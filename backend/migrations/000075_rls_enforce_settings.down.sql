DO $$
DECLARE t REGCLASS;
BEGIN
    FOREACH t IN ARRAY ARRAY['vat_carry_forwards', 'export_settings']::regclass[] LOOP
        EXECUTE format('DROP POLICY IF EXISTS rls_system ON %s', t);
        EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', t);
        PERFORM rls_legacy(t);
    END LOOP;
END
$$;
