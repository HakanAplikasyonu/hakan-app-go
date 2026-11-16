DO $$
DECLARE t REGCLASS;
BEGIN
    FOREACH t IN ARRAY ARRAY['engagements', 'ubl_upload_jobs', 'receipt_captures', 'document_tasks',
                             'document_task_uploads', 'taxpayer_onboarding_requests']::regclass[] LOOP
        EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer_party ON %s', t);
    END LOOP;
END
$$;
DROP FUNCTION IF EXISTS rls_apply_taxpayer_party(REGCLASS);

DROP POLICY IF EXISTS rls_scoped ON legal_acceptances;
CREATE POLICY rls_scoped ON legal_acceptances TO hakan_rls_app, hakan_rls_worker
    USING (user_id = app_current_user_id()
           OR organization_id = app_current_organization_id()
           OR taxpayer_id = ANY (app_current_taxpayer_ids()))
    WITH CHECK (user_id = app_current_user_id()
                AND (organization_id IS NULL OR organization_id = app_current_organization_id())
                AND (taxpayer_id IS NULL OR taxpayer_id = ANY (app_current_taxpayer_ids())));
