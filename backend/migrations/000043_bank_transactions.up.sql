-- Banka hareketi modeli (issue #20). Kaynak: docs/kaynak/musavir-entegrasyon-paketi/21-banka-veri-modeli.md
--
-- Hepsi mükellefin kaynak verisi (taxpayer_id NOT NULL, taxpayer_tables.txt).
-- Paketteki company_id burada taxpayer_id'dir; iç anahtar BIGSERIAL, dışa uuid.
-- Tutarlar NUMERIC(18,4) ve her zaman pozitif; yön debit_credit ile (A giriş, B çıkış).
-- kind / kind_detail ÖHVPS işlem türü listesi + portal alt türleri (21 §3);
-- enum'lar TEXT + CHECK, liste Go'da internal/modules/banking/model.go ile aynı.

-- ---------------------------------------------------------------------------
-- cash_boxes (01 §4.8 + 21 §1.1): banka hesabı, kredi kartı, valörlü POS kasası.
-- Hareketi hesaba bağlayan ana anahtar IBAN'dır.
-- ---------------------------------------------------------------------------
CREATE TABLE cash_boxes (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    type                 TEXT         NOT NULL DEFAULT 'bank_account',
    name                 TEXT         NOT NULL DEFAULT '',
    bank_id              TEXT         NULL,
    iban                 VARCHAR(34)  NULL,
    currency             CHAR(3)      NOT NULL DEFAULT 'TRY',
    bank_account_number  TEXT         NULL,
    bank_branch_code     TEXT         NULL,
    bank_account_suffix  TEXT         NULL,
    bank_customer_no     TEXT         NULL,
    bank_account_kind    TEXT         NULL,
    bank_connection_id   BIGINT       NULL,
    bank_sync_from       DATE         NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cash_boxes_uuid UNIQUE (uuid),
    CONSTRAINT uq_cash_boxes_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT fk_cash_boxes_connection
        FOREIGN KEY (taxpayer_id, bank_connection_id) REFERENCES source_connections (taxpayer_id, id),
    CONSTRAINT chk_cash_boxes_type CHECK (type IN ('cash', 'bank_account', 'credit_card', 'valor')),
    CONSTRAINT chk_cash_boxes_iban CHECK (iban IS NULL OR iban ~ '^[A-Z]{2}[0-9]{2}[A-Z0-9]{10,30}$'),
    CONSTRAINT chk_cash_boxes_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_cash_boxes_account_kind CHECK (bank_account_kind IS NULL OR bank_account_kind IN ('demand', 'term', 'credit', 'investment', 'gold', 'other'))
);

CREATE UNIQUE INDEX uq_cash_boxes_taxpayer_iban ON cash_boxes (taxpayer_id, iban) WHERE iban IS NOT NULL;

CREATE TRIGGER trg_cash_boxes_set_updated_at
    BEFORE UPDATE ON cash_boxes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- bank_transactions (21 §2): hareketin tek kaydı. Kanıt kaydıdır, muhasebe
-- kaydı değildir. source_system / source_id / capture_method / raw_payload
-- birincil kaynağın kopyasıdır (öncelik api > mt940 > excel_import > pdf_import).
-- ---------------------------------------------------------------------------
CREATE TABLE bank_transactions (
    id                            BIGSERIAL      PRIMARY KEY,
    uuid                          UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id                   BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id                   BIGINT         NOT NULL,
    source_system                 TEXT           NOT NULL,
    source_id                     TEXT           NOT NULL,
    capture_method                TEXT           NOT NULL,
    booked_at                     TIMESTAMPTZ    NOT NULL,
    has_time                      BOOLEAN        NOT NULL DEFAULT FALSE,
    bank_sequence                 INTEGER        NULL,
    transaction_date              DATE           NOT NULL,
    value_date                    DATE           NULL,
    debit_credit                  CHAR(1)        NOT NULL,
    amount                        NUMERIC(18,4)  NOT NULL,
    currency                      CHAR(3)        NOT NULL,
    balance_after                 NUMERIC(18,4)  NULL,
    balance_before                NUMERIC(18,4)  NULL,
    description                   TEXT           NOT NULL DEFAULT '',
    description_extra             TEXT           NULL,
    -- Sınıflandırma için 21 §2.1'e göre normalize edilmiş açıklama.
    description_normalized        TEXT           NOT NULL DEFAULT '',
    bank_reference                TEXT           NULL,
    receipt_number                TEXT           NULL,
    bank_txn_code                 TEXT           NULL,
    mt940_code                    VARCHAR(4)     NULL,
    channel                       TEXT           NULL,
    counterparty_name             TEXT           NULL,
    counterparty_iban             VARCHAR(34)    NULL,
    counterparty_tax_number       VARCHAR(10)    NULL,
    counterparty_identity_number  VARCHAR(11)    NULL,
    counterparty_bank_code        TEXT           NULL,
    eft_query_number              TEXT           NULL,
    payment_purpose               VARCHAR(3)     NULL,
    is_cancelled                  BOOLEAN        NOT NULL DEFAULT FALSE,
    kind                          TEXT           NOT NULL DEFAULT 'DIGER',
    kind_detail                   TEXT           NOT NULL DEFAULT 'unknown',
    kind_source                   TEXT           NULL,
    kind_confidence               NUMERIC(3,2)   NULL,
    -- Sınıflandırmanın gerekçesi (G12): eşleşen kod, kural, kelime.
    kind_reason                   TEXT           NULL,
    classified_at                 TIMESTAMPTZ    NULL,
    match_status                  TEXT           NOT NULL DEFAULT 'unmatched',
    -- 21 §5 kural 3: aynı tarih + yön + tutar + açıklama başı, bakiyesiz.
    possible_duplicate_of         BIGINT         NULL,
    period                        CHAR(7)        NOT NULL,
    raw_payload                   JSONB          NOT NULL DEFAULT '{}'::jsonb,
    content_hash                  CHAR(64)       NULL,
    created_at                    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_transactions_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_transactions_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT fk_bank_transactions_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id),
    CONSTRAINT fk_bank_transactions_possible_duplicate
        FOREIGN KEY (taxpayer_id, possible_duplicate_of) REFERENCES bank_transactions (taxpayer_id, id),
    CONSTRAINT chk_bank_transactions_source_system CHECK (source_system ~ '^bank_[a-z0-9_]+$'),
    CONSTRAINT chk_bank_transactions_capture_method CHECK (capture_method IN ('api', 'mt940', 'excel_import', 'pdf_import')),
    CONSTRAINT chk_bank_transactions_debit_credit CHECK (debit_credit IN ('A', 'B')),
    CONSTRAINT chk_bank_transactions_amount CHECK (amount >= 0),
    CONSTRAINT chk_bank_transactions_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_bank_transactions_counterparty_tax_number CHECK (counterparty_tax_number IS NULL OR counterparty_tax_number ~ '^[0-9]{10}$'),
    CONSTRAINT chk_bank_transactions_counterparty_identity_number CHECK (counterparty_identity_number IS NULL OR counterparty_identity_number ~ '^[0-9]{11}$'),
    CONSTRAINT chk_bank_transactions_kind CHECK (kind IN (
        'HAVALE', 'EFT', 'FAST', 'PARA_YATIRMA', 'PARA_CEKME', 'YABANCI_PARA_HAVALE',
        'YATIRIM_HESABINA_AKTARIM', 'YATIRIM_HESABINDAN_AKTARIM', 'KURUM_FATURA_ODEMESI',
        'CEK', 'SENET', 'SIGORTA_ODEMESI', 'UCRET_KOMISYON_FAIZ', 'SGK_ODEMESI', 'VERGI_ODEMESI',
        'DOVIZ_ALIM', 'DOVIZ_SATIM', 'KREDI_ODEMESI', 'KREDI_KULLANIM', 'KK_ODEMESI', 'KK_NAKIT_AVANS',
        'SANS_OYUNU', 'UYE_ISYERI_ISLEMLERI', 'HGS_OGS_ISLEMLERI', 'DOGRUDAN_BORCLANDIRMA_SISTEMI', 'DIGER')),
    CONSTRAINT chk_bank_transactions_kind_detail CHECK (kind_detail IN (
        'customer_collection', 'supplier_payment', 'own_account_transfer', 'bank_fee', 'commission',
        'bsmv', 'interest_expense', 'interest_income', 'withholding_tax', 'loan_principal',
        'loan_disbursement', 'tax_payment', 'sgk_payment', 'salary_payment', 'rent_payment',
        'utility_payment', 'pos_settlement', 'pos_commission', 'insurance_payment', 'vehicle_toll',
        'fx_trade', 'cash_deposit', 'cash_withdrawal', 'private_or_non_business', 'unknown')),
    CONSTRAINT chk_bank_transactions_kind_source CHECK (kind_source IS NULL OR kind_source IN (
        'bank_code', 'mt940_code', 'payment_purpose', 'counterparty', 'keyword', 'company_rule', 'ai', 'manual')),
    CONSTRAINT chk_bank_transactions_kind_confidence CHECK (kind_confidence IS NULL OR (kind_confidence >= 0 AND kind_confidence <= 1)),
    CONSTRAINT chk_bank_transactions_match_status CHECK (match_status IN ('unmatched', 'suggested', 'matched', 'ignored')),
    CONSTRAINT chk_bank_transactions_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_bank_transactions_taxpayer_period ON bank_transactions (taxpayer_id, period, kind_detail);
CREATE INDEX idx_bank_transactions_cash_box_date ON bank_transactions (taxpayer_id, cash_box_id, transaction_date, bank_sequence);
-- 21 §5 kural 2 ve 3'ün aday araması.
CREATE INDEX idx_bank_transactions_dedupe ON bank_transactions (taxpayer_id, cash_box_id, transaction_date, debit_credit, amount);

CREATE TRIGGER trg_bank_transactions_set_updated_at
    BEFORE UPDATE ON bank_transactions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- bank_transaction_sources (21 §2): aynı hareketin her kanaldaki kaydı.
-- Dosya yüklemesi geri alınınca yalnızca o yüklemenin satırları silinir.
-- ---------------------------------------------------------------------------
CREATE TABLE bank_transaction_sources (
    id                   BIGSERIAL    PRIMARY KEY,
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    bank_transaction_id  BIGINT       NOT NULL,
    cash_box_id          BIGINT       NOT NULL,
    source_system        TEXT         NOT NULL,
    capture_method       TEXT         NOT NULL,
    source_id            TEXT         NOT NULL,
    -- Dosya yüklemesi ise yükleme kaydının uuid'i (#19).
    import_id            UUID         NULL,
    raw_payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    received_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_transaction_sources_key UNIQUE (taxpayer_id, cash_box_id, source_system, source_id),
    CONSTRAINT fk_bank_transaction_sources_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_transaction_sources_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id),
    CONSTRAINT chk_bank_transaction_sources_source_system CHECK (source_system ~ '^bank_[a-z0-9_]+$'),
    CONSTRAINT chk_bank_transaction_sources_capture_method CHECK (capture_method IN ('api', 'mt940', 'excel_import', 'pdf_import'))
);

