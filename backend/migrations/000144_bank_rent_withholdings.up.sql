-- Faturasız kira stopajı (#575): şahıstan kiralanan işyerinin kirası bankadan
-- net ödenir; stopaj (GVK 94/5-a, %20) mükellefin muhtasarına girer. Ofis
-- banka çıkışını "kira ödemesi" işaretler: net = hareket tutarı,
-- brüt = net / (1 − oran), stopaj = brüt − net. Dönem hareketin dönemi
-- (stopaj ödendiği ayın muhtasarında beyan edilir). bank_transaction_id
-- mükellefin bank_transactions.id'si; bir hareket bir kez işaretlenir.
CREATE TABLE bank_rent_withholdings (
    id                  BIGSERIAL     PRIMARY KEY,
    organization_id     BIGINT        NOT NULL,
    engagement_id       BIGINT        NOT NULL,
    bank_transaction_id BIGINT        NOT NULL REFERENCES bank_transactions (id) ON DELETE CASCADE,
    period              CHAR(7)       NOT NULL,
    rate                NUMERIC(5,2)  NOT NULL,
    net_amount          NUMERIC(18,2) NOT NULL,
    gross_amount        NUMERIC(18,2) NOT NULL,
    tax_amount          NUMERIC(18,2) NOT NULL,
    created_by          BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_rent_withholdings_tx UNIQUE (organization_id, engagement_id, bank_transaction_id),
    CONSTRAINT fk_bank_rent_withholdings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_rent_withholdings_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_bank_rent_withholdings_amounts
        CHECK (rate > 0 AND rate < 100 AND net_amount > 0 AND gross_amount = net_amount + tax_amount)
);

CREATE INDEX idx_bank_rent_withholdings_period ON bank_rent_withholdings (organization_id, engagement_id, period);
CREATE INDEX idx_bank_rent_withholdings_tx ON bank_rent_withholdings (bank_transaction_id);

CREATE TRIGGER trg_bank_rent_withholdings_set_updated_at
    BEFORE UPDATE ON bank_rent_withholdings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('bank_rent_withholdings'::regclass);
