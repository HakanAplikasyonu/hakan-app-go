# 20 · Banka Entegrasyonu: Genel Mimari

Mükellefin banka hesap hareketlerini portala almak için yapı. Bu dosya kanalları, bağlantı akışını (şube talimatı, sabit IP), adaptör sözleşmesini ve senkron kurallarını tanımlar.

| Dosya | İçerik |
|---|---|
| `20-banka-genel.md` | Bu dosya: kanallar, IP mimarisi, başvuru akışı, adaptör arayüzü, senkron, güvenlik, kabul testleri |
| `21-banka-veri-modeli.md` | `bank_transaction` şeması, merkezi işlem türü listesi, sınıflandırma, beyan varlıklarıyla eşleştirme, müşavir kontrolleri |
| `22-banka-servisleri.md` | **Banka banka servis detayları:** uç nokta, protokol, istek/yanıt alanları, limitler, hata kodları, alan eşlemesi |
| `23-banka-dosya-ice-aktarma.md` | PDF, Excel ve MT940 ekstre yükleme (her banka için) |

## 1. Banka verisi beyanda ne işe yarar

Banka hareketleri tek başına bir beyan kalemi değildir. Ön muhasebe verisinin **doğruluğunu ve eksiksizliğini** kontrol eden bağımsız kaynaktır. Portal bu veriden şunları üretir:

| Çıktı | Beyandaki yeri | Ayrıntı |
|---|---|---|
| Faturasız tahsilat uyarısı | KDV, gelir/kurumlar | Bankaya giren ama karşılığında satış faturası olmayan tutar |
| Belgesiz ödeme uyarısı | KDV (indirim), gider | Bankadan çıkan ama alış faturası / masraf kaydı olmayan ödeme |
| Banka masrafı, komisyon, BSMV | Gider | Ön muhasebede atlanabiliyor. Bankadan `expense` önerisi üretilir, müşavir onaylar. |
| Kredi faizi, mevduat faizi, stopaj | Gelir/kurumlar, geçici vergi | Faiz gideri, faiz geliri ve kesilen stopaj |
| Vergi ve SGK ödemeleri | Tahakkuk–ödeme mutabakatı | Hangi beyanın ödendiği, gecikme |
| Kira ödemeleri | Muhtasar (stopaj) | Gerçek kişiye işyeri kirası ödemesinde stopaj kontrolü |
| Maaş ödemeleri | Muhtasar, SGK | `personnel_transaction` ile mutabakat |
| Kendi hesapları arası transfer | Mizan | `cash_box_transfer` (gelir/gider sayılmaz) |
| POS tahsilatları | KDV, ÖKC mutabakatı | Kartlı satış toplamı ↔ fatura / Z raporu |
| Döviz alım satımı | Kur farkı | Dövizli hesaplar arası işlemler |
| Dönem sonu bakiye | Mizan | Ön muhasebedeki banka bakiyesi ↔ gerçek banka bakiyesi |
| Ödeme türü mutabakatı | Kayıt doğruluğu | "Nakit tahsilat" girilmiş ama bankaya havale gelmiş (`01` §4.9) |

Ayrıntılı kural seti `21` §6'da.

## 2. Erişim kanalları

Mükellef iki yoldan birini seçer. Portal ikisini de **aynı `bank_transaction` modeline** yazar.

| Yol | Nasıl çalışır | Artı | Eksi |
|---|---|---|---|
| **1. Banka hesabını bağlama** | Doğrudan bankanın kendi servisi, arada üçüncü taraf yok. **A.** Bankanın kurumsal hesap hareketi web servisi: mükellef şubeye talimat verir, banka Easytrade'in **sabit IP'sini** tanımlar, kullanıcı adı/şifre verir. **B.** Bankanın kendi API portalı: Garanti BBVA API Store'da mükellef kendi uygulamasını açıp onay (Consent ID) üretir; QNB'de başvuru Tüzel İnternet Bankacılığı'ndan ıslak imzasız yapılıyor. İkisinin de uç nokta şemaları herkese açık değil (`22` §4B, §14). | Otomatik akış. Tam açıklama, karşı taraf VKN/IBAN, banka işlem kodu, dekont no. Servis sadece okur, para hareketi yapamaz. | Banka başına ayrı entegrasyon. Başvuru birkaç gün sürer. Bazı bankalar mükelleften servis ücreti alır (ücret banka ile mükellef arasında). |
| **2. Ekstre yükleme** | Mükellef bankasının internet şubesinden **PDF veya Excel** ekstreyi indirip portala yükler. MT940 dosyası da kabul edilir. | Her bankada çalışır. Başvuru gerekmez. Geçmiş dönem için de kullanılır. | Elle yükleme. Dönem kapanışından önce yüklenmesi gerekir (portal hatırlatır). |

