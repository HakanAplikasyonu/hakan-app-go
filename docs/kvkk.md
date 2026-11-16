# KVKK çerçevesi (issue #57)

> **Durum:** İlk dilim. Metinler **taslak**tır (`legal_review_pending`) ve hukuk
> onayı bekler (docs/00 E9, docs/01 A11). Bu belge uygulamanın gerçekte ne
> yaptığını ve hukukçunun neyi doğrulaması gerektiğini anlatır; hukuki görüş
> değildir.

## 1. Roller

| Veri | Veri sorumlusu | Veri işleyen |
|---|---|---|
| Mükellefin mali verileri (fatura, belge, banka hareketi, muhasebe kaydı) | Mükellefle hizmet ilişkisi kuran **mali müşavirlik ofisi** | Platform |
| Platform hesabı (ad, e-posta, oturum, güvenlik kayıtları) | Platform işletmecisi | — |

Ofis ile platform arasındaki ilişki **Veri İşleyen Sözleşmesi** ile kurulur
(KVKK md. 12/2). Kaynak bağlantılarında (Easytrade, Paraşüt …) aynı ilişki
`02` §10'da tarif edilir.

## 2. Metinler ve sürümleme

Metinler `legal_documents` tablosundadır (migration `000071`). Türler:

| Tür (`kind`) | Metin | Kim onaylar | Hangi akışta |
|---|---|---|---|
| `privacy_notice` | Aydınlatma metni (md. 10) | Mükellef kullanıcısı; ofis kullanıcısı kendisi için | Ofisin mükellef ekleme talebinin onayı (bağlantı ve sahip paneli), profil |
| `history_share_consent` | Geçmiş dönem verilerinin yeni ofisle paylaşılması: açık rıza (md. 5/1) | Mükellef sahibi | Mükellef panelinde "geçmiş veri paylaşımı" |
| `data_processing_agreement` | Veri işleyen sözleşmesi (md. 12/2) | Ofis sahibi ya da yöneticisi, ofis adına | Ofis ayarları |
| `retention_policy` | Saklama ve imha politikası (özet) | Bilgilendirme | Herkese açık sayfa |

Kurallar:

- Bir tür ve dilde birden çok **sürüm** vardır. Uygulama yalnızca en son
  **yayımlanmış** sürümü gösterir. Yayımlanmış metin değiştirilemez (veritabanı
  tetikleyicisi); düzeltme yeni sürümdür. Yeni sürüm yayımlanınca eskisi
  `retired` olur ama okunabilir kalır: kişinin onayladığı metin her zaman
  gösterilebilir.
- Akışlar yalnızca **güncel** sürümün onayını kabul eder. İstemci eski metni
  gösterdiyse API `409 LEGAL_VERSION_OUTDATED` döner ve güncel metin yeniden
  gösterilir.
- Türkçe metin bağlayıcıdır. İstenen dilde sürüm yoksa Türkçe döner.
- Yeni sürüm platform yöneticisinin uçlarıyla (`/v1/platform/legal/documents`)
  taslak olarak açılır ve yayımlanır. Yayımlarken `legal_reviewed: true`
  verilirse taslak uyarısı kalkar.

## 3. Onay kayıtları

Her onay `legal_acceptances` tablosunda kanıt olarak saklanır:

- kim (`user_id`), hangi metin ve sürüm (`document_id`, ayrıca metnin
  `content_sha256` özeti), hangi akışta (`context`), kimin adına
  (`organization_id` ofis, `taxpayer_id` mükellef), hangi kayda bağlı
  (`subject_uuid`, ör. paylaşım), IP ve tarayıcı bilgisi, zaman.
- Kayıt silinmez. Açık rıza geri alınınca `withdrawn_at` dolar (md. 5, 11).
  Geçmiş veri paylaşımı geri alındığında rıza kaydı aynı işlemde geri alınmış
  olur.
- Geçmiş veri paylaşımı ile rıza kaydı **tek işlemde** yazılır; rıza
  kaydedilemezse paylaşım da açılmaz.
- Kişi kendi onaylarını görebilir (`GET /v1/me/legal-acceptances`, md. 11).

## 4. Saklama ve silme süreleri

