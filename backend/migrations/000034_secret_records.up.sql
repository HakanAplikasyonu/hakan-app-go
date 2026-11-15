-- Sır kasası (G9, #18): kayıt başına veri anahtarı (DEK) ile zarf şifreleme.
-- Düz metin bu tabloya hiçbir zaman yazılmaz. ciphertext = nonce|AES-256-GCM(DEK),
-- wrapped_dek = nonce|AES-256-GCM(KEK). KEK ortam sırrındadır (SECRETS_KEYRING /
-- APP_ENCRYPTION_KEY), veritabanında değil. Rotasyon yalnızca wrapped_dek ve kek_id'yi
-- değiştirir; ciphertext aynı kalır.
--
-- Tablo kiracı tablosu değildir: satırlar yalnızca sahibi olan kaydın
-- (ör. source_connection.credentials_ref) referansıyla okunur.
CREATE TABLE secret_records (
    id          BIGSERIAL    PRIMARY KEY,
    uuid        UUID         NOT NULL,
    purpose     VARCHAR(64)  NOT NULL,
    alg         VARCHAR(16)  NOT NULL DEFAULT 'A256GCM',
    kek_id      VARCHAR(32)  NOT NULL,
    wrapped_dek BYTEA        NOT NULL,
    ciphertext  BYTEA        NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    rotated_at  TIMESTAMPTZ  NULL,
    CONSTRAINT uq_secret_records_uuid UNIQUE (uuid),
    CONSTRAINT chk_secret_records_alg CHECK (alg IN ('A256GCM')),
    CONSTRAINT chk_secret_records_purpose CHECK (purpose ~ '^[a-z][a-z0-9_.-]*$')
);

CREATE INDEX idx_secret_records_kek_id ON secret_records (kek_id);

CREATE TRIGGER trg_secret_records_set_updated_at
    BEFORE UPDATE ON secret_records
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