**Kapsam dışı:** Aracı firmalar (banka verisini toplayıp API ile satan şirketler) ve lisanslı sağlayıcı üzerinden açık bankacılık kullanılmaz. İşlem / mükellef başı ücret ve üçüncü taraf bağımlılığı getiriyorlar.

**Karar:**

1. Bankanın servisi varsa ve adaptörü yazıldıysa mükellefe önce **banka hesabını bağlama** önerilir.
2. **Ekstre yükleme her zaman açıktır:** servisi olmayan banka, başvurusunu henüz yapmamış mükellef ve bankanın servisinin izin verdiği süreden eski geçmiş için.
3. Aynı hesap için iki yol birlikte kullanılabilir. Mükerrer kayıt oluşmaz (`21` §5).

## 3. Banka kapsamı

`bank_id` değerleri Easytrade `cash_box.bank_id` enum'udur. Easytrade enum'unda olmayan bankalar ⊕ ile işaretli.

| Banka | `bank_id` | Kanal | Protokol | Mükellefin portala girdiği bilgiler | Tek sorguda en fazla | Geçmiş | Sabit IP | Doküman |
|---|---|---|---|---|---|---|---|---|
| Ziraat Bankası | `ziraat` | A | SOAP / HTTP POST (.asmx) | Kurum kodu, şifre, hesap no veya IBAN | ⚠️ | ⚠️ (`111` hatası: izin verilmeyen eski tarih) | ✅ form | ✅ |
| Vakıfbank | `vakifbank` | A | SOAP 1.2 (WCF, WS-Addressing) | Müşteri no, kurum kullanıcı, şifre | ⚠️ | **365 gün** | ✅ | ✅ |
| Halkbank | `halkbank` | A | SOAP (WCF, WS-Security UsernameToken) | Kullanıcı adı, şifre | ⚠️ | ⚠️ | ⚠️ (dokümanda yok, topluluk kodunda var) | ✅ |
| Garanti BBVA | `garanti` | A ve B | A: SOAP 1.1 · B: REST + OAuth2 | A: kurum adı, firma kodu, IP'ye bağlı token · B: consent | A: **1 gün**, sorgular arası **10 dk** | ⚠️ | A: ✅ (en fazla 6 IP) | ✅ (A) · ⚠️ (B) |
| Akbank | `akbank` | A | SOAP (Basic Auth) | Kullanıcı adı, şifre, URF (firma kodu) | ⚠️ | **2 ay** | ✅ | ✅ |
| TEB | ⊕ `teb` | A | SOAP (XML girdi) | Kullanıcı adı, şifre, firma adı, firma anahtarı, şube, hesap no | **30 gün** | ⚠️ | ✅ | ✅ |
| Fibabanka | `fibabank` | A | SOAP 1.1 | Müşteri no, şifre, hesap no | ⚠️ | ⚠️ | ✅ | ✅ |
| Türkiye Finans | `turkiye-finans` | A | SOAP (WCF basic) | Kullanıcı kodu, şifre, hesap no | ⚠️ | ⚠️ | ⚠️ | ✅ |
| Ziraat Katılım | ⊕ `ziraat-katilim` | A | REST JSON (Basic Auth) | Kullanıcı adı, şifre, kurum kodu | **7 gün** | ⚠️ | ⚠️ | ✅ |
| Vakıf Katılım | ⊕ `vakif-katilim` | A | SOAP (WCF, BOA) | Kullanıcı adı, şifre, hesap no + ek no | **31 gün** | ⚠️ | ✅ | ✅ |
| Kuveyt Türk | `kuveyt-turk` | A | SOAP (WCF, BOA) | Kullanıcı adı, şifre, hesap no + ek no | ⚠️ | ⚠️ | ⚠️ | ⚠️ topluluk kodu |
| Yapı Kredi | `yapi-kredi` | A | SOAP (WS-Security UsernameToken) | Kullanıcı adı, şifre, firma kodu | ⚠️ | ⚠️ | ⚠️ | ⚠️ topluluk kodu |
| İş Bankası | `is-bankasi` | A | ⚠️ HTTP POST | Kullanıcı adı, şifre | ⚠️ | ⚠️ | ⚠️ | ⚠️ topluluk kodu |
| QNB | `qnb` | A / B | SOAP (WSDL'i banka verir) · API portalı | Kullanıcı adı, şifre, WSDL adresi, IBAN | ⚠️ | ⚠️ | ⚠️ | ⚠️ |
| Enpara.com | `enpara` | A | QNB ile aynı altyapı | Kullanıcı adı, şifre, WSDL adresi, IBAN | ⚠️ | ⚠️ | ⚠️ | ⚠️ |
| ING | `ing` | A | ⚠️ | Kullanıcı adı, şifre | ⚠️ | ⚠️ | ⚠️ | ❌ bulunamadı |
| Albaraka Türk | `albaraka` | A | ⚠️ | Müşteri no, kullanıcı adı, şifre | ⚠️ | ⚠️ | ⚠️ | ❌ bulunamadı |
| DenizBank | `denizbank` | A | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ❌ bulunamadı |
| Şekerbank, Odeabank, Anadolubank, Emlak Katılım, HSBC, diğer | ⊕ | — | Servis bilgisi yok, **sadece ekstre yükleme** | — | — | — | — | ❌ |

Okuma: ✅ bankanın teknik dokümanı (Netsim'in yayımladığı kopyalar, bazıları 2016–2021 tarihli; `22` özet tablosu) · ⚠️ topluluk kodu veya muhasebe programı kılavuzu, alanlar eksik · ❌ teknik doküman bulunamadı, bankadan istenecek. "Tek sorguda en fazla" ve "Geçmiş" sütunlarındaki ⚠️ = dokümanda yazmıyor. **Hiçbir servis canlı hesapla test edilmedi.**

**Mükellefin girdiği bilgiler** sütunu banka dokümanından alındı. Paraşüt'ün banka bağlama kılavuzunda yer alan bankalarda (Akbank, Albaraka, Fibabanka, Garanti, ING, QNB, TEB, İş Bankası, Vakıfbank, Yapı Kredi, Ziraat, Enpara) Paraşüt'ün istediği bilgilerle de karşılaştırıldı. Bu, bankaların bu servisi üçüncü taraf muhasebe yazılımları için açtığını gösteriyor.

Hangi bankada ne zaman geliştirme yapılacağı §10'da.

## 4. Sabit IP mimarisi (bank-egress)

Dokümanı incelenen bankaların çoğu (Vakıfbank, Ziraat, Garanti, Akbank, TEB, Fibabanka, Vakıf Katılım) servisi **sadece tanımlı sabit IP'den** gelen çağrıya açıyor. Tanımsız IP'de Vakıfbank açık hata kodu dönüyor (`VBH0009`, `VBD0003`). Çağrıyı Easytrade sunucusu yapacağı için **talimata yazılan IP Easytrade'in IP'sidir.**

Üçüncü tarafın IP'sinin kabul edildiğine dair kanıt: Paraşüt kılavuzu Akbank bağlantısında mükelleften **Paraşüt'ün iki IP adresini** bankaya tanımlatmasını istiyor. Ziraat formundaki alanın adı ise "Şirket Statik IP". Bazı şubeler IP'nin şirkete ait olmasını isteyebilir. Bu yüzden **ilk mükelleflerde her banka için teyit edilmelidir.** Talimatta Easytrade'in hizmet sağlayıcı ve teknik yetkili olduğu açıkça yazılır.

```
 Portal (bulut, ölçeklenen) ──► Banka iş kuyruğu ──► bank-egress worker havuzu ──► NAT Gateway ──► Banka
                                                      (sadece banka adaptörleri)    Sabit IP-1 (birincil)
                                                                                    Sabit IP-2 (yedek)
```

Kurallar:

1. **İki sabit çıkış IP'si** (farklı erişilebilirlik bölgelerinde). İkisi de her talimata yazılır. Biri düşerse diğerinden devam edilir.
2. Bu IP'ler **asla değişmez.** Değişirse bütün mükelleflerin şubeye yeniden gitmesi gerekir. IP'ler bulut sağlayıcısında "rezerve / elastic" olarak alınır, silinmesi yetkiyle kilitlenir.
3. Banka adaptörleri **sadece** bu worker havuzunda çalışır. Portalın geri kalanı bu IP'leri kullanmaz.
4. Test ortamı için ayrı bir sabit IP. Bankaların test ortamı varsa (Vakıfbank, Halkbank, Ziraat, Fibabanka, Akbank, Vakıf Katılım) ilk entegrasyon bu IP'den yapılır.
5. Bazı bankalar sunucu adını belirli bir IP'ye çözmeyi şart koşuyor (Fibabanka: `apis.fibabanka.com.tr` → `195.39.224.71`; Garanti: `inboundrstintws.garanti.com.tr` → `194.29.211.237`). Worker konteynerine bu kayıtlar `hosts` / iç DNS olarak eklenir (`22`'de banka bazında).
6. Garanti A kanalında token **IP'ye bağlı üretilir** (`22` §4). Her çıkış IP'si için ayrı token saklanır, çağrı hangi IP'den çıkıyorsa onun token'ı kullanılır.
7. TLS 1.2 ve üstü. Bankalar zayıf şifre takımlarını kapatıyor (TEB 2024 duyurusu). İstemci kütüphanesinin güncel olması yeterli.
8. SOAP yanıtları büyük olabilir. Mesaj boyutu limiti en az 50 MB ayarlanır (Vakıfbank dokümanı "maximum message size quota" uyarısı veriyor).

Talimat yazılarında Easytrade **teknik yetkili** olarak yer alır (bkz. §5.2).

## 5. Bağlantı akışı (mükellef ve müşavir)

### 5.1 Durumlar

```
draft ──► form_ready ──► submitted_to_bank ──► credentials_entered ──► verifying ──► active
                                                                          │
                                                                          └──► failed (neden kodu)
active ──► auth_failed / ip_rejected / unauthorized / instruction_missing / disabled
```

| Durum | Anlamı | Kim ilerletir |
|---|---|---|
| `draft` | Müşavir veya mükellef "banka bağla" dedi, banka seçildi | Kullanıcı |
| `form_ready` | Portal bankaya özel talimat yazısını hazırladı (PDF) | Portal |
| `submitted_to_bank` | Mükellef talimatı şubeye verdiğini işaretledi | Mükellef |
| `credentials_entered` | Banka bilgileri geldi, mükellef portala girdi | Mükellef |
| `verifying` | Portal `healthCheck` + `listAccounts` çalıştırıyor | Portal |
| `active` | Hesaplar listelendi, `cash_box` eşleşmesi yapıldı, ilk çekim başladı | Portal |
| `failed` | Doğrulama başarısız. Neden kodu ve mükellefe ne yapacağı yazılır. | Portal |

`submitted_to_bank` durumunda 5 iş günü sonra mükellefe hatırlatma gider.

### 5.2 Talimat yazısı (portal üretir)

Bankaların çoğu kendi formunu ister. Portal, bankanın formu varsa onu, yoksa aşağıdaki genel talimatı **önceden doldurulmuş** PDF olarak verir. Mükellef imzalar (kaşe + yetkili imzası) ve şubeye teslim eder.

Formda olması gerekenler (Ziraat Bankası başvuru formu ve Garanti/Vakıfbank dokümanlarındaki alanlardan):

| Alan | Kaynak |
|---|---|
| Şirket unvanı, VKN, vergi dairesi | `company` |
| İdari yetkili ad soyad, imza | Mükellef doldurur |
| Hesap listesi: IBAN, döviz cinsi | Mükellefin seçtiği `cash_box` kayıtları (IBAN'ı olan) |
| Teknik yetkili ad soyad, GSM, e-posta | Easytrade entegrasyon ekibi (sabit) |
| Sabit IP adresleri | §4'teki iki üretim IP'si (+ test IP'si) |
| Talep edilen hizmet | "Online hesap hareketleri / elektronik hesap özeti web servisi" |
| Periyot | Günlük / gün içi |

Genel talimat metni (bankanın formu yoksa):

```
T.C. ......................... BANKASI A.Ş.
.................................. ŞUBESİ

Bankanız nezdindeki aşağıda IBAN'ları yazılı hesaplarımızın bakiye ve hareket bilgilerinin,
online hesap hareketleri / elektronik hesap özeti web servisi üzerinden, aşağıda belirtilen
sabit IP adreslerinden sorgulanabilmesi için gerekli tanımların yapılmasını ve servis kullanıcı
bilgilerinin tarafımıza iletilmesini talep ederiz. Bu servis yalnızca bilgi sorgulama amaçlıdır;
para transferi yetkisi talep edilmemektedir.

Şirket unvanı / VKN      : ...........................  /  ..........
Hesaplar (IBAN / döviz)  : TR.... / TRY   TR.... / USD   ...
Sabit IP adresleri       : <IP-1>, <IP-2>
Teknik yetkili           : Easytrade entegrasyon ekibi, <telefon>, <e-posta>

Tarih, kaşe, yetkili imza
```

Bankanın kendi formu olan bankalarda (`22`'de "Başvuru" satırı) form şablonu portala yüklenir ve aynı alanlar otomatik basılır.

### 5.3 Banka bilgilerini girme ekranı

- Form alanları adaptörün `credentialFields` tanımından otomatik üretilir (§6). Banka başına farklı alanlar (Vakıfbank: müşteri no + kullanıcı + şifre; TEB: 6 alan; Garanti A: kurum adı + firma kodu + idhash).
- Bilgilerin nasıl geldiği bankaya göre değişiyor (Halkbank: kullanıcı adı e-posta, parola SMS — topluluk kodu açıklaması; Vakıf Katılım: SMS; Ziraat Katılım: e-posta). Ekranda banka bazında yazılır.
- Girilen bilgiler doğrudan kasaya yazılır (`02` §4). Uygulama veritabanına düşmez, loglanmaz.
- "Kaydet" sonrası `verifying`: bağlantı testi + hesap listesi. Başarılıysa bankadan gelen hesaplar IBAN ile mevcut `cash_box` kayıtlarına eşlenir, eşleşmeyen için yeni `cash_box` (`type=bank_account`) önerilir.

### 5.4 Doğrulama hatası → mükellefe mesaj

| Neden kodu | Örnek banka yanıtı | Mükellefe gösterilen |
|---|---|---|
| `ip_rejected` | Vakıfbank `VBH0009`, `VBD0003` | "Bankanız IP tanımını henüz yapmamış. Şubeye IP adreslerinin tanımlanıp tanımlanmadığını sorun." |
| `unauthorized` | Ziraat `101` (yetkisiz kullanım; IP mi yetki mi ayrımı yok) | "Banka erişime izin vermedi. Şubeye talimatın ve IP tanımının tamamlandığını sorun." |
| `bad_credentials` | Vakıfbank `VBH0012`, Ziraat `102`, TEB `210`, Akbank SOAP fault 401 | "Kullanıcı adı veya şifre hatalı. Bankadan gelen bilgileri kontrol edin." |
| `instruction_missing` | Vakıfbank `VBH0011`, TEB `85` | "Hesap için bankada talimat bulunamadı. Şubeye talimatın işlenip işlenmediğini sorun." |
| `account_not_defined` | TEB `84`, Ziraat `07` (`SorgulaHesapHareket`) | "Bu hesap servise tanımlı değil. Şubeden hesabın talimata eklenmesini isteyin." |
| `bank_error` | Vakıfbank `VBH0002`, Halkbank `500`/`999` | "Banka geçici olarak yanıt vermiyor. Otomatik tekrar denenecek." |

Banka bazında tam liste `22`'de.

### 5.5 Ücret

Bazı bankalar bu servis için ücret alıyor. Örnek: İş Bankası hesap başına yıllık ücret alıyor; Vakıfbank ve Yapı Kredi de ücret alabiliyor (kaynak: Paraşüt kılavuzu). Ücret bankayla mükellef arasındadır. Portal bağlantı ekranında "bankanız bu hizmet için ücret alabilir" uyarısı gösterir.

## 6. Banka adaptör arayüzü

`02` §2'deki `SourceAdapter` ile aynı mantık. Banka adaptörleri daha dar bir sözleşme uygular.

```ts
type BankId = 'ziraat' | 'vakifbank' | 'halkbank' | 'garanti' | 'akbank' | 'teb' | 'fibabank'
  | 'turkiye-finans' | 'ziraat-katilim' | 'vakif-katilim' | 'kuveyt-turk' | 'yapi-kredi'
  | 'is-bankasi' | 'qnb' | 'enpara' | 'ing' | 'albaraka' | 'denizbank';

type BankChannel = 'web_service' | 'bank_api';

interface BankLimits {
  maxRangeDays: number | null;      // tek sorguda en geniş aralık (Garanti A: 1, TEB: 30, ZK: 7, VK: 31)
  maxHistoryDays: number | null;    // en eski sorgulanabilir gün (Vakıfbank: 365, Akbank: ~60)
  minIntervalSeconds: number;       // aynı hesap için iki sorgu arası (Garanti A: 600)
  supportsTimeOfDay: boolean;       // saat aralığıyla sorgu (Ziraat *ZamanIle, Vakıfbank, Akbank)
  supportsIncrementalKey: boolean;  // son hareket anahtarından devam (TEB SON_HAREKET_KEY, Garanti TransactionReferenceId)
  perAccountQuery: boolean;         // sorgu hesap bazında mı (TEB, Fibabanka, ZB HesapEkstre) yoksa müşteri bazında mı (Halkbank, Vakıfbank)
}

interface CredentialField {
  key: string;                      // 'customer_no', 'username', 'password', 'firm_code', 'firm_key' ...
  label: string;                    // ekranda görünen ad, bankanın kullandığı terimle ("Kurum Kodu", "URF")
  secret: boolean;
  pattern?: string;                 // ör. Vakıfbank hesap no: ^00158\d{12}$
  help?: string;                    // "Bankanız SMS ile gönderir"
}

interface BankAdapter {
  readonly bankId: BankId;
  readonly channel: BankChannel;
  readonly limits: BankLimits;
  readonly credentialFields: CredentialField[];
  readonly hostOverrides?: Record<string, string>;   // §4 madde 5

  healthCheck(conn: BankConnection): Promise<HealthStatus>;          // en hafif çağrı (bugünün tek hesabı)
  listAccounts(conn: BankConnection): Promise<BankAccountDTO[]>;    // IBAN, hesap no, şube, döviz, tür
  fetchBalances(conn: BankConnection): Promise<BankBalanceDTO[]>;   // sorgu anı bakiyeleri
  fetchTransactions(
    conn: BankConnection,
    q: { account?: BankAccountRef; start: string; end: string; afterKey?: string } // ISO, Europe/Istanbul
  ): AsyncIterable<Batch<BankTransactionDTO>>;
  fetchDailySummary?(conn: BankConnection, range: DateRange): Promise<BankDailySummaryDTO[]>; // Vakıfbank GetirHareketOzet, Ziraat SorgulaOzetWS
  fetchReceipt?(conn: BankConnection, ref: string): Promise<Binary>;  // Vakıfbank GetirDekont, Ziraat GetirPdfDekont (PDF)
}
```

Kurallar:

- Adaptör **hesap no / IBAN / tarih aralığı bölmesini** kendisi yapmaz. Senkron motoru `limits`'e göre pencereleri üretir, adaptör tek pencereyi çalıştırır.
- DTO'lar `21` §2'deki `bank_transaction` alanlarıyla birebir. Ham banka kaydı `raw` alanında.
- Tutar ayrıştırma bankaya özeldir ve adaptörün içindedir (ör. Halkbank `"+1234,56"`, Garanti `"27,299.33"`, Fibabanka negatif sayı + `borc`). Kurallar `22`'de her bankada.
- Bankanın kendi hata kodu adaptörde §5.4'teki neden koduna çevrilir.

## 7. Senkron

### 7.1 İlk yükleme

1. Banka servisinin izin verdiği geçmiş kadar çekilir (`maxHistoryDays`). Hedef pencere `02` §3.1 ile aynı (içinde bulunulan yıl + önceki yıl).
2. Bankanın izin vermediği eski dönem için mükellefe **"geçmiş ekstreyi yükle"** görevi açılır (`23`). Hangi tarihten önce dosya gerektiği ekranda yazar.
3. Pencere: `min(maxRangeDays, 7)` gün. Hesap bazında sorgulanan bankalarda her hesap ayrı.
4. Her pencere sonrası bakiye sürekliliği kontrolü (§7.4).

### 7.2 Düzenli çekim

| İş | Sıklık | Not |
|---|---|---|
| Dünün tam çekimi (gün kapanışı) | Her gün 07:00 | Gece işlenen faiz, komisyon, gün sonu kayıtları için. **Esas kayıt bu çekimdir.** |
| Gün içi çekim | 60 dakikada 1 (09:00–19:00) | Bankanın `minIntervalSeconds` sınırı daha büyükse o (Garanti A: 10 dk alt sınır). Artımlı anahtar varsa onunla. |
| Bakiye | Gün içi çekimle birlikte | |
| Son 7 günün yeniden çekimi | Gece 1 | Geç düşen ve iptal edilen hareketleri yakalar |
| Müşavir "şimdi çek" | Anlık, bağlantı başına 5 dakikada 1 | |
| Ayın 1–3'ü | Önceki ayın tamamı yeniden | Dönem kapanışı öncesi tam liste |

Bankanın günlük sorgu limiti olabilir (Vakıfbank `VBH0014`). Limit hatası gelirse o gün için gün içi çekim durur, 07:00 çekimi bozulmaz.

### 7.3 Mükerrer ve güncelleme

- Her hareketin bankaya özel tekil anahtarı vardır (`22`'de her bankada "Tekil anahtar" satırı). `source_id` bu anahtardır. Anahtarı olmayan bankada: `sha256(iban | tarih-saat | tutar | yön | yürüyen bakiye | açıklama)`.
- Aynı hareket API ve MT940/Excel ile iki kez gelirse `21` §5'teki kuralla birleşir.
- İptal edilen hareket (Halkbank `Iptal=E`, Ziraat `kayitDurumu=I`) silinmez, `is_cancelled=true` olur, kontrollerden düşer.

### 7.4 Bakiye sürekliliği

Bankaların çoğu her harekette **işlem sonrası bakiye** veriyor. Her hesap için tarih-saat sırasıyla:

```
önceki_bakiye + (A ise +tutar, B ise −tutar) = işlem_sonrası_bakiye   (±0,01)
```

- Tutmazsa: arada eksik hareket var. O gün yeniden çekilir. Yine tutmazsa `BNK_GAP` uyarısı (`05`).
- Gün başı bakiyesi + günün hareketleri = gün sonu bakiyesi (bankanın özet servisi varsa onunla da karşılaştırılır).
- Dönem sonu banka bakiyesi `cash_box.balance` (ön muhasebe) ile karşılaştırılır (`21` §6).

### 7.5 Dönem kilidi

`02` §3.4 aynen geçerli. Kilitli döneme yeni banka hareketi düşerse (geç işlenen faiz gibi) kayıt yazılır ama `change_request` olarak işaretlenir, müşavire uyarı gider.

## 8. Güvenlik ve KVKK

- Bu servislerin hepsi **salt okunur.** Para transferi yapamaz. Mükellefe bağlantı ekranında açıkça yazılır. Talimatta da "transfer yetkisi talep edilmemektedir" ifadesi var (§5.2).
- B kanalında (API portalı) uygulamaya **sadece hesap bilgisi ve hesap hareketleri** ürünleri eklenir. Ödeme, transfer, kart yönetimi ürünleri eklenmez.
- Banka kimlik bilgileri kasada (`02` §4), ayrı bir anahtarla şifrelenir. Erişim sadece bank-egress worker'larına açıktır.
- Banka yanıtlarının ham hali (`raw_payload`) karşı taraf adı, TCKN, IBAN içerir. Bu alanlar veritabanında alan seviyesinde şifrelenir. Müşavir paneli dışında (log, hata izleme, analitik) gösterilmez.
- Loglarda: şifre, token, `Encoded`, `idhash`, Basic Auth başlığı, SOAP gövdesindeki `Sifre` / `password` alanları maskelenir. SOAP istek/yanıt loglaması varsayılan olarak kapalıdır, sadece hata ayıklama modunda ve maskeli açılır.
- Mükellef bağlantıyı kestiğinde: kimlik bilgileri silinir, mükellefe "bankanıza talimatı iptal ettirmeniz önerilir" bilgisi gösterilir.
- Onay metni (KVKK) bağlantı kaydında saklanır (`02` §10).

## 9. Kabul testleri (her banka adaptörü için)

1. Doğru bilgilerle bağlantı + hesap listesi. Yanlış şifrede `bad_credentials`, tanımsız IP'den (test IP'si) `ip_rejected`.
2. 1 günlük hareket: adet ve toplam borç / toplam alacak banka internet şubesi ekstresiyle birebir.
3. 1 aylık hareket (bankanın aralık sınırına göre bölünerek): adet, toplamlar ve ay sonu bakiyesi birebir.
4. Bakiye sürekliliği (§7.4) bütün ayda kesintisiz.
5. Aynı pencere iki kez çekildiğinde mükerrer kayıt oluşmuyor.
6. Dövizli hesap: `currency` doğru, tutar dönüşümü yok.
7. Gelen EFT'de karşı taraf adı, IBAN ve (banka veriyorsa) VKN/TCKN dolu.
8. Banka masrafı, BSMV, faiz hareketleri doğru türe (`21` §3) sınıflanıyor.
9. İptal edilen hareket (banka veriyorsa) `is_cancelled=true`.
10. Banka limiti (aralık, sıklık, günlük sorgu) aşıldığında iş kaybı olmadan bir sonraki çalışmada devam.
11. Saat dilimi: 23:59'da yapılmış hareket doğru güne yazılıyor.
12. Dekont PDF (banka veriyorsa) indirilip `attachment` olarak bağlanıyor.

## 10. Geliştirme sırası

| Sıra | Bankalar | Neden |
|---|---|---|
| 1 | Ortak altyapı: bank-egress (sabit IP), `bank_transaction`, sınıflandırma, PDF + Excel + MT940 yükleme (`23`) | Bütün bankalar ve dosya kanalı buna dayanıyor |
| 2 | Vakıfbank, Ziraat, Halkbank | Teknik doküman + test ortamı adresi var |
| 3 | Garanti BBVA (A), Akbank | Teknik doküman var |
| 4 | TEB, Fibabanka, Türkiye Finans, Ziraat Katılım, Vakıf Katılım | Teknik doküman var |
| 5 | Yapı Kredi, Kuveyt Türk, İş Bankası, QNB / Enpara | Topluluk kodu var. İlk mükellef bağlantısında bankadan teknik doküman alınıp `22` güncellenir. |
| 6 | ING, Albaraka, DenizBank | Teknik doküman bankadan istenecek. O zamana kadar dosya kanalı. |
| 7 | Garanti B (API Store), QNB API portalı | Easytrade adına uygulama hesabı açıldıktan sonra |

Banka sırası, müşavirin mükellef listesindeki banka dağılımına göre değiştirilebilir. Portal, mükelleflerin `cash_box.bank_id` dağılımını raporlar. Bu rapor sırayı belirlemek için kullanılır.
