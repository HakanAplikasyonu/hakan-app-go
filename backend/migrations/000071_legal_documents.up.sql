-- KVKK (issue #57): sürümlü hukuki metinler ve onay kayıtları.
--
-- legal_documents: platform geneli metinler. Bir tür (kind) ve dilde birden
-- çok sürüm olur; uygulamada yalnızca yayımlanmış (published) en son sürüm
-- gösterilir. Yayımlanmış metin değiştirilmez: düzeltme yeni sürümdür.
-- legal_review_pending: hukuk onayı bekleyen taslak; arayüz uyarı gösterir.
--
-- legal_acceptances: kim, hangi metnin hangi sürümünü, hangi akışta, kimin
-- adına onayladı. Kanıt kaydıdır; silinmez. Açık rıza geri alınınca
-- withdrawn_at dolar (6698 sayılı Kanun md. 5, 11).
CREATE TABLE legal_documents (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    kind                  TEXT         NOT NULL,
    language              TEXT         NOT NULL DEFAULT 'tr',
    version               INTEGER      NOT NULL,
    title                 TEXT         NOT NULL,
    body                  TEXT         NOT NULL,
    status                TEXT         NOT NULL DEFAULT 'draft',
    legal_review_pending  BOOLEAN      NOT NULL DEFAULT TRUE,
    content_sha256        TEXT         NOT NULL,
    created_by            BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    published_at          TIMESTAMPTZ  NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_legal_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_legal_documents_version UNIQUE (kind, language, version),
    CONSTRAINT chk_legal_documents_kind CHECK (kind IN (
        'privacy_notice', 'history_share_consent', 'data_processing_agreement', 'retention_policy')),
    CONSTRAINT chk_legal_documents_language CHECK (language IN ('tr', 'en')),
    CONSTRAINT chk_legal_documents_version CHECK (version >= 1),
    CONSTRAINT chk_legal_documents_status CHECK (status IN ('draft', 'published', 'retired')),
    CONSTRAINT chk_legal_documents_published CHECK ((status = 'draft') = (published_at IS NULL)),
    CONSTRAINT chk_legal_documents_sha CHECK (content_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_legal_documents_current ON legal_documents (kind, language, version DESC)
    WHERE status = 'published';

CREATE TRIGGER trg_legal_documents_set_updated_at
    BEFORE UPDATE ON legal_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Yayımlanmış metnin içeriği değişmez.
CREATE OR REPLACE FUNCTION legal_documents_freeze_published() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'draft' AND (NEW.title IS DISTINCT FROM OLD.title
        OR NEW.body IS DISTINCT FROM OLD.body OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.kind IS DISTINCT FROM OLD.kind OR NEW.language IS DISTINCT FROM OLD.language) THEN
        RAISE EXCEPTION 'published legal document % v% is immutable', OLD.kind, OLD.version;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_legal_documents_freeze
    BEFORE UPDATE ON legal_documents FOR EACH ROW EXECUTE FUNCTION legal_documents_freeze_published();

CREATE TABLE legal_acceptances (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    document_id      BIGINT       NOT NULL REFERENCES legal_documents (id) ON DELETE RESTRICT,
    user_id          BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    -- Kimin adına: ofis (veri işleyen sözleşmesi) ya da mükellef (açık rıza).
    organization_id  BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    taxpayer_id      BIGINT       NULL REFERENCES taxpayers (id) ON DELETE CASCADE,
    context          TEXT         NOT NULL,
    -- Onayın bağlı olduğu kayıt (ör. geçmiş veri paylaşımının uuid'i).
    subject_uuid     UUID         NULL,
    content_sha256   TEXT         NOT NULL,
    ip               TEXT         NOT NULL DEFAULT '',
    user_agent       TEXT         NOT NULL DEFAULT '',
    accepted_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    withdrawn_at     TIMESTAMPTZ  NULL,
    CONSTRAINT uq_legal_acceptances_uuid UNIQUE (uuid),
    CONSTRAINT chk_legal_acceptances_context CHECK (context IN (
        'self', 'history_share', 'onboarding_consent', 'office_dpa')),
    CONSTRAINT chk_legal_acceptances_user_agent CHECK (length(user_agent) <= 512)
);

CREATE INDEX idx_legal_acceptances_user ON legal_acceptances (user_id, accepted_at DESC);
CREATE INDEX idx_legal_acceptances_org ON legal_acceptances (organization_id, accepted_at DESC)
    WHERE organization_id IS NOT NULL;
CREATE INDEX idx_legal_acceptances_subject ON legal_acceptances (subject_uuid)
    WHERE subject_uuid IS NOT NULL;

-- RLS (issue #6): platform geneli; kapsamlı rollere satır vermez.
SELECT rls_apply_platform('legal_documents');
SELECT rls_apply_platform('legal_acceptances');

INSERT INTO permissions (name, slug) VALUES
    ('Accept office legal agreements', 'tenant.legal.accept'),
    ('Manage legal documents', 'platform.legal.manage')
ON CONFLICT (slug) DO NOTHING;

-- Veri işleyen sözleşmesini ofis adına sahip (kendiliğinden) ve yönetici kabul eder.
SELECT grant_tenant_role_permission('admin', 'tenant.legal.accept');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'platform.legal.manage'
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- İlk sürümler. TASLAK: hukuk onayı bekliyor (legal_review_pending). Metinler
-- uygulamanın gerçek işleyişine göre yazıldı; hukuki dayanak ve süreler
-- hukukçu tarafından doğrulanmalı. content_sha256 = sha256(title || '\n' || body).
-- ---------------------------------------------------------------------------
INSERT INTO legal_documents (kind, language, version, title, body, status, legal_review_pending, content_sha256, published_at)
SELECT kind, 'tr', 1, title, body, 'published', TRUE,
       encode(sha256(convert_to(title || E'\n' || body, 'UTF8')), 'hex'), NOW()
FROM (VALUES
('privacy_notice', 'Kişisel Verilerin İşlenmesine İlişkin Aydınlatma Metni',
$md$Bu metin, 6698 sayılı Kişisel Verilerin Korunması Kanunu'nun ("KVKK") 10. maddesi uyarınca, platformu kullanan mali müşavirlik ofisi çalışanlarını ve mükellef kullanıcılarını bilgilendirmek için hazırlanmıştır.

## 1. Veri sorumlusu ve veri işleyen

- Mükellefin mali verileri bakımından **veri sorumlusu, mükellefle hizmet ilişkisi kuran mali müşavirlik ofisidir.** Platform bu verileri ofis adına ve ofisin talimatıyla işleyen **veri işleyendir**; ofisle aramızdaki ilişki Veri İşleyen Sözleşmesi'yle düzenlenir.
- Platform hesabınıza ait veriler (ad, soyad, e-posta, telefon, oturum ve güvenlik kayıtları) bakımından veri sorumlusu platform işletmecisidir.

## 2. İşlenen kişisel veriler

- **Kimlik ve iletişim:** ad, soyad, e-posta, telefon; gerçek kişi mükelleflerde T.C. kimlik numarası (vergi kimlik numarası olarak).
- **Mali veriler:** e-fatura ve e-arşiv faturaları, fişler, banka hareketleri, masraf belgeleri ve bunlardan üretilen muhasebe kayıtları. Bu belgelerde karşı tarafların adı, vergi numarası ve adresi de yer alabilir.
- **İşlem güvenliği:** oturum kayıtları, IP adresi, cihaz bilgisi, işlem ve denetim kayıtları.

## 3. İşleme amaçları ve hukuki sebepler

| Amaç | Hukuki sebep (KVKK md. 5/2) |
|---|---|
| Mükellefin muhasebe ve beyan hizmetinin yürütülmesi | Sözleşmenin ifası (c), hukuki yükümlülük (ç) |
| Vergi mevzuatı kapsamında belge saklama | Kanunlarda açıkça öngörülme (a), hukuki yükümlülük (ç) |
| Hesap güvenliği, kötüye kullanımın önlenmesi, denetim izi | Meşru menfaat (f) |
| Ofis değişikliğinde önceki döneme ait verilerin yeni ofisle paylaşılması | **Açık rıza** (md. 5/1); ayrı metinle alınır ve geri alınabilir |

## 4. Aktarım

- Veriler, hizmetin gerektirdiği ölçüde mükellefin çalıştığı mali müşavirlik ofisine ve kanunen yetkili kamu kurumlarına aktarılır.
- Barındırma yurt içindedir. Yapay zekâ destekli işlemlerde kişi adları ve kimlik numaraları maskelenir; verileriniz model eğitiminde kullanılmaz.

## 5. Toplama yöntemi

Veriler; yüklediğiniz belgeler, e-fatura entegratörü, ön muhasebe programı ve banka bağlantıları ile platform kullanımı sırasında elektronik ortamda toplanır.

## 6. Saklama

Veriler, Saklama ve İmha Politikası'nda belirtilen süreler boyunca saklanır; süre bitince silinir, yok edilir veya anonim hâle getirilir.

## 7. Haklarınız (KVKK md. 11)

Kişisel verinizin işlenip işlenmediğini öğrenme, bilgi talep etme, düzeltilmesini veya silinmesini isteme, aktarıldığı üçüncü kişileri öğrenme, itiraz etme ve zararın giderilmesini talep etme haklarına sahipsiniz. Başvurularınızı veri sorumlusuna iletebilirsiniz; mali verileriniz için başvuru, hizmet aldığınız ofise yapılır.
$md$),
('history_share_consent', 'Geçmiş Dönem Verilerinin Yeni Ofisle Paylaşılmasına İlişkin Açık Rıza Metni',
$md$Mali müşavirlik ofisinizi değiştirdiğinizde, önceki ofisinizle çalıştığınız döneme ait belge ve kayıtlarınız **varsayılan olarak yeni ofisinize açılmaz.**

Bu metni onaylarsanız:

- Seçtiğiniz tarih aralığındaki (aralık seçmezseniz tüm önceki dönemlere ait) faturalarınız, belgeleriniz ve bunlardan üretilen kayıtlarınız **şu anda hizmet aldığınız ofisle** paylaşılır.
- Paylaşımın amacı, yeni ofisinizin muhasebe ve beyan işlerinizi önceki dönemlerle tutarlı yürütebilmesidir.
- Önceki ofisiniz, kendi hizmet döneminde işlediği verileri yasal saklama yükümlülüğü kapsamında saklamaya devam eder; bu onay ona yeni bir erişim vermez.

Bu onay KVKK md. 5/1 kapsamında **açık rızanızdır**. Vermek zorunda değilsiniz; vermemeniz hizmet ilişkinizi etkilemez. Onayınızı istediğiniz zaman bu ekrandan geri alabilirsiniz; geri aldığınızda yeni ofis geçmiş dönem verilerinize erişemez. Geri alma, o ana kadar yapılmış işlemleri etkilemez.
$md$),
('data_processing_agreement', 'Veri İşleyen Sözleşmesi',
$md$Bu sözleşme, platformu kullanan mali müşavirlik ofisi ("Veri Sorumlusu") ile platform işletmecisi ("Veri İşleyen") arasında, KVKK md. 12/2 uyarınca akdedilir.

## 1. Konu

Veri İşleyen, Veri Sorumlusu'nun mükelleflerine ait mali verileri yalnızca muhasebe, kontrol ve beyan hizmetinin yürütülmesi amacıyla, Veri Sorumlusu adına ve onun talimatlarıyla işler.

## 2. Veri İşleyen'in yükümlülükleri

1. Verileri talimat dışında ve kendi amaçları için işlemez; model eğitiminde kullanmaz.
2. KVKK md. 12/1'deki teknik ve idari tedbirleri alır: erişim yetkilendirmesi, satır düzeyinde ofis ve mükellef izolasyonu, şifreli bağlantı, sırların kasada tutulması, denetim kayıtları, çok aşamalı kimlik doğrulama.
3. Verileri yurt içinde barındırır.
4. Alt veri işleyen (barındırma, e-posta, yapay zekâ sağlayıcısı) kullandığında aynı yükümlülükleri ona da yükler; yapay zekâ sağlayıcısına kişi adları ve kimlik numaraları maskelenerek gönderilir.
5. Bir veri ihlalini öğrendiğinde Veri Sorumlusu'na gecikmeksizin bildirir.
6. Veri sahibi başvurularının yanıtlanmasında Veri Sorumlusu'na destek olur.

## 3. Veri Sorumlusu'nun yükümlülükleri

1. Mükelleflerini aydınlatır ve gereken hâllerde açık rızalarını alır.
2. Platforma yalnızca hizmet için gerekli verileri yükler ve kullanıcı yetkilerini güncel tutar.

## 4. Süre ve sona erme

Sözleşme, ofisin platform kullanımı süresince geçerlidir. Sona erdiğinde veriler, Saklama ve İmha Politikası'ndaki yasal süreler saklı kalmak kaydıyla Veri Sorumlusu'na teslim edilir veya silinir.
$md$),
('retention_policy', 'Saklama ve İmha Politikası (özet)',
$md$| Veri | Saklama süresi | Dayanak |
|---|---|---|
| Faturalar, belgeler, muhasebe kayıtları | İlgili takvim yılını izleyen yıldan başlayarak **5 yıl** | 213 sayılı VUK md. 253 |
| Ticari defterler ve belgeler (mükellef tarafı) | **10 yıl** | 6102 sayılı TTK md. 82 |
| Hizmet ilişkisi bittikten sonra ofisin salt okunur arşivi | Ofis ayarı (0–3650 gün) | Ofisin saklama yükümlülüğü |
| Onay ve rıza kayıtları | İlişki süresince ve sona ermesinden itibaren **10 yıl** | İspat yükü, TBK md. 146 |
| Oturum ve erişim kayıtları | **2 yıl** | 5651 sayılı Kanun ve meşru menfaat |
| Kullanıcı hesabı | Hesap kapatılınca, yasal süresi dolan veriler hariç silinir | KVKK md. 7 |

Süresi dolan veriler periyodik imha işlemiyle silinir, yok edilir veya anonim hâle getirilir; imha kayıt altına alınır.
$md$)
) AS t(kind, title, body);
