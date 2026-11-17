-- Banka hareketinin ödeyen / alıcı takma adı (issue #383). Bankada
-- "X Yazılım Teknolojileri A.Ş." hesabından gelen ödeme mizanda başka bir
-- carinin (ör. "Oğuz Yılmaz", 120.01.015) hesabıdır. Müşavirin kararı
-- mükellef (engagement) bazında karşı tarafın IBAN'ı, VKN / TCKN'si ya da
-- normalize edilmiş adıyla hatırlanır; sonraki hareketlerde hesap kodu
-- buradan önerilir. Ofisin iş ürünüdür: organization_id + engagement_id.
--
-- status: learned = müşavir kararlarından öğrenildi (inceleme kuyruğundan
-- geçer); rule = müşavir tanımladı ya da kural adayını onayladı (güven
-- yüksek, eşik izin verirse kendiliğinden uygulanır). Aday eşiği ve hangi
-- kararların sayılacağı account_rule_settings'tedir (accountrules ile aynı
-- terfi mantığı); otomatik onaylar sayılmaz.
CREATE TABLE bank_counterparty_aliases (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    -- iban: normalize IBAN; tax_id: 10/11 hane; name: banking.NormalizeName.
    match_type       TEXT         NOT NULL,
    match_value      TEXT         NOT NULL,
    -- Bankada görülen ad (gösterim için).
    display_name     TEXT         NOT NULL DEFAULT '',
    -- A giriş, B çıkış; NULL iki yönde.
    debit_credit     CHAR(1)      NULL,
    account_code     VARCHAR(64)  NOT NULL,
    account_name     TEXT         NOT NULL DEFAULT '',
    status           TEXT         NOT NULL DEFAULT 'learned',
    origin           TEXT         NOT NULL DEFAULT 'decision',
    -- Aynı hesap koduna verilen elle onay ve düzeltme sayıları.
    accepts          INTEGER      NOT NULL DEFAULT 0,
    corrections      INTEGER      NOT NULL DEFAULT 0,
    hits             INTEGER      NOT NULL DEFAULT 0,
    -- Son kararı yazan inceleme önerisi (geri alınınca sayaç düşer).
    suggestion_id    BIGINT       NULL,
    decided_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    last_decided_at  TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_counterparty_aliases_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_counterparty_aliases_key UNIQUE NULLS NOT DISTINCT (engagement_id, match_type, match_value, debit_credit),
    CONSTRAINT fk_bank_counterparty_aliases_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_counterparty_aliases_type CHECK (match_type IN ('iban', 'tax_id', 'name')),
    CONSTRAINT chk_bank_counterparty_aliases_value CHECK (btrim(match_value) <> ''),
    CONSTRAINT chk_bank_counterparty_aliases_tax_id CHECK (match_type <> 'tax_id' OR match_value ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_bank_counterparty_aliases_iban CHECK (match_type <> 'iban' OR match_value ~ '^[A-Z]{2}[0-9]{2}[A-Z0-9]{10,30}$'),
    CONSTRAINT chk_bank_counterparty_aliases_dc CHECK (debit_credit IS NULL OR debit_credit IN ('A', 'B')),
    CONSTRAINT chk_bank_counterparty_aliases_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_bank_counterparty_aliases_status CHECK (status IN ('learned', 'rule')),
    CONSTRAINT chk_bank_counterparty_aliases_origin CHECK (origin IN ('decision', 'manual', 'candidate')),
    CONSTRAINT chk_bank_counterparty_aliases_counts CHECK (accepts >= 0 AND corrections >= 0 AND hits >= 0)
);

CREATE INDEX idx_bank_counterparty_aliases_lookup ON bank_counterparty_aliases (organization_id, engagement_id, match_type, match_value);

CREATE TRIGGER trg_bank_counterparty_aliases_set_updated_at
    BEFORE UPDATE ON bank_counterparty_aliases FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('bank_counterparty_aliases'::regclass);
