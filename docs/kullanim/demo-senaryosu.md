---
roles: []
internal: true
---

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
  **Fişler ve aktarım** ekranında taslak olarak belirir.

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

### 5. Fişler ve aktarım

Kenar menüden **Fişler ve aktarım**.

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
**e-Fatura yükle** ekranında Atlas Gıda'ya yükleyin.

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

## Büyük demo (sunum)

Küçük demo tek mükellefin bir ayını anlatır. Sunumda ise portalın dolu ve
geçmişe dönük hâlini göstermek gerekir. Büyük demo, küçük demonun (Atlas,
Selin ve yukarıdaki bütün adımlar) **üstüne** şunları kurar:

- **3 ofis:** Demir Mali Müşavirlik (`/t/demo-smmm`, İstanbul), Başkent Mali
  Müşavirlik (`/t/demo-baskent`, Ankara) ve Ege Mali Müşavirlik
  (`/t/demo-ege`, İzmir).
- **Ofis başına 30 mükellef:** şahıs, limited ve anonim; perakende, inşaat,
  hizmet, e-ticaret ve restoran. Bir kısmı son iki yılda ofise katılmıştır.
- **24 ay ve içinde bulunulan ay:** e-Fatura / e-Arşiv, tevkifat, ihracat
  istisnası (döviz), iade ve iptal; Z raporları; masraf fişleri; banka
  ekstresi ve fatura eşleşmeleri; bordro ve SGK; KDV1, MUHSGK ve Ba/Bs;
  aylık yevmiye fişleri ve Luca aktarım geçmişi; aylık mizanlar; mutabakat ve
  cari mektupları; görevler, inceleme kuyruğu, uyarılar, bildirimler,
  WhatsApp kayıtları, rapor e-postaları ve denetim kaydı.

Tutarlar mevsimsellik ve büyüme eğilimi taşır, birbirleriyle de tutarlıdır.
Bankadan ödenen KDV verilen beyannameye, SGK ödemesi tahakkuka, mizan da
Luca'ya aktarılan fişlere eşittir. Bütün kişiler, unvanlar, VKN / TCKN'ler
(sağlamalı ama `93…` / `1000…` test aralığında), adresler, e-postalar
(`example.com`, `.test`) ve telefonlar (`0555 000 xx xx`) uydurmadır.

### Kurulum

```bash
make demo-big                    # make demo ile aynı akış, seed -scale=big
make demo-big ARGS=--no-infra    # altyapı zaten çalışıyorsa
```

Komut veritabanını **siler ve baştan kurar**. Boş bir veritabanında seed
yaklaşık 2–3 dakika sürer ve süre çıktıda yazılır. Sonra `make backend-dev`
ile `make frontend-dev` çalıştırın. Mevcut bir veritabanında yalnızca demo
ofislerini yeniden kurmak için: `make demo-seed ARGS="-reset -scale=big"`.

Veri **bugünden geriye** üretilir; içinde bulunulan ay her zaman "güncel
ay"dır. Üretim deterministiktir: aynı gün aynı veriyi verir. Belirli bir
günü sabitlemek için (ör. prova ile sunumun aynı görünmesi):
`DEMO_BIG_NOW=2026-11-18 make demo-big`.

### Hesaplar

Parolalar küçük demoyla aynıdır: `Demo1234!` (ya da `DEMO_PASSWORD`).
Platform yöneticisi `admin@example.com` / `Password1`.

| Ofis | Sahip | Müşavir | Personel | Mükellef portalı (`/m`) |
|---|---|---|---|---|
| `/t/demo-smmm` | `sahip@demo.hakan.local` | `musavir@demo.hakan.local` | `stajyer@demo.hakan.local` | `mukellef2@demo.hakan.local` (Anadolu Market Sanayi ve Ticaret A.Ş.) |
| `/t/demo-baskent` | `sahip@baskent.demo.test` | `musavir@baskent.demo.test` | `personel@baskent.demo.test` | `mukellef@baskent.demo.test` (Ergin Gıda Ticaret Ltd. Şti.) |
| `/t/demo-ege` | `sahip@ege.demo.test` | `musavir@ege.demo.test` | `personel@ege.demo.test` | `mukellef@ege.demo.test` (Marmara Kuruyemiş Ticaret Ltd. Şti.) |

Küçük demonun `mukellef@demo.hakan.local` hesabı (Atlas Gıda) olduğu gibi
kalır. Sahip bütün mükellefleri görür. Müşavir ve personel yalnızca
atandıkları mükellefleri görür; aradaki fark yetki gösterimi için
kullanılabilir.

### Durum (bugüne göre)

