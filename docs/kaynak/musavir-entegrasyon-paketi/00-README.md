# Mali Müşavir Portalı: Entegratör, Program ve Banka Entegrasyonları

Mükelleflerin e-fatura entegratörlerinden, ön muhasebe programlarından ve bankalarından beyan için gereken veriyi tek bir ortak modele almak için entegrasyon dokümanları. Backend ekibi her kaynağı bu dokümanlardaki sözleşmelere göre bağlar.

**Neden:** Müşavirin görüşmede ne istediği ve bu paketin o taleplerin hangisini karşıladığı `06-mali-musavir-talepleri.md`'de. Kısaca: bu paket verinin **evraksız ve elle işlemeden toplanmasını** çözer; müşavirin asıl gördüğü değer olan **Luca'ya otomatik aktarım**, Z raporu ve mobil masraf fişi bu paketin dışında (`06` §5).

**Referans standart: Easytrade Muhasebe API.** Ortak modelin alan adları, enum değerleri, yanıt zarfı, sayfalama ve filtre kuralları Easytrade API ile birebir aynı. Diğer programlar bu yapıya eşlenir. Easytrade'de karşılığı olmayan ama beyan için gereken alanlar **⊕** ile işaretli "portal uzantısı"dır. Easytrade API'sinde değişiklik önerilmez.

## 1. Doğrulama seviyesi (bütün dosyalarda aynı)

