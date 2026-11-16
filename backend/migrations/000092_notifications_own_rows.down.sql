DROP POLICY IF EXISTS rls_user_own ON notifications;
DROP POLICY IF EXISTS rls_user_read ON notifications;
CREATE POLICY rls_user_read ON notifications FOR SELECT TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id()
           OR (organization_id IS NULL AND (user_id IS NULL OR user_id = app_current_user_id())));
