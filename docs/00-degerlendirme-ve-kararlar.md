# HAKAN (hakan-app-go): Değerlendirme, Kararlar ve Haftalık Plan

| | |
|---|---|
| Hazırlanma | 15 Kasım 2026 (Pazar) |
| Teslim | **19 Kasım 2026 Perşembe 23:59** |
| Okunan kaynaklar | `docs/kaynak/mali-musavir-portali-talepler.md`, `docs/kaynak/musavir-entegrasyon-paketi/` (31 dosyanın hepsi, transkript dahil), referans repolar: `nextjs-go-boilerplate`, `otopoly-go`, `rentopia-app-go`, `clickfix-app-go` |
| Durum | Plan. Kod yok. |

> **Güncelleme (15.11.2026):** Bu belgedeki K1, K2, K5 ve §5 kısmen geçersiz. Güncel kararlar `01-cekirdek-hiyerarsi-ve-acik-kararlar.md` §0'da. Özellikle:
> - Ürün senin, Easytrade yalnızca bir kaynak (D13).
> - UBL ayrıştırma go-ubltr ile yapılacak.
> - Entegratör kodu go-ubltr'nin yeni sürümünü bekliyor.
> - Perşembe teslimi çekirdek + ilk dilim ve yalnızca iç kontrol.

İşaretler: **[K]** karar verildi, **[Ö]** öneri (onay bekliyor), **[S]** dışarıdan cevap gerekiyor.

---

## 1. Tek paragrafta durum

Paket iyi bir **veri toplama katmanı** tarif ediyor. Kaynaklar entegratörler, ön muhasebe programları, banka ve Excel. Ortak bir model, uyarı kodları ve mükerrer kontrolü var. Ancak müşavirin para ödeyeceği üç şeyi kapsam dışı bırakıyor:

1. **Luca'ya aktarım**
2. **Z raporu ve masraf fişi**
3. **Banka onay linki**

Easytrade'in kendi temsilcisi görüşmede bunun tersini söylüyor: *"ilk aksiyonumuz sadece Luca ile entegre olmak olacak"* (transkript, satır 453). Paket bu haliyle kodlanırsa müşavir demoda "veri geldi ama Luca'ya nasıl gidecek?" diye soracak, cevap da olmayacak.

Ayrıca paket Laravel/PHP varsayıyor. Hiçbir entegrasyon canlı test edilmedi. En büyük dayanağı olan "%80 İşNet" bilgisi pakette bulunmayan tek bir yazılı kaynaktan geliyor. Bu kaynak **"Y"**, yani Mustafa Bey'in 8 Kasım tarihli yazısı.

**Ana karar:** Paketin veri modeli ve kuralları korunur. Kapsam ise "topla → kontrol et → **Luca'ya hazır çıktı ver**" olarak genişletilir. Perşembe teslimi, gerçek veriyle çalışan uçtan uca bir dilimdir. Canlı bağlantı izni gerektiren entegrasyonlar, izinler gelene kadar yükleme kanalıyla karşılanır.

---

## 2. Paketin eleştirel değerlendirmesi

### 2.1 Doğru ve korunacak olanlar

| Konu | Neden doğru |
|---|---|
| Fatura içeriğinin **her zaman UBL'den** gelmesi (`31`) | Beyan için tek güvenilir kaynak. Liste yanıtlarındaki başlık tutarlarına güvenmemek doğru. |
| Tek UBL ayrıştırıcı + entegratör başına ince adaptör (`30`, `37`) | Beş entegratörün işi tek ayrıştırıcıda toplanıyor. Test yükü düşüyor. |
| Okurken durum değiştirmeme listesi (`30` §5) | Mükellefin ERP'sini bozmamak kritik. Doküman metot düzeyinde net. |
| Mükerrer kuralı (ETTN → VKN+no+tarih → olası mükerrer) ve kaynak önceliği | Müşavirin T4 talebini doğrudan karşılıyor. |
| Kod tabanlı uyarılar (`05`), seviyenin yapılandırmadan okunması | Doğru tasarım. Metin UI'da, eşik ürün kararında. |
| Ondalık (asla float), `raw_payload`, `content_hash`, `(company_id, source_system, source_id)` tekilliği | Muhasebe verisi için zorunlu disiplin. |
| İndeks tablosu `entegrator_document` + `missing_since` (silmeden işaretleme) | Kaynakta kaybolan belgeyi yakalamanın doğru yolu. |
| Artımlı çekimde **oluşturma tarihi**, gece tam taramada fatura tarihi | Geç düşen ve geçmiş tarihli faturayı kaçırmamanın doğru yolu. |
| İşNet'te VKN'nin **sadece bağlantı kaydından** okunması (`32` §2) | IP + VKN modelinde tek savunma hattı. Pazarlık konusu olmayan bir kural. |
| Dönem kilidi → `change_request` | Beyan sonrası değişiklik müşaviri korur. |