| İşaret | Anlamı |
|---|---|
| ✅ | Üreticinin / bankanın kendi teknik dokümanından |
| ⚠️ | Topluluk kodu, üçüncü taraf kılavuzu veya dokümanda eksik. İlk gerçek bağlantıda doğrulanır. |
| ❌ / **Yok** | Bu veri bu kaynaktan alınamıyor veya hiçbir teknik bilgi bulunamadı. Dosyada alternatif yol yazılı. |
| ⊕ | Portal uzantısı (ortak modelde var, Easytrade API'de yok) |

**Önemli:** Bu dokümanlar araştırmaya dayanıyor. **Hiçbir kaynak Easytrade tarafından canlı hesapla test edilmedi.** ✅ olan kaynaklarda da her adaptör `02` §11 (programlar), `20` §9 (bankalar) ve `30` §9 (entegratörler) kabul testlerini gerçek veya sandbox hesapta geçmeden üretime alınmaz. Doküman ile kaynağın o günkü davranışı çelişirse kaynak kazanır ve doküman güncellenir.

## 2. E-fatura entegratörleri: ne var, ne yok (faturaların ana kaynağı)

Müşavir görüşmesi: ofisler faturayı ön muhasebe programından değil **mükellefin e-fatura entegratöründen** alıyor. Bir ofisin mükelleflerinde yaklaşık **%80 İşNet**, kalanı Uyumsoft, QNB eFinans, EDM. Bu yüzden faturalar için **ilk kaynak entegratördür**; içerik her zaman UBL XML'den tek ayrıştırıcıyla okunur (`31`).

| Entegratör | Kaynak seviyesi | Gelen e-fatura | Giden e-fatura | E-arşiv | UBL XML | Kimlik | En kritik açık iş |
|---|---|---|---|---|---|---|---|
| **İşNet** (`32`) | ✅ İşNet arayüz dokümanları (V2, 2024) + çalışan bir topluluk modülünün gözlemleri ⚠️ | Var | Var | Var | Var | Şifre yok: **sabit IP + VKN** | Easytrade'in hangi statüde, hangi mükellef onayıyla erişeceği (İşNet ile görüşme) |
| **Uyumsoft** (`33`) | ✅ Uyumsoft dokümanı (v2.0.6, 2018) + test ortamı şemaları | Var | Var | Giden kutusunda | Var | Mükellef kullanıcı/şifre | Integration WSDL adresi, güncel doküman |
| **QNB eFinans** (`34`) | **Easytrade'de mevcut entegrasyon** | Mevcut istemciye göre | Mevcut istemciye göre | Bulunan kaynaklarda liste ucu yok | Var | Kullanıcı/şifre + VKN | Mevcut istemcinin portal ihtiyaç listesini (`34` §2) karşılaması |
| **EDM** (`35`) | **Easytrade'de mevcut entegrasyon** | Mevcut istemciye göre | Mevcut istemciye göre | ⚠️ | Var | Kullanıcı/şifre → oturum | Mevcut istemcinin portal ihtiyaç listesini (`35` §2) karşılaması |
| **İzibiz** (`36`) | ✅ İzibiz'in kendi istemcileri (test) | Var | Var | Var | Var | Kullanıcı/şifre → oturum | Canlı adres, durum kodları |

Adaptörler `37` (uygulama rehberi) üzerine kurulur: SOAP katmanı, hata modeli, indeks tablosu ve senkron akışı bir kez yazılır, her entegratör sadece kendi istemcisini ve eşleyicisini ekler.

Entegratör **sadece e-fatura ve e-arşivi** verir. Kâğıt fatura, fiş, kasa, banka ve gider kategorisi için aşağıdaki program, banka ve yükleme kanalları devam eder (`30` §2).

**Luca notu:** Luca Yazılım'ın duyurusuna göre TÜRMOB tercihli entegratör kullanan firmaların kayıtları Luca'ya zaten otomatik aktarılıyor. Portalın Luca'ya aktarımında çift kayıt riski var; Luca çıkış dokümanında ele alınmalı (`30` §10).

## 3. Ön muhasebe programları: ne var, ne yok

| Kaynak | Kaynak seviyesi | Satış | Alış | Gelen e-fatura kutusu | Kasa / banka | Gelir-gider türü | Masraf | Artımlı senkron | Veri nasıl alınır |
|---|---|---|---|---|---|---|---|---|---|
| **Easytrade** (`09`) | ✅ resmi API dokümanı | Var | Var | Var | Var | Var | Var | ⚠️ `sync/pull` parametreleri netleşmeli | REST API. 14 açık soru Easytrade backend'e (`09` §9). |
| **Paraşüt** (`10`) | ✅ resmi doküman + resmi OpenAPI'den üretilmiş SDK | Var | Var | **Yok** (liste ucu yok) | Var | Var | Var | Yok | REST API (OAuth). Easytrade'in Paraşüt'e entegratör kaydı gerekli. |
| **KolayBi** (`11`) | ✅ resmi geliştirici dokümanı | Var | Var | Var | Var | Var | Var | Yok | REST API. `Channel` bilgisi KolayBi'den alınacak. |
| **Logo Tiger / GO** (`14`) | ⚠️ topluluk kütüphanesi (resmi REST dokümanına ulaşılamadı) | Var | Var | Yok | Var | Var (hesap planı) | Var | Var (`DATE_MODIFIED`) ⚠️ | Mükellef sunucusunda bağlantı ajanı + Logo REST servisi (lisans/bayi). |
| **Mikro** (`15`) | ✅ resmi doküman, ama okuma ucu yok | SQL ile ⚠️ | SQL ile ⚠️ | Yok | SQL ile ⚠️ | SQL ile ⚠️ | SQL ile ⚠️ | ⚠️ | API uçları yazma yönünde. Kurulu sürümde ajan + salt okunur SQL (tablolar resmi olarak yayınlanmış). JumpBulut'ta okuma yolu **yok** (Mikro'ya sorulacak). |
| **BizimHesap** (`12`) | ⚠️ topluluk SDK'sı, resmi doküman yok | **Yok** | **Yok** | Yok | Sadece cari ekstresi ⚠️ | **Yok** | **Yok** | Yok | API sadece cari ve stok okuyor. Faturalar Excel yükleme (ve ileride entegratör). |
| **Logo İşbaşı** (`13`) | Herkese açık API dokümanı **yok** | Yok | Yok | Yok | Yok | Yok | Yok | Yok | Excel yükleme. Logo'dan API talebi (`13` §1). |
| **Robom** (`16`) | Herkese açık API **yok** | Yok | Yok | Yok | Yok | Yok | Yok | Yok | Excel yükleme. Mükellef (üretici) ile görüşme. |

