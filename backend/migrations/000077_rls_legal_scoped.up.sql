-- Hukuki metinler kapsamlı işlemlerde de okunur ve onaylanır (issue #166).
-- 000071 iki tabloyu rls_apply_platform ile açmıştı: kapsamlı rollere hiç
-- politika yok. Tenant istekleri artık ofis kapsamında çalıştığı için
-- (rls.Middleware) metinler 404 dönüyor, ofisin veri işleyen sözleşmesi
-- onayı RLS hatası alıyordu.
--
-- legal_documents: yayımlanmış ve emekli metinler herkese açık; taslaklar
--   yalnızca platform yönetiminin (kapsamsız) işi.
-- legal_acceptances: kapsam, kendi kullanıcısının, ofisinin ve
--   erişebildiği mükelleflerin onaylarını görür ve yalnızca onları yazar.
CREATE POLICY rls_scoped_read ON legal_documents
    FOR SELECT TO hakan_rls_app, hakan_rls_worker
    USING (status <> 'draft');

CREATE POLICY rls_scoped ON legal_acceptances TO hakan_rls_app, hakan_rls_worker
    USING (user_id = app_current_user_id()
           OR organization_id = app_current_organization_id()
           OR taxpayer_id = ANY (app_current_taxpayer_ids()))
    WITH CHECK (user_id = app_current_user_id()
                AND (organization_id IS NULL OR organization_id = app_current_organization_id())
                AND (taxpayer_id IS NULL OR taxpayer_id = ANY (app_current_taxpayer_ids())));
