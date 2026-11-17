-- Mizan raporu (#393): mizan (trial balance) içe aktarmalarının borç / alacak
-- / bakiye satırları. account_plan_accounts yalnızca etkin planın kodlarını
-- tutar; mizan satırları her içe aktarma için ayrı saklanır ki son mizan ile
-- önceki dönem karşılaştırılabilsin (ofis mobil uygulaması, mükellef
-- ayrıntısı → Belgeler → Mizan).
--
-- Ofis iş ürünü: organization_id + engagement_id (account_plan_imports gibi),
-- engagement kapsamlı RLS.

-- Mizanın dönemi ("YYYY-MM"): yüklemede verilir ya da dosya adından okunur;
-- bilinmiyorsa NULL (liste yükleme tarihini gösterir).
ALTER TABLE account_plan_imports ADD COLUMN period CHAR(7) NULL;
ALTER TABLE account_plan_imports ADD CONSTRAINT chk_account_plan_imports_period
    CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$');

CREATE INDEX idx_account_plan_imports_trial_balance
    ON account_plan_imports (organization_id, engagement_id, id DESC)
    WHERE source_kind = 'trial_balance';

CREATE TABLE trial_balance_lines (
    id              BIGSERIAL      PRIMARY KEY,
    organization_id BIGINT         NOT NULL,
    engagement_id   BIGINT         NOT NULL,
    import_id       BIGINT         NOT NULL,
    code            VARCHAR(64)    NOT NULL,
    name            TEXT           NOT NULL DEFAULT '',
    main_code       CHAR(3)        NOT NULL,
    level           SMALLINT       NOT NULL,
    is_leaf         BOOLEAN        NOT NULL DEFAULT true,
    -- Dönem sonu toplamları (TL). balance = borç bakiyesi − alacak bakiyesi;
    -- dosyada bakiye sütunu yoksa debit − credit.
    debit           NUMERIC(18, 2) NOT NULL DEFAULT 0,
    credit          NUMERIC(18, 2) NOT NULL DEFAULT 0,
    balance         NUMERIC(18, 2) NOT NULL DEFAULT 0,
    CONSTRAINT uq_trial_balance_lines_code UNIQUE (import_id, code),
    CONSTRAINT fk_trial_balance_lines_import
        FOREIGN KEY (organization_id, import_id) REFERENCES account_plan_imports (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_trial_balance_lines_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_trial_balance_lines_code CHECK (code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_trial_balance_lines_level CHECK (level >= 1)
);

CREATE INDEX idx_trial_balance_lines_list ON trial_balance_lines (organization_id, engagement_id, import_id, code);

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('trial_balance_lines'::regclass);
