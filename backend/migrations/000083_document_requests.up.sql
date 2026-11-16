-- Müşavirin elle açtığı belge talebi (issue #240): banka hareketine bağlı
-- olmayan görev (kind = 'document_request', code = 'DOC_REQUEST'). Başlık
-- mükellefe gösterilir; not açıklamadır. Durum akışı aynıdır:
-- open → (mükellef yükler) submitted → (ofis onaylar) matched, ya da ofis
-- yeniden ister: submitted → open.
ALTER TABLE document_tasks ALTER COLUMN bank_transaction_id DROP NOT NULL;
ALTER TABLE document_tasks ADD COLUMN title TEXT NOT NULL DEFAULT '';
ALTER TABLE document_tasks ADD COLUMN request_count INTEGER NOT NULL DEFAULT 1;

ALTER TABLE document_tasks DROP CONSTRAINT chk_document_tasks_code;
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_code
    CHECK (code IN ('BNK_UNDOCUMENTED_PAYMENT', 'BNK_UNINVOICED_COLLECTION', 'DOC_REQUEST'));
ALTER TABLE document_tasks DROP CONSTRAINT chk_document_tasks_kind;
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_kind
    CHECK (kind IN ('undocumented_payment', 'uninvoiced_collection', 'document_request'));
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_request
    CHECK ((kind = 'document_request') = (bank_transaction_id IS NULL)
       AND (kind = 'document_request') = (code = 'DOC_REQUEST'));
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_title
    CHECK (char_length(title) <= 200 AND (kind <> 'document_request' OR char_length(btrim(title)) > 0));
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_request_count CHECK (request_count >= 1);

-- Bildirimler: mükellefe yeni talep, ofise yükleme (inapp satırı mobil
-- push'a da yansır).
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('doc_tasks.requested', 'inapp', 'tr', 'Yeni belge talebi',
     '{{org_name}} sizden belge istiyor: {{title}}', TRUE),
    ('doc_tasks.requested', 'inapp', 'en', 'New document request',
     '{{org_name}} requests a document: {{title}}', TRUE),
    ('doc_tasks.requested', 'email', 'tr', '{{org_name}} belge istiyor',
     'Merhaba, {{org_name}} {{taxpayer_name}} için belge istiyor: {{title}}. Uygulamadan fotoğrafını ya da PDF''ini yükleyebilirsiniz: {{link}}', TRUE),
    ('doc_tasks.requested', 'email', 'en', '{{org_name}} requests a document',
     'Hello, {{org_name}} requests a document for {{taxpayer_name}}: {{title}}. Upload a photo or PDF in the app: {{link}}', TRUE),
    ('doc_tasks.submitted', 'inapp', 'tr', 'Mükellef belge yükledi',
     '{{taxpayer_name}} belge yükledi: {{title}}', TRUE),
    ('doc_tasks.submitted', 'inapp', 'en', 'Document uploaded',
     '{{taxpayer_name}} uploaded a document: {{title}}', TRUE),
    ('doc_tasks.submitted', 'email', 'tr', '{{taxpayer_name}} belge yükledi',
     'Merhaba, {{taxpayer_name}} istenen belgeyi yükledi: {{title}}. İncelemek için: {{link}}', TRUE),
    ('doc_tasks.submitted', 'email', 'en', '{{taxpayer_name}} uploaded a document',
     'Hello, {{taxpayer_name}} uploaded the requested document: {{title}}. Review it here: {{link}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
