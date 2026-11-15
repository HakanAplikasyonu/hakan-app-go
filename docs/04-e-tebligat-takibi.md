# 04 · e-Tebligat takibi: kapsam ve yöntem (#54)

| | |
|---|---|
| Talep | T13 (`kaynak/musavir-entegrasyon-paketi/06-mali-musavir-talepleri.md`): *"E-tebligatı kaçırmak çok ciddi sorunlar demek. … benim programım e-tebligatı geceleri sorguluyor. Gündüz bana rapor atıyor."* (G§16) |
| Sorumluluk sınırı | `06` §4 ilke 7: *"biz bunu mükellef adına üstlenemiyoruz, yükümlülük altına girmiyoruz."* Bildirim mükellefe gider; ürün tebligatın sorumluluğunu üstlenmez. |
| Tarih | 2026-11-15 |
| Kaynak düzeyi | Yasa metni resmi kaynaktan okundu (mevzuat.gov.tr, VUK). **GİB'in e-Tebligat sayfalarına bu ortamdan ulaşılamadı** (gib.gov.tr ve dijital.gib.gov.tr zaman aşımı / 404; tebligat.gib.gov.tr bağlanmadı). Rakip bilgisi rakibin kendi sitesinden. |

## Karar

**Şartlı yapılabilir; ilk dilim otomatik sorgulama içermez.**

