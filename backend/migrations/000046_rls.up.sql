-- Postgres RLS (issue #6, G8): mali veride izolasyon yalnızca uygulama koduna
-- bırakılmaz.
--
-- Roller:
--   hakan_rls_app     API'nin kapsamlı işlemleri (SET LOCAL ROLE ile)
--   hakan_rls_worker  arka plan işleri (SET LOCAL ROLE ile)
--   İkisi de NOLOGIN ve BYPASSRLS değildir. Tabloların sahibi (göç kullanıcısı)
--   RLS'ten muaftır: tablolar FORCE değil, böylece göçler ve henüz kapsama
--   alınmamış kod yolları eskisi gibi çalışır. BYPASSRLS yalnızca göç
--   kullanıcısında (tablo sahibi) kalır.
--
-- İşlem başına bağlam (SET LOCAL, internal/platform/rls):
--   app.organization_id        ofis (bigint)
--   app.user_id                kullanıcı (bigint)
--   app.taxpayer_ids           erişilebilir mükellefler ('{1,2}')
--   app.engagements_read_all   'on' ise ofisin bütün engagement'ları
--
-- Politika şablonları (tablo türü başına):
--   org         organization_id = app.organization_id
--   engagement  org + engagement'a atanmış ya da read_all
--   taxpayer    taxpayer_id = ANY(app.taxpayer_ids)
--   platform    kapsamlı rollere hiç satır yok
-- Bağlam yoksa (ayar boş) hiçbir politika satır döndürmez.

-- Roller küme genelidir. Aynı sunucuda eşzamanlı göçler (paralel test
-- veritabanları) aynı rolü aynı anda oluşturursa ikinci işlem
-- duplicate_object yerine unique_violation (pg_authid) alır; ikisi de
-- "rol zaten var" demektir.
DO $$
BEGIN
    BEGIN
        CREATE ROLE hakan_rls_app NOLOGIN NOBYPASSRLS;
    EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
    END;
    BEGIN
        CREATE ROLE hakan_rls_worker NOLOGIN NOBYPASSRLS;
    EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
    END;
END
$$;

-- Bağlantı kullanıcısı (göç ve API kullanıcısı) bu rollere geçebilmeli.
DO $$
BEGIN
    EXECUTE format('GRANT hakan_rls_app TO %I', current_user);
    EXECUTE format('GRANT hakan_rls_worker TO %I', current_user);
END
$$;

GRANT USAGE ON SCHEMA public TO hakan_rls_app, hakan_rls_worker;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO hakan_rls_app, hakan_rls_worker;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO hakan_rls_app, hakan_rls_worker;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hakan_rls_app, hakan_rls_worker;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO hakan_rls_app, hakan_rls_worker;

-- Bağlam okuyucuları. Boş ya da tanımsız ayar NULL / boş dizi döner.
CREATE OR REPLACE FUNCTION app_current_organization_id() RETURNS BIGINT
    LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.organization_id', true), '')::bigint $$;

CREATE OR REPLACE FUNCTION app_current_user_id() RETURNS BIGINT
    LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.user_id', true), '')::bigint $$;

CREATE OR REPLACE FUNCTION app_current_taxpayer_ids() RETURNS BIGINT[]
    LANGUAGE sql STABLE AS
$$ SELECT COALESCE(NULLIF(current_setting('app.taxpayer_ids', true), '')::bigint[], '{}'::bigint[]) $$;

CREATE OR REPLACE FUNCTION app_engagements_read_all() RETURNS BOOLEAN
    LANGUAGE sql STABLE AS
$$ SELECT COALESCE(current_setting('app.engagements_read_all', true), '') = 'on' $$;

-- engagement_assignments'ı politika içinden okur. SECURITY DEFINER: tablo
-- sahibi olarak çalışır, böylece atama tablosunun kendi politikası döngüye
-- girmez. Yalnızca o anki kullanıcı ve ofis için bakar.
CREATE OR REPLACE FUNCTION app_engagement_visible(p_organization_id BIGINT, p_engagement_id BIGINT)
RETURNS BOOLEAN
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS
$$
    SELECT p_organization_id = app_current_organization_id()
       AND (app_engagements_read_all()
            OR EXISTS (SELECT 1 FROM engagement_assignments a
                       WHERE a.organization_id = p_organization_id
                         AND a.engagement_id = p_engagement_id
                         AND a.user_id = app_current_user_id()));
$$;

-- Şablonlar. Yeni tablo eklerken migration'da tek satırla çağrılır
-- (DATABASE_RULES.md). RLS etkin ve politikası olmayan tablo kapsamlı rollere
-- satır vermez; TestRLSCoversScopedTables bunu tablo listelerine karşı denetler.
CREATE OR REPLACE FUNCTION rls_apply_org(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_org ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_org ON %s TO hakan_rls_app, hakan_rls_worker
        USING (organization_id = app_current_organization_id())
        WITH CHECK (organization_id = app_current_organization_id())', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_engagement(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_engagement ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_engagement ON %s TO hakan_rls_app, hakan_rls_worker
        USING (app_engagement_visible(organization_id, engagement_id))
        WITH CHECK (app_engagement_visible(organization_id, engagement_id))', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_taxpayer(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
    EXECUTE format('DROP POLICY IF EXISTS rls_taxpayer ON %s', p_table);
    EXECUTE format('CREATE POLICY rls_taxpayer ON %s TO hakan_rls_app, hakan_rls_worker
        USING (taxpayer_id = ANY (app_current_taxpayer_ids()))
        WITH CHECK (taxpayer_id = ANY (app_current_taxpayer_ids()))', p_table);
END $$;

CREATE OR REPLACE FUNCTION rls_apply_platform(p_table REGCLASS) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    -- RLS etkin, kapsamlı rollere politika yok: hiçbir satır görünmez.
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', p_table);
END $$;

-- org (tenant_tables.txt, ikinci sözcük yok)
SELECT rls_apply_org(t) FROM unnest(ARRAY[
    'organization_members', 'organization_invitations', 'engagements',
    'ai_usage_events', 'warning_rule_configs', 'review_settings'
]::regclass[]) AS t;

-- engagement (tenant_tables.txt, "engagement")
SELECT rls_apply_engagement(t) FROM unnest(ARRAY[
    'engagement_assignments', 'data_warnings', 'change_requests', 'ubl_upload_jobs',
    'taxpayer_invitations', 'suggestions', 'suggestion_decisions'
]::regclass[]) AS t;

-- taxpayer (taxpayer_tables.txt)
SELECT rls_apply_taxpayer(t) FROM unnest(ARRAY[
    'source_connections', 'customers', 'customer_source_links', 'attachments',
    'invoices', 'invoice_items', 'invoice_tax_summaries', 'sync_windows', 'sync_runs',
    'taxpayer_members', 'invoice_sources', 'cash_boxes', 'bank_transactions',
    'bank_transaction_sources', 'bank_rules', 'bank_balance_snapshots', 'bank_daily_summaries'
]::regclass[]) AS t;

-- Mükellef kimliği: kapsamlı rol yalnızca erişebildiği mükellefi görür.
ALTER TABLE taxpayers ENABLE ROW LEVEL SECURITY;
CREATE POLICY rls_taxpayer_identity ON taxpayers TO hakan_rls_app, hakan_rls_worker
    USING (id = ANY (app_current_taxpayer_ids()));

-- platform: sır kasası kapsamlı işlemlerde hiç okunmaz.
SELECT rls_apply_platform('secret_records'::regclass);