CREATE INDEX idx_bank_transaction_sources_transaction ON bank_transaction_sources (taxpayer_id, bank_transaction_id);
CREATE INDEX idx_bank_transaction_sources_import ON bank_transaction_sources (taxpayer_id, import_id) WHERE import_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- bank_rules (21 §4 adım 4): mükellefe özel öğrenilmiş sınıflandırma kuralı.
-- Müşavirin her düzeltmesi bir kural olur. match_type ile karşı taraf
-- kimliği ya da normalize açıklama kalıbı eşlenir.
-- ---------------------------------------------------------------------------
CREATE TABLE bank_rules (
    id             BIGSERIAL    PRIMARY KEY,
    uuid           UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id    BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    match_type     TEXT         NOT NULL,
    match_value    TEXT         NOT NULL,
    -- Kural yalnızca bu yönde uygulanır; NULL ise iki yönde.
    debit_credit   CHAR(1)      NULL,
    kind           TEXT         NOT NULL,
    kind_detail    TEXT         NOT NULL,
    hits           INTEGER      NOT NULL DEFAULT 0,
    created_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_rules_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_rules_match UNIQUE NULLS NOT DISTINCT (taxpayer_id, match_type, match_value, debit_credit),
    CONSTRAINT chk_bank_rules_match_type CHECK (match_type IN ('counterparty_iban', 'counterparty_tax_id', 'description')),
    CONSTRAINT chk_bank_rules_debit_credit CHECK (debit_credit IS NULL OR debit_credit IN ('A', 'B')),
    CONSTRAINT chk_bank_rules_match_value CHECK (btrim(match_value) <> '')
);