### 2.2 Yanlış, eksik veya çelişkili olanlar

| # | Bulgu | Kanıt | Etki |
|---|---|---|---|
| E1 | **Luca çıkışı kapsam dışı**, ama ürünün ana değeri bu | `00-README` kapsam dışı listesi; transkript s.319 "Luca, Mikro, Orka, Zirve… entegrasyonumuz bulunuyor", s.453 "ilk aksiyonumuz sadece Luca ile entegre olmak" | Ürün demoda değersiz görünür |
| E2 | Müşavir **bugün zaten** e-faturayı ara programla Luca'ya aktarıyor (Rahat Aktarım, MKLF) | `06` satır 62 (Y), transkript s.443 | Rakip Luca değil, **Rahat Aktarım**. Sadece "faturayı çekmek" yeni değer sayılmaz. Değer: tek yer + kontroller + banka + fiş. |
| E3 | Z raporu, masraf fişi, e-tebligat, mobil, AI yorum, dijital VD için **kod yazmak yasak** (`CLAUDE.md`) | `CLAUDE.md` kapsam dışı + `06` "Yok" maddeleri | Müşavirin en çok vurguladığı maddeler (T5, T6, T14, T15) demoda yok |
| E4 | Paket **Laravel/PHP** varsayıyor (ext-soap, brick/math, Laravel kuyrukları) | `CLAUDE.md` hazır kararlar | Bizim yığın Go. Sözleşmeler TS ile yazılmış ama dilden bağımsız; taşıma tablosu §4.3'te. |
| E5 | "%80 İşNet" **tek müşavirin tahmini** ve kaynağı pakette yok | `06` satır 10 ve 63: "Büyük çoğunluğu yüzde 80 vardır heralde" (Y); `kaynak/` klasöründe Y yok | Öncelik sırası tek bir ofisin tahminine dayanıyor. Y belgesi istenmeli, ofisin mükellef listesi entegratöre göre sayılmalı. |
| E6 | **Hiçbir entegrasyon canlı test edilmedi.** Banka dokümanları 2016–2021 tarihli Netsim kopyaları. İşNet adresleri topluluk SDK'sından. | `30` §3, `22` kaynaklar, `32` §1 | Takvim tahminleri bu belirsizliği hesaba katmalı |
| E7 | İşNet canlı erişimi **İşNet ile sözleşmeye bağlı**. Süreç fatura *gönderen* entegrasyon için yazılmış. Tanımsız IP hata değil, zaman aşımı alıyor. | `32` §0, §2 | En kritik kaynak bu hafta canlıya alınamaz. Perşembe için UBL/ZIP yükleme yedeği şart. |
| E8 | Banka "aracı firma / açık bankacılık yok" kararı, müşavirin "ideal" dediği **onay linki** ile çelişiyor. Şube talimatı yolu müşavirin şikayet ettiği sürtünmenin ta kendisi. | `20`, `06` T9 | Ürün kararı, bkz. §3 K7 |
| E9 | TÜRMOB tercihli entegratörlerde faturalar Luca'ya zaten gidiyor olabilir → **çift kayıt riski** | `30` §1 not | Luca çıkışında "Luca'da zaten var mı" kontrolü gerekir |
| E10 | Kaynak aynı zamanda rakip: Uyumsoft (müşavir mobil uygulaması), Mikro Müşavir, Robom/Mükellef | `15`, `16`, transkript §17 | Bu firmalar API erişimini kısabilir. Her adaptörün yanında dosya kanalı mutlaka olmalı. |
| E11 | İç tutarsızlık: İzibiz önceliği `30` §1'de 3, `36` başlığında 5 | `30` satır 25, `36` satır 5 | Küçük; İzibiz ve Uyumsoft'un **herkese açık test hesabı** olduğu için bu hafta ikisi öne alınmalı (§3 K5) |
| E12 | `talepler.md` hataları: e-fatura satırı "mükelleften alınıyor, tek tek işleniyor" diyor, transkript G§3 bunun tersini söylüyor. KDV eşiği önerisi Easytrade'e mal edilmiş. | Önceki Claude cevabında da tespit edildi, transkriptle doğrulandı | Kaynak doküman düzeltilmeli; bu repoda düzeltme notu tutulur |
| E13 | Easytrade'in **"içeride kendi mali müşavirlik portalımız var"** demesi pakette hiç ele alınmamış | Transkript s.475 | Sıfırdan mı yapıyoruz, mevcut portalı mı genişletiyoruz? En kritik [S] sorusu. |
| E14 | Müşavirin istediği "Easytrade'e bağlı olmayan, herhangi bir ofisin kullanabileceği yapı" | Transkript s.143 | Easytrade bu ürünün **kaynaklarından biri** olur, sahibi değil. Mimari ayrı bir servis olmalı (K1). |

