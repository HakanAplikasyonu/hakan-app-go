# Demo senaryosu

> Pilot ofise gösterim: tek komutla kurulan demo ofisinde bir mükellefin Ekim kapanışı, ekran ekran.

Bu senaryo demo ofisinde (**Demir Mali Müşavirlik (demo)**) bir
mükellefin, **Atlas Gıda Ticaret A.Ş.**'nin Ekim 2026 kapanışını anlatır.
Bütün kişiler, vergi numaraları ve tutarlar uydurmadır. Modüller aynı
rakamları gösterir: bordrodaki net maaş bankadan ödenen maaşa, beyannamedeki
KDV bankadan ödenen vergiye, mizan da Luca'ya aktarılan fişlere eşittir.
Gösterim yaklaşık 30 dakika sürer.

## Hazırlık

Depo kökünde (Docker çalışırken):

```bash
make demo
```

Komut altyapıyı başlatır, veritabanını **siler ve baştan kurar**, migration'ları
uygular, demo verisini ve platform yöneticisini oluşturur. Sonra uygulamayı iki
ayrı terminalde başlatın:

```bash
make backend-dev
make frontend-dev
```

Her şeyin yerinde olduğunu duman testiyle denetleyin (API çalışırken):

```bash
make smoke
```

Çıktının sonunda `0 kontrol düştü` yazmalıdır.

| Hesap | Rol | Nereye girer |
|---|---|---|
| `sahip@demo.hakan.local` | Ofis sahibi (SMMM) | Ofis paneli |
| `musavir@demo.hakan.local` | Müşavir (yönetici) | Ofis paneli, gösterimin ana hesabı |
| `stajyer@demo.hakan.local` | Stajyer (üye) | Ofis paneli, yalnızca atandığı mükellef |
| `mukellef@demo.hakan.local` | Atlas Gıda yetkilisi | Mükellef paneli (`/m`) |
| `admin@example.com` | Platform yöneticisi | `/platform/login` |

Demo hesaplarının parolası `Demo1234!`, platform yöneticisininki `Password1`.
Başka parola için `make demo` öncesinde `DEMO_PASSWORD`, `SA_EMAIL` ve
`SA_PASSWORD` verin. Giriş adresi `http://localhost:3000/login`.

## Hikâye

- **Eylül kapandı.** Eylül bordrosu, KDV1 ve MUHSGK beyanları verildi.
  Eylül mizanı yüklü.
- **Ekim'de bu borçlar ödendi.** Banka ekstresinde Eylül maaşları
  (124.741,51 TL), SGK primi (50.701,79 TL, 30 Ekim), KDV (10.150,00 TL) ve
  MUHSGK (7.863,16 TL) var.
- **Ekim'in işi sürüyor.** Masraflar ve Z raporları yüklendi, bir kısmı
  onaylandı. Ekim mizanı Luca'dan geldi, mutabakat yapıldı. Ekim KDV1'i taslak.

## Adım adım

### 1. Pano ve sıradaki iş

`musavir@demo.hakan.local` ile girin. Ofis paneli **Pano** ile açılır. Üst
çubuktaki **Dönem** seçicisinden **Ekim 2026**'yı seçin.

- **Sıradaki iş:** her mükellefin kapanıştaki sıradaki adımı görünür. Atlas
  Gıda'da banka kuyruğunda bekleyen bir hareket ve onay bekleyen fişler
  vardır.
- **Mükellefler** listesinde iki mükellef görünür: Atlas Gıda ve Selin Yılmaz.

### 2. Personel ve bordro

**Mükellefler → Atlas Gıda → Personel ve bordro**.

- Eylül ve Ekim bordroları dört personelle yüklü. Net ücretler toplamı
  124.741,51 TL.
- Eylül SGK primi **ödendi** (30.10.2026, 50.701,79 TL). Ekim primi tahakkuk
  etti.
- İki ayın tahakkuk fişi **Aktarıldı** durumunda.
- Kasım'da işe giren Mert Yıldız elle eklendi, henüz bordroda yok.
- **Excel'e aktar** ile personel ve bordro listeleri indirilir.

### 3. Banka

**Banka** sekmesi Ziraat TL hesabının Ekim ekstresini gösterir (9 hareket).

- Maaş, SGK, KDV ve MUHSGK ödemeleri bordro ve beyanname ekranındaki
  tutarlarla aynıdır.
- X Yazılım Teknolojileri'nin tahsilatı tanımlı **takma ad** ile kendiliğinden
  Oğuz Yılmaz carisine eşleşti.
- **Deniz Yıldırım** havalesi (4.800,00 TL) kuyrukta bekler. Gösterimde
  cariyi seçip **Onayla**'ya, sonra **Fiş taslağı oluştur**'a basın. Fiş
  **Yevmiye ve çıktı** ekranında taslak olarak belirir.

### 4. Masraflar ve Z raporları

- **Masraflar:** Ekim fişleri Excel'den yüklendi. Kategoriler satıcı adından
  seçildi (akaryakıt, kargo, temsil …). İlk günlerin masrafları onaylandı ve
  fişleri Luca'ya aktarıldı. Kalanlar onay bekler.
- **Z raporları:** ÖKC'nin Ekim Z raporları. 0105 numaralı Z eksik, 11–12
  Ekim'de rapor yok. İki süreklilik uyarısı **Uyarılar** ekranında açıktır.
  İlk üç Z onaylandı, satış fişi taslakları henüz Luca'ya aktarılmadı.
- Ön muhasebe programı sekmesi aynı mükellefin bağlantısını, senkron
  geçmişini, finansal işlemlerini ve Z raporlarını gösterir. Bu bağlantı
  demodadır ve dışarıya istek atmaz.

### 5. Yevmiye ve çıktı

