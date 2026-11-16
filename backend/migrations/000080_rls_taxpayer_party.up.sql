-- Mükellef paneli kapsamı (issue #166). Mükellef uçları (/v1/taxpayer/*)
-- artık rls.Middleware ile mükellef kapsamında çalışıyor: ofis yok, kullanıcı
-- ve üyesi olduğu mükellef var. Panel, ofis ve engagement tablolarında
-- mükellefin kendisine ait satırları okuyup yazıyor (engagement'ları, belge
-- görevleri, fiş yakalamaları, yükleme işleri, ekleme talepleri). Bu
-- tabloların ofis politikaları ofis kimliği istediği için bu satırlar
-- mükellef kapsamında görünmüyordu.
--
-- rls_taxpayer_party yalnızca OFİS OLMAYAN kapsamda geçerli
-- (app_current_organization_id() IS NULL) ve yalnızca kapsamın kendi
-- mükelleflerinin satırlarını açıyor. Ofis kapsamında hiç devreye girmiyor:
-- aynı mükellefe hizmet vermiş iki ofis birbirinin satırını yine görmez.
CREATE OR REPLACE FUNCTION rls_apply_taxpayer_party(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer_party ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_taxpayer_party ON %s TO hakan_rls_app, hakan_rls_worker
        USING (app_current_organization_id() IS NULL AND taxpayer_id = ANY (app_current_taxpayer_ids()))
        WITH CHECK (app_current_organization_id() IS NULL AND taxpayer_id = ANY (app_current_taxpayer_ids()))', p_table);
END $$;

SELECT rls_apply_taxpayer_party(t) FROM unnest(ARRAY[
    'engagements', 'ubl_upload_jobs', 'receipt_captures', 'document_tasks', 'document_task_uploads'
]::regclass[]) AS t;

-- Ekleme talepleri onaydan önce mükellef kimliğine bağlı değil: talep VKN/TCKN
-- ile bulunur. Mükellef kapsamı kendi kimliğinin taleplerini görür.
DROP POLICY IF EXISTS rls_taxpayer_party ON taxpayer_onboarding_requests;
CREATE POLICY rls_taxpayer_party ON taxpayer_onboarding_requests TO hakan_rls_app, hakan_rls_worker
    USING (app_current_organization_id() IS NULL AND (
        taxpayer_id = ANY (app_current_taxpayer_ids())
        OR tax_id IN (SELECT t.tax_id FROM taxpayers t WHERE t.id = ANY (app_current_taxpayer_ids()))))
    WITH CHECK (app_current_organization_id() IS NULL AND (
        taxpayer_id = ANY (app_current_taxpayer_ids())
        OR tax_id IN (SELECT t.tax_id FROM taxpayers t WHERE t.id = ANY (app_current_taxpayer_ids()))));

-- KVKK onayları (000077): mükellef kapsamında geçmiş veri paylaşımı onayı,
-- paylaşımın yapıldığı ofisi (organization_id) adlandırır. Kapsamda ofis
-- olmadığı için 000077'nin yazma koşulu bunu reddediyordu. Ofis olmayan
-- kapsamda, kapsamın kendi mükellefi için verilen onay bir ofisi
-- adlandırabilir.
DROP POLICY IF EXISTS rls_scoped ON legal_acceptances;
CREATE POLICY rls_scoped ON legal_acceptances TO hakan_rls_app, hakan_rls_worker
    USING (user_id = app_current_user_id()
           OR organization_id = app_current_organization_id()
           OR taxpayer_id = ANY (app_current_taxpayer_ids()))
    WITH CHECK (user_id = app_current_user_id()
                AND (taxpayer_id IS NULL OR taxpayer_id = ANY (app_current_taxpayer_ids()))
                AND (organization_id IS NULL
                     OR organization_id = app_current_organization_id()
                     OR (app_current_organization_id() IS NULL AND taxpayer_id = ANY (app_current_taxpayer_ids()))));
