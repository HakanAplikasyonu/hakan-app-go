# Kod ajanı için başlangıç

Bu klasör **Mali Müşavir Portalı'nın veri toplama katmanının** dokümantasyonu: mükellefin e-fatura entegratöründen, ön muhasebe programından ve bankasından veriyi alıp tek bir ortak modele (Easytrade Muhasebe API adlarıyla) yazan sistem. Kodu bu dosyalara göre yaz. Bu dosya nereden başlayacağını, hangi kuralların değişmez olduğunu ve hangi entegrasyonun bugün ne kadar yazılabildiğini söyler.

Durum özeti ve dış bağımlılıklar: `00-README.md`.

## 1. Okuma sırası

1. `00-README.md`: ne var, ne yok
2. `01-ortak-veri-modeli.md`: şema, veritabanı kuralları (§1.3)
3. `05-uyari-kodlari.md`: uyarı tablosu ve kod listesi
4. `06-mali-musavir-talepleri.md`: müşavir gerçekte ne istiyor, bu paket neyi karşılıyor. Bir özelliğin neden var olduğunu veya kapsamda olup olmadığını buradan kontrol et.
5. `02-adaptor-mimarisi.md`: program adaptör arayüzü, senkron motoru, hata davranışı, portal API kuralları
6. Yazdığın parçanın dosyası (§4'teki iş paketleri hangi dosyaya bakılacağını söyler)

## 2. Değişmez kurallar

1. **Kaynaklarda sadece okuma.** Fatura gönderen, onaylayan, "okundu / alındı" işareti koyan, arşive kaldıran hiçbir metot çağrılmaz, istemcide de bulunmaz (`30` §5, her entegratör dosyasında "Kullanılmayacak" listesi).
2. **⚠️ ile işaretli bilgiyi kesin bilgi gibi kodlama.** §6'daki kurala göre çalış.
3. **Bilgi uydurma.** Dokümanda olmayan bir alan adı, uç nokta, hata metni, durum kodu gerekiyorsa tahmin etme: yapılandırılabilir bırak, "bilinmeyen değer" yolunu yaz (uyarı + ham veri sakla), insana sor.
4. **Easytrade API'sine dokunma.** Portalın ihtiyacı olan ek alanlar ⊕ işaretli portal tablolarında tutulur (`01` §0). Easytrade tarafında yeni uç veya alan açma.
5. **Fatura içeriği UBL XML'den gelir.** Entegratörün liste yanıtındaki tutarlar sadece liste ve kontrol içindir (`30` §6, `31`).
6. **Tutarlar ondalık tiple.** `float` yok (`01` §1.2).
7. **Uyarılar kodla yazılır** (`05`). Yeni uyarı gerekiyorsa önce `05`'e eklenir.
8. **Kimlik bilgileri kasada,** veritabanında ve logda açık değil (`02` §4, `37` §2).
9. **İşNet'te VKN her zaman bağlantı kaydından** alınır, istekten / kullanıcı girdisinden değil (`32` §2).
10. **Bankanın / entegratörün verdiği WSDL dokümandan önce gelir.** Çelişki varsa WSDL'e göre yaz, ilgili doküman dosyasını güncelle.

## 3. Hazır kararlar

| Konu | Karar |
|---|---|
| Dil ve çatı | Repodaki yığın. Dokümanlardaki TypeScript kodları **dil bağımsız sözleşme**dir; repo dilinde yaz. Easytrade API Laravel üzerinde (`09`). |
| PHP ile yazılıyorsa | SOAP için `ext-soap` (`SoapClient`); Uyumsoft'un WS-Security başlığı `SoapClient` tarafından üretilmez, `SoapHeader` / XML ile elle eklenir (`33` §3). XML okumada `LIBXML_NONET`, DTD ve dış varlık kapalı (`37` §3.1). Ondalık için `brick/math` veya `bcmath`. ZIP için `ext-zip`. |
| Veritabanı | `01` §1.3: UUID kimlik, ortak kolonlar, düz `amounts` kolonları, `TEXT` + kontrol kısıtı ile enum, `company_id` ile çok kiracılık |
| Portal API'si | Easytrade kuralları: yanıt zarfı, sayfalama, `filters[...]`, `X-Company-ID` (`02` §8) |
| Kuyruk ve zamanlayıcı | Repodaki altyapı. İşler ve sıklıkları §8'de. |
| Dosya saklama | Nesne deposu, SHA-256 ile tekil, en fazla 20 MB (`02` §7) |
| Sabit çıkış IP'si | Banka ve entegratör çağrıları aynı `fixed-egress` havuzundan (`20` §4, `30` §4.2) |

## 4. İş paketleri (sırayla)

Her paketin sonunda testler geçer ve paket kendi başına kullanılabilir.

| # | Paket | Dosyalar | Bitti sayılması için | Dış erişim gerekir mi |
|---|---|---|---|---|
| A | **Çekirdek:** şema ve göçler, `source_connection` kayıt ve kasa entegrasyonu, senkron motoru (`sync_window`, `sync_run`), `data_warning` servisi, `attachment` deposu, mükerrer eşleme (`01` §6), temel portal uçları | `01`, `02`, `05`, `37` §5.1 | Göçler çalışıyor; birim testleri; boş bir sahte adaptörle uçtan uca senkron | Hayır |
| B | **UBL-TR ayrıştırıcı** + XML/ZIP yükleme ucu | `31`, `37` §3 | `31` §8 testlerinin hepsi | Hayır. Test faturaları: GİB'in UBL-TR örnek faturaları ve elle hazırlanmış vakalar (tevkifat, istisna, iade, dövizli, çok oranlı) |
| C | **Excel içe aktarma** | `04` | `04` §9 testleri (sentetik dosyalarla) | AI kolon eşleme için model sağlayıcısı anahtarı (§7) |
| D | **Banka ekstre yükleme** (MT940, Excel, PDF) + `bank_transaction` + sınıflandırma | `21`, `23` | `23` §7 testleri (sentetik MT940 ile) | Hayır |
| E | **Entegratör ortak altyapısı** + İşNet, Uyumsoft, İzibiz | `30`, `37`, `32`, `33`, `36` | Her dosyanın "Test planı" bölümü + `37` §7 kontrol listesi | Uyumsoft ve İzibiz: dokümandaki **herkese açık test kullanıcılarıyla** gerçek test yapılabilir. İşNet: test erişimi İşNet'ten alınmadan sadece şablon ve birim testi (§5). |
| F | **Easytrade adaptörü** | `09` | `02` §11 testleri | Önce `09` §9'daki 14 soruyu **Easytrade reposundaki route ve controller kodundan** cevapla, cevapları `09`'a yaz. Easytrade API dokümanı (`easytrade-muhasebe-api.md`) bu klasörde yok; Easytrade reposunda / backend ekibinde. |
| G | **QNB eFinans ve EDM** ince adaptörleri | `34`, `35` | `34` §2 ve `35` §2 ihtiyaç listeleri dolduruldu; eksik metotlar eklendi | Easytrade'in mevcut QNB / EDM istemcisi ve test hesapları |
| H | **Paraşüt, KolayBi** | `10`, `11` | `02` §11 testleri | Paraşüt: `CLIENT_ID` / `CLIENT_SECRET` / `REDIRECT_URL` gelene kadar sahte sunucuyla. KolayBi: sandbox var, `Channel` değeri gerekir. |
| I | **Banka hesap bağlama** (✅ bankalar) | `20`, `22` | `20` §9 testleri | Gerçek test için mükellefin şube talimatı + IP tanımı. O zamana kadar WSDL'den istemci + kayıtlı yanıtlarla test. |
| J | Logo Tiger/GO, Mikro bağlantı ajanı | `02` §6, `14`, `15` | `14` §11, `15` §5 | Pilot mükellef |

BizimHesap, Logo İşbaşı, Robom için API adaptörü yazılmaz; bu programlar Excel yükleme (paket C) ile desteklenir (`12`, `13`, `16`).

## 5. Entegrasyon bazında hazırlık

| Entegrasyon | Bugün yazılabilir mi | Ne eksik |
|---|---|---|
| UBL ayrıştırıcı, XML/ZIP yükleme | Tamamen | — |
| Excel yükleme | Tamamen (şablonlar sonra) | Programların gerçek dışa aktarım örnekleri (`04` §6) |
| Banka ekstre yükleme (MT940/Excel/PDF) | Tamamen | Bankaların gerçek örnek dosyaları (`23` §6) |
| Uyumsoft | Tamamen; test ortamında denenebilir | Integration WSDL adresi ve SOAPAction doğrulaması ilk çağrıda (`33` §1) |
| İzibiz | Tamamen; test ortamında denenebilir | Canlı adres, durum ve hata kodları (`36` §12). Canlıya alınmaz, test ortamında biter. |
| İşNet | İstemci, eşleme, durum tablosu, senkron tamamen | Test ve canlı erişimi İşNet görüşmesine bağlı (`32` §2). Kod, test erişimi gelince `32` §12'yle doğrulanır. |
| QNB eFinans, EDM | Mevcut istemciye bağlı | `34` §2, `35` §2 |
| Easytrade | Büyük ölçüde | `09` §9 soruları (repodan cevaplanır) |
| Paraşüt | Tamamen (resmi doküman) | Entegratör uygulama kaydı (`10` §1) |
| KolayBi | Tamamen (resmi doküman) | `Channel` (`11` §1) |
| ✅ bankalar (10 banka) | İstemci ve eşleme | Her banka için gerçek WSDL ve canlı test; şube talimatı |
| ⚠️ bankalar (Yapı Kredi, Kuveyt Türk, QNB/Enpara) | Kısmen | Alan adları eksik (`22` ilgili bölümler) |
| İş Bankası, ING, Albaraka, DenizBank, diğerleri | **Yazılmaz** | Teknik doküman yok; sadece ekstre yükleme |
| Logo Tiger/GO | Ajan iskeleti | Resmi REST dokümanı, pilot mükellef (`14` §11) |
| Mikro | SQL okuma (kurulu sürüm) | Pilot mükellef; JumpBulut için yol yok (`15`) |

## 6. ⚠️ işaretli bilgiyle çalışma kuralı

⚠️ bir bilginin **doğrulanmadığını** gösterir (topluluk kaynağı, eski doküman veya çıkarım). Böyle bir bilgiyi kullanırken:

1. Değeri tek bir yerde tut (adaptörün sabitler / yapılandırma dosyası), kod içine dağıtma.
2. Bilinmeyen / beklenmeyen değer gelirse çökme: uyarı kodu üret (`STATUS_UNMAPPED` gibi), ham veriyi sakla, devam et.
3. İlk gerçek yanıtı **fixture** olarak kaydet (maskeleyerek) ve eşleyiciyi ona göre test et (`37` §6).
4. Doğruladığında ilgili doküman dosyasındaki ⚠️'yi kaldır ve "Doğrulanacaklar" listesinde işaretle.

## 7. Yapılandırma ve gizli bilgiler

| Anahtar | Ne | Nereden |
|---|---|---|
| `SECRETS_BACKEND` / KMS anahtarı | Bağlantı kimlik bilgileri kasası (`02` §4) | Altyapı |
| `OBJECT_STORAGE_*` | Ek dosyalar, ham yanıt logları | Altyapı |
| `FIXED_EGRESS_PROXY` | Sabit IP'li çıkış (banka, entegratör) | Altyapı (`20` §4) |
| `EASYTRADE_API_BASE_URL` | `https://pro.easytradetr.com/api` | `09` |
| `PARASUT_CLIENT_ID`, `PARASUT_CLIENT_SECRET`, `PARASUT_REDIRECT_URL` | Paraşüt entegratör uygulaması | Paraşüt (`10` §1) |
| `KOLAYBI_CHANNEL`, `KOLAYBI_BASE_URL` | KolayBi kanal değeri, üretim / sandbox adresi | KolayBi (`11` §1) |
| `ISNET_ENV`, `ISNET_INVOICE_URL`, `ISNET_ADDRESSBOOK_URL` | Test / canlı adresler | `32` §1 |
| `UYUMSOFT_INTEGRATION_URL` | Test / canlı | `33` §1 |
| `IZIBIZ_AUTH_URL`, `IZIBIZ_EINVOICE_URL`, `IZIBIZ_EARCHIVE_URL` | Test adresleri; canlı boş | `36` §1 |
| `AI_PROVIDER_*` | Excel kolon eşleme ve banka açıklaması sınıflandırma önerisi | `04` §4.3, `21` §4 |

Mükellefe ait kimlik bilgileri (Uyumsoft / İzibiz / QNB / EDM kullanıcı-şifre, KolayBi `api_key`, Paraşüt token, banka servis kullanıcısı) ortam değişkeni değildir; bağlantı başına kasada durur.

## 8. Zamanlanmış işler

| İş | Sıklık | Kaynak |
|---|---|---|
| Program artımlı akış (destekleyen) | 15 dk | `02` §3.3 |
| Program açık dönem taraması | 4 saatte 1 (iş saati) + gece 1 | `02` §3.3 |
| Program referans verileri | Günde 1 | `02` §3.3 |
| Dönem kapanışı öncesi (ayın 20–28'i) | 2 saatte 1 | `02` §3.3 |
| Entegratör artımlı | 60 dk | `30` §7, `37` §5.5 |
| Entegratör gece tam tarama | Gece 1 | `37` §5.5 |
| Entegratör durum güncelleme | Günde 1 | `37` §5.5 |
| UBL tekrar indirme (`ubl_status=failed`) | 6 saatte 1 | `37` §5.5 |
| Banka dünün tam çekimi | Her gün 07:00 | `20` §7.2 |
| Banka gün içi | 60 dk (09:00–19:00) | `20` §7.2 |
| Banka son 7 gün yeniden | Gece 1 | `20` §7.2 |
| Banka önceki ay yeniden | Ayın 1–3'ü | `20` §7.2 |
| Ekstre yükleme hatırlatması | Ayın 1'i ve 15'i | `23` §2 |
| Bağlantı ajanı sağlık kontrolü | 5 dk heartbeat, 30 dk sessizlikte uyarı | `02` §6 |
| "Şimdi senkronla" | Anlık, bağlantı başına 5 dk'da 1 | `02` §3.3, `20` §7.2, `30` §7 |

## 9. Kapsam dışı (bu klasörde dokümanı yok, kod yazılmaz)

- Luca'ya çıkış (fiş aktarımı). Not: TÜRMOB tercihli entegratör kullananlarda çift kayıt riski (`30` §1).
- OCR ile fiş okuma (ortak modelde sadece alanlar ayrıldı: `capture_method=ocr`, `ocr_confidence`).
- Z raporu (ÖKC) verisi, vergi / SGK tahakkuk verisi.
- Müşavir kontrol kurallarının iş mantığı. İstisnalar: `21` §6.3 banka kontrolleri ve `05` §3.3'teki kontroller (kuralları `06` T3, T8); bunlarda eşikler yapılandırmadan okunur.
- `06` §5'te "Yok" olan talepler (Z raporu, dijital vergi dairesi, e-tebligat, müşavir mobil uygulaması, AI yorum, dilekçe). Bunlar için kod yazılmaz.

## 10. Ne zaman insana sor

- Bir uyarının seviyesi (`05` §2) veya "beyana hazır" kuralı gibi **ürün kararı** gerekiyorsa.
- Dış taraftan bilgi gerekiyorsa (İşNet erişimi, Paraşüt kaydı, KolayBi `Channel`, banka dokümanı): bilgiyi bekleyen kısmı yapılandırılabilir bırakıp devam et, listeyi insana ver.
- Doküman ile gerçek yanıt çelişiyorsa ve çelişki veri kaybına yol açabiliyorsa.