### 2.3 Önceki Claude cevabının iddiaları: doğrulama

| İddia | Sonuç |
|---|---|
| 06'da [Talep]/[Onay]/[Referans]/[Easytrade] etiketleri var | Doğru |
| Luca export, Z raporu ve Easytrade dışı masraf fişi pakette yok | Doğru (E1, E3) |
| Banka onay linki kararı açık | Doğru (E8) |
| 05'e 5 uyarı kodu eklendi | Doğru: `FIXED_ASSET_CANDIDATE`, `UNUSUAL_RECORD_TYPE`, `UNUSUAL_AMOUNT`, `NEW_PARTY_HIGH_AMOUNT`, `PLATE_NOT_REGISTERED` |
| CLAUDE.md "Yok" maddeleri için kod yazmayı yasaklıyor | Doğru. Bu yasağı bu repoda **kaldırıyoruz** (K3). |
| talepler.md'de iki hata var | Doğru (E12) |
| Sıradaki doküman Luca export olmalı | **Katılıyoruz.** Ancak format bilgisi Easytrade'in mevcut Luca entegrasyonundan gelmeli, tahmin edilmemeli. |

---

## 3. Kararlar

### K1 [K] Ürün sınırı: ayrı servis, Easytrade bir kaynak

- HAKAN, Easytrade backend'inden (Laravel) **ayrı** bir Go servisidir. Easytrade API'sine `09`'daki kurallarla, salt okuma bir kaynak olarak bağlanır (paket kuralı 4 korunur).
- Kiracı modeli:
  - **Kiracı = mali müşavirlik ofisi** (`organization`)
  - **Mükellef = `company`**, ofisin altında
  - Bir mükellef tek ofise bağlıdır. Ofis değişikliği taşıma işlemidir.
- Kullanıcı rolleri:
  - ofis sahibi
  - ofis personeli (mükellef bazında yetki)
  - mükellef kullanıcısı (sadece kendi şirketi; mobil fiş ve onay ekranları)
- E13 cevabı gelene kadar bu karar geçerlidir. Easytrade "mevcut portalı genişletin" derse yeniden değerlendirilir.

### K2 [K] Yığın

