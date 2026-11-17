-- Mutabakat (#389): mizan – kaynak çifte mutabakatı ve cari mutabakat.
--
-- Mizan (accountplan, trial_balance_lines) ile portal kaynakları (fatura,
-- banka, masraf, Z raporu, bordro) hesap bazında karşılaştırılır. Kaynaklar
-- yevmiye taslağı satırlarına (journal_draft_lines.source_kind) indiği için
-- karşılaştırma hesap kodu üzerinden yapılır; fişlenmemiş kaynak kayıtları
-- "eksik kayıt" adayı olarak listelenir.
--
-- Hepsi ofis iş ürünüdür: organization_id + engagement_id, engagement
-- kapsamlı RLS (FORCE + sahip politikası şablonla gelir, 000086).

-- roles ve role_permissions FORCE altında (000086): izin kopyaları ofis
-- rollerine ancak sistem kipinde yazılır (DATABASE_RULES).
SET LOCAL app.system = 'on';

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant reconciliation', 'tenant.reconciliation.read'),
    ('Write tenant reconciliation', 'tenant.reconciliation.write')
ON CONFLICT (slug) DO NOTHING;

-- Sahip kendiliğinden alır (000044). Stajyer okur ama karar vermez.
SELECT grant_tenant_role_permission(r, p)
FROM unnest(ARRAY['admin', 'accountant', 'member']) AS r,
     unnest(ARRAY['tenant.reconciliation.read', 'tenant.reconciliation.write']) AS p;
SELECT grant_tenant_role_permission('intern', 'tenant.reconciliation.read');

-- Bir mükellefin bir dönemi için bir karşılaştırma çalıştırması. Yeniden
-- çalıştırma yeni satır açar; pano en yenisini gösterir.
CREATE TABLE reconciliation_runs (
    id                       BIGSERIAL      PRIMARY KEY,
    uuid                     UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id          BIGINT         NOT NULL,
    engagement_id            BIGINT         NOT NULL,
    taxpayer_id              BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period                   CHAR(7)        NOT NULL,
    -- Karşılaştırılan mizan ve hareketi bulmak için düşülen önceki mizan
    -- (aynı yılın daha eski dönemi; yoksa yıl başından birikimli).
    trial_balance_import_id  BIGINT         NULL,
    previous_import_id       BIGINT         NULL,
    -- Kaynakların okunduğu dönem aralığı (dahil).
    window_from              CHAR(7)        NOT NULL,
    window_to                CHAR(7)        NOT NULL,
    tolerance                NUMERIC(18,2)  NOT NULL DEFAULT 1,
    -- Sayılar ve toplamlar: {"accounts":..,"matched":..,"differences":..,
    -- "counterparties":..,"babs_issues":..,"difference_total":"..."}.
    summary                  JSONB          NOT NULL DEFAULT '{}'::jsonb,
    created_by               BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at               TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reconciliation_runs_uuid UNIQUE (uuid),
    CONSTRAINT uq_reconciliation_runs_eng_id UNIQUE (engagement_id, id),
    CONSTRAINT fk_reconciliation_runs_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reconciliation_runs_import
        FOREIGN KEY (organization_id, trial_balance_import_id) REFERENCES account_plan_imports (organization_id, id)
        ON DELETE SET NULL (trial_balance_import_id),
    CONSTRAINT fk_reconciliation_runs_previous
        FOREIGN KEY (organization_id, previous_import_id) REFERENCES account_plan_imports (organization_id, id)
        ON DELETE SET NULL (previous_import_id),
    CONSTRAINT chk_reconciliation_runs_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_reconciliation_runs_window CHECK (
        window_from ~ '^[0-9]{4}-(0[1-9]|1[0-2])$' AND window_to ~ '^[0-9]{4}-(0[1-9]|1[0-2])$' AND window_from <= window_to),
    CONSTRAINT chk_reconciliation_runs_tolerance CHECK (tolerance >= 0)
);

CREATE INDEX idx_reconciliation_runs_list ON reconciliation_runs (organization_id, engagement_id, period, id DESC);

CREATE TRIGGER trg_reconciliation_runs_set_updated_at
    BEFORE UPDATE ON reconciliation_runs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Hesap (ana hesap, 3 hane) bazında mizan hareketi ile kaynak toplamı.
CREATE TABLE reconciliation_items (
    id                     BIGSERIAL      PRIMARY KEY,
    uuid                   UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT         NOT NULL,
    engagement_id          BIGINT         NOT NULL,
    run_id                 BIGINT         NOT NULL,
    main_code              CHAR(3)        NOT NULL,
    account_name           TEXT           NOT NULL DEFAULT '',
    ledger_debit           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    ledger_credit          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    source_debit           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    source_credit          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    -- (mizan borç − alacak) − (kaynak borç − alacak).
    difference             NUMERIC(18,2)  NOT NULL DEFAULT 0,
    result                 TEXT           NOT NULL,
    status                 TEXT           NOT NULL DEFAULT 'open',
    -- Kaynak türüne göre kırılım: [{"kind","debit","credit","lines","draft_only_net"}].
    sources                JSONB          NOT NULL DEFAULT '[]'::jsonb,
    -- Fişlenmemiş kaynak kayıtları (eksik kayıt adayları):
    -- [{"kind","id","date","label","amount"}].
    unposted               JSONB          NOT NULL DEFAULT '[]'::jsonb,
    -- Alt hesap kırılımı: [{"code","ledger_net","source_net"}].
    sub_accounts           JSONB          NOT NULL DEFAULT '[]'::jsonb,
    suggestion             TEXT           NULL,
    suggestion_code        TEXT           NULL,
    suggestion_confidence  NUMERIC(5,4)   NULL,
    note                   TEXT           NULL,
    review_suggestion_id   BIGINT         NULL,
    decided_by             BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at             TIMESTAMPTZ    NULL,
    created_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reconciliation_items_uuid UNIQUE (uuid),
    CONSTRAINT uq_reconciliation_items_account UNIQUE (run_id, main_code),
    CONSTRAINT fk_reconciliation_items_run
        FOREIGN KEY (engagement_id, run_id) REFERENCES reconciliation_runs (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reconciliation_items_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_reconciliation_items_main CHECK (main_code ~ '^[0-9]{3}$'),
    CONSTRAINT chk_reconciliation_items_result CHECK (result IN ('matched', 'difference', 'missing_in_ledger', 'missing_in_sources')),
    CONSTRAINT chk_reconciliation_items_status CHECK (status IN ('open', 'explained', 'in_review', 'resolved', 'ignored')),
    CONSTRAINT chk_reconciliation_items_confidence CHECK (suggestion_confidence IS NULL OR (suggestion_confidence >= 0 AND suggestion_confidence <= 1))
);

CREATE INDEX idx_reconciliation_items_run ON reconciliation_items (organization_id, engagement_id, run_id, main_code);

CREATE TRIGGER trg_reconciliation_items_set_updated_at
    BEFORE UPDATE ON reconciliation_items FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Cari mutabakat: karşı firma (VKN/TCKN) bazında mizan bakiyesi (cari
-- hafızasındaki 120 / 320 alt hesabı) ile faturalardan ve eşleşen banka
-- ödemelerinden hesaplanan bakiye; dönemin BA/BS tutarı ile beyan edilen
-- form satırı.
CREATE TABLE counterparty_reconciliations (
    id                     BIGSERIAL      PRIMARY KEY,
    uuid                   UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT         NOT NULL,
    engagement_id          BIGINT         NOT NULL,
    run_id                 BIGINT         NOT NULL,
    counterparty_tax_id    VARCHAR(11)    NOT NULL,
    counterparty_name      TEXT           NOT NULL DEFAULT '',
    -- 120 (alıcı) ya da 320 (satıcı) alt hesabı; cari hafızasında yoksa NULL.
    account_code           VARCHAR(64)    NULL,
    -- İşaretli: pozitif = karşı taraf borçlu (alacağımız), negatif = borcumuz.
    ledger_balance         NUMERIC(18,2)  NULL,
    source_balance         NUMERIC(18,2)  NOT NULL DEFAULT 0,
    difference             NUMERIC(18,2)  NULL,
    result                 TEXT           NOT NULL,
    -- Dönemin faturalarından (KDV hariç) BA (alış) ve BS (satış).
    ba_amount              NUMERIC(18,2)  NOT NULL DEFAULT 0,
    ba_count               INT            NOT NULL DEFAULT 0,
    bs_amount              NUMERIC(18,2)  NOT NULL DEFAULT 0,
    bs_count               INT            NOT NULL DEFAULT 0,
    declared_ba_amount     NUMERIC(18,2)  NULL,
    declared_ba_count      INT            NULL,
    declared_bs_amount     NUMERIC(18,2)  NULL,
    declared_bs_count      INT            NULL,
    ba_result              TEXT           NOT NULL DEFAULT 'not_required',
    bs_result              TEXT           NOT NULL DEFAULT 'not_required',
    status                 TEXT           NOT NULL DEFAULT 'open',
    note                   TEXT           NULL,
    review_suggestion_id   BIGINT         NULL,
    decided_by             BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at             TIMESTAMPTZ    NULL,
    created_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_counterparty_reconciliations_uuid UNIQUE (uuid),
    CONSTRAINT uq_counterparty_reconciliations_party UNIQUE (run_id, counterparty_tax_id),
    CONSTRAINT fk_counterparty_reconciliations_run
        FOREIGN KEY (engagement_id, run_id) REFERENCES reconciliation_runs (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_counterparty_reconciliations_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_counterparty_reconciliations_tax_id CHECK (counterparty_tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_counterparty_reconciliations_result CHECK (result IN ('matched', 'difference', 'no_ledger_account')),
    CONSTRAINT chk_counterparty_reconciliations_ba CHECK (ba_result IN ('not_required', 'ok', 'missing_in_form', 'extra_in_form', 'amount_mismatch', 'count_mismatch', 'no_declaration')),
    CONSTRAINT chk_counterparty_reconciliations_bs CHECK (bs_result IN ('not_required', 'ok', 'missing_in_form', 'extra_in_form', 'amount_mismatch', 'count_mismatch', 'no_declaration')),
    CONSTRAINT chk_counterparty_reconciliations_status CHECK (status IN ('open', 'explained', 'in_review', 'resolved', 'ignored'))
);

CREATE INDEX idx_counterparty_reconciliations_run ON counterparty_reconciliations (organization_id, engagement_id, run_id, counterparty_name);

CREATE TRIGGER trg_counterparty_reconciliations_set_updated_at
    BEFORE UPDATE ON counterparty_reconciliations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Verilen (ya da verilecek) BA / BS formunun satırları: muhasebe
-- programından ya da GİB'den alınan döküm, Excel/CSV ile ya da elle.
CREATE TABLE babs_declarations (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT         NOT NULL,
    engagement_id        BIGINT         NOT NULL,
    taxpayer_id          BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period               CHAR(7)        NOT NULL,
    form_type            TEXT           NOT NULL,
    counterparty_tax_id  VARCHAR(11)    NOT NULL,
    counterparty_name    TEXT           NOT NULL DEFAULT '',
    document_count       INT            NOT NULL DEFAULT 0,
    amount               NUMERIC(18,2)  NOT NULL DEFAULT 0,
    source               TEXT           NOT NULL DEFAULT 'manual',
    created_by           BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_babs_declarations_uuid UNIQUE (uuid),
    CONSTRAINT uq_babs_declarations_line UNIQUE (engagement_id, period, form_type, counterparty_tax_id),
    CONSTRAINT fk_babs_declarations_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_babs_declarations_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_babs_declarations_form CHECK (form_type IN ('ba', 'bs')),
    CONSTRAINT chk_babs_declarations_tax_id CHECK (counterparty_tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_babs_declarations_amounts CHECK (document_count >= 0 AND amount >= 0),
    CONSTRAINT chk_babs_declarations_source CHECK (source IN ('import', 'manual', 'demo'))
);

CREATE INDEX idx_babs_declarations_period ON babs_declarations (organization_id, engagement_id, period, form_type);

CREATE TRIGGER trg_babs_declarations_set_updated_at
    BEFORE UPDATE ON babs_declarations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Cari mutabakat mektubu: PDF anlık görüntüden (snapshot) üretilir,
-- e-posta kuyruğa yazılır ve worker gönderir; karşı taraf bağlantıdaki
-- jetonla "mutabıkız / değiliz" yanıtı verir ya da müşavir yanıtı elle girer.
CREATE TABLE reconciliation_letters (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT         NOT NULL,
    engagement_id        BIGINT         NOT NULL,
    taxpayer_id          BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    counterparty_tax_id  VARCHAR(11)    NOT NULL,
    counterparty_name    TEXT           NOT NULL DEFAULT '',
    recipient_email      TEXT           NULL,
    account_code         VARCHAR(64)    NULL,
    as_of_date           DATE           NOT NULL,
    -- İşaretli, mükellefin defterindeki bakiye (counterparty_reconciliations gibi).
    balance              NUMERIC(18,2)  NOT NULL,
    currency             CHAR(3)        NOT NULL DEFAULT 'TRY',
    -- Mektubun basıldığı anlık görüntü (ofis, mükellef, metin).
    snapshot             JSONB          NOT NULL DEFAULT '{}'::jsonb,
    status               TEXT           NOT NULL DEFAULT 'draft',
    -- Yanıt bağlantısının jetonu yalnızca özetiyle saklanır.
    token_digest         CHAR(64)       NULL,
    token_expires_at     TIMESTAMPTZ    NULL,
    attempts             INT            NOT NULL DEFAULT 0,
    next_attempt_at      TIMESTAMPTZ    NULL,
    last_error           TEXT           NULL,
    sent_at              TIMESTAMPTZ    NULL,
    response             TEXT           NULL,
    response_balance     NUMERIC(18,2)  NULL,
    response_note        TEXT           NULL,
    responder_name       TEXT           NULL,
    response_channel     TEXT           NULL,
    responded_at         TIMESTAMPTZ    NULL,
    created_by           BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reconciliation_letters_uuid UNIQUE (uuid),
    CONSTRAINT fk_reconciliation_letters_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_reconciliation_letters_tax_id CHECK (counterparty_tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_reconciliation_letters_status CHECK (status IN ('draft', 'queued', 'sent', 'failed', 'responded')),
    CONSTRAINT chk_reconciliation_letters_response CHECK (response IS NULL OR response IN ('agreed', 'disputed')),
    CONSTRAINT chk_reconciliation_letters_channel CHECK (response_channel IS NULL OR response_channel IN ('link', 'manual')),
    CONSTRAINT chk_reconciliation_letters_responded CHECK ((response IS NULL) = (responded_at IS NULL)),
    CONSTRAINT chk_reconciliation_letters_digest CHECK (token_digest IS NULL OR token_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_reconciliation_letters_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX uq_reconciliation_letters_token ON reconciliation_letters (token_digest) WHERE token_digest IS NOT NULL;
CREATE INDEX idx_reconciliation_letters_list ON reconciliation_letters (organization_id, engagement_id, counterparty_tax_id, id DESC);
CREATE INDEX idx_reconciliation_letters_queue ON reconciliation_letters (next_attempt_at) WHERE status = 'queued';

CREATE TRIGGER trg_reconciliation_letters_set_updated_at
    BEFORE UPDATE ON reconciliation_letters FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY[
    'reconciliation_runs', 'reconciliation_items', 'counterparty_reconciliations',
    'babs_declarations', 'reconciliation_letters'
]::regclass[]) AS t;
