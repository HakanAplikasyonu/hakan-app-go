-- Evrak yüklerken tür ve dönem seçimi (Oğuz'un demo geri bildirimi).
--
-- Ofis kullanıcısı mükellef adına evrak yüklerken türü (fatura, Z raporu,
-- masraf, banka ekstresi, diğer) ve dönemi (ay) seçer. Dönem belge
-- tarihinden önerilir (girilen tarih ya da fotoğraftan okunan tarih), yoksa
-- içinde bulunulan ay. Seçilen ay kilitliyse kullanıcı üç seçenekten birini
-- seçer (#816 kilitli dönem talebiyle aynı kararlar): reddet, geçmiş döneme düzeltme olarak işle (düzeltme beyannamesi
-- gerekebilir) ya da seçilen (açık) aya uygula. Bu tablo her yüklemenin ve
-- verilen kararın izidir; dosyanın kendisi işlendiği yerde durur (fiş
-- yakalama, e-fatura yükleme işi ya da belge arşivi).
--
-- period: kaydın düştüğü ay. requested_period: kullanıcının ilk seçtiği ay.
-- period_source: dönemin nereden geldiği (document_date: girilen tarih,
-- ocr: fotoğraftan okunan tarih, current_month: içinde bulunulan ay,
-- manual: kullanıcı elle seçti). lock_choice / locked_period: seçilen ay
-- kilitliyken verilen karar ve kilitli ay (ikisi birlikte dolu ya da boş).
-- ocr_period: fotoğraftaki tarih kilitli bir aya düştüğü için kayıt
-- seçilen aya işlendiyse o ay.
CREATE TABLE office_document_uploads (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT       NOT NULL,
    engagement_id         BIGINT       NOT NULL,
    taxpayer_id           BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    doc_type              TEXT         NOT NULL,
    period                CHAR(7)      NOT NULL,
    requested_period      CHAR(7)      NOT NULL,
    period_source         TEXT         NOT NULL,
    document_date         DATE         NULL,
    lock_choice           TEXT         NULL,
    locked_period         CHAR(7)      NULL,
    ocr_period            CHAR(7)      NULL,
    status                TEXT         NOT NULL,
    file_name             TEXT         NOT NULL DEFAULT '',
    content_type          TEXT         NOT NULL DEFAULT '',
    size_bytes            BIGINT       NOT NULL,
    sha256                CHAR(64)     NOT NULL,
    receipt_uuid          UUID         NULL,
    upload_job_uuid       UUID         NULL,
    archive_document_uuid UUID         NULL,
    error                 TEXT         NOT NULL DEFAULT '',
    created_by            BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_office_document_uploads_uuid UNIQUE (uuid),
    CONSTRAINT fk_office_document_uploads_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_office_document_uploads_doc_type
        CHECK (doc_type IN ('invoice', 'z_report', 'expense', 'bank_statement', 'other')),
    CONSTRAINT chk_office_document_uploads_period
        CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$' AND requested_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_office_document_uploads_source
        CHECK (period_source IN ('document_date', 'ocr', 'current_month', 'manual')),
    CONSTRAINT chk_office_document_uploads_lock
        CHECK ((lock_choice IS NULL) = (locked_period IS NULL)
            AND (lock_choice IS NULL OR lock_choice IN ('reject', 'correction', 'target_period'))),
    CONSTRAINT chk_office_document_uploads_status
        CHECK (status IN ('processed', 'duplicate', 'failed', 'rejected')),
    CONSTRAINT chk_office_document_uploads_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_office_document_uploads_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_office_document_uploads_engagement
    ON office_document_uploads (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_office_document_uploads_taxpayer ON office_document_uploads (taxpayer_id);
CREATE INDEX idx_office_document_uploads_created_by ON office_document_uploads (created_by);

-- Ofis ayarı: yüklemede tür ve dönem seçimi açık/kapalı (varsayılan açık).
-- Satırı olmayan ofis açık sayılır.
CREATE TABLE document_upload_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_upload_settings_updated_by ON document_upload_settings (updated_by);

-- RLS: yükleme izi engagement kapsamlı, ayar ofis kapsamlı.
SELECT rls_apply_engagement('office_document_uploads'::regclass);
SELECT rls_apply_org('document_upload_settings');