CREATE TRIGGER trg_bank_rules_set_updated_at
    BEFORE UPDATE ON bank_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- bank_balance_snapshots (21 §1.2) ve bank_daily_summaries (21 §1.3).
-- Bakiye sürekliliği (20 §7.4) ve BNK_BALANCE_MISMATCH için.
-- ---------------------------------------------------------------------------
CREATE TABLE bank_balance_snapshots (
    id                     BIGSERIAL      PRIMARY KEY,
    taxpayer_id            BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id            BIGINT         NOT NULL,
    captured_at            TIMESTAMPTZ    NOT NULL,
    balance                NUMERIC(18,4)  NOT NULL,
    available_balance      NUMERIC(18,4)  NULL,
    blocked_amount         NUMERIC(18,4)  NULL,
    credit_limit           NUMERIC(18,4)  NULL,
    available_with_credit  NUMERIC(18,4)  NULL,
    source                 TEXT           NOT NULL,
    import_id              UUID           NULL,
    created_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_bank_balance_snapshots_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id),
    CONSTRAINT chk_bank_balance_snapshots_source CHECK (source IN ('api', 'mt940', 'excel_import', 'pdf_import'))
);

CREATE INDEX idx_bank_balance_snapshots_cash_box ON bank_balance_snapshots (taxpayer_id, cash_box_id, captured_at);

CREATE TABLE bank_daily_summaries (
    id                 BIGSERIAL      PRIMARY KEY,
    taxpayer_id        BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id        BIGINT         NOT NULL,
    date               DATE           NOT NULL,
    opening_balance    NUMERIC(18,4)  NULL,
    closing_balance    NUMERIC(18,4)  NULL,
    total_credit       NUMERIC(18,4)  NULL,
    total_debit        NUMERIC(18,4)  NULL,
    transaction_count  INTEGER        NULL,
    source             TEXT           NOT NULL,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_daily_summaries_day UNIQUE (taxpayer_id, cash_box_id, date, source),
    CONSTRAINT fk_bank_daily_summaries_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id),
    CONSTRAINT chk_bank_daily_summaries_source CHECK (source IN ('api', 'mt940', 'excel_import', 'pdf_import'))
);

CREATE TRIGGER trg_bank_daily_summaries_set_updated_at
    BEFORE UPDATE ON bank_daily_summaries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
