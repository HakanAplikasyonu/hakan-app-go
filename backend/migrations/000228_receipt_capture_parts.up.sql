-- Parçalı Z raporu (Oğuz'un demo geri bildirimi): uzun bir Z raporu tek
-- karede çekilemez; mükellef ya da ofis birkaç fotoğrafı (parça 1/2/3 …)
-- sırayla gönderir. Bunlar tek bir fiş yakalaması (receipt_captures) ve tek
-- bir Z raporu (retail_sales) olur; ofis tek Z raporunu parçalarıyla görür.
--
-- receipt_capture_parts: yakalamanın sıralı parçaları. Parça 1 yakalamanın
-- kendi görselidir (receipt_captures.attachment_id); tek fotoğraflı
-- yakalamada satır açılmaz. Her parça mükellefin eklerinde (attachments)
-- durur; aynı yakalamada bir parça numarası bir kez kullanılır.
CREATE TABLE receipt_capture_parts (
    id                 BIGSERIAL    PRIMARY KEY,
    organization_id    BIGINT       NOT NULL,
    engagement_id      BIGINT       NOT NULL,
    taxpayer_id        BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    receipt_capture_id BIGINT       NOT NULL REFERENCES receipt_captures (id) ON DELETE CASCADE,
    part_no            SMALLINT     NOT NULL,
    attachment_id      BIGINT       NOT NULL,
    file_name          TEXT         NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_receipt_capture_parts_part UNIQUE (receipt_capture_id, part_no),
    CONSTRAINT fk_receipt_capture_parts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_receipt_capture_parts_attachment
        FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_receipt_capture_parts_part_no CHECK (part_no BETWEEN 1 AND 5)
);

CREATE INDEX idx_receipt_capture_parts_engagement ON receipt_capture_parts (organization_id, engagement_id);
CREATE INDEX idx_receipt_capture_parts_attachment ON receipt_capture_parts (taxpayer_id, attachment_id);

SELECT rls_apply_engagement('receipt_capture_parts'::regclass);
