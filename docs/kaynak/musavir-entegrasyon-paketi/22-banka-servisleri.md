# 22 · Banka Servisleri (banka banka)

Her bankanın hesap hareketi servisinin teknik detayı ve `21` §2 `bank_transaction` alanlarına eşlemesi. Her bölüm aynı düzende:

**Durum · Uç nokta · Protokol ve kimlik · Operasyonlar · İstek · Yanıt → eşleme · Tekil anahtar · Biçimler · Limitler · Hata kodları → neden kodu · Başvuru · Doğrulanacaklar · Kaynak**

Doğrulama seviyesi:

| İşaret | Anlamı |
|---|---|
| ✅ | Bankanın kendi teknik dokümanından. Dokümanlar bankaların sitesinde herkese açık değil; muhasebe yazılımı firması Netsim'in yayımladığı kopyalardan okundu. Bazıları eski tarihli (özet tablodaki tarih sütunu). |
| ⚠️ | Topluluk kodu (çalışan açık kaynak entegrasyon) veya muhasebe programı kılavuzundan. Alan adları eksik veya doğrulanmamış. |
| ❌ | Teknik bilgi bulunamadı. Bankadan doküman istenecek, o zamana kadar bu bankada sadece ekstre yükleme var. |

**Hiçbir servis Easytrade tarafından canlı hesapla test edilmedi.** ✅ olan bankalarda da kod, bankanın o gün verdiği WSDL ile karşılaştırılır ve `20` §9 kabul testleri geçmeden üretime alınmaz.

Genel kurallar (bütün bankalar):