**Excel yükleme** (`04`) bütün programlar ve listede olmayan her program için açık. Ama hiçbir programın Excel dışa aktarım biçimi henüz doğrulanmadı; her program için örnek dosya toplanıp şablon hazırlanacak (`04` §6).

Alan bazında karşılıklar: `03-alan-esleme-matrisi.md`.

## 4. Bankalar: ne var, ne yok

Mükellef iki yoldan birini seçer: **banka hesabını bağlama** (bankanın kendi servisi, şube talimatı + Easytrade'in sabit IP'si) veya **PDF/Excel ekstre yükleme.** Aracı firma ve açık bankacılık kullanılmaz (`20` §2).

| Banka | Hesap bağlama | Kaynak seviyesi | Ekstre yükleme |
|---|---|---|---|
| Vakıfbank, Ziraat, Halkbank, Garanti BBVA, Akbank, TEB, Fibabanka, Türkiye Finans, Ziraat Katılım, Vakıf Katılım | Var | ✅ bankanın teknik dokümanı (Netsim'in yayımladığı kopyalar, bazıları 2016–2021 tarihli) | Var |
| Yapı Kredi, Kuveyt Türk, QNB / Enpara | Var, alan adları eksik | ⚠️ topluluk kodu / muhasebe programı kılavuzu | Var |
| İş Bankası | Servis var (Paraşüt bağlıyor), ama **teknik bilgi yetersiz**; bu bilgiyle adaptör yazılmaz | ⚠️ | Var |
| ING, Albaraka Türk, DenizBank | Servis var (Paraşüt kılavuzunda), **teknik doküman yok** | ❌ | Var |
| Şekerbank, Odeabank, Anadolubank, Emlak Katılım, HSBC, diğer | **Yok** (bilgi bulunamadı) | ❌ | Var |

Ayrıntı: `20` §3 (kapsam tablosu), `22` (banka banka servisler).

## 5. Açıkça yok olanlar (beklenti yaratılmamalı)

- Paraşüt'te işlenmemiş gelen e-fatura listesi.
- BizimHesap API'sinde fatura, kasa/banka hareketi, gider kategorisi ve masraf okuma.
- Logo İşbaşı ve Robom'da doğrulanmış okuma API'si.
- Mikro'da fatura okuma ucu (sadece yazma uçları dokümante); JumpBulut'ta doğrulanmış okuma yolu.
- Logo Objects REST için ulaşılabilen resmi doküman.
- ING, Albaraka, DenizBank ve listede olmayan bankalar için teknik servis dokümanı.
- Banka servislerinin hiçbirinde vergi/SGK **tahakkuk** verisi (sadece ödeme hareketi var).
- Entegratörlerde: kâğıt fatura, mükellef e-fatura kullanıcısı değilse ona kesilen e-arşiv faturalar, fişler, kasa/banka.
- QNB eFinans'ta tarih aralığıyla e-arşiv listeleme (bulunan kaynaklarda yok; Easytrade'in mevcut dokümanında varsa kullanılır).
- İzibiz'de canlı servis adresi, durum ve hata kodu listesi.
- İşNet'te şifreyle erişim (yok; IP + VKN yetkisi var, Easytrade için süreci belli değil).

## 6. Dışarıdan alınması gereken bilgiler (geliştirmeyi bekletmeyen paralel işler)

