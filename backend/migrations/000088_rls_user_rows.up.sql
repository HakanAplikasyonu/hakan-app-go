-- Bildirimler ve etkinlik kaydı (issue #307, ikinci grup): ofis satırlarının
-- yanında kullanıcı ve platform satırları (organization_id NULL) taşıyan iki
-- tablo. rls_apply_org_shared'e ek olarak NULL satırda kullanıcı koşulu:
-- bir kapsam başka kullanıcının ofissiz bildirimini ya da etkinliğini
-- görmez.
--
--   okuma  kapsamın ofisinin satırları (ofis kendi bildirimlerini ve
--          etkinliklerini bütün kullanıcıları için görür; INSERT ... RETURNING
--          de bunu ister) ya da ofissiz satırlardan kapsamın kullanıcısına
--          ait olanlar (bildirimlerde kullanıcısız satırlar da)
--   yazma  yalnızca kapsamın ofisi; ofissiz satır yalnızca rls.System'den ya
--          da geçişteki kapsamsız sorgudan (rls_legacy) yazılır.
-- Teslim işçisi ve kullanıcı uçları (/v1/notifications, /v1/me) kapsamsız
-- çalışır, değişmez.

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_user_read ON notifications;
CREATE POLICY rls_user_read ON notifications FOR SELECT TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id()
           OR (organization_id IS NULL AND (user_id IS NULL OR user_id = app_current_user_id())));
DROP POLICY IF EXISTS rls_user_write ON notifications;
CREATE POLICY rls_user_write ON notifications TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id())
    WITH CHECK (organization_id = app_current_organization_id());
SELECT rls_legacy('notifications');
SELECT rls_owner_force('notifications');

ALTER TABLE activity_events ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_user_read ON activity_events;
CREATE POLICY rls_user_read ON activity_events FOR SELECT TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id()
           OR (organization_id IS NULL AND actor_user_id = app_current_user_id()));
DROP POLICY IF EXISTS rls_user_write ON activity_events;
CREATE POLICY rls_user_write ON activity_events TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id())
    WITH CHECK (organization_id = app_current_organization_id());
SELECT rls_legacy('activity_events');
SELECT rls_owner_force('activity_events');