- **Geçmiş aylar kapandı:** beyanlar verildi, dönemler kilitli, fişler
  Luca'ya aktarıldı, mizanlar yüklü.
- **Geçen ay** (KDV son günü ayın 28'i): beyanlar çoğu mükellefte hazırlandı,
  bir kısmında taslakta. Ay sonu mutabakatı yapıldı; her mükellefte cari
  mektuplarından biri onaylandı, biri itirazlı, biri yanıt bekliyor.
- **Bu ay:** faturalar ve banka hareketleri bugüne kadar geliyor. Fiş ve Z
  raporlarının bir kısmı onay bekliyor, fişler taslakta.
- **Dikkat isteyen mükellefler:** her ofiste beş mükellef var. Bunlarda bir
  önceki ayın KDV beyanı gecikmiş, banka bakiyesi engelleyicisi açık,
  son haftalarda belgesiz tahsilat ve ödeme var, Luca'da elle girilmiş bir
  fiş mutabakatta 120'de fark bırakıyor, bir Luca aktarımı da başarısız.
  - Demir: Asil Organizasyon, Burcu Şahin (e-ticaret), Akdeniz Pide Salonu,
    Ilgaz Kuruyemiş, Güneş Yapı.
  - Başkent: Pera Temizlik Hizmetleri, Cem Kurt, Toros Lokanta, Umut
    Kuruyemiş, Akdeniz Taahhüt.
  - Ege: Çınar Organizasyon, Doruk Ev Tekstili, Murat Yıldırım, Sibel
    Öztürk, Murat Arslan.

### Önerilen sunum akışı (yaklaşık 20 dakika)

1. **Pano** — `sahip@baskent.demo.test` ile girin, `/t/demo-baskent`. Dönem
   matrisinde kilitli geçmiş aylar ve geride kalan mükellefler, inceleme
   kuyruğu, açık ve gecikmiş belge görevleri, engelleyiciler, son yüklemeler
   ve KDV tahmini görünür.
2. **Mükellef listesi** — "uyarısı olanlar" ve "görevi olanlar" süzgeçlerini
   açın. Dikkat isteyen beş mükellef (aşağıda) engelleyicileriyle görünür.
3. **Dikkat isteyen mükellef: Toros Lokanta Sanayi ve Ticaret A.Ş.**
   (restoran). Uyarılar sayfasında belgesiz banka hareketleri ve dönem
   engelleyicisi var. **Banka** sekmesinde eşleşmeyen hareketlerle bu ayın
   onay bekleyen hesap önerileri yer alır. **Beyannameler** sekmesinde geçen
   ay başlanmamış, bir önceki ayın KDV1'i hazırlanmış ama verilmemiş
   (gecikmiş). **Mutabakat** sekmesinde 120 farkı açıklamalı duruyor;
   onaylanmamış Z fişleri 100 / 108 / 391 / 600'de "aktarılmamış taslak"
   farkı bırakıyor. Cari mektuplarından biri onaylı, biri itirazlı (bakiye ve
   notla), biri gönderilmiş ve yanıt bekliyor.
4. **Sağlıklı mükellef: Nehir Taahhüt Ticaret Ltd. Şti.** (inşaat).
   Faturalarda tevkifatlı (601, 4/10) satışlar var. Beyannamelerde 24 aylık
   KDV1 / MUHSGK geçmişi, verilme tarihleri ve tahakkuk numaraları görünür.
   **Bordro** sekmesinde aylık bordrolar, işe giriş ve çıkışlar, ödenmiş SGK
   primleri yer alır. **Yevmiye** sekmesinde aylık fişler ve Luca aktarım
   geçmişi, **Rapor** sekmesinde iki yılın gelir–gider grafiği.
5. **E-ticaret:** Başkent'te `Vadi Online Sanayi ve Ticaret A.Ş.`. Yüzlerce
   e-Arşiv satış, USD / EUR ihracat faturaları (istisna 301), Kasım ve
   Aralık'ta sezon artışı.
6. **Perakende:** `Ergin Gıda Ticaret Ltd. Şti.`. Günlük Z raporları (oran
   kırılımlı), haftalık POS ve nakit yatırma hareketleri. Mükellef panelinde
   `mukellef@baskent.demo.test` ile aynı mükellef görünür.
7. **Ofisler arası ayrım:** `sahip@ege.demo.test` ile girin. Yalnızca Ege
   ofisinin mükellefleri görünür (kiracı izolasyonu). Platform yöneticisi
   üç ofisi birden görür.
8. **Yetki:** `personel@baskent.demo.test` ile girin. Yalnızca atandığı
   mükellefler listelenir.

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