- Bankanın verdiği **WSDL esastır.** Bu dosyadaki alan adları WSDL ile çelişirse WSDL kazanır ve bu dosya güncellenir.
- Adaptörler SOAP için hazır istemci üretimi (WSDL → kod) kullanabilir. Elle XML yazılacaksa aşağıdaki örnek zarflar kullanılır.
- Bankaların test ortamı varsa önce orada çalışılır (`20` §4 test IP'si).
- Neden kodları `20` §5.4'te: `ip_rejected`, `unauthorized`, `bad_credentials`, `instruction_missing`, `account_not_defined`, `bank_error`, `no_data` (hata değil), `rate_limited`, `invalid_request`.

## Özet tablo

| § | Banka | Durum | Doküman sürümü / tarihi | Operasyon (hareket) | Tekil anahtar | Aralık / geçmiş |
|---|---|---|---|---|---|---|
| 1 | Vakıfbank | ✅ | v1.20 · 01.07.2019 | `GetirHareket` | `IslemNo` (dokümanda işlem no; dekont servisi bununla çağrılıyor) | geçmiş 365 gün |
| 2 | Ziraat Bankası | ✅ | tarih yok | `SorgulaHesapHareketZamanIle` (önerilen), `HesapEkstre` | `timeStamp` (örnek yanıtta hareket başına farklı; tekilliği dokümanda yazmıyor ⚠️) | eski tarih sınırı var (`111`), gün sayısı yok |
| 3 | Halkbank | ✅ | tarih yok | `EkstreSorgulama` | Dokümanda tekil alan tanımı yok. Önerilen: `Tarih` + `SiraNo` + `DekontNo` ⚠️ | dokümanda yok |
| 4 | Garanti BBVA | ✅ A · ⚠️ B | tarih yok | `FirmAccountActivity` | `TransactionReferenceId` (dokümanda: işlem tablosundaki instance id) | 1 gün / sorgu, 10 dk aralık |
| 5 | Akbank | ✅ | tarih yok | `GetExtreWithParams` | Alan adı WSDL'den alınacak ⚠️ | geçmiş 2 ay |
| 6 | TEB | ✅ | v1.8 · 16.05.2016 | Hesap Hareketleri Bildirim Servisi | `HAREKET_KEY` (dokümanda: hareket birincil anahtarı) | 30 gün / sorgu |
| 7 | Fibabanka | ✅ | v1.8 · 23.02.2021 | `GetStatementInfo` | `referenceNumber` (dokümanda: tekil işlem referansı) | dokümanda yok |
| 8 | Türkiye Finans | ✅ | tarih yok | `GetCustomStatementExtended` | Dokümanda tekil alan tanımı yok. Önerilen: `ReferenceNumber` + `SlipId` ⚠️ | dokümanda yok |
| 9 | Ziraat Katılım | ✅ | tarih yok | `accountService` (REST) | Dokümanda tekil alan tanımı yok. Önerilen: `ReferansNo` + `SiraNo` ⚠️ | 7 gün / sorgu |
| 10 | Vakıf Katılım | ✅ | v1.3 · 02.10.2019 | `GetCustomerTransactionDetails` | `BusinessKey` (dokümanda: tekil işlem referansı) | 31 gün / sorgu |
| 11 | Kuveyt Türk | ⚠️ | — | `GetAccountStatement` | bilinmiyor | bilinmiyor |
| 12 | Yapı Kredi | ⚠️ | — | `sorgula` | bilinmiyor | bilinmiyor |
| 13 | İş Bankası | ⚠️ | — | bilinmiyor | bilinmiyor | bilinmiyor |
| 14 | QNB / Enpara.com | ⚠️ | — | bilinmiyor (WSDL'i banka verir) | bilinmiyor | bilinmiyor |
| 15 | ING, Albaraka, DenizBank | ❌ | — | — | — | — |

Tekil anahtarı dokümanda tanımlı olmayan bankalarda, ilk bağlantıda aynı gün iki kez çekilip anahtarın çakışmadığı kontrol edilir; çakışırsa `20` §7.3'teki hash kullanılır.

---

## 1. Vakıfbank ✅

| | |
|---|---|
| Kaynak doküman | "Online Hesap Hareketleri Web Servis Dokümanı" Sürüm 1.20, 01.07.2019 |
| Test | `https://vbtestservice.vakifbank.com.tr/HesapHareketleri.OnlineEkstre/SOnlineEkstreServis.svc` (WSDL: `?wsdl`) |
| Üretim | Testler bitince banka iletir. Topluluk kodlarında kullanılan adres: `https://vbservice.vakifbank.com.tr/HesapHareketleri.OnlineEkstre/SOnlineEkstreServis.svc` ⚠️ |
| Protokol | SOAP 1.2, WCF. **WS-Addressing zorunlu** (`wsa:Action`, `wsa:To`). Eksikse güvenlik duvarı "The requested URL was rejected" döner. |
| Kimlik | Gövdede `MusteriNo`, `KurumKullanici`, `Sifre` + sabit IP |
| Mükellefin girdiği | Müşteri no, kurum kullanıcı adı, şifre |
| Destek | nakitpazarlama@vakifbank.com.tr |

**Operasyonlar**

| Operasyon | İş |
|---|---|
| `GetirHareket` | Hesaplar + hareketler (detaylı). **Ana operasyon.** |
| `GetirHareketOzet` | Gün sonu özetleri → `bank_daily_summary` |
| `GetirDekont` | Bir işlemin dekont PDF'i (base64 liste) → `attachment` |
| `GetirYatirimHareket` | Yatırım hesabı hareketleri (kapsam dışı, ileride) |

**İstek** (`DtoEkstreSorgu`)

| Alan | Zorunlu | Biçim |
|---|---|---|
| `MusteriNo` | ✓ | Varchar2(12) |
| `KurumKullanici` | ✓ | Varchar2(50) |
| `Sifre` | ✓ | Varchar2(8) |
| `SorguBaslangicTarihi` | ✓ | `yyyy-MM-dd HH:mm` (Ozet'te `yyyy-MM-dd HH:mm:ss`) |
| `SorguBitisTarihi` | ✓ | aynı |
| `HesapNo` | | Varchar2(17), `00158` ile başlar. Boşsa tüm hesaplar. |
| `HareketTipi` | | `B` / `A` |
| `EnDusukTutar`, `EnYuksekTutar` | | Number(26,2) |

Örnek zarf (topluluk kodundan, namespace'ler WSDL ile teyit edilecek):

```xml
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
  xmlns:peak="Peak.Integration.ExternalInbound.Ekstre"
  xmlns:dto="http://schemas.datacontract.org/2004/07/Peak.Integration.ExternalInbound.Ekstre.DataTransferObjects">
  <soap:Header xmlns:wsa="http://www.w3.org/2005/08/addressing">
    <wsa:Action>Peak.Integration.ExternalInbound.Ekstre/ISOnlineEkstreServis/GetirHareket</wsa:Action>
    <wsa:To>{SERVIS_URL}</wsa:To>
  </soap:Header>
  <soap:Body>
    <peak:GetirHareket><peak:sorgu>
      <dto:MusteriNo>{musteri_no}</dto:MusteriNo>
      <dto:KurumKullanici>{kullanici}</dto:KurumKullanici>
      <dto:Sifre>{sifre}</dto:Sifre>
      <dto:SorguBaslangicTarihi>2026-11-01 00:00</dto:SorguBaslangicTarihi>
      <dto:SorguBitisTarihi>2026-11-01 23:59</dto:SorguBitisTarihi>
      <dto:HesapNo>{opsiyonel}</dto:HesapNo>
    </peak:sorgu></peak:GetirHareket>
  </soap:Body>
</soap:Envelope>
```

`Content-Type: application/soap+xml`

**Yanıt → eşleme**

Üst: `BankaKodu`, `BankaAdi`, `IslemKodu`, `IslemAciklamasi`, `Hesaplar[]` (`DtoEkstreHesap`).

Hesap (`DtoEkstreHesap`) → `cash_box` / `bank_balance_snapshot`:

| Vakıfbank | Portal |
|---|---|
| `HesapNo` | `cash_box.bank_account_number` |
| `HesapNoIban` | `cash_box.iban` |
| `DovizTipi` | `currency` |
| `SubeKodu`, `SubeAdi` | `bank_branch_code` |
| `MusteriNo`, `MusteriUnvani`, `VergiKimlikNumarasi` | Bağlantı doğrulama (VKN = `company.tax_number` olmalı) |
| `HesapRumuzu` | `cash_box.name` önerisi |
| `HesapTipi` | `1` → `demand`, `2` → `term` |
| `HesapDurumu` | `true` açık / `false` kapalı → `is_active` |
| `AcilisBakiye` | Sorgu başlangıcındaki bakiye (süreklilik kontrolü) |
| `CariBakiye` | `balance` (sorgu anı) |
| `KullanilabilirBakiye` | `available_balance` |
| `KrediliBankomatLimiti`, `KrediBakiyesi` | `credit_limit`, KMH bakiyesi |
| Vadeli alanlar (`VadeFaizOrani`, `VadeSonuOdenecekBrutFaizTutari`, `Stopaj` …) | `raw_payload` (vadeli hesap faiz/stopaj kontrolü için) |

Hareket (`DtoEkstreHareket`) → `bank_transaction`:

| Vakıfbank | Portal |
|---|---|
| `IslemNo` | `source_id` |
| `IslemTarihi` (`yyyy-MM-dd HH:mm:ss`) | `booked_at`, `transaction_date` |
| `Tutar` | `amount` (mutlak değer) |
| `BorcAlacak` | `debit_credit` |
| `Aciklama` | `description` |
| `IslemAdi` | `description_extra` |
| `IslemOncesiBakiye` / `IslemSonrasıBakiye` | `balance_before` / `balance_after` (alan adında `ı` harfi var, WSDL'deki yazımla birebir alınmalı) |
| `IslemKodu` | `bank_txn_code` |
| `IslemKanal`, `IslemYeri` | `channel` |
| `Detaylar` (anahtar–değer listesi) | aşağıda |

`Detaylar` anahtarları:

| Anahtar | Portal |
|---|---|
| `Id` | `bank_reference` |
| `IslemTarihZamani` | `booked_at` (daha hassas, salise dahil) |
| `GonderenAdi` / `AliciAdi` | `counterparty_name` (yön `A` ise gönderen, `B` ise alıcı) |
| `GonderenKimlikNumarasi` / `AliciKimlikNumarasi` | `counterparty_tax_number` / `counterparty_identity_number` (uzunluğa göre) |
| `GonderenIbanKumarasi` / `AliciIbanKumarasi` | `counterparty_iban` (dokümandaki yazım "Kumarasi") |
| `GonderenBankaKodu` / `AliciBankaKodu` | `counterparty_bank_code` |
| `EftSorguNumarasi` | `eft_query_number` |
| `EftIslemTuru` | `payment_purpose` (`M01`–`M09`, `K14`; `21` §3.3) |
| `HavaleReferansNo` | `bank_reference` (Id yoksa) |
| `SenetNo`, `SenetBorclu`, `SenetVade`, `SenetBorcluKimlik` | `raw_payload` + SENET eşleştirmesi |
| `DovizKuru` | `raw_payload` (kur farkı) |
| `TosAliciVergiNo`, `TosAliciTcKimlikNo`, `TosAliciAdSoyad` | Toplu ödeme alıcısı → `counterparty_*` |

**Limitler:** Geçmiş en fazla 365 gün (`VBH0010`). Günlük sorgu limiti var, değeri dokümanda yok (`VBH0014`). Büyük yanıtta istemcinin mesaj boyutu limitini artır (`maxReceivedMessageSize`).

**Hata kodları**

| Kod | Anlam | Neden kodu |
|---|---|---|
| `VBB0001` | Başarılı | — |
| `VBH0002` | İşlem gerçekleştirilemedi | `bank_error` |
| `VBH0003`, `VBH0004` | Kullanıcı adı / şifre boş | `invalid_request` |
| `VBH0005` | Müşteri no hatalı | `bad_credentials` |
| `VBH0006`, `VBH0007`, `VBH0008` | Tarih / tutar / hareket tipi parametresi hatalı | `invalid_request` |
| `VBH0009` | IP adresi tanımlı değil | `ip_rejected` |
| `VBH0010` | 365 günden eski hareket sorgulanamaz | `invalid_request` (pencere kısaltılır) |
| `VBH0011` | Talimat bulunamadı | `instruction_missing` |
| `VBH0012` | Kullanıcı adı veya şifre hatalı | `bad_credentials` |
| `VBH0013` | Talep bulunamadı | `instruction_missing` |
| `VBH0014` | Günlük sorgu limiti aşıldı | `rate_limited` (ertesi güne) |
| `VBD0000`–`VBD0005` | Dekont servisi (başarılı, parametre, talimat, IP, dekont yok) | aynı mantık |

**Başvuru:** Sabit IP bankaya bildirilir → test ortamında denenir → banka üretim adresi, kullanıcı ve şifreyi iletir.

**Doğrulanacaklar:** üretim adresi · günlük sorgu limiti değeri · `IslemNo`'nun hesap bazında mı müşteri bazında mı tekil olduğu.

---

## 2. Ziraat Bankası ✅

| | |
|---|---|
| Kaynak doküman | "Hesap Hareketleri Sorgulama Webservisi" (Ziraat) + Ziraat başvuru formu |
| Test WSDL | `https://odemelertest.ziraatbank.com.tr/HEK_NKYWS/HesapHareketleri.asmx?wsdl` |
| Üretim WSDL | `https://hesap.ziraatbank.com.tr/HEK_NKYWS/HesapHareketleri.asmx?wsdl` |
| Protokol | ASMX (SOAP; HTTP POST form da kabul ediyor, topluluk kodu `.../HesapHareketleri.asmx/HesapEkstre` adresine form POST ediyor) |
| Kimlik | `KurumKod` + `Sifre` her istekte + sabit IP |
| Mükellefin girdiği | Kurum kodu, şifre. Hesap için: IBAN, ya da müşteri no + ek no |
| Zaman aşımı | Dokümanda 1000 sn |

**Operasyon seçimi**

| Operasyon | Giriş | Neden |
|---|---|---|
| **`SorgulaHesapHareketZamanIle`** (önerilen) | `MusteriNo` (8), `EkNo` (4), `baslangicZamanı`, `bitisZamanı`, `KurumKod`, `Sifre`, `iptalFisGetir` (`E`/`H`) | En zengin yanıt: karşı taraf TCKN/VKN, ad-unvan, IBAN, dekont no, iptal durumu. Saat aralığıyla gün içi çekim. |
| `SorgulaHesapHareket` | Aynı, tarih bazlı (`DDMMYYYY`) | Günlük çekim alternatifi |
| `HesapEkstre` / `HesapEkstreZamanIle` | `KurumKod`, `Sifre`, `HesapNo` (en fazla 12), `BaslangicTarihi` / `BitisTarihi` (`YYYY.MM.DD`) veya zaman | Müşteri no + ek no bilinmiyorsa. Karşı taraf alanları yok. |
| `SorgulaOzetWS` | `ibanNo`, tarih (`DDMMYYYY`) | `bank_daily_summary` |
| `GetirPdfDekont` / `...DekontNoIle` / `...ZamanIle` | müşteri no, ek no, tarih/dekont no, sayfa | Dekont PDF (`dosyaFormati` = `BASE64`) |

**Yanıt → eşleme** (`SorgulaHesapHareketZamanIleCevap`)

| Ziraat | Portal |
|---|---|
| `hesapNo`, `iban`, `dovizTipi`, `subeKodu` | `cash_box` eşleşmesi |
| `AcilisBakiye`, `CariBakiye`, `Bakiye`, `BlokeliBakiye` | `bank_balance_snapshot` |
| `timeStamp` | `source_id` |
| `islemTarihi` / `muhasebetarihi` | `booked_at` / `transaction_date` |
| `valorTarihi` | `value_date` |
| `tutar` | `amount` |
| `borcAlacak` | `debit_credit` |
| `aciklama` | `description` |
| `islemAciklama` | `description_extra` |
| `islemTipi` | `bank_txn_code` (tablo `21` §4.1) |
| `tcknVkn` | `counterparty_tax_number` / `counterparty_identity_number` |
| `adUnvan` | `counterparty_name` |
| `iban` (hareket satırında) | `counterparty_iban` ⚠️ (hesabın kendi IBAN'ı mı karşı taraf mı, ilk bağlantıda kontrol) |
| `dekontNo` | `receipt_number` |
| `muhref` | `bank_reference` |
| `programKod` | `raw_payload` |
| `kayitDurumu` | `I` → `is_cancelled=true`, `A` → `false` |

`HesapEkstre` yanıtında alan adları farklı (`IslemTarihi` `yyMMdd`, `Valor`, `Referans`, `Aciklama`, `Tutar`, `BorcAlacak`, `IslemTipi`, `TimeStamp`, `musteriVergiNumarasi`, `Bakiye`). Topluluk örneğinde `TimeStamp` = `202012211572FNCC636` biçiminde ve hareket başına tekil.

**Limitler:** Eski tarih sınırı var (`111`), gün sayısı dokümanda yok. Topluluk kodunda "en fazla 21 gün geriye" notu var ⚠️. Başlangıç ve bitiş aynı olamaz (`ZamanIle` operasyonlarında hata `7` / `10`).

**Hata kodları**

| Operasyon ailesi | Kod → neden |
|---|---|
| `HesapEkstre` | `00` başarılı · `01` hareket yok → `no_data` · `02` diğer → `bank_error` · `101` yetkisiz kullanım → `unauthorized` (IP tanımı veya yetki; dokümanda ayrım yok) · `102` kullanıcı/şifre → `bad_credentials` · `103` IBAN hatalı → `account_not_defined` · `104`/`105` tarih boş → `invalid_request` · `111` izin verilmeyen eski tarih → `invalid_request` · `500` → `bank_error` |
| `SorgulaHesapHareket*` | `00` başarılı · `01`–`05`, `08` boş alan → `invalid_request` · `06` kayıt yok → `no_data` · `07` hesap tanımsız → `account_not_defined` · `09` doğrulama hatası → `bad_credentials` · `10` başlangıç = bitiş → `invalid_request` |
| `GetirPdfDekont*` | `0` başarılı · `1` kullanıcı bilgisi → `bad_credentials` · `2` dekont yok → `no_data` · diğerleri → `invalid_request` / `bank_error` |

**Başvuru:** Ziraat'in kendi formu var: "online elektronik hesap özeti uygulaması" talebi. Alanlar: şirket unvanı, idari yetkili, teknik yetkili (ad, GSM, e-posta), **şirket statik IP**, en fazla 10 IBAN. Şube operasyon merkezine "Kurumsal ödeme/tahsilat işlemleri → Web servis hareketleri izleme, günlük" seçerek gönderir. Portal bu formu önceden doldurur (`20` §5.2).

**Doğrulanacaklar:** geçmiş sınırı (gün) · `SorgulaHesapHareketZamanIle` için `MusteriNo`/`EkNo`'nun hesap listesinden nasıl alınacağı (müşteri no 8 + ek no 4 hane) · hareket satırındaki `iban` alanının anlamı.

---

## 3. Halkbank ✅

| | |
|---|---|
| Kaynak doküman | "Hesap Ekstre Sorgulama Web Servisi Teknik Doküman" |
| Test | `https://webservicestest.halkbank.com.tr/HesapEkstreOrtakWS/HesapEkstreOrtak.svc` |
| Üretim | `https://webservice.halkbank.com.tr/HesapEkstreOrtakWS/HesapEkstreOrtak.svc` (WSDL: `?wsdl`) |
| Protokol | SOAP, WCF. Kimlik: **WS-Security UsernameToken, `PasswordText`** (dokümandaki .NET örneği `ClientCredentials.UserName`; topluluk kodu `wsse:Security` başlığı ekliyor) |
| Mükellefin girdiği | Kullanıcı adı (banka e-postayla gönderir), parola (SMS ile) |

**Operasyonlar:** `EkstreSorgulama` (mükellefin hesapları) · `BagliMusteriEkstreSorgulama` (grup şirketi hesapları, `BagliMusteriNumarasi` String(8)). Topluluk kodunda `DekontSorgulama` da anılıyor ⚠️ (dokümanda yok).

**İstek:** `request` → `BaslangicTarihi` (DateTime), `BitisTarihi` (DateTime). Hesap filtresi yok, bütün tanımlı hesaplar döner.

Örnek başlık:

```xml
<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
               xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
  <wsse:UsernameToken>
    <wsse:Username>{kullanici}</wsse:Username>
    <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText">{parola}</wsse:Password>
    <wsse:Nonce EncodingType="...#Base64Binary">{nonce}</wsse:Nonce>
    <wsu:Created>{UTC ISO}</wsu:Created>
  </wsse:UsernameToken>
</wsse:Security>
```

**Yanıt → eşleme**

Üst: `HataKodu` (4), `HataAciklama` (50).

Hesap (`Hesap[]`):

| Halkbank | Portal |
|---|---|
| `HesapNo` (22, şube–müşteri–hesap) | `bank_account_number` |
| `HesapTuru` (`VDLI` / `VDSZ`) | `bank_account_kind` (`term` / `demand`) |
| `HesapCinsi` | `currency` |
| `SubeKodu`, `MusteriNo` | `bank_branch_code`, `bank_customer_no` |
| `Bakiye` | **Sorgu aralığının son bakiyesi, güncel bakiye değil** |
| `KullanilabilirBakiye` | Güncel bakiye → `bank_balance_snapshot.balance` |
| `BlokeMeblag`, `KrediLimit`, `KrediliKullanilabilirBakiye` | `blocked_amount`, `credit_limit`, `available_with_credit` |

Hesap yanıtında IBAN alanı yok. `cash_box` eşleşmesi `HesapNo` ile yapılır; mükellef ilk bağlantıda IBAN ↔ hesap no eşleşmesini onaylar.

Hareket (`Hareket[]`):

| Halkbank | Portal |
|---|---|
| `Tarih` + `Saat` | `booked_at` |
| `SiraNo` + `DekontNo` | `source_id` = `Tarih\|SiraNo\|DekontNo` |
| `HareketTutari` | İşaretli (`+`/`-`), **virgül ondalık**. İşaret → `debit_credit`, mutlak değer → `amount` |
| `Bakiye` | `balance_after` |
| `EkstreAciklama` | `description` |
| `Aciklama` | `description_extra` |
| `IslemKod` | `mt940_code` (MT940 kodları: `MSC`, `TRF`, `FEX` …) |
| `ReferansNo` | `bank_reference` |
| `DekontNo` | `receipt_number` |
| `Iptal` | `E` → `is_cancelled=true` |
| `KarsiAdSoyad` | `counterparty_name` |
| `KarsiHesapIBAN` | `counterparty_iban` |
| `KarsiKimlikNo` | `counterparty_tax_number` / `counterparty_identity_number` |
| `KarsiBankaKod` | `counterparty_bank_code` |
| `IslemYapanKimlikNo`, `IslemYapanAdSoyad` | `raw_payload` |

**Hata kodları:** `0` başarılı · `300` kayıt yok → `no_data` · `500`, `999` sistem hatası → `bank_error`. Kimlik hatasının nasıl döndüğü dokümanda yok ⚠️ (WCF servislerinde genelde SOAP fault). İlk test bağlantısında yanlış şifreyle denenip eşleme yazılır.

**Başvuru:** Dokümanda başvuru ve IP bilgisi yok. Topluluk kodu açıklamasına göre: şubeye talimat verilir, banka verilen IP için kullanıcı adı/parola tanımlar, kullanıcı adını e-postayla, parolayı SMS ile gönderir ⚠️.

**Doğrulanacaklar:** tarih aralığı / geçmiş sınırı · IP hatasının hangi biçimde döndüğü · `DekontSorgulama` operasyonu.

---

## 4. Garanti BBVA

Garanti'de iki yol var. **A** yıllardır kullanılan kurumsal servis, **B** bankanın API Store'u. Muhasebe programları (Netsim N4) B'ye geçmeye başlamış. İkisi de desteklenir; mükellef hangisini aldıysa.

### 4A. Firma Hesap Hareketleri SOAP servisi ✅

| | |
|---|---|
| Kaynak doküman | Garanti firma hesap hareketleri entegrasyon kılavuzu |
| Uç nokta (sabit IP ile) | `https://inboundrstintws.garanti.com.tr/services/FirmAccountActivitySoap` → host kaydı `194.29.211.237`, port 443 |
| Uç nokta (VPN ile) | `http://inboundextws.garanti.com.tr/services/FirmAccountActivitySoap` → `194.29.210.158` (portal kullanmaz) |
| Protokol | SOAP 1.1, namespace `http://odemeler.garanti.com.tr/`, istek `FirmAccountActivity` |
| Kimlik | `securetoken/UserId` (kurum adı) + `securetoken/Encoded` (IP'ye bağlı token) + `FirmCode` |
| Mükellefin girdiği | Kurum adı, firma kodu, `idhash` (bankanın verdiği değer) |
| Destek | MHIZ@garanti.com.tr |

**Token (`Encoded`):** Banka bir "Secure Token" programı veriyor. Programa **çağrıyı yapacak IP** ve bankanın verdiği `idhash` girilir, token üretilir. Dokümanda MD5 tabanlı olduğu yazıyor, ama girdi sırası net değil. Bu yüzden:

- Token, mükellefin (veya Easytrade destek ekibinin) bankanın programıyla **her çıkış IP'si için ayrı** ürettiği değer olarak kasaya girilir. Kod içinde yeniden üretilmeye çalışılmaz (algoritma bankayla teyit edilene kadar).
- Bir kurumda en fazla 6 IP tanımlanabiliyor. Banka IP başına ayrı token tercih ediyor.

**İstek**

| Alan | Zorunlu | Not |
|---|---|---|
| `securetoken/UserId` | ✓ | Kurum adı (Mutabakat Kurum Tanımı) |
| `securetoken/Password` | boş | |
| `securetoken/CreateTimestamp` | boş | |
| `securetoken/Encoded` | ✓ | IP'ye bağlı token |
| `FirmCode` | ✓ | Firma kodu |
| `StartDate`, `EndDate` | ✓ | `YYYY-MM-DD-HH.MM.SS.ffffff` (ör. `2026-11-01-00.00.00.000001`) |
| `BranchNum` + `AccountNum` veya `IBAN` | ✓ | IBAN verilirse şube/hesap boş bırakılabilir |
| `TransactionId` | | İşlem koduna göre filtre (ör. `WPSO`) |

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:odem="http://odemeler.garanti.com.tr/">
  <soapenv:Body>
    <odem:FirmAccountActivity>
      <odem:securetoken>
        <odem:UserId>{kurum_adi}</odem:UserId>
        <odem:Password></odem:Password>
        <odem:CreateTimestamp></odem:CreateTimestamp>
        <odem:Encoded>{token}</odem:Encoded>
      </odem:securetoken>
      <odem:FirmCode>{firma_kodu}</odem:FirmCode>
      <odem:StartDate>2026-11-01-00.00.00.000001</odem:StartDate>
      <odem:EndDate>2026-11-01-23.59.59.999999</odem:EndDate>
      <odem:BranchNum></odem:BranchNum>
      <odem:AccountNum></odem:AccountNum>
      <odem:IBAN>{iban}</odem:IBAN>
      <odem:TransactionId></odem:TransactionId>
    </odem:FirmAccountActivity>
  </soapenv:Body>
</soapenv:Envelope>
```

**Yanıt → eşleme**

Üst: `ReturnVal` (`00` başarılı), `MessageText`, `CurrentDateTime`, `CustomerNum`.

Hesap (`FirmAccounts`): `AccountName`, `AccountType` (`S` vadesiz / vadeli / kredi), `BranchNum`, `AccountNum`, `CurrencyCode` (`TL` → `TRY`), `IBAN`, `OpenDate`, `LastActivityDate`, `Balance`, `BlockedAmount`, `AvailableBalance`, `CreditLimit`, `AvailableBalanceWithCredit` → `cash_box` + `bank_balance_snapshot`.

Hareket (`AccountActivities`):

| Garanti | Portal |
|---|---|
| `TransactionReferenceId` (ör. `2020-12-07-10.56.25.052081`) | `source_id` + artımlı çekim anahtarı |
| `ActivityDate` (`dd.MM.yyyy`) | `transaction_date`. Saat `TransactionReferenceId` içindeki zaman damgasından → `booked_at` |
| `Amount` | İşaretli. İşaret → `debit_credit`, mutlak değer → `amount` |
| `Balance` | `balance_after`. **Virgül binlik, nokta ondalık** (`27,299.33`) |
| `Explanation` | `description` |
| `TransactionId` (ör. `EL07`, `ELMP`, `WPSO`, `AT03`) | `bank_txn_code` |
| `CorrBankNum`, `CorrBranchNum` | `counterparty_bank_code` |
| `CorrIBAN` | `counterparty_iban` (sonundaki boşluklar kırpılır) |
| `CorrVKN` | `counterparty_tax_number` (banka içi işlemlerde dolu) |

**Limitler:** Standart en fazla **1 günlük** aralık ve sorgular arası **en az 10 dakika**. Dışına çıkan istek yanıtsız kalır. Banka iki tasarım öneriyor, portal ikisini birlikte kullanır:

- Gün içi: `StartDate` = önceki yanıtın en büyük `TransactionReferenceId`'si, `EndDate` = şimdi (her 60 dakikada, `20` §7.2).
- Günlük: `D 00:00:00.000001` – `D 23:59:59.999999` sabit pencere (07:00 çekimi).
- Geçmiş yükleme: gün gün, her gün arasında 10 dakika. 30 gün ≈ 5 saat. Bu yüzden geçmiş için mükellefe MT940 / Excel yükleme seçeneği de sunulur (`23`).

**Hata kodları:** Dokümanda `00` dışında kod yok. `00` dışı her `ReturnVal` `MessageText` ile loglanır, `bank_error`. "Hesap bilgisi bulunamadı" türü mesaj → `account_not_defined` (hesap kurum tanımına eklenmemiş).

**Başvuru:** Şubeye talimat; banka kurum tanımını sabit IP'ye göre yapar, kurum adı + firma kodu + idhash verir; token programı bankadan alınır.

### 4B. Garanti BBVA API Store ⚠️

| | |
|---|---|
| Portal | `https://developers.garantibbva.com.tr` |
| Token | `https://apis.garantibbva.com.tr/auth/oauth/v2/token` (topluluk projesinde `client_credentials`) |
| Ürünler | **Account Information** ve **Account Transactions**. Ödeme/transfer ürünleri eklenmez. |
| Akış (Netsim kılavuzundan) | Geliştirici portalında uygulama oluştur → API sekmesinden iki ürünü Production için ekle → anahtar oluştur (Callback URL, Scope, `Confidential`) → API Key + Secret → müşteri **Consent ID** oluşturur (hesap seçimi, veri tercihleri) → uygulama Garanti onayından sonra `Enabled` |
| Uç nokta yolları ve şemalar | Portal girişinden sonra görünüyor. Herkese açık değil ⚠️ |

Netsim kılavuzundaki akışta **her müşteri kendi geliştirici hesabında kendi uygulamasını** açıyor ve yazılıma üç değer giriyor: API Key, Secret Key, Consent ID. Bir entegratörün tek uygulamayla birden çok müşteriye hizmet verip veremeyeceği bulunamadı ⚠️.

Portal için ilk sürüm: mükellef Netsim akışındaki gibi kendi uygulamasını açar, üç değeri portala girer. Uç nokta yolları ve alan adları herkese açık olmadığı için bu kanalın adaptörü, Easytrade bir test hesabıyla portala girip şemaları aldıktan sonra yazılır. O zamana kadar Garanti için 4A kullanılır.

---

## 5. Akbank ✅

| | |
|---|---|
| Kaynak doküman | Akbank Nakit Yönetimi "Web Servis Online Extre" teknik dokümanı |
| Test | `https://apigateuat.akbank.com/api/v1/cashmanagement/bankstatement?wsdl` |
| Üretim | Bankadan alınır ⚠️. Eski topluluk kodunda `https://firmahizmetleri.akbank.com/Extre_InterfaceService/service.asmx` kullanılıyor (güncel olmayabilir). |
| Protokol | SOAP, **HTTP Basic Authentication** (kullanıcı adı:şifre). TLS 1.2+. |
| Mükellefin girdiği | Web servis kullanıcı adı ve şifresi (Akbank Direkt'te tanımlanır), URF (firma kodu) |
| IP | Çağrıyı yapan IP şube kanalıyla "güvenli IP" listesine eklenir. Paraşüt kılavuzunda da Akbank için iki IP'nin tanımlatılması isteniyor. |

**Operasyonlar**

| Operasyon | Giriş | Davranış |
|---|---|---|
| `GetExtre` | yok | Kullanıcıya tanımlı bütün hesapların **son 5 günlük** hareketleri |
| **`GetExtreWithParams`** (kullanılacak) | `urf`, `subeKodu`, `hesapNo`, `dovizKodu`, `baslangicTarihi`, `bitisTarihi` (hepsi opsiyonel) | Tarih biçimi 20 hane `yyyyMMddHHmmssffffff` |

Döviz kodları Akbank'a özel (ör. `888` = TRY). Adaptörde tablo.

**Yanıt → eşleme:** Dokümanda alanlar açıklamalarıyla var, **alan adları WSDL'den alınacak** ⚠️:

| Dokümandaki alan | Portal |
|---|---|
| Hesap: firma kodu, şube kodu/adı, döviz, hesap no, IBAN, açılış ve gün içi bakiye, kullanılabilir bakiye, bloke, açılış tarihi, son hareket tarihi, hesap türü, aktiflik | `cash_box` + `bank_balance_snapshot` |
| Hareket: fiş (dekont) no | `receipt_number` |
| İşlem tarihi, valör | `transaction_date`, `value_date` |
| Zaman damgası | `booked_at` (+ `source_id` adayı) |
| Referans no | `bank_reference` |
| Tutar + borç/alacak işareti | `amount`, `debit_credit` |
| Yürüyen bakiye | `balance_after` |
| Açıklama, dekont açıklamaları | `description`, `description_extra` |
| Fonksiyon kodu (ör. `0086`, `3117`) | `bank_txn_code` |
| MT940 kodu (ör. `EFT`, `TRF`, `VRM`, `CCP`) | `mt940_code` |
| Karşı IBAN, borçlu/alacaklı IBAN | `counterparty_iban` |
| VKN/TCKN, alıcı/gönderen VKN/TCKN | `counterparty_tax_number` / `identity_number` |
| Abone no, fatura no alanları | `raw_payload` (kurum fatura ödemesi eşleşmesi) |
| Durum | `is_cancelled` ⚠️ (değerler WSDL'den) |

**Limitler:** Geçmiş en fazla **2 ay**. Sadece web servise tanımlı hesaplar döner.

**Hata davranışı:** Geçersiz tarih veya URF → **boş yanıt** · geçersiz hesap/şube → boş XML · tanımsız kullanıcı → SOAP fault `401` → `bad_credentials`. Boş yanıt `no_data` sayılmadan önce: aynı gün için `GetExtre` ile hesap görünüyor mu kontrol edilir (yanlış parametre ile gerçekten hareketsiz günü ayırmak için).

**Başvuru:** "Web Servis Online Extre Tanım Formu" ile hesaplar tanımlanır; IP şube kanalından eklenir; web servis kullanıcısı Akbank Direkt'te URF ile oluşturulur.

**Doğrulanacaklar:** üretim adresi · alan adları (WSDL) · tekil anahtar alanı · hesap türü kodları (dokümanda belirsiz).

---

## 6. TEB ✅

| | |
|---|---|
| Kaynak doküman | "TEB Online Hesap Hareketleri Bildirim Servisi" (v1.8, son değişiklik 16.05.2016) |
| Üretim | `https://extws.teb.com.tr/heshar/HesHarSrv` (WSDL bankadan) |
| Protokol | SOAP. Servis parametreleri + `InputDataXML` (kök `HESHARSORGU`) |
| Kimlik | `UserName`, `Password` (servis kullanıcısı) + `FIRMA_AD`, `FIRMA_ANAHTAR` + `ServiceID` (üretim `874`) + `Environment` (`P`) |
| Mükellefin girdiği | Kullanıcı adı, şifre, firma adı (rumuz), firma anahtarı, her hesap için şube no + hesap no |
| TLS | 20.06.2024'ten itibaren sadece ECDHE + AES-GCM / CHACHA20 şifre takımları |

**İstek** (`InputDataXML`)

| Alan | Tip | Değer |
|---|---|---|
| `FIRMA_AD` | VARCHAR2(25) | Firma rumuzu |
| `FIRMA_ANAHTAR` | VARCHAR2(25) | Firma anahtarı |
| `SUBENO` | NUMBER(4) | Şube no |
| `HESNO` | NUMBER(8) | Hesap no |
| `BASTAR`, `BITTAR` | `DD/MM/YYYY` | |
| `GON_IBAN_EH` | `E` | Gönderen IBAN gelsin |
| `ALICI_IBAN_EH` | `E` | Alıcı IBAN gelsin |
| `SON_BKY_EH` | `E` | Son bakiye gelsin |
| `DEKONTNO_EH` | `E` | Dekont no gelsin |
| `BLOKE_BKY_EH` | `E` | Bloke bakiye gelsin |
| `SON_HAREKET_KEY` | NUMBER(20) | Son alınan hareketin anahtarı (artımlı çekim) |

Portal bütün `_EH` alanlarını `E` gönderir.

**Yanıt → eşleme**

Başlık (`HAREKETLER`): `SUBENO`, `HESNO`, `BASTAR`, `BITTAR`, `SONBKY` (bakiye), `BLOKE_BKY`.

Hareket (`DETAY`):

| TEB | Portal |
|---|---|
| `HAREKET_KEY` | `source_id` + bir sonraki istekte `SON_HAREKET_KEY` |
| `ISLEM_TAR` + `ISLEM_TAR_SAAT` | `booked_at`, `transaction_date` |
| `BA` | `debit_credit` |
| `PARAKOD` (`TL`, `USD`, `EUR`) | `currency` (`TL` → `TRY`) |
| `TUTAR` | `amount` |
| `ANLIK_BKY` | `balance_after` |
| `ACIKLAMA` | `description` |
| `ISLEM_ACK` | `description_extra` |
| `ISLEM_TUR` | `bank_txn_code` (tablo `21` §4.1) |
| `MUSTERI_REF` | `bank_reference` |
| `DEKONTNO` | `receipt_number` |
| `GONDEREN_AD`, `GONDEREN_BANKA` | `counterparty_name`, `counterparty_bank_code` (yön `A`) |
| `GONDEREN_IBAN` / `ALICI_IBAN` | `counterparty_iban` (yöne göre) |
| `BORCLU_VKN` / `ALACAKLI_VKN` | `counterparty_tax_number` / `identity_number` (yöne göre: `A` ise borçlu karşı taraftır) |

Not: Dokümandaki örnek çıktıda `ALICI_IBAN` etiketi "Gönderen IBAN" yazıyor (doküman hatası). İlk bağlantıda yön kontrolü yapılır.

**`ISLEM_TUR` kodları:** Dokümanda 15 kod ve açıklaması var (`22`, `23`, `24`, `25`, `28`, `63`, `227`, `260`, `378`, `418`, `624`, `626`, `633`, `688`, `1207`). Bilinen: `22` şubeler arası TL havale, `1207` DBS tahsilatı. Tablonun tamamı geliştirme sırasında TEB dokümanından `bank_code_map`'e aktarılır.

**Limitler:** Tek sorguda en fazla **30 gün** (`74`). Sorgu hesap bazında.

**Hata kodları**

| Kod | Anlam | Neden |
|---|---|---|
| `00` | Başarılı | — |
| `74` | 30 günden uzun aralık | `invalid_request` (pencere bölünür) |
| `75`, `76`, `77`, `78`, `102`, `228` | Hesap/şube boş veya sayısal değil, tarih biçimi, tarih boş | `invalid_request` |
| `84` | Hesap bulunamadı | `account_not_defined` |
| `85` | Hesap için bildirim talimatı yok | `instruction_missing` |
| `86`, `87` | Firma adı / anahtarı boş | `invalid_request` |
| `210` | Şifre yanlış | `bad_credentials` |
| `99`, `101` | Genel hata, XML ayrıştırılamadı | `bank_error` / `invalid_request` |

**Başvuru:** Bağlanacak IP'ler bankaya bildirilir ve yetkilendirilir; sorgulanacak her hesap için bankada "hareket bildirim talimatı" açılır.

---

## 7. Fibabanka ✅

| | |
|---|---|
| Kaynak doküman | "Fibabanka Online Hesap Hareket Web Servisi" v1.8, 23.02.2021 |
| Üretim | `https://apis.fibabanka.com.tr/services/StatementService.svc` (WSDL: `?wsdl`) |
| Test | `https://testapis.fibabanka.com.tr/services/StatementService.svc` |
| Host kaydı | Üretim `apis.fibabanka.com.tr` → `195.39.224.71` · test → `195.39.224.70` (iç DNS / hosts) |
| Protokol | SOAP 1.1, namespace `http://fibabanka.com/`, operasyon `GetStatementInfo`. WSDL'deki endpoint IP olarak geliyor, yukarıdaki URL ile değiştirilir. |
| Kimlik | Gövdede `corporationCode` (müşteri no) + `password` + sabit IP |
| Mükellefin girdiği | Müşteri no, şifre, her hesap için hesap no ve döviz |

**İstek:** `corporationCode`, `accountNo`, `startDate` (`yyyyMMdd`), `endDate`, `password`, `summary` (`0` hareket detayı, `1` sadece özet bakiye).

**Yanıt → eşleme**

Hesap: `branchInfo`, `balance`, `usableBalance`, `startBalance`, `endBalance`, `blockBalance`, `errorCode`, `errorText` → `bank_balance_snapshot` + `bank_daily_summary` (`startBalance` / `endBalance`).

Hareket (`transactionTable`):

| Fibabanka | Portal |
|---|---|
| `referenceNumber` | `source_id` |
| `transactionNo` | `raw_payload` (sıra no) |
| `transactionDate` + `transactionTime` | `booked_at` |
| `accountingDate` | `transaction_date` |
| `transactionAmount` | İşaretli (borç eksi). Mutlak değer → `amount` |
| `transactionType` (`borc` / alacak) | `debit_credit` (esas alınır) |
| `currentBalance` | `balance_after` |
| `accountTransactionExplanation` | `description` |
| `MT940` | `mt940_code` (kod listesi dokümanda: `BND`, `BOE`, `COM`, `CHG`, `CHK`, `EXP`, `IMC`, `IMP`, `INT`, `LDP`, `PRO`, `TRF`; `21` §4.1) |
| `IBAN` | `counterparty_iban` (giden EFT'de alıcı, gelen EFT'de gönderen) |
| `identityNo` | `counterparty_tax_number` / `identity_number` |
| `title1` | `counterparty_name` |
| `eftInqueryNo` | `eft_query_number` |
| `currency` | `currency` |

**Hata kodları:** `errorCode` / `errorText` dönüyor, değer listesi dokümanda yok ⚠️. İlk bağlantılarda loglanıp tablo oluşturulur.

**Başvuru:** Sabit IP'ler bildirilir → hosts kaydı → imzalı talimat → banka erişimi açar. Bağlantı testi: `telnet 195.39.224.71 443`.

---

## 8. Türkiye Finans ✅

| | |
|---|---|
| Kaynak doküman | "Türkiye Finans Hesap Hareketleri Sorgulama Webservisi" (Genele Açık) |
| Uç nokta | `https://appgateway.turkiyefinans.com.tr/TurkiyeFinans.Banking/ICustomStatementIntegrationExtendedService.svc/basic` (WSDL: diğer iki servis adresinde `?wsdl`) |
| Protokol | SOAP (WCF basicHttp). Örnekte WS-Security UsernameToken `PasswordText` başlığı da var. |
| Kimlik | `UserCode` + `password` (gövdede) |
| Mükellefin girdiği | Kullanıcı kodu, şifre, hesap no (ör. `3492495-1`) |

**Operasyon:** `GetCustomStatementExtended`

**İstek:** `UserCode`, `password`, `accountNumber`, `beginDate` / `endDate` (`dd.MM.yyyy`), `referenceNumber` (zorunlu; dokümanda "üretilen referans numarası" deniyor, biçimi ve uzunluğu yazmıyor ⚠️ — ilk bağlantıda bankaya sorulur).

**Yanıt → eşleme**

Hesap: `ErrorCode`, `ErrorDescription`, `AccountNumber`, `IbanNumber`, `CurrencyCode`, `CustomerId`, `BranchCode`, `BranchName`, `AccountOpenDate`, `LastMovementDate`, `Balance`.

Hareket (`AccountActivities`, hepsi string):

| Türkiye Finans | Portal |
|---|---|
| `ReferenceNumber` + `SlipId` | `source_id` |
| `Date` (`dd/MM/yyyy`) + `TransactionHour` | `booked_at` |
| `Valor` | `value_date` |
| `Amount` | ⚠️ işaret / yön alanı ilk bağlantıda kontrol (örnekte `TransactionType` = "Giden" görülüyor) |
| `BalanceAfterTransaction` | `balance_after` (yön tespitinde de kullanılır: bakiye farkının işareti) |
| `DescriptionOne` | `description` |
| `DescriptionTwo` | `description_extra` |
| `ProgramCode` | `bank_txn_code` |
| `TransactionBranchId` | `raw_payload` |

Karşı taraf alanları yanıtta yok. Sınıflandırma açıklama ve kodla yapılır.

**Hata kodları:** `0` = OK. Diğerleri dokümanda yok ⚠️.

**Doğrulanacaklar:** yön alanı · aralık sınırı · IP şartı.

---

## 9. Ziraat Katılım ✅

| | |
|---|---|
| Kaynak doküman | Ziraat Katılım "Hesap Hareketleri Kullanıcı Kılavuzu" |
| Uç nokta | `POST https://zkapigw.ziraatkatilim.com.tr:8443/api/accountService` (port **8443**, çıkış güvenlik duvarında açık olmalı) |
| Protokol | REST, JSON, `Content-Type: application/json; charset=utf-8` |
| Kimlik | `Authorization: Basic base64(kullanici:sifre)` |
| Mükellefin girdiği | Kullanıcı adı, şifre, kurum kodu (`associationCode`) (banka e-postayla gönderir) |
| Destek | ZTKBUGTBnakitkurumyonetimi@ziraatkatilim.com.tr |

**İstek**

```json
{ "associationCode": "{kurum_kodu}", "startDate": "2026-11-01T00:00:00", "endDate": "2026-11-07T23:59:59" }
```

Bütün tanımlı hesaplar döner.

**Yanıt:** `AccountReportV2Response` → `AccountReportV2Result` → `AccountDetail` → `BankaHesaplariClassDetail` (`SysTarih`, `SysSaat`, `HataKodu`, `HataAciklama`) → `BankaHesaplari` → `HesapBilgisiDetail` → `HesapTanimi` + `HesapHareketleri`.

**Dikkat:** Tek eleman varsa **nesne**, birden fazlaysa **dizi** dönüyor. Ayrıştırıcı her seviyede "nesne ise diziye çevir" kuralı uygular.

Hesap (`HesapTanimi`): `SubeAdi`, `SubeNumarasi`, `AcilisTarihi`, `Bakiye`, `KullanilabilirBakiye`, `KrediliKullanilabilirBakiye`, `KrediLimiti`, `BlokeMeblag`, `HesapCinsi`, `HesapTuru` (ör. `CARIHSP`), `HesapAdi`, `HesapNumarasi`, `MusteriNo`, `KarOrani`, `VadeTarihi`, `SonHareketTarihi`.

Hesap yanıtında IBAN yok. Eşleşme `HesapNumarasi` ile, ilk bağlantıda mükellef onayıyla.

Hareket (`HesapHareketiDetail`):

| Ziraat Katılım | Portal |
|---|---|
| `ReferansNo` + `SiraNo` | `source_id` |
| `Tarih` (`dd/MM/yyyy`) + `Saat` (`HH:mm:ss`) | `booked_at` |
| `HareketTutari` | İşaretli (borç eksi) → `debit_credit` + `amount` |
| `SonBakiye` | `balance_after` |
| `Aciklamalar` | `description` |
| `IslemKodu` (ör. `ITAHS`, `CHVLH`) | `bank_txn_code` (liste yok, öğrenilerek) |
| `DekontNo` | `receipt_number` |
| `KarsiHesapVKNo` | `counterparty_tax_number` / `identity_number` |
| `EkBilgi1`–`EkBilgi4` | `description_extra` (birleştirilmiş) |

**Limitler:** Tek sorguda en fazla **7 gün**.

---

## 10. Vakıf Katılım ✅

| | |
|---|---|
| Kaynak doküman | "API Bankacılığı Müşteri İşlemleri Servisi" v1.3 |
| Test | `https://boatest.vakifkatilim.com.tr/BOA.Integration.WCFServiceTest/BOA.Integration.CustomerTransaction/CustomerTransactionService.svc` |
| Üretim | `https://boa.vakifkatilim.com.tr/BOA.Integration.WCFService/BOA.Integration.CustomerTransaction/CustomerTransactionService.svc` (üretimde `?wsdl` engelli, WSDL testten alınır) |
| Protokol | SOAP, WCF |
| Kimlik | `ExtUName`, `ExtUPassword` (gövdede) + IP + müşteri no kontrolü |
| Mükellefin girdiği | Kullanıcı adı, şifre (SMS ile gelir), her hesap için hesap no + ek no |

**Operasyonlar**

| Operasyon | Giriş | Kullanım |
|---|---|---|
| `GetAccountBalance` | `AccountNumber` (Int32), `AccountSuffix` (Int16), `BalanceDate` | Bakiye + hesap bilgisi (`IBAN`, `FECName` döviz, `IsActive`, `AccountType`) |
| `GetCustomerTransactions` | + `BeginDate`, `EndDate`, `DebitCreditCode` (`Debit` / `Credit` / `All`) | Temel hareket |
| **`GetCustomerTransactionDetails`** (kullanılacak) | aynı | Temel hareket + gönderen / alıcı bilgileri |

Hesap listesi operasyonu yok. Hesaplar mükellefin girdiği hesap no + ek no listesinden gelir, `GetAccountBalance` ile doğrulanır.

**Yanıt → eşleme** (`GetCustomerTransactionDetails`)

| Vakıf Katılım | Portal |
|---|---|
| `BusinessKey` | `source_id` |
| `TranDate` | `booked_at`, `transaction_date` |
| `SystemDate` | `raw_payload` |
| `Amount` | `amount` (yön: ayrı `DebitCreditCode` sorgusuyla veya alıcı/gönderen alanlarından; ilk bağlantıda işaret kontrolü ⚠️) |
| `Comment` | `description` |
| `FEC` / `FECName` | `currency` |
| `ChannelId` | `channel` (`0` batch, `1` şube, `2` internet, `3` ATM, `5` mobil, `6` çağrı merkezi, `8` gün sonu, `22` API) |
| `TranBranchId`, `TranBranchName` | `raw_payload` |
| `TransactionType` + `TransactionTypeDescription` | `bank_txn_code` (`21` §4.1 Vakıf Katılım tablosu) |
| Sender `TaxNumber`, `Name`, `AccountNumber`, `BankCode` | Yön `A` ise `counterparty_*` |
| Receiver `TaxNumber`, `Name`, `AccountNumber`, `BankCode` | Yön `B` ise `counterparty_*` |

Yön netleşene kadar güvenli yol: aynı pencere için `DebitCreditCode=Debit` ve `Credit` ile iki ayrı çağrı yapmak; yön çağrıdan bilinir.

**Limitler:** Aralık en fazla **31 gün** (`279437`). 15 dakikadan sık çağrı gerekiyorsa bankaya bildirilmeli → portal gün içi çekimi 60 dakikada tutar.

**Hata kodları:** `279438` müşteri no yetkisi yok → `account_not_defined` · `279436` başlangıç > bitiş, `279437` 31 gün → `invalid_request` · `70009` şifre yanlış → `bad_credentials` · `NotAuthorized` kullanıcı adı yanlış → `bad_credentials`.

---

## 11. Kuveyt Türk ⚠️

| | |
|---|---|
| Kaynak | Topluluk kodu (`phpdev/kuveytturk`). Vakıf Katılım ile aynı çekirdek bankacılık (BOA) ailesinde. |
| Uç nokta | `https://boa.kuveytturk.com.tr/BOA.Integration.WCFService/BOA.Integration.AccountStatement/AccountStatementService.svc/Basic` |
| SOAPAction | `http://boa.net/BOA.Integration.CoreBanking.Teller/Service/IAccountStatementService/GetAccountStatement` |
| Operasyon | `GetAccountStatement` |
| İstek | `ExtUName`, `ExtUPassword` (ns `http://schemas.datacontract.org/2004/07/BOA.Integration.Base`), `AccountNumber`, `AccountSuffix`, `BeginDate`, `EndDate` (ns `http://schemas.datacontract.org/2004/07/BOA.Integration.Model.CoreBanking.Teller`) |
| Mükellefin girdiği | Kullanıcı adı, şifre, müşteri / hesap no, ek no (ör. `001`) |
| Yanıt alanları | Topluluk kodunda yok ⚠️ |

**Yapılacak:** İlk Kuveyt Türk mükellefinde bankadan WSDL ve teknik doküman alınır, yanıt alanları bu bölüme yazılır. BOA ailesi olduğu için Vakıf Katılım eşlemesi başlangıç noktasıdır.

---

## 12. Yapı Kredi ⚠️

| | |
|---|---|
| Kaynak | Topluluk kodu (`phpdev/yapikredi`) + Paraşüt kılavuzu |
| Uç nokta | `https://dpextprd.yapikredi.com.tr/Hmn/EhoAccountTransactionService` |
| Protokol | SOAP, namespace `http://intf.service.electronicaccountsummary.eho.hmn.ykb.com/`, operasyon `sorgula`. WS-Security UsernameToken `PasswordText`. |
| İstek (`arg0`) | `baslangicTarih` (`yyyyMMdd`), `baslangicSaat` (`HHmm`), `bitisTarih`, `bitisSaat`, `dovizKodu` (boş = hepsi), `firmaKodu` |
| Yanıt | `sorgulaResponse.return` (alan adları topluluk kodunda yok ⚠️) |
| Mükellefin girdiği | Kullanıcı adı, şifre, firma kodu |
| Ücret | Banka ücret alabiliyor (Paraşüt kılavuzu) |

Yapı Kredi API Portal'da (`apiportal.yapikredi.com.tr`) `currentAccounts / accountTransactionList` dokümanı var ama şema herkese açık değil.

**Yapılacak:** İlk mükellefte WSDL alınır, yanıt eşlemesi yazılır.

---

## 13. İş Bankası ⚠️

| | |
|---|---|
| Kaynak | Paraşüt kılavuzu (kullanıcı adı + şifre, şubeye form) + topluluk kodu (`phpdev/isbank`) |
| Topluluk kodundaki çağrı | `POST https://posmatik2.isbank.com.tr/Authenticate.aspx`, form alanları `uid`, `pwd`. Yanıt XML (`Tarih` alanı içeren). Tarih parametresi yok. ⚠️ |
| Ücret | Hesap başına yıllık ücret (Paraşüt kılavuzunda 295 TL/yıl veya 25 TL/ay + BSMV) |

Topluluk kodu tarih aralığı göndermiyor; servisin hangi aralığı döndürdüğü (gün içi mi, son alınandan beri mi) belli değil. **Bu bilgiyle üretim adaptörü yazılmaz.**

**Yapılacak:** İş Bankası'ndan kurumsal "hesap hareketleri entegrasyonu" teknik dokümanı istenir (Easytrade iş ortaklığı başvurusuyla). O zamana kadar İş Bankası mükellefleri dosya kanalını kullanır (`23`).

---

## 14. QNB ve Enpara.com ⚠️

| | |
|---|---|
| Kaynak | Paraşüt kılavuzu + QNB basın duyurusu (21.10.2024) |
| Mükellefin girdiği | Kullanıcı adı, şifre, **WSDL adresi** (banka verir), her hesap için IBAN ve döviz |
| Başvuru | Paraşüt kılavuzu: başvuruda "3. Şablon" belirtilir. Salı akşamına kadar gelen talepler perşembe işleniyor. Enpara'da başvuru, Enpara'ya kayıtlı e-postadan gönderilir. |
| 2024 güncellemesi | QNB "Online Ekstre" ve "Online Para Transferi" API'lerini yeniledi. Başvuru **Tüzel İnternet Bankacılığı**'ndan, ıslak imzasız. Servis detayları, test ve kullanım başvurusu **QNB API Developer Portal**'da. |

WSDL adresinin mükellef bazında verilmesi, şablona göre farklı yanıt yapısı olabileceğini gösteriyor. Adaptör "3. Şablon" yanıtına göre yazılır.

**Yapılacak:** QNB API Developer Portal'a Easytrade adına kayıt; "3. Şablon" WSDL'i ve yanıt örneği alınır. Enpara aynı adaptörü kullanır (WSDL adresi farklı).

---

## 15. ING, Albaraka Türk, DenizBank ❌

| Banka | Bilinen | Yapılacak |
|---|---|---|
| ING | Paraşüt: bankadan kullanıcı adı + şifre alınır | Teknik doküman bankadan |
| Albaraka Türk | Paraşüt: müşteri no, kullanıcı adı, şifre | Teknik doküman bankadan |
| DenizBank | Paraşüt listesinde var, "güncelleme nedeniyle geçici olarak kullanım dışı" | Teknik doküman bankadan |

Bu bankalarda adaptör yazılana kadar dosya kanalı (`23`).

---

## 16. Bankalardan istenecek bilgi listesi (şablon)

Dokümanı olmayan bankaya (ve ⚠️ olanlara) Easytrade adına gönderilecek talep:

1. Kurumsal "online hesap hareketleri / elektronik hesap özeti" web servisi teknik dokümanı ve WSDL
2. Test ortamı adresi ve test kullanıcısı
3. Üçüncü taraf yazılım firmasının sabit IP'sinden erişim kabul ediliyor mu (talimatta teknik yetkili olarak Easytrade)
4. Tek sorguda en geniş tarih aralığı, en eski sorgulanabilir tarih, sorgu sıklığı ve günlük limit
5. Hareket satırında: tekil anahtar, işlem saati, yürüyen bakiye, karşı taraf adı / IBAN / VKN-TCKN, işlem kodu ve kod listesi, dekont no, iptal göstergesi
6. Dekont PDF servisi var mı
7. Hata kodu listesi
8. Ücret (hesap başına / firma başına)
9. Başvuru formu (portalın önceden dolduracağı)

## Kaynaklar

- Vakıfbank Online Hesap Hareketleri Web Servis Dokümanı v1.20: https://docs.netsim.com.tr/N4v2/Con012_Vakifbank.pdf
- Ziraat Hesap Hareketleri Sorgulama Webservisi: https://docs.netsim.com.tr/N4v1/Con014_Ziraat.pdf
- Halkbank Hesap Ekstre Sorgulama Web Servisi: https://docs.netsim.com.tr/N4v1/Con06_Halkbank.pdf
- Garanti firma hesap hareketleri kılavuzu: https://docs.netsim.com.tr/N4v2/Con05_Garanti.pdf
- Garanti API Store bağlantı adımları: https://docs.netsim.com.tr/N4v2/garanti_bankasi_web_servis_baglanti.html
- Akbank Web Servis Online Extre: https://docs.netsim.com.tr/N4v2/Con01_Akbank.pdf
- TEB Online Hesap Hareketleri: https://docs.netsim.com.tr/N4v2/Con09_Teb.pdf
- TEB şifre takımı güncellemesi (2024): https://mdpgroup.com/blog/teb-ekstre-hesap-ozeti-hareketleri-servisinde-guncellemeler/
- Fibabanka Online Hesap Hareket: https://docs.netsim.com.tr/N4v2/Con04_Fibabank.pdf
- Türkiye Finans Hesap Hareketleri: https://docs.netsim.com.tr/N4v1/Con010_TurkiyeFinans.pdf
- Ziraat Katılım Hesap Hareketleri: https://docs.netsim.com.tr/N4v2/Con015_ZiraatKatilim.pdf
- Vakıf Katılım Müşteri İşlemleri Servisi: https://docs.netsim.com.tr/N4v1/Con011_Vakif%20Katilim.pdf
- Topluluk kodları (Ziraat, Halkbank, Garanti, Vakıfbank, İş Bankası, Yapı Kredi, Kuveyt Türk, Akbank, Ziraat Katılım): https://github.com/selimhallac
- Vakıfbank topluluk kodu (ikinci): https://github.com/barisdemir49/VakifbankExtre
- Garanti API Store istemcisi: https://github.com/OnurByte/TurkishBankMCP
- Paraşüt banka bağlama kılavuzu (bankaların istediği bilgiler): https://www.parasut.com/kullanim-kilavuzu/banka-hesaplarinin-baglanmasi
- QNB Online Ekstre API duyurusu: https://www.qnb.com.tr/medium/document-file-4086.vsf