Kenar menüden **Yevmiye ve çıktı**.

- Ekim fişleri: aktarılanlar (banka, bordro, masraf, kira, akaryakıt) ve
  bekleyen Z taslakları.
- **Önceki çıktılar:** Luca ve Zirve çıktıları.
- **Programa aktarım:**
  - Luca çıktısı masaüstü ajanıyla **Programa aktarıldı**.
  - Zirve çıktısı **Hata verdi**: aktarım klasörü bulunamadı. Hata ekranını
    ve **Yeniden dene**'yi gösterin.
- Z taslaklarını seçip **Onayla** ve **Çıktı al** (Luca) ile aktarın.
  Mutabakat bir sonraki adımda bu yüzden değişir.

### 6. Mizan ve mutabakat

- Eylül ve Ekim 2026 Luca mizanları yüklü, ikisi de dengeli. Ekim mizanı Eylül
  mizanına Luca'ya aktarılan Ekim fişleri eklenerek oluşur.
- **Mutabakat** sekmesinde Ekim karşılaştırmasını açın:
  - Banka, personel, vergi, SGK ve gider hesapları **eşleşti**.
  - Kasa, diğer hazır değerler, hesaplanan KDV ve satışlar (100, 108, 391,
    600) farkı aktarılmamış Z taslaklarının tutarına eşittir. Öneri:
    "taslakları onaylayıp dışa aktarın". 600 inceleme kuyruğundadır.
  - Alıcılar (120) farkı 1.200,00 TL'dir. Bu fark Luca'da elle girilmiş bir kur
    farkı fişinden gelir ve notla **açıklandı**.
- Mizan Luca'nın gördüğünü gösterir. Adım 5'te Z taslaklarını aktarsanız da
  farklar, Luca'dan yeni mizan alınıp yüklenene kadar kalır. Gerçek ofiste
  bu adım Luca içe aktarımından sonra mizanı yeniden yükleyip
  **Karşılaştır**'a basmaktır.
- **Cari mutabakat:** en büyük cari farkına mektup taslağı hazırdır. Bir
  cariye gönderilen mektup **mutabık** yanıtlanmıştır. Form BA/BS satırlarında
  bir tutar farkı bilerek bırakıldı.

### 7. Beyannameler

**Beyannameler** sekmesi:

- **Eylül KDV1** verildi. Ödenecek 10.150,00 TL, tahakkuk fiş numarası
  `KDV1-2026-09-4471`.
- **Eylül MUHSGK** verildi. Gelir ve damga vergisi 7.863,16 TL.
- **Eylül Ba/Bs** hazırlandı.
- **Ekim KDV1** taslak. Devreden KDV Eylül beyanından gelir. Kontrol
  listesindeki uyarıları onaylayıp **Hazırla**'ya basın.

### 8. UBL yükleme (isteğe bağlı)

Faturalar demoya bilerek yüklenmez; yükleme akışı canlı gösterilir. Depodaki
`backend/internal/demo/ubl/` klasöründen `atlas-` ile başlayan XML dosyalarını
**UBL yükle** ekranında Atlas Gıda'ya yükleyin.

- **Yükleme raporu** eklenen faturaları gösterir.
- **İnceleme kuyruğu**nda hesap kodu önerileri belirir.
- **Uyarılar**'da kontrollerin bulduğu sorunlar listelenir.
- Mutabakatta **Karşılaştır**'a yeniden basın: faturalar aktarılmamış kayıt
  olarak görünür.

### 9. Rapor, belgeler, WhatsApp ve sıradaki iş

- **Rapor** sekmesi Ekim 2026 özetini gösterir. Rapor mükellefe gönderilebilir.
- **Belgeler:** belge arşivi ve İSG evrakları. Birinin süresi dolmuş, biri
  30 gün içinde bitiyor.
- **WhatsApp:** hatırlanan mükellef sohbeti ve uzantıdan kaydedilmiş iki
  belge.
- **Sıradaki iş** ve **Co-pilot:** sahip hesabının komut geçmişi.

### 10. Yetki: stajyer

Çıkış yapıp `stajyer@demo.hakan.local` ile girin.

- Listede yalnızca atandığı **Selin Yılmaz** görünür.
- Atlas Gıda'nın sayfalarına erişemez.

### 11. Mükellef paneli

Çıkış yapıp `mukellef@demo.hakan.local` ile girin. Mükellef paneli (`/m`)
açılır.

- **Bordro:** Eylül ve Ekim bordro dökümleri ve SGK tahakkuk fişleri (dört
  PDF).
- **Rapor:** ofisin hazırladığı Ekim özeti.
- **Faturalar:** adım 8'de yüklenen faturalar.
- **Fişler** ve **Belge yükle:** mükellefin ofise belge gönderdiği ekranlar.

### 12. Platform yöneticisi

`/platform/login` adresinden `admin@example.com` ile girin. **Organizasyonlar**
altında demo ofisi görünür. Yeni ofis açma akışı buradan gösterilir.

## Sıfırlama

Gösterimden sonra veriyi ilk hâline döndürmek için:

```bash
make demo ARGS=--no-infra
```

Bu komut altyapıyı yeniden başlatmadan veritabanını sıfırlar ve demoyu
yeniden kurar. Yalnızca demo ofisini yeniden kurmak (başka verilere
dokunmadan) için: `make demo-seed ARGS=-reset`.

## Demoda gerçek bağlantı yok

Ön muhasebe programı, destek penceresi, banka bağlantısı ve Luca / Zirve
aktarımı demoda sahte ya da örnek veriyle çalışır. Hiçbiri dışarıya istek
atmaz. Ayrıntı ve gerçek bağlantıya geçiş:
`docs/runbook/demo-dis-bagimliliklar.md`.