| Veri | Süre | Dayanak (doğrulanacak) | Uygulamadaki durum |
|---|---|---|---|
| Faturalar, belgeler, muhasebe kayıtları | İzleyen yıldan başlayarak 5 yıl | VUK md. 253 | Silme işi **yok**; veri saklanıyor |
| Ticari defter ve belgeler (mükellef tarafı) | 10 yıl | TTK md. 82 | Mükellefin yükümlülüğü; platform 5 yılın ötesinde saklayabilir mi, hukuk kararı |
| Hizmet ilişkisi bittikten sonra eski ofisin salt okunur erişimi | Ofis ayarı, 0–3650 gün | Ofisin saklama yükümlülüğü | **Var** (`engagements.archive_until`, #8) |
| Onay ve rıza kayıtları | İlişki süresince + 10 yıl | İspat; TBK md. 146 | Silme işi **yok** |
| Oturum ve erişim kayıtları | 2 yıl | 5651 sayılı Kanun, meşru menfaat | Log saklama kuralları mevcut (`logs` modülü, `ApplyDueRules`); süreler ayarlanmalı |
| Kullanıcı hesabı | Kapanınca, yasal süresi dolan veri hariç silinir | KVKK md. 7 | Hesap kapatma ve imha akışı **yok** |

Periyodik imha işi (süresi dolan veriyi silme, yok etme ya da anonimleştirme,
imha tutanağı) bu dilimin kapsamı dışında. Önce süreler hukukça
kesinleşmeli.

## 5. Yapay zekâ sağlayıcısına giden veri (issue #169)

Sağlayıcıya giden her istek sunucuda maskelenir (`AI_MASKING=true`). Yanıttaki belirteçler sunucuda geri açılır.

- **Metin:**
  - VKN, TCKN, IBAN, e-posta ve telefon numarası maskelenir.
  - Çağrıyı yapan modülün verdiği adlar (mükellef unvanı, karşı taraflar) maskelenir.
  - Kayıtlı olmayan adlar da sezgisel olarak yakalanır:
    - hukuki biçimle biten unvanlar (A.Ş., Ltd. Şti. …),
    - yaygın bir Türkçe ad ve ardından büyük harfle başlayan kelime,
    - "Sayın …" ve "… Bey/Hanım" kalıpları,
    - "Ad Soyad:", "Müşteri:", "Kasiyer:" gibi etiketlerin değeri.
  - Sezgi gizleme yönünde hata yapar.
- **Görsel (fiş fotoğrafı):** `AI_IMAGE_REDACTION=required` varsayılandır.
  - Görsel önce sunucuda OCR'dan (tesseract, Türkçe) geçer.
  - Metindeki kurallara uyan bölgeler siyahla kapatılır.
  - Sağlayıcıya kapatılmış görsel ve kapatılan bölgelerin belirteçleri gider. Belirteçler `[VKN_1]` gibidir, değerler gitmez.
  - Yeniden kodlama EXIF ve konum bilgisini de siler.
  - OCR motoru yoksa görsel hiç gönderilmez ve fiş okuma başarısız olur.
  - `off` görseli olduğu gibi gönderir; yalnızca yerel geliştirme içindir.
- **Durum:** gizlilik ucu (`GET /v1/tenant/ai/privacy`) etkin durumu `image_redaction` alanında gösterir.

OCR okuyamadığı ya da yanlış okuduğu bölgeyi kapatamaz. Bulanık ya da eğik fotoğrafta kişisel veri kalabilir; bu yüzden maskeli verinin hukuki niteliği (md. 9) hâlâ hukukçunun kararıdır (6. bölüm, 5. madde).

## 6. Hukukçunun doğrulayacakları

1. Rol dağılımı: platform mali verilerde veri işleyen, hesap verisinde veri sorumlusu.
2. Geçmiş dönem paylaşımının hukuki sebebi. Açık rıza mı, yoksa ofis değişikliğinde sözleşmenin ifası mı? Açık rıza seçildiği için geri alınabilir ve hizmet şartına bağlanmadı.
3. Aydınlatmanın nerede gösterileceği. Şu an ofisin mükellef ekleme onayında zorunlu, profilde isteğe bağlı. Ofis çalışanları için kayıt anında gösterilmesi gerekir mi?
4. Saklama süreleri, özellikle 5 / 10 yıl ayrımı ve onay kayıtlarının saklanması.
5. Yurt dışı aktarım. Yapay zekâ sağlayıcısına maskeli veri gidiyor (docs/01 D4). Maskeli veri kişisel veri sayılır mı, md. 9 kapsamına girer mi?
6. VERBİS kaydı ve alt veri işleyenlerin listesi.
7. Kaynak bağlantılarındaki `source_connections.consent_text` alanı. Bu alanın sürümlü metne (`legal_documents`) bağlanması.