| Kimden | Ne | Dosya |
|---|---|---|
| **İşNet** | Easytrade'in erişim modeli (IP tanımı, VKN bazında mükellef onayı, bayi/çözüm ortağı), okuma için canlıya geçiş şartları | `32` §2 |
| Uyumsoft, İzibiz | Uyumsoft: Integration WSDL ve güncel doküman · İzibiz: canlı adresler, durum ve hata kodları | `33` §14, `36` §12 |
| Easytrade backend (QNB, EDM) | Mevcut istemcinin portal ihtiyaç listesini karşılayıp karşılamadığı; eksikse QNB / EDM'ye mevcut kanaldan sorular | `34` §2 ve §4, `35` §2 ve §5 |
| Easytrade backend | 14 soru (yetkilendirme, filtre anahtarları, enum değerleri, `sync/pull`) | `09` §9 |
| Paraşüt | Entegratör uygulama kaydı (`CLIENT_ID`, `CLIENT_SECRET`, `REDIRECT_URL`) | `10` §1 |
| KolayBi | `Channel` değeri ve entegratör başına mı mükellef başına mı | `11` §1 |
| Mikro | "SQL Sorgulama" ucunun kapsamı, JumpBulut okuma yolu, API başvurusu | `15` |
| Logo | Objects REST resmi dokümanı; İşbaşı API erişimi | `13` §1, `14` |
| BizimHesap, Mükellef (Robom) | Okuma API'si talebi | `12` §5, `16` §1 |
| Bankalar | İş Bankası, Yapı Kredi, Kuveyt Türk, QNB, ING, Albaraka, DenizBank teknik dokümanı; üçüncü taraf IP'sinin talimatta kabulü | `22` §16, `20` §4 |
| Mükellefler / Easytrade | Her program ve banka için örnek Excel/PDF/MT940 dışa aktarım dosyaları | `04` §6, `23` §6 |

## 7. Dosyalar (okuma sırası)

Dosyalar numara sırasıyla okunur. Numaralar konuya göre gruplu: `0x` genel, `1x` ön muhasebe programları, `2x` bankalar, `3x` e-fatura entegratörleri. Kod ajanı (Claude Code) `CLAUDE.md`'yi kendiliğinden okur.

