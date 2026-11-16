-- Cari hafızası (#319): müşavirin onayladığı ya da düzelttiği cari hesap,
-- mükellef (engagement) bazında karşı tarafın VKN / TCKN'si ile hatırlanır.
-- Mizanda VKN yok; sonraki faturalarda cari önce bu eşlemeyle, olmazsa
-- unvanla bulunur. Bir firma hem alıcı (120) hem satıcı (320) olabilir:
-- anahtar VKN + ana hesap.
CREATE TABLE counterparty_accounts (
    id                  BIGSERIAL    PRIMARY KEY,
    organization_id     BIGINT       NOT NULL,
    engagement_id       BIGINT       NOT NULL,
    taxpayer_id         BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    counterparty_tax_id VARCHAR(11)  NOT NULL,
    main_code           CHAR(3)      NOT NULL,
    account_code        VARCHAR(64)  NOT NULL,
    account_name        TEXT         NOT NULL DEFAULT '',
    -- Eşlemeyi yazan inceleme kararı (geri alınınca eşleme silinir).
    suggestion_id       BIGINT       NULL,
    decided_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_counterparty_accounts_key UNIQUE (engagement_id, counterparty_tax_id, main_code),
    CONSTRAINT fk_counterparty_accounts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_counterparty_accounts_tax_id CHECK (counterparty_tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_counterparty_accounts_main CHECK (main_code ~ '^[0-9]{3}$'),
    CONSTRAINT chk_counterparty_accounts_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$')
);

CREATE TRIGGER trg_counterparty_accounts_set_updated_at
    BEFORE UPDATE ON counterparty_accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('counterparty_accounts'::regclass);
