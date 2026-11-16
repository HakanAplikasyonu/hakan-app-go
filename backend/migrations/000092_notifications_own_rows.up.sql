-- Kullanıcının kendi bildirimleri her kapsamda (issue #333): mükellef paneli
-- ve ofis paneli bildirim listesi.
--
-- 000088 ofissiz satırlarda kullanıcı koşulu koymuştu. Ama mükellefe giden
-- bildirimler (belge isteği, hatırlatma) ofisin satırıdır (organization_id =
-- ofis): mükellef kapsamında (ofis yok) kullanıcı kendi bildirimini
-- göremiyor, okundu işaretleyemiyordu. Kullanıcı kendi bildirimini (user_id)
-- her kapsamda okur ve okundu olarak günceller; yeni satır yazma kuralı
-- (yalnızca kapsamın ofisi, rls_user_write) değişmez, güncelleme satırı
-- başka bir kullanıcıya taşıyamaz.
DROP POLICY IF EXISTS rls_user_read ON notifications;
CREATE POLICY rls_user_read ON notifications FOR SELECT TO hakan_rls_app, hakan_rls_worker
    USING (organization_id = app_current_organization_id()
           OR user_id = app_current_user_id()
           OR (organization_id IS NULL AND user_id IS NULL));
DROP POLICY IF EXISTS rls_user_own ON notifications;
CREATE POLICY rls_user_own ON notifications FOR UPDATE TO hakan_rls_app, hakan_rls_worker
    USING (user_id = app_current_user_id())
    WITH CHECK (user_id = app_current_user_id());
