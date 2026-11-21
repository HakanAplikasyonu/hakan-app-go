-- Düzeltme beyannamesi (#575): verilmiş beyan düzeltme için yeniden açılır.
-- Verilmiş hali (donmuş önizleme, ödenecek / devreden, verildi bilgisi,
-- tahakkuk PDF'i, ödeme günü) tax_return_versions'a sürüm olarak yazılır;
-- tax_returns satırı taslağa döner ve version bir artar. Düzeltme
-- beyannamesi öncekiyle farkıyla gösterilir.
ALTER TABLE tax_returns ADD COLUMN version INT NOT NULL DEFAULT 1;
ALTER TABLE tax_returns ADD CONSTRAINT chk_tax_returns_version CHECK (version >= 1);

CREATE TABLE tax_return_versions (
    id                  BIGSERIAL     PRIMARY KEY,
    organization_id     BIGINT        NOT NULL,
    engagement_id       BIGINT        NOT NULL,
    tax_return_id       BIGINT        NOT NULL REFERENCES tax_returns (id) ON DELETE CASCADE,
    version             INT           NOT NULL,
    snapshot            JSONB         NOT NULL,
    payable_amount      NUMERIC(18,2) NULL,
    carry_out           NUMERIC(18,2) NULL,
    filed_on            DATE          NOT NULL,
    accrual_number      TEXT          NULL,
    accrual_storage_key TEXT          NULL,
    accrual_file_name   TEXT          NULL,
    paid_on             DATE          NULL,
    filed_by            BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    amended_by          BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    amended_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tax_return_versions UNIQUE (tax_return_id, version),
    CONSTRAINT fk_tax_return_versions_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_tax_return_versions_return ON tax_return_versions (organization_id, engagement_id, tax_return_id, version DESC);

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('tax_return_versions'::regclass);