| Sıra | Dosya | İçerik |
|---|---|---|
| — | `CLAUDE.md` | Kod ajanı için başlangıç: değişmez kurallar, hazır kararlar, iş paketleri, entegrasyon bazında hazırlık, yapılandırma, zamanlanmış işler |
| 1 | `00-README.md` | Bu dosya: ne var, ne yok |
| 2 | `01-ortak-veri-modeli.md` | Ortak şema (Easytrade adlarıyla), veritabanı kuralları, normalizasyon |
| 3 | `02-adaptor-mimarisi.md` | Program adaptör arayüzü, senkron motoru, hata yönetimi, bağlantı ajanı, portal API kuralları, kabul testleri |
| 4 | `03-alan-esleme-matrisi.md` | Her alanın her programdaki karşılığı |
| 5 | `04-excel-ice-aktarma.md` | Excel/CSV içe aktarma (programlar ve banka ekstresi) |
| 6 | `05-uyari-kodlari.md` | `data_warning` tablosu ve bütün uyarı kodları |
| 7 | `06-mali-musavir-talepleri.md` | **Müşavir gerçekte ne istiyor:** her talep görüşmedeki cümlesiyle; bu paketin hangisini karşılayıp hangisini karşılamadığı; karar bekleyenler |
| 8 | `09-easytrade.md` | Easytrade adaptörü |
| 9 | `10-parasut.md` | Paraşüt |
| 10 | `11-kolaybi.md` | KolayBi |
| 11 | `12-bizimhesap.md` | BizimHesap (API yetersiz, Excel) |
| 12 | `13-logo-isbasi.md` | Logo İşbaşı (API yok, Excel) |
| 13 | `14-logo-tiger-go.md` | Logo Tiger / GO (bağlantı ajanı) |
| 14 | `15-mikro.md` | Mikro (SQL okuma, bağlantı ajanı) |
| 15 | `16-robom.md` | Robom (API yok, Excel) |
| 16 | `20-banka-genel.md` | Banka: iki yol, sabit IP, şube talimatı, adaptör arayüzü, senkron, kabul testleri |
| 17 | `21-banka-veri-modeli.md` | `bank_transaction`, işlem türü listesi, sınıflandırma, eşleştirme, müşavir kontrolleri |
| 18 | `22-banka-servisleri.md` | Banka banka servis detayları |
| 19 | `23-banka-dosya-ice-aktarma.md` | PDF, Excel, MT940 ekstre yükleme |
| 20 | `30-entegrator-genel.md` | Entegratör: kapsam, bağlantı modeli, okuma kuralları, adaptör arayüzü, senkron, durum kuralları |
| 21 | `31-ubl-tr-esleme.md` | UBL-TR XML → ortak model (tek ayrıştırıcı) |
| 22 | `32-isnet.md` | İşNet (kod yazmadan önce `37`) |
| 23 | `33-uyumsoft.md` | Uyumsoft |
| 24 | `34-qnb-efinans.md` | QNB eFinans: Easytrade'de mevcut entegrasyon, ihtiyaç listesi |
| 25 | `35-edm.md` | EDM: Easytrade'de mevcut entegrasyon, ihtiyaç listesi |
| 26 | `36-izibiz.md` | İzibiz |
| 27 | `37-entegrator-uygulama-rehberi.md` | Entegratör adaptörlerinin ortak altyapısı: SOAP katmanı, XML ayrıştırma, hata modeli, indeks tablosu, senkron akışı, test yaklaşımı |
| — | `kaynak/mali-musavir-gorusme-transkript.md` | 7 Kasım 2026 müşavir görüşmesinin tam dökümü (`06`'daki alıntıların kaynağı) |

## 8. Çekilen veri kalemleri

| # | Kalem | Ortak varlık |
|---|---|---|
| 1 | Satış faturaları | `invoice` (`direction=outgoing`) + `invoice_item` + `invoice_tax_summary` |
| 2 | Alış faturaları (işlenmemiş gelen e-faturalar dahil, kaynak veriyorsa) | `invoice` (`direction=incoming`) |
| 3 | Finansal işlemler | `financial_transaction`, `cash_box_transfer` |
| 4 | Gelir ve gider türleri | `expense_category` |
| 5 | Masraf ve giderler | `expense` |
| 6 | Perakende satışlar (kaynak veriyorsa) | `retail_sale`, `retail_refund` |
| 7 | Personel ödemeleri (kaynak veriyorsa) | `personnel_transaction` |
| 8 | Çek ve senetler (kaynak veriyorsa) | `check`, `promissory_note` |
| 9 | Referans kartlar | `customer`, `cash_box` |
| 10 | KDV raporu, mutabakat için (kaynak veriyorsa) | `vat_report` |
| 11 | Banka hareketleri | `bank_transaction` ⊕ (`21`) |

## 9. Geliştirme sırası

1. `01` şeması ve `02` adaptör iskeleti + senkron motoru.
2. **UBL-TR ayrıştırıcı** (`31`) + **UBL XML/ZIP yükleme**. Mükellef entegratör portalından XML indirip yükleyerek ilk günden fatura verebilir; bütün entegratör adaptörleri bunun üzerine kurulur.
3. **Sabit çıkış IP altyapısı** (`20` §4; entegratör ve banka ortak).
4. **Entegratör ortak altyapısı** (`37`), sonra **İşNet adaptörü** (test ortamında; canlıya geçiş İşNet görüşmesine bağlı) → Uyumsoft → İzibiz. QNB eFinans ve EDM: mevcut Easytrade istemcisini saran ince adaptör (`34`, `35`).
5. **Easytrade adaptörü.**
6. **Excel içe aktarma motoru** (`04`) ve **banka ekstre yükleme** (`23`).
7. Paraşüt, KolayBi adaptörleri; banka adaptörleri (`20` §10).
8. Logo Tiger/GO ve Mikro için bağlantı ajanı.

## 10. Kapsam dışı (henüz dokümanı yok)

- Luca'ya çıkış (fiş aktarımı, hesap planı, kural ezberi; `06` T1)
- Z raporu (ÖKC) verisi (müşavirin en yüksek vurguyla istediği taleplerden; `06` T5)
- Easytrade kullanmayan mükellef için mobil masraf fişi girişi ve OCR (`06` T6)
- Banka onay linki (`06` T9 seviye 3; karar bekliyor)
- Vergi/SGK tahakkuk verisi
