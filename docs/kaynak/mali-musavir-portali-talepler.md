# Mali Müşavir Portalı: Saha Görüşmesi Talep Dokümanı

**Görüşme:** Mali müşavirlik ofisi (Kadir Bey ve kıdemli ortak)
**Tarih:** 7 Kasım 2026
**Amaç:** Mali müşavirin günlük iş akışındaki gerçek sorunları ve portaldan beklentilerini kayda geçirmek.
**Kapsam notu:** Bu dokümanda yalnızca karşı tarafın dile getirdiği talepler, onayladığı fikirler ve verdiği uyarılar yer alır. Easytrade tarafının önerip müşavirin açıkça onayladığı maddeler **[Onaylandı]** etiketiyle işaretlenmiştir.

---

## İçindekiler

1. [Genel Bağlam ve Müşavirin Ana Derdi](#1-genel-bağlam-ve-müşavirin-ana-derdi)
2. [Muhasebe Programıyla Canlı Senkron](#2-muhasebe-programıyla-canlı-senkron)
3. [Fatura Kontrolü ve Akıllı Uyarılar](#3-fatura-kontrolü-ve-akıllı-uyarılar)
4. [Z Raporu Entegrasyonu](#4-z-raporu-entegrasyonu)
5. [Masraf ve Yakıt Fişleri](#5-masraf-ve-yakıt-fişleri)
6. [Banka Entegrasyonu](#6-banka-entegrasyonu)
7. [KDV Erken Uyarı Sistemi](#7-kdv-erken-uyarı-sistemi)
8. [Tahsilat Makbuzu ve Tediye Aktarımı](#8-tahsilat-makbuzu-ve-tediye-aktarımı)
9. [Dijital Vergi Dairesi ve SGK Entegrasyonu](#9-dijital-vergi-dairesi-ve-sgk-entegrasyonu)
10. [E-Tebligat Takibi](#10-e-tebligat-takibi)
11. [Mali Müşavire Özel Mobil Uygulama](#11-mali-müşavire-özel-mobil-uygulama)
12. [AI Destekli Finansal Yorum ve Sunum](#12-ai-destekli-finansal-yorum-ve-sunum)
13. [Dilekçe ve Resmi Evrak Üretici](#13-dilekçe-ve-resmi-evrak-üretici)
14. [Luca'ya Bakış ve Geçiş Riskleri](#14-lucaya-bakış-ve-geçiş-riskleri)
15. [Müşavirin Uyarıları ve Ürün İlkeleri](#15-müşavirin-uyarıları-ve-ürün-ilkeleri)
16. [Açık İşler ve Sonraki Adımlar](#16-açık-işler-ve-sonraki-adımlar)
17. [Özet Tablo](#17-özet-tablo)

---

## 1. Genel Bağlam ve Müşavirin Ana Derdi

Müşavirin tüm taleplerinin ortak hedefi şu: **mükelleften fiziksel evrak toplama ihtiyacını tamamen ortadan kaldırmak** ve **tek tek manuel işlemeyi bitirmek.**

Bugünkü durum şöyle:

| Belge türü | Bugünkü durum |
|---|---|
| E-arşiv fatura | Sistemden görüntüleniyor, evrak toplamaya gerek yok |
| E-fatura | Mükelleften alınıyor, tek tek işleniyor |
| Z raporu | Mükelleften alınıyor ya da QR ile tek tek okutuluyor |
| Masraf fişi | Mükelleften fiziksel olarak alınıyor |
| Banka ekstresi | Mükelleften isteniyor, haftalarca beklenebiliyor |
| Tahsilat makbuzu | Tek tek manuel işleniyor |

Bu boşlukları kapatmak için bugün Luca'nın yanında ek ücretli programlar kullanıyorlar (MKLF, Rahat Aktarım, e-Mükellef). Hepsinin **tek bir yerde** toplanmasını istiyorlar.

---

## 2. Muhasebe Programıyla Canlı Senkron

### Mevcut durum (referans: Paraşüt–Luca)
- Paraşüt, Luca ile eşleştirildiği anda hesap planını otomatik çekiyor.
- Her girişte hesap planını yeniden çekiyor. Bir tarafta açılan hesap diğerinde de otomatik oluşuyor.
- Mükellef bilgileri Luca'da bir kez tanımlandıktan sonra aktarımlar kendiliğinden doğru hesaplara gidiyor.

### İstenen
1. **Çift yönlü hesap planı senkronu.** Hesap planı her oturumda güncel çekilmeli. Easytrade'de veya muhasebe programında açılan hesap karşı tarafta otomatik oluşmalı.
2. **Kural öğrenme ("ezberleme").** Müşavir bir kez kural tanımlar, sistem sonraki tüm kayıtlarda uygular. Örnek:
   - Kesilen satış faturaları → matrah **600**, KDV **391**
   - Cari → **120**, TCKN / vergi numarasına göre eşleştir
3. **Otomatik cari açılışı.** Cari bulunamazsa sistem kendisi açmalı, iki tarafta da.
4. **Sıfır müdahale hedefi.** Cari havuzu oturduktan sonra satış faturaları hiç ek işlem yapmadan muhasebeleşmeli.

### Kabul kriteri
- Yeni mükellef bağlandığında hesap planı manuel işlem olmadan aktarılıyor.
- Aynı firmadan gelen ikinci faturada müşavir hiçbir hesap seçimi yapmıyor.

---

## 3. Fatura Kontrolü ve Akıllı Uyarılar

### 3.1 Fatura görüntüsünün kayıt yanında açılması

**Sorun:** Faturanın görüntüsüne ulaşmak için ayrı programa girip menülerde gezinmek gerekiyor.

**Mevcut çözümleri:** Tarayıcıya eklenti kurulu. Yevmiye kaydının yanında faturanın PDF'i doğrudan açılıyor.

**Neden ihtiyaç var:**
- Bazı mükellefler ayda 2.000–3.000 fatura alıyor, bazıları 100.
- Fatura içeriğinde demirbaş gibi farklı kayıt gerektiren kalemler gözden kaçabiliyor. Program uyarı verse bile hata olmasın diye görüntüye bakıyorlar.
- Faturanın görüntüsü başka işler için de lazım oluyor.

**İstenen:** Her muhasebe kaydının yanında, tek tıkla faturanın orijinal görüntüsü.

### 3.2 Akıllı uyarılar [Onaylandı]

Müşavirin bugün kullandığı aktarım programı benzer uyarılar veriyor. Portalda da olması isteniyor:

| Uyarı | Örnek |
|---|---|
| Demirbaş olabilir | Mal alımı faturasında "bilgisayar" açıklaması var |
| Kayıt türü farklı olabilir | Danışmanlık satan firmanın faturasında araç satışı görünüyor |
| Olağandışı tutar | Normalde ayda belli seviyede fatura alan firmaya bir anda çok yüksek tutarlı fatura gelmiş |
| Yeni tedarikçi + yüksek tutar | Daha önce çalışılmamış firmadan büyük fatura |

**Not:** Mükellef bazında eşik öğretilebilir olmalı (ör. "bu firmanın ortalama alışı 150.000 TL, 2.000.000 TL gelirse uyar").

### 3.3 Mükerrer fatura kontrolü

**Referans (Luca):**
- Luca içinden işlenen faturanın yanında hangi fişte işlendiği gösteriliyor.
- Manuel olarak iki kez işlenirse ayrı bir "mükerrer fatura kontrolü" ekranı var: "Bu fatura hem şu fişte hem şu fişte var, kontrol edin."
- Eşleştirme **fatura numarası + cari** üzerinden.

**İstenen:** Aynı mantığın portalda olması. Hem otomatik aktarımda hem manuel girişte çalışmalı.

---

## 4. Z Raporu Entegrasyonu

**Müşavirin ifadesiyle:** "Çok büyük yük kaldırır." Evrak toplamaya gitmeyi bitiren ana kalemlerden biri.

### Sorun
- Firma Z raporu kesiyor, müşavir bunu almaya gidiyor ya da fotoğraf/QR ile tek tek okutuyor.
- 3 şubesi olan bir mükellef ayda ~30 Z raporu demek.
- QR okutma çözümü "pratik değil", mükellef de müşavir de istemiyor.

### İstenen
1. Z raporlarının yazarkasadan otomatik olarak muhasebe programına düşmesi.
2. Easytrade → Luca ya da yazarkasa firması → Luca, yolu fark etmez, sonuç önemli.
3. **Tüm yazarkasa markalarıyla** entegrasyon (Paygo, Beko, Hugin, Ingenico vb.). Mükellef mevcut cihazını değiştirmek istemeyebilir.

### Müşavirin uyarısı: cihaz tavsiyesi riski
- Geçmişte Luca, Vera adlı yazarkasa için "Luca ile entegre edeceğiz" demiş. Müşavir ofisi bunu tüm mükelleflerine tavsiye etmiş.
- Entegrasyon hiç gelmemiş, cihaz da kötü çıkmış.
- Mükellefler "bunlar firmadan para alıyor" diye düşünmüş, ofis mahcup olmuş.
- **Sonuç:** Paygo'yu önerecekseniz Paygo gerçekten diğerlerinden üstün olmalı (hız, pratiklik). Cihaz değiştiren mükellef pişman olmamalı. Kullanıcılardan düzenli geri bildirim toplanmalı.

---

## 5. Masraf ve Yakıt Fişleri

### 5.1 Masraf fişleri

**Müşavirin ifadesiyle:** "Muhasebe bürolarının en önemli sorunu."

**Sorun:**
- Masraf fişlerinin bir kısmı vergi açısından gider kabul edilmiyor, ama mükellef "geçse de geçmese de işle" diyor. Müşavir mecburen işliyor.
- Z raporu çözülse bile masraf fişi yüzünden mükellefe gitmek gerekiyor.

**İstenen (asgari):** Piyasanın yaptığı kadarı. Mükellef telefondan bir ekrana girer, fişi yükler, müşavirin önüne düşer.

**İleride:** E-fiş yaygınlaşırsa doğrudan oradan çekilmeli.

**Müşavirin gözlemi:** Tek tek QR okutmak mükellefi yoruyor ("ben niye bunları tek tek okutayım"). Kullanım kolaylığı belirleyici.

### 5.2 Yakıt fişleri

**Sorun:** Araç masrafları yüksek, yakıt fişleri sıklıkla müşavire ulaşmıyor.

**Referans:** Luca'nın yakıt fişi çekme özelliği var ama "çok sağlıklı değil."

**İstenen:** Luca'nın bu sistemi incelenmeli. Onlar çekebiliyorsa portal da çekmeli, hatta daha sağlıklı çekmeli.

**Neden önemli:** Plakalı yakıt fişinin gider kabulü tartışmasız. Yakıt fişleri tam işlenirse diğer masraflar hafif kalıyor.

### 5.3 Plaka kontrolü [Onaylandı]

**İstenen:** Yakıt ve araç masrafı fişindeki plakanın mükellef adına kayıtlı olup olmadığı kontrol edilmeli. Kayıtlı değilse uyarı verilmeli.

**Müşavirin eklediği değer:**
- Bugün kayıtlı olmayan aracın fişi de işleniyor, sonradan fark ediliyor.
- Uyarı ayrıca şu durumları ortaya çıkarır:
  - Mükellef aracı satmış ama fatura kesmemiş
  - Mükellef araç almış ama fatura almamış
- "Büyük bir yükten kurtarır."

---

## 6. Banka Entegrasyonu

### 6.1 Seviye 1: Ekstre dönüştürme (temel)

**Sorun:** Müşavir Excel istiyor, mükellef PDF gönderiyor. PDF'i Excel'e çevirmek iş yükü yaratıyor.

**İstenen:**
1. PDF veya Excel ekstre yüklenir.
2. Sistem bunu hedef muhasebe programının (Luca vb.) anlayacağı Excel formatına çevirir.
3. Banka hesabı (**102**) bir kez tanımlanır, karşı hesaplar ezberden gelir.
4. Sonuç: manuel işlemden yarı otomatiğe geçiş.

### 6.2 Seviye 2: Banka API (mevcut Luca çözümü)

- Luca, mükellefe banka sözleşmesi imzalatıp API ile hareketleri günlük çekiyor.
- **Müşavirin değerlendirmesi:** Diğerlerinden çok iyi ama hâlâ yük var. Mükellefi bankaya sözleşme götürmeye ikna etmek zor. Götürse bile kopyasını göndermiyor. Bazen hatalı çekiyor.

### 6.3 Seviye 3: Onay linki (ideal)

**Referans:** Kullandıkları bir program, bazı bankalarla (ör. Garanti, muhtemelen 3 banka) anlaşmalı.

**Akış:**
1. Müşavir mükellefe link gönderir (ör. WhatsApp'tan).
2. Mükellef bir kez girip onaylar.
3. 3 ay boyunca ekstreler PDF olarak otomatik gelir.
4. Program PDF'i işlenebilir veriye çözer.

**İstenen:** Aynı yapının kurulması ve **banka ağının olabildiğince genişletilmesi.**

**Müşavirin ifadesi:** "Bir haftadır mükelleften banka almak için uğraşıyoruz."

---

## 7. KDV Erken Uyarı Sistemi

### Sorun
- Mükellef evrakı takip eden ayın 20'sinde gönderiyor, KDV ödemesini o zaman öğreniyor.
- "Keşke önceden haberim olsaydı" deniyor, iş işten geçmiş oluyor.
- Bazı ön muhasebe programları KDV durumunu fatura muhasebeleşmeden göstermiyor.
- Gösterse bile mükellefin bakacak vakti yok.
- 100.000–200.000 TL'lik beklenmedik ödemeler müşavir–mükellef ilişkisini geriyor.

### İstenen
1. **Anlık KDV durumu:** Alış KDV, satış KDV ve devreden KDV mükellefe görünür olmalı.
2. **Eşik uyarısı:** Ödenecek KDV belirli tutarları geçtikçe uyarı ("KDV ödemen şu tutara çıktı").
3. **Ay sonu uyarısı:** Ayın 28'inde gibi: "X TL KDV ödemen var. Fatura bulmak istiyorsan son 2 gün."
4. **Kanal:** SMS, e-posta, uygulama bildirimi.
5. **Sıklık:** Mükellefi sıkmayacak dozda.

### Kabul kriteri
- Mükellef beyan döneminden önce tahmini KDV yükünü biliyor.
- Müşavir eşiği mükellef bazında bir kez ayarlıyor, gerisi otomatik.

---

## 8. Tahsilat Makbuzu ve Tediye Aktarımı

### Sorun
- Mükellefin ön muhasebede kestiği tahsilat makbuzları ve tediyeler müşavir tarafında **tek tek** işleniyor.
- Diğer programlarda bu aktarım yok.

### İstenen
1. Tahsilat ve tediyelerin muhasebe programına (Luca vb.) otomatik aktarılması.
2. **Ödeme türü tutarlılık kontrolü [Onaylandı]:**
   - Mükellef EFT ile gelen parayı nakit olarak girebiliyor ya da hepsini nakit giriyor.
   - Nakit, banka, kart ve çek farklı hesaplara işleniyor. Karışık veri mizana bozuk yansıyor.
   - Sistem banka hareketleriyle eşleştirip "bu tutar bankadan gelmiş ama nakit girilmiş" gibi uyarı vermeli.

### Müşavirin uyarısı
Aktarılan veri yanlışsa müşavir tek tek düzeltmek yerine baştan manuel işler ve aktarımı bir daha kullanmaz. **Veri kalitesi yoksa entegrasyonun değeri sıfır.**

---

## 9. Dijital Vergi Dairesi ve SGK Entegrasyonu

### Sorun
- Borcu yoktur yazısı, mükellefiyet yazısı, SGK borcu yoktur yazısı gibi belgeleri bugün müşavir üretip gönderiyor.
- Ya da mükellef müşavirin uygulamasını telefonuna kurup kendisi alıyor.
- Mükellef dijital vergi dairesine girip şifreyle uğraşmak istemiyor.

### İstenen
1. Easytrade'e dijital vergi dairesi ve SGK entegrasyonu.
2. Mükellef ihtiyaç duyduğu belgeyi kendisi tek tıkla almalı.
3. Dijital vergi dairesindeki veriler mükellefin ekranında görünmeli.

---

## 10. E-Tebligat Takibi

**Müşavirin ifadesiyle:** "E-tebligat kaçırmak çok ciddi sorunlara yol açar."

### Mevcut durum
- e-Mükellef programıyla e-tebligat **geceleri** sorgulanıyor, sabah rapor geliyor.
- Hukuki sorumluluk mükellefte. Müşavir sorumluluk almıyor, "tebligat senin sorumluluğunda" diyor.

### İstenen
- Mükellefe tebligat geldiğinde Easytrade ekranına anında bildirim düşmeli.
- Böylece mükellef kendi sorumluluğundaki tebligatı kaçırmaz, müşavirin üzerindeki risk de azalır.

---

## 11. Mali Müşavire Özel Mobil Uygulama

### Referans: Uyumsoft
- Mesleki kahvaltılara sponsor olup 15–20 dakikalık sunumlarla müşavirlere ulaşıyorlar.
- Müşavirlere özel bir mobil uygulama yapmışlar. Çok basit bir arayüz.
- Müşavirin ifadesi: "Luca'yı sorgulamamız gerektiğini orada düşündüm."

### Uygulamada olması istenenler
- Mizan çekme
- Fatura listesi
- Bordro
- Ücret pusulası
- Genel ilke: mükellefin arayıp isteyebileceği her şeyi müşavir telefondan hızlıca gönderebilmeli.

### Müşavirin ifadesi
"Sadece bunu satsalardı alırdım, Luca ile entegrasyonu olsaydı." Uyumsoft bunu kendi programını alana veriyor.

### Ek fikir
Müşavirler için mesleğe özel küçük programlar (mükellef yönetimi vb.) düşünülmeli.

---

## 12. AI Destekli Finansal Yorum ve Sunum

### Çıkış noktası
Bir müşavir tanıdıkları, mizanı ve beyannameyi yükleyince AI ile yorumlayan bir program yazıyor. Müşavir bunu daha da geliştirerek önerdi.

### İstenen
1. Mizan, beyanname ve çok yıllık (ör. 5 yıllık) verinin yüklenmesi.
2. Satış eğrisi, kârlılık, kasa durumu gibi analizlerin grafiklerle sunuma dönüştürülmesi.
3. **Ofise özel markalı** çıktı.
4. **Otomatik e-posta** ile mükellefe gönderim, standart periyotla.

### Neden değerli
- Veri zaten elde var ama 100 mükellefi tek tek arayıp yorum yapma imkânı yok.
- Mükellef bunu istiyor.
- Müşavir için "artı not" ve tercih sebebi olur.

---

## 13. Dilekçe ve Resmi Evrak Üretici

**Müşavirin konumlandırması:** İleri faz. Temel ihtiyaçlar önce gelir.

### İstenen
- Verileri girince hazır çıkan dilekçe ve evraklar ("dilekçematik").
- Örnekler:
  - Vergi dairesi yazışmaları
  - Ticaret odası yazışmaları
  - Tür değişikliği evrakları

---

## 14. Luca'ya Bakış ve Geçiş Riskleri

### 14.1 Luca'da süreç nasıl işliyor
- Veriler hesap planına (600, 391 vb.) göre Luca'ya giriliyor, orada toplanıyor.
- Beyanname ekranı toplu rakamları veriyor. "KDV beyannamesi oluştur" denince oluşturup gönderiyor.
- **E-defter mükellefleri:** Kayıtlar aylık veya 3 aylık olarak e-defter sistemine gönderiliyor, berat çıkıyor.
- **İşletme defteri tutan küçük mükellefler:** Defter-Beyan sistemine Luca üzerinden gönderiliyor.
- Dışarı export alıp GİB portalına yükleme yok, her şey Luca içinde.

### 14.2 Kadir Bey'in görüşü
- **Tercihi:** Doğrudan ön muhasebe programından göndermek.
- **Çekincesi:** Luca çöktüğünde, çok kullanıldığı için maliye uzatma veriyor. Yeni bir programda bu güvence yok.
- **Luca'nın artısı:** Beyanname tarafında güncel. Örnek: il dışı mükelleflerin KDV'sini yeni e-beyan sistemine otomatik gönderebiliyor. Mikro ise hâlâ "BDP'den gönderin ya da manuel girin" diyor.
- **Luca'nın eksisi:** Bazı beyanname ekranları güncel değil. Telefon desteği fiilen bitmiş.

### 14.3 Kıdemli ortağın görüşü
- **Geçmiş:** 2003'ten beri ETA kullanırken format sırasında yedek kaybı yaşamışlar. Mükelleflerin üçte ikisinin verisi gitmiş. Luca'ya bulut güvenliği için geçmişler.
- **Bulutun değeri:** "Bilgisayar çalınsa da sorun yok, veriler bulutta."
- **Bugünkü şikâyet:**
  - Luca "nasıl olsa bana muhtaçlar" özgüveniyle kendini geliştirmiyor.
  - Destek almak zor.
  - Eksikleri kapatmak için MKLF, Rahat Aktarım gibi programlara ayrıca para ödüyorlar.
  - "İnsan alıştığı için iyi."
- **İsteği:** Her şeyi tek yerden yapmak.

### 14.4 Luca maliyeti
| Kalem | Tutar |
|---|---|
| Yıllık lisans | 12.000 TL |
| Her ek 10 mükellef | ~1.500–2.000 TL |

### 14.5 Geçiş zamanlaması
- Muhasebe ofislerinde yoğun dönem hiç bitmez. "Kör testereyi bilemeye vakti yok" durumu.
- Kendileri Luca'ya Nisan'da, en yoğun dönemde geçmişler.
- Geçiş süreci ofise ek yük getirmeyecek şekilde kurgulanmalı.

---

## 15. Müşavirin Uyarıları ve Ürün İlkeleri

Görüşmeden çıkan, ürün geliştirmede gözetilmesi gereken ilkeler:

1. **Çok güçlü girin.** Luca'nın işini üstlenecekseniz eksiksiz yapın. Başaramazsanız zarar büyük olur.
2. **Mevzuat güncellemesi otomatik olmalı.** Yeni düzenlemeler sisteme kendiliğinden yansımalı.
3. **Yorum farklılıklarında yön belirleyin.** Luca yoruma açık konularda devlet lehine yorum yapıyor ve bu kabul görüyor.
4. **Veri kalitesi > entegrasyon sayısı.** Yanlış veri aktaran entegrasyon kullanılmaz. Müşavir manuele döner.
5. **Mükellef kullanmazsa sistem çalışmaz.** Fiş okuma yanlış çıkarsa mükellef bir daha kullanmaz. Kullanım kolaylığı belirleyici.
6. **Tavsiye ettiğiniz ürünün arkasında durun.** Müşaviri mükellef karşısında mahcup edecek bir şey önermeyin (Vera örneği).
7. **Sorumluluk sınırlarını koruyun.** E-tebligat gibi alanlarda müşavir hukuki sorumluluk almak istemiyor. Ürün bunu mükellefe yönlendirerek çözmeli.

---

## 16. Açık İşler ve Sonraki Adımlar

| # | İş | Sorumlu | Durum |
|---|---|---|---|
| 1 | Banka onay linki örneği gönderilecek | Müşavir ofisi | Bekleniyor |
| 2 | e-Mükellef ekran görüntüleri ve/veya video gönderilecek | Müşavir ofisi | Bekleniyor |
| 3 | Luca yakıt fişi çekme sistemi incelenecek | Easytrade | Açık |
| 4 | Proje taslağı ve ilk ekranlar hazırlanacak | Easytrade | Açık |
| 5 | ~15 gün sonra demo için randevu talep edilecek | Easytrade | Açık |
| 6 | Ara dönemde soru sorulabilecek (müşavir onayladı) | Easytrade | Sürekli |

---

## 17. Özet Tablo

| # | Talep | Çözdüğü sorun | Müşavirin vurgusu |
|---|---|---|---|
| 1 | Hesap planı canlı senkronu | Manuel hesap seçimi | Temel beklenti |
| 2 | Kural ezberleme + otomatik cari | Tekrarlı kayıt işi | Temel beklenti |
| 3 | Kayıt yanında fatura görüntüsü | Program arası gezinme | Yüksek |
| 4 | Akıllı uyarılar (demirbaş, tutar, kayıt türü) | Gözden kaçan kalemler | Yüksek |
| 5 | Mükerrer fatura kontrolü | Çift kayıt | Orta |
| 6 | Z raporu otomatik aktarımı | Evrak toplama, tek tek işleme | Çok yüksek |
| 7 | Masraf fişi mobil girişi | Evrak toplama | Çok yüksek |
| 8 | Yakıt fişi otomatik çekme | Kayıp fişler | Yüksek |
| 9 | Plaka kontrolü | Geçersiz gider, faturasız araç işlemleri | Yüksek |
| 10 | Banka ekstre dönüştürme | PDF–Excel çevirme yükü | Yüksek |
| 11 | Banka onay linki + geniş banka ağı | Mükelleften ekstre alamama | Çok yüksek |
| 12 | KDV erken uyarı | Sürpriz ödeme, ilişki gerginliği | Yüksek |
| 13 | Tahsilat/tediye aktarımı + tür kontrolü | Tek tek işleme, bozuk mizan | Yüksek |
| 14 | Dijital VD + SGK entegrasyonu | Belge talebi trafiği | Orta |
| 15 | E-tebligat bildirimi | Kaçırılan tebligat riski | Yüksek |
| 16 | Müşavir mobil uygulaması | Mükellef taleplerine hızlı yanıt | Yüksek |
| 17 | AI finansal yorum ve sunum | Mükellefe yorum yapamama | Orta |
| 18 | Dilekçe/evrak üretici | Resmi yazışma yükü | Düşük (ileri faz) |

> **Vurgu sütunu hakkında:** Müşavirin kullandığı ifadelere göre belirlenmiştir ("çok büyük yük kaldırır", "en önemli sorun", "çok ciddi" → çok yüksek / yüksek; "ileriki boyutlar" → düşük). Easytrade'in iç önceliklendirmesi değildir.
