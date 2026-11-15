-- Tenant-core faz 6b (issue #3): global mükellef kimliği.
--
-- 000035 taxpayers tablosunu yalnızca engagement'ın bağlanacağı sade hâliyle
-- açtı. Burada kimlik alanları eklenir. Tablo bu noktada boş olmalı: bugüne
-- kadar mükellef satırı oluşturan bir uç yoktu. Satır varsa NOT NULL ekleme
-- başarısız olur ve migration durur (sessiz varsayılan değer yazılmaz).
--
-- kind:       legal (tüzel kişi, VKN) | individual (gerçek kişi, VKN veya TCKN)
-- tax_id:     10 hane VKN ya da 11 hane TCKN. Platform genelinde tekil.
--             Biçim ve sağlama (checksum) uygulamada doğrulanır
--             (internal/platform/taxid); burada yalnızca biçim kısıtı var.
-- title:      unvan (tüzel) ya da ad soyad (gerçek kişi)
-- tax_office: vergi dairesi adı, isteğe bağlı
--
-- D1b: karşı taraflar (cari) bu tabloya bağlanmaz; burada yalnızca
-- mükellefin kendi kimliği tutulur.
ALTER TABLE taxpayers
    ADD COLUMN kind       VARCHAR(16)  NOT NULL,
    ADD COLUMN tax_id     VARCHAR(11)  NOT NULL,
    ADD COLUMN title      VARCHAR(255) NOT NULL,
    ADD COLUMN tax_office VARCHAR(128) NOT NULL DEFAULT '',
    ADD CONSTRAINT chk_taxpayers_kind CHECK (kind IN ('legal', 'individual')),
    ADD CONSTRAINT chk_taxpayers_tax_id CHECK (
        (tax_id ~ '^[0-9]{10}$')
        OR (kind = 'individual' AND tax_id ~ '^[1-9][0-9]{10}$')
    ),
    ADD CONSTRAINT chk_taxpayers_title CHECK (btrim(title) <> ''),
    ADD CONSTRAINT uq_taxpayers_tax_id UNIQUE (tax_id);

-- Platform yöneticisinin mükellef kimlik kataloğu.
INSERT INTO permissions (name, slug) VALUES
    ('Read platform taxpayers', 'platform.taxpayers.read'),
    ('Write platform taxpayers', 'platform.taxpayers.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.taxpayers.read', 'platform.taxpayers.write')
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;