1. **Hemen yapılabilir (Faz 0):** tebligatın süresini takip eden bir kayıt ve hatırlatma katmanı. Tebligat portala elle ya da belge olarak girilir; tebliğ tarihi ve süreler hesaplanır; mükellefe ve müşavire bildirim gider. Dış bağımlılık yok, hukuki risk düşük.
2. **Otomatik sorgulama (Faz 1–2) ancak iki şartla:**
   - (a) resmi bir kanal (GİB ya da e-Devlet API'si, meslek mensubu yetkilendirmesi) bulunur ya da açılır; **veya**
   - (b) mükellefin GİB şifresiyle portal okuma için hukuki görüş ve mükellef açık rızası alınır.

   Rakiplerin (e-Mükellef) bu işi mükellef giriş bilgilerini saklayarak yaptığı anlaşılıyor (§3). Bu yol teknik olarak mümkün, ama ürünün sorumluluk sınırıyla ve KVKK ile birlikte değerlendirilmeli.

## 1. Yasal çerçeve (doğrulandı)

VUK madde 107/A (yürürlükteki metin, mevzuat.gov.tr, `1.4.213.pdf`):

- Zorunlu olanlar: *"1. Kurumlar vergisi mükellefleri, 2. Ticari, zirai ve mesleki kazançları dolayısıyla gerçek usulde vergilendirilen gelir vergisi mükellefleri, 3. Kollektif şirketler ile adi komandit şirketler,"* ve (II) sayılı liste kapsamındaki kayıt/tescile tabi malların ilk iktisabında adına tescil yapılanlar. Diğerleri talep ederse dahil olur.
- Tebliğ anı: *"Elektronik ortamda tebligat, tebliğin bu sistem ile muhatabına iletildiği tarihi izleyen beşinci günün sonunda yapılmış sayılır."*
- Teknik altyapı ve usul Hazine ve Maliye Bakanlığına bırakılmış.

Ürüne etkisi: portaldaki mükelleflerin büyük kısmı (şirketler, bilanço/serbest meslek) **zorunlu kapsamda**. Süre, okunup okunmamasından bağımsız işliyor; "okumadım" savunması yok. T13'teki kaygı bu yüzden haklı.

Hesaplanacak tarihler (Faz 0):

| Alan | Kural |
|---|---|
| `delivered_at` | Tebligatın sisteme düştüğü (muhataba iletildiği) gün |
| `served_on` | `delivered_at` + 5 gün, günün sonu (VUK 107/A) |
| Cevap / itiraz / dava süreleri | Tebligat türüne göre değişir (ör. vergi mahkemesinde dava açma süresi). **Tür ve süre tablosu ayrıca hukuki kontrolden geçmeli**; ilk dilimde süre tebligatı giren kişi tarafından seçilir/girilir, ürün varsayılan süre önermez. |

## 2. Erişim yolları

| Yol | Durum | Not |
|---|---|---|
| GİB e-Tebligat portalı (mükellef girişi) | **Doğrulanmadı** (sayfalara erişilemedi) | Mükellef kendi İnternet Vergi Dairesi / dijital vergi dairesi girişiyle okuyor. |
| Meslek mensubu (SMMM) yetkilendirmesiyle müşavirin kendi kullanıcısından okuma | **Doğrulanmadı** | Beyanname için aracı (meslek mensubu) girişi var; e-tebligatın bu girişle okunup okunamadığı GİB kaynağından teyit edilmeli. Varsa en temiz yol bu: mükellef şifresi saklanmaz. |
| Resmi API / web servis | **Bulunamadı** | Herkese açık bir GİB e-Tebligat API'si kaydı bulunamadı; GİB sayfaları okunamadığı için "yok" kesinliğiyle söylenemez. |
| UETS (PTT Ulusal Elektronik Tebligat Sistemi, e-Devlet) | Kısmen doğrulandı | e-Devlet sayfası (`turkiye.gov.tr/ptt-uets`) yalnızca giriş yöntemlerini söylüyor (e-Devlet şifresi, mobil/e-imza, kimlik kartı, internet bankacılığı). Vergi tebligatlarının UETS'e mi GİB sistemine mi gittiği ve UETS'in üçüncü taraf API'si olup olmadığı **doğrulanmadı**. Vergi tebligatı için yasal dayanak VUK 107/A ve Hazine ve Maliye Bakanlığı sistemi. |
| Mükellef şifresiyle gece sorgulama (portal okuma) | Teknik olarak mümkün görünüyor | Rakip yaklaşımı (§3). Riskler: §4. |

## 3. Rakip: e-Mükellef (Zirve Bilgi Teknolojileri)

Kendi sitesinden (emukellef.com.tr):

- Ana sayfa: *"e-Tebligatları Düzenli Sorgular, Dilerseniz Mükellefe e-Posta ile Gönderir"*.
- Özellikler, "E-Tebligat Toplu Sorgulama": *"Mükelleflere gelen tebligatları görebilir ve dilerseniz mükelleflere e-posta, SMS veya WhatsApp üzerinden tebligatları toplu olarak iletebilirsiniz."*
- Özellikler, "Hızlı Giriş Sistemi": *"Tüm mükelleflerin giriş bilgilerini her defasında yazmadan GİB, SGK, E-Arşiv gibi kamu web sitelerine şifresiz ve tek tuşla giriş yaparak işlemlerinizi gerçekleştirebilirsiniz."* Bundan mükellef giriş bilgilerinin üründe saklandığı **anlaşılıyor** (açıkça yazmıyor).
- Gece sorgulama sıklığı ve mekanizma sitede yazmıyor. Müşavirin T13'teki anlatımı ("geceleri sorguluyor, gündüz rapor atıyor") bununla uyumlu.

Luca, Zirve masaüstü ve diğer "mükellef takip" ürünleri bu araştırmada okunmadı.

## 4. Mükellef şifresiyle sorgulamanın riskleri

| Risk | Açıklama |
|---|---|
| Yetki genişliği | Vergi dairesi girişi yalnızca tebligat değil, beyanname, borç sorgulama ve başvuru işlemlerine de açılır. Saklanan şifre "salt okuma" ile sınırlanamaz; sızması mükellef için ciddi zarar. |
| KVKK ve sözleşme | Açık rıza, aydınlatma metni, veri işleyen sözleşmesi (A11) ve saklama süresi gerekir. Hukuki görüş alınmadan başlanmamalı. |
| Sorumluluk | T13'teki ilke 7 ile çelişme riski: ürün "her gece kontrol ediyorum" dediği anda kaçan bir tebligatın sorumluluğu tartışılır hale gelir. Arayüz ve sözleşme metni bunu açıkça sınırlamalı. |
| Teknik | Portal değişiklikleri, CAPTCHA / ek doğrulama, oturum sınırları, hız sınırı ve IP engeli; sabit çıkış IP'si (W5 #45) gerekebilir. Kazıma (scraping) her portal değişikliğinde kırılır. |
| Şifre yaşam döngüsü | Mükellef şifresini değiştirince bağlantı kopar; "bağlantı çalışmıyor" uyarısı ve yeniden giriş akışı şart. |

## 5. Kapsam önerisi

### Faz 0: tebligat kaydı ve süre takibi (otomatik sorgulama yok, önerilen ilk dilim)

- Mükellef başına **tebligat kaydı**: gönderen birim, konu, tür, `delivered_at`, belge (PDF / fotoğraf). Girişi mükellef (mobil, #38 belge yükleme altyapısı) ya da müşavir yapar.
- **Hesaplanan tarihler:** `served_on = delivered_at + 5 gün` (VUK 107/A); varsa elle girilen cevap/dava son günü.
- **Bildirim:** yeni kayıtta ve son günlere yaklaşırken (ör. 7/3/1 gün kala) mükellefe ve atanmış müşavire uygulama içi + push + e-posta. Mevcut bildirim altyapısı ve uyarı motoru (#13) kullanılır; uyarı kodu `05`'e eklenir (ör. `TEBLIGAT_DUE`).
- **Ofis görünümü:** bütün mükelleflerin açık tebligatları, son güne göre sıralı ("komuta merkezi" #34 içinde bir sütun).
- **Sorumluluk metni:** "Tebligatı takip etmek mükellefin sorumluluğundadır; bu ekran hatırlatma amaçlıdır."

### Faz 1: resmi kanal

- GİB / Hazine ve Maliye Bakanlığından meslek mensubu yetkilendirmesiyle okuma ya da API olup olmadığı sorulur (§6). Varsa entegrasyon Faz 0 kayıtlarını otomatik doldurur; kayıt modeli aynı kalır.

### Faz 2: mükellef şifresiyle gece sorgulama (yalnızca hukuki onay ve açık rızayla)

- Kimlik bilgisi sır kasasında (bağlantı başına, banka ve kaynak adaptörleriyle aynı düzen), sabit çıkış IP'li worker'da gece bir kez, yalnızca liste okunur. Hiçbir işlem yapılmaz.
- Bağlantı hatası (şifre değişti, ek doğrulama) mükellefe ve müşavire uyarı olarak düşer.

## 6. Açık sorular

- [ ] GİB e-Tebligat'ın meslek mensubu (SMMM) yetkilendirmesiyle müşavir kullanıcısından okunup okunamadığı (GİB / TÜRMOB)
- [ ] GİB e-Tebligat için üçüncü taraf API / web servis var mı, planlanıyor mu
- [ ] Vergi tebligatlarının UETS ile ilişkisi (aynı kutu mu, ayrı sistem mi)
- [ ] Mükellef GİB şifresinin üçüncü taraf yazılımda saklanmasına dair hukuki görüş (KVKK, vergi mahremiyeti, sözleşme)
- [ ] Tebligat türlerine göre cevap / itiraz / dava süreleri tablosu (hukuki kontrol)
- [ ] Pilot ofisin bugünkü aracının (e-Mükellef) gece sorgulamayı hangi bilgilerle yaptığı (müşavire sorulabilir)

## Kaynaklar

- 213 sayılı Vergi Usul Kanunu, madde 107/A, yürürlükteki metin: https://www.mevzuat.gov.tr/mevzuatmetin/1.4.213.pdf
- e-Devlet, PTT UETS: https://www.turkiye.gov.tr/ptt-uets
- e-Mükellef: https://emukellef.com.tr/ , https://www.emukellef.com.tr/ozellikler , https://www.emukellef.com.tr/sikca-sorulan-sorular
- Talep: `docs/kaynak/musavir-entegrasyon-paketi/06-mali-musavir-talepleri.md` T13, §4 ilke 7
- Erişilemeyen: https://www.gib.gov.tr/e-tebligat (404), https://dijital.gib.gov.tr (zaman aşımı), https://tebligat.gib.gov.tr (bağlantı yok)