| Katman | Seçim | Kaynak |
|---|---|---|
| API + worker | Go 1.26, pgx/v5 + sqlc kalıbı, Asynq (zamanlanmış işler), Redis | `nextjs-go-boilerplate` |
| Kiracı katmanı | `organizations` modülü + `RequireOrganization` middleware | `otopoly-go` / `rentopia-app-go` (boilerplate'te tenant yok, çıkarılmış) |
| Web | Next.js App Router BFF: `/platform` (Easytrade operasyon), ofis paneli | boilerplate |
| Mobil | Expo (masraf/yakıt/Z fişi fotoğrafı, müşavir özeti) | `otopoly-go/mobile` |
| Dosya | S3 (SeaweedFS), SHA-256 tekil `attachment` | boilerplate storage |
| İçe/dışa aktarım | boilerplate I/O motoru (excelize) → Excel import ve Luca çıktısı | boilerplate |
| Arama | Meilisearch (fatura/cari arama) | boilerplate |
| AI | Kolon eşleme (`04`), fiş OCR, finansal yorum. Model: Claude (en güncel sürüm). **Maskeli veri** kuralı (`04`) korunur. | `otopoly-go/ai` deseni |
| Dağıtım | Dokploy (compose.prod) | boilerplate |

### K3 [K] Paket kurallarının bu repoya uyarlanması

Paket `CLAUDE.md`'sinin 10 kuralından 9'u aynen korunur:

- kaynaklara salt okuma
- ⚠️ işaretli bilgiyi kesin gibi kodlamamak
- uydurmamak
- Easytrade API'sine dokunmamak
- içeriğin UBL'den gelmesi
- ondalık
- uyarı kodları
- sırların kasada tutulması
- İşNet VKN'sinin bağlantı kaydından okunması
- WSDL'in dokümandan önce gelmesi

**Değişen kural:** "06'da 'Yok' olan maddeler için kod yazılmaz" kuralı kaldırılır. Bu maddeler (Luca çıkışı, fiş yakalama, Z raporu, KDV erken uyarı ekranı, AI özet) kapsamdadır. Ancak her biri **doğrulanmamış dış formata** dayandığı sürece arayüz arkasında, değiştirilebilir bir adaptörle yazılır.

### K4 [K] Laravel varsayımlarının Go karşılıkları

| Paket varsayımı | Go karşılığı |
|---|---|
| `ext-soap` | WSDL'den kod üretimi yok. Entegratör dosyalarındaki istek XML'leri **elle şablon** (`text/template`), yanıt `encoding/xml` ile ad alanı URI'sine göre okunur (`37` §3.2). İşNet alfabetik sıra kuralı şablonda sabit. |
| `brick/math` | `shopspring/decimal` (uygulama) + PostgreSQL `NUMERIC(18,4)` (pgx `pgtype.Numeric` ↔ decimal dönüşümü tek pakette) |
| Laravel queue / scheduler | Asynq kuyrukları + `asynq.Scheduler`; bağlantı başına kilit Redis ile (`37` §5.6) |
| Kasada sır (Vault) | İlk sürümde zarf şifreleme: veri anahtarı başına AES-GCM, ana anahtar ortam değişkeninde/sırda. `credentials_ref` alanı sonra Vault'a taşınabilecek arayüz. |
| XXE kapalı ayrıştırıcı | Go `encoding/xml` dış varlık çözmez; DTD içeren belge reddedilir (`37` §3.1) |
| TS adaptör sözleşmeleri (`02`, `30` §6) | Go arayüzleri: `SourceAdapter`, `EntegratorAdapter`, `BankAdapter`; `AsyncIterable` → sayfa sayfa dönen iterator (`iter.Seq2`) |

### K5 [K] Entegrasyon sırası: canlıya alınabilirliğe göre

Paket sırayı İşNet → Uyumsoft → İzibiz olarak veriyordu (pazar payına göre). Biz **bu hafta test edilebilirliğe göre** sıralıyoruz. Pazar payı sırası, izinler geldikçe geri döner.

| Sıra | Kaynak | Neden bu hafta |
|---|---|---|
| 1 | **UBL/ZIP yükleme** (`31`) | İzin gerektirmez. Müşavir entegratör portalından ZIP indirip yükler. İşNet mükellefleri dahil **herkes** ilk günden çalışır. |
| 2 | **Uyumsoft** (`33`) | Dokümanda herkese açık test kullanıcısı var (`Uyumsoft`/`Uyumsoft`). Adaptör bu hafta test ortamında uçtan uca denenebilir. |
| 3 | **İzibiz** (`36`) | İzibiz'in kendi test projesindeki kullanıcı (`izibiz-test2`). Aynı şekilde denenebilir. |
| 4 | **İşNet** (`32`) | Kod test şablonuna göre yazılır; canlı ve test IP tanımı için İşNet başvurusu **Pazartesi** yapılır [S]. |
| 5 | QNB / EDM | Easytrade'in mevcut istemcisine bağlı [S]. |
| 6 | Easytrade API (`09`) | 14 açık soru cevaplanınca [S]. |
| — | Paraşüt, KolayBi, Logo, Mikro, BizimHesap | Perşembe sonrası. Excel import (`04`) hepsinin yedeği. |

Not: Test kullanıcı bilgileri yalnızca entegratörlerin kendi yayımladığı **test ortamlarında** ve fixture üretmek için kullanılır. Gerçek mükellef bilgisi bu hafta kullanılmaz.

### K6 [Ö] Luca çıkışı stratejisi

- Formatı **tahmin etmiyoruz.** İki yol açık:
  - (a) Easytrade'in **mevcut Luca entegrasyonunun** yöntemi [S-E1]
  - (b) Luca'nın müşaviri kendi içe aktarma ekranı; örnek dosyayı müşavirden istiyoruz [S-M2]
- Kod tarafında `AccountingExportAdapter` arayüzü tanımlanır. Luca dışında Mikro, Orka ve Zirve için de aynı arayüz kullanılır (transkript s.319).
- Hesap kodu eşleme katmanı (T1) formattan bağımsızdır ve **bu hafta yazılır**:
  - mükellef bazında kural hafızası (600/391/120 vb.)
  - cari hesap otomatik açma önerisi
  - "bu satıcıdan gelen fatura hep 770'e" öğrenmesi
- Çift kayıt (E9): her çıktı satırı ETTN taşır. "Luca'da zaten var" kontrolü için müşaviren Luca'dan dönem fiş listesi alınıp eşlenir. İlk sürümde bu manuel yükleme ile yapılır.

### K7 [Ö] Banka: üç seviye

| Seviye | Ne | Bu hafta |
|---|---|---|
| 1 | Ekstre yükleme: Excel / PDF / MT940 (`23`) | **Evet.** Sınıflandırma (`21` §5), eşleme, `BNK_*` kontrolleri dahil. |
| 2 | Banka web servisi + şube talimatı + sabit IP (`20`, `22`) | Hayır. Önce 1–2 banka; talimat formu akışı tasarlanır. Hangi bankanın önce yapılacağı müşavirin mükellef listesine göre belirlenir [S-M3]. |
| 3 | Onay linki (müşavirin "ideal"i) | **Ürün kararı Easytrade'in [S-E6].** Lisanslı ÖHVPS sağlayıcısı veya aracı firma gerektirir; paket bunu yasaklamış. Öneri: yasak kaldırılsın, bir sağlayıcıyla ön görüşme yapılsın. Karar gelene kadar seviye 1 + 2. |

### K8 [Ö] Fiş yakalama: masraf, yakıt, Z raporu

- **Mobil (Expo):** mükellef fotoğraf çeker.
  - AI OCR (tarih, VKN, tutar, KDV oranı, plaka) → `expense` taslağı (`approval_status=pending`) → müşavir onayı
  - Güven skoru düşükse alan sarı gösterilir
  - Masraf fişi T6, yakıt fişi T7
- **Plaka kontrolü (T8):** mükellefin araç listesi + `PLATE_NOT_REGISTERED` (kod pakette zaten var)
- **Z raporu (T5):**
  - Kısa vade: aynı OCR akışı ile Z fişi fotoğrafı → `retail_sale` günlük toplamı
  - Orta vade: Paygo GMP-3 verisi [S-E3]
  - Hugin / Beko / Ingenico mükellefleri için kalıcı yol fotoğraf ve Excel
- Görsel her zaman kaydın yanında saklanır (T2).

### K9 [K] Kontroller ve uyarılar

`05`'teki bütün kodlar uygulanır. Eşikler `warning_rule_config` tablosunda, ofis ve mükellef bazında ayarlanır. Ek olarak:

- **KDV erken uyarı (T10):** dönem içi hesaplanan − indirilecek KDV tahmini, önceki dönem devri, beyan tarihine kalan gün. Kaynak ortak model, hesap `invoice_tax_summary` üzerinden.
- **Tahsilat / ödeme yöntemi uyumsuzluğu (T11):** `BNK_PAYMENT_METHOD_MISMATCH`, yani transkript s.475'teki "nakit girildi ama havale geldi" örneği.

### K10 [Ö] Perşembe kapsamı dışında kalanlar (sırayla sonraki faz)

- e-tebligat (T13)
- dijital VD/SGK (T12)
- dilekçe (T16)
- markalı otomatik e-posta (T15'in e-posta kısmı)
- Logo / Mikro bağlantı ajanı (.NET 8)

AI finansal yorumun **ekran taslağı** Perşembe demosuna girer; otomatik gönderim girmez.

---

## 4. Mimari özet

```
 Kaynaklar                         HAKAN (Go)                                   Çıktı
 ─────────                         ──────────                                   ─────
 UBL/ZIP yükleme ──┐
 Uyumsoft / İzibiz ┤ EntegratorAdapter ─► entegrator_document ─► UBL ayrıştırıcı ─┐
 İşNet (IP+VKN)  ──┘     (fixed-egress)                                            │
 Easytrade API ────── SourceAdapter ───────────────────────────────────────────────┤
 Excel (AI eşleme) ── import state machine (04) ───────────────────────────────────┼─► ortak model (01)
 Banka ekstresi ───── bank parser (23) ─► sınıflandırma (21) ──────────────────────┤      │
 Mobil fiş foto ───── OCR ─► expense/retail_sale taslağı ──────────────────────────┘      │
                                                                                          ▼
                                                     kontroller ─► data_warning (05) ─► dönem hazır mı
                                                          │
                                       hesap kodu eşleme (T1) ─► AccountingExportAdapter ─► Luca / Mikro / Zirve
```

Kod yerleşimi (öneri, boilerplate `internal/modules` kalıbına uygun):

```
backend/internal/modules/
  organizations/ companies/        kiracı + mükellef
  connections/                     source_connection, şifreli kimlik bilgisi
  sync/                            sync_window, sync_run, zamanlayıcı
  entegrator/{core,uyumsoft,izibiz,isnet,qnb,edm}
  ubl/                             tek ayrıştırıcı (31)
  invoices/ expenses/ retail/ customers/
  bank/{parse,classify,match}
  imports/                         Excel + AI eşleme (04)
  warnings/                        data_warning + kurallar
  ledger/                          hesap kodu eşleme, kural hafızası
  exports/accounting/{luca,...}
  receipts/                        mobil fiş + OCR
  periods/                         dönem kilidi, readiness, change_request
```

---

## 5. Perşembe teslimi: tanım

**"Bitti" demek:** Bir demo ofisi, iki demo mükellef ve gerçek formatta veriyle şu akış çalışır. Müşavir panelinde:

1. e-fatura ZIP'i yüklenir; Uyumsoft ve İzibiz test hesabından faturalar çekilir.
2. Faturalar UBL'den ayrıştırılır, oran bazında KDV özeti çıkar, mükerrer ve tutarsızlık uyarıları görünür.
3. Banka ekstresi (Excel ve MT940) yüklenir, hareketler sınıflanır, faturalarla eşlenir, `BNK_*` uyarıları üretilir.
4. Mobilden çekilen masraf ve yakıt fişi OCR ile taslak olur, plaka kontrolü yapılır, müşavir onaylar.
5. Hesap kodu kuralları uygulanır ve dönem için **Luca'ya aktarılacak kayıt listesi** üretilir. Format [S-E1]/[S-M2]'ye göre; cevap yoksa Excel ara format ve dosya formatı tek adaptör değişikliği kalır.
6. Dönem ekranında "beyana hazır mı" durumu, KDV erken uyarısı ve AI özet taslağı görünür.

**Bilerek dışarıda kalan:** İşNet canlı, banka web servisleri, Easytrade API, Paraşüt/KolayBi/Logo/Mikro, e-tebligat, VD/SGK.

### 5.1 Gün gün plan

| Gün | Hedef | Çıktı | Dış bağımlılık (aynı gün gönderilir) |
|---|---|---|---|
| **Pzt 16** | Repo iskeleti: boilerplate + organizations (otopoly'den) + company katmanı. Şema göçleri (`01` tabloları, `entegrator_document`, `data_warning`, `attachment`). UBL ayrıştırıcı + GİB örnek faturalarıyla `31` §8 testleri. | `make local-dev` ile açılan boş ofis paneli; ayrıştırıcı testleri yeşil | Soru listeleri gönderilir: Easytrade (§6.1), müşavir (§6.2), İşNet başvurusu (§6.3) |
| **Sal 17** | ZIP/UBL yükleme akışı. Uyumsoft ve İzibiz adaptörleri test ortamında; fixture kaydı. Mükerrer + `05` §3.1–3.2 uyarıları. Fatura listesi ve detay ekranı (görsel yanında). | İki entegratörden gerçek test yanıtı ile fatura listesi | Müşavirden örnek Luca aktarım dosyası ve banka ekstresi beklenir |
| **Çar 18** | Banka dosya kanalı (Excel + MT940), sınıflandırma, eşleme, `BNK_*`. Excel import + AI kolon eşleme. Hesap kodu kural hafızası. | Banka ekranı, eşleşme önerileri, kural ekranı | Easytrade cevapları (Luca yöntemi, mevcut portal) |
| **Per 19** | Mobil fiş (Expo) + OCR + plaka. Luca çıktı adaptörü. Dönem readiness + KDV erken uyarı + AI özet taslağı. Uçtan uca demo verisi, kabul kontrol listesi, dağıtım. **23:59 teslim.** | §5'teki 6 adımlık demo | — |

Risk payı: Perşembe öğleden sonrası tampon olarak ayrılmalı. Perşembe günü yeni bir modül başlatılmaz. Bir şey kayarsa ilk kesilecek madde AI özet taslağıdır, ikincisi Excel import AI eşlemesidir (manuel eşleme kalır).

---

## 6. Sorulacaklar (Pazartesi sabah gönderilecek)

### 6.1 Easytrade (iş + backend)

| # | Soru | Neden |
|---|---|---|
| E1 | Mevcut **Luca / Mikro / Orka / Zirve entegrasyonu** nasıl çalışıyor? Dosya mı, API mı? Örnek çıktı ve alan listesi var mı? | K6, ürünün ana değeri |
| E2 | "İçeride kendi mali müşavirlik portalımız var" dediğiniz sistem ne? HAKAN onun yerine mi geçiyor, yanında mı duruyor? | K1, E13 |
| E3 | Paygo ile GMP-3 / Z raporu verisi için teknik doküman veya API var mı? | K8 |
| E4 | `09-easytrade.md` içindeki 14 açık soru (auth modeli, filtreler, `sync/pull`, enumlar, `tax_breakdowns` …) | Easytrade adaptörü |
| E5 | QNB eFinans ve EDM istemcileri mükellefin **kendi** kimlik bilgisiyle çağrılabiliyor mu? `34` §2 / `35` §2 tablolarını doldurabilir misiniz? | K5 |
| E6 | Banka onay linki için lisanslı sağlayıcı / aracı firma yasağı kalıcı mı? | K7 |
| E7 | Sabit çıkış IP'leri (2 adet) hangi sunucudan verilecek? Dokploy sunucusu mu? | İşNet + banka |
| E8 | Perşembe teslimi kime yapılıyor: iç demo mu, müşavire gösterim mi? 15 günlük randevu (~22 Kasım) ile ilişkisi ne? | §5 kapsamı |
| E9 | KVKK: veri işleyen sözleşmesi ve aydınlatma metni hukuk tarafından mı hazırlanıyor? | `02` KVKK |

### 6.2 Müşavir (Mustafa Bey / Kadir Bey)

| # | Soru |
|---|---|
| M1 | **Y belgesi**: 8 Kasım yazılı görüşmenin tam metni (pakette yok) |
| M2 | Luca'ya bugün Rahat Aktarım / MKLF ile aktardığınız dosyanın örneği ve Luca hesap planı çıktısı (bir mükellef, maskeli) |
| M3 | Mükellef listesi entegratöre ve bankaya göre adet olarak (isim gerekmez). "%80 İşNet" bunu doğrular, banka önceliğini belirler. |
| M4 | e-Mükellef ekran görüntüleri / video (Easytrade görüşmede istemişti) |
| M5 | Banka onay linki örneği (gördüğünüz bir ürün) |
| M6 | Örnek banka ekstreleri (Excel/PDF/MT940), 2–3 banka, maskeli |
| M7 | TÜRMOB tercihli entegratör kullanan mükelleflerde faturalar Luca'ya zaten geliyor mu? |

### 6.3 Entegratörler

- **İşNet:** `32` §2'deki 5 soru + test ortamı IP tanımı başvurusu (`efaturadestek@nettefatura.com.tr`, `servisyazilim@is.net.tr`). **Easytrade adına gönderilir; bizim göndereceğimiz bir e-posta değil.**
- **İzibiz:** canlı adres, kendi test hesabımız, durum ve hata kodları (`36` §12)
- **Uyumsoft:** Integration servisinin SOAPAction değerleri, salt görüntüleme kullanıcısı açılabiliyor mu (`33` §14)

---

## 7. Riskler

| Risk | Olasılık | Etki | Önlem |
|---|---|---|---|
| Luca formatı Perşembe'ye kadar gelmez | Yüksek | Yüksek | Hesap eşleme formattan bağımsız yazılır, çıktı adaptörü tek dosya. Ara Excel formatıyla demo. |
| İşNet erişimi haftalar sürer | Kesin | Yüksek | ZIP/UBL yükleme ilk gün çalışır. İşNet mükellefleri bu kanaldan devam eder. |
| Rakip-kaynaklar API'yi kısar (Uyumsoft, Mikro, Robom) | Orta | Orta | Her kaynağın yanında dosya kanalı. Bağımlılık tek kanala bırakılmaz. |
| Çift kayıt (TÜRMOB → Luca) | Orta | Yüksek | ETTN taşıyan çıktı + Luca fiş listesiyle eşleme (K6) |
| Test yanıtı ile doküman farklı çıkar | Yüksek | Orta | Fixture önce, eşleyici sonra (`37` §6); büyük/küçük harf duyarsız okuma |
| OCR yanlış okur | Orta | Orta | Her OCR kaydı taslak, müşavir onayı zorunlu; düşük güvenli alan vurgulu |
| AI'a kişisel veri gider | Düşük | Yüksek | Maskeleme katmanı (`04`); AI sadece eşleme/okuma yapar, karar müşavirde |
| 4 günde kapsam taşar | Yüksek | Yüksek | §5.1 kesme sırası; Perşembe yeni modül yok |

---

## 8. Kaynak doküman düzeltme notları

| Doküman | Düzeltme |
|---|---|
| `mali-musavir-portali-talepler.md` e-fatura satırı | "Mükelleften alınıyor, tek tek işleniyor" yerine: müşavir e-faturayı mükellefin entegratöründen ara programla (Rahat Aktarım, MKLF) Luca'ya aktarıyor (G§3, Y) |
| `mali-musavir-portali-talepler.md` KDV eşiği | Öneri Easytrade'e değil, görüşmedeki ilgili kişiye ait (transkriptle teyit edilerek düzeltilir) |
| Paket `00-README` / `CLAUDE.md` kapsam dışı listesi | Bu repoda K3 geçerli |
| `30` §1 / `36` İzibiz önceliği | Tek değere çekilmeli; bu repoda K5 geçerli |
| `06` kaynak Y | Belge `kaynak/` altına eklenene kadar "%80 İşNet" doğrulanmamış kabul edilir |
