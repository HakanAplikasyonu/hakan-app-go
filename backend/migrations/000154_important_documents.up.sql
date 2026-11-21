-- Mükellefin önemli belgeleri (#780 B): vergi levhası, imza sirküleri,
-- ticaret sicil gazetesi, kira sözleşmesi, ruhsat, vekâletname... Belge
-- arşivinin (taxpayer_documents, #393) bir alt kümesidir: ikinci bir dosya
-- deposu yok. important_kind dolu olan belge "önemli"dir; mükellef sayfasında
-- ayrı bölümde, mükellef panelinde ("Mükellef de görsün" açıksa) görünür.
ALTER TABLE taxpayer_documents
    ADD COLUMN important_kind TEXT NULL,
    ADD CONSTRAINT chk_taxpayer_documents_important_kind
        CHECK (important_kind IS NULL OR important_kind IN (
            'tax_certificate', 'signature_circular', 'trade_registry_gazette',
            'lease_contract', 'license', 'power_of_attorney', 'other'));

CREATE INDEX idx_taxpayer_documents_important
    ON taxpayer_documents (taxpayer_id, valid_until) WHERE important_kind IS NOT NULL;

-- Ofis ayarı: "Önemli belgeler" bölümü açık/kapalı (varsayılan açık).
-- Satırı olmayan ofis açık sayılır.
CREATE TABLE important_document_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_important_document_settings_updated_by ON important_document_settings (updated_by);

SELECT rls_apply_org('important_document_settings');
