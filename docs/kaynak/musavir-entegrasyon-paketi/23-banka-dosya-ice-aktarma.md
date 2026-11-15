# 23 · Ekstre Yükleme (PDF, Excel, MT940)

Banka hesabını bağlamayan mükellefin yolu ve bağlayan mükellef için geçmiş dönem yolu (`20` §2). Mükellef bankasının internet şubesinden ekstreyi indirir, portala yükler. Şu durumlarda kullanılır:

- Mükellef banka hesabını bağlamak istemiyor veya şube başvurusunu henüz yapmadı.
- Bankanın servisi yok veya adaptörü henüz yazılmadı (`20` §3, `22` §13–15).
- Bankanın servisinin izin verdiği süreden eski geçmiş (Vakıfbank 365 gün, Akbank 2 ay).
- Garanti A kanalında uzun geçmiş (günde 1 gün, 10 dakika aralık sınırı).

Bütün dosyalar `21` §2 `bank_transaction` modeline yazılır. Servisten gelen hareketle mükerrer kontrolü `21` §5.

## 1. Kabul edilen biçimler

| Biçim | Nereden | Kalite | Not |
|---|---|---|---|
| **Excel / CSV** | İnternet şubesi "Excel'e aktar" | İyi: bankaya göre kolon yapısı değişir, banka şablonuyla otomatik tanınır | `04` motoruyla (§4) |
| **PDF** (metin tabanlı, e-imzalı dahil) | İnternet şubesi "PDF ekstre" | İyi | Tablo çıkarma, bakiye sürekliliği zorunlu doğrulama (§5) |
| **MT940 / MT942** | Bankaya göre internet şubesinden indirilebiliyor veya talimatla e-posta/SFTP ile gönderiliyor (Ziraat, Akbank, Garanti sayfalarında yazıyor) | Çok iyi: açılış/kapanış bakiyesi, işlem kodu, referans | §3 |
| PDF (taranmış görüntü) | Kâğıt ekstre fotoğrafı | Düşük | Kabul edilir ama OCR sonucu her satır müşavir onayına düşer |

Yükleme ekranında mükellefe banka seçimine göre "internet şubesinde ekstre nereden indirilir" kısa yönlendirmesi gösterilir (banka bazında metin, konfigürasyonda).

## 2. Yükleme akışı

1. Mükellef (veya müşavir) dosyayı yükler: `POST /accountant/bank-statements/import` (`21` §7). Mükellef uygulamasında da aynı ekran.
2. Hesap tespiti: dosyadaki IBAN (`TR\d{24}`) → `cash_box`. Bulunamazsa hesap seçimi istenir.
3. Banka tespiti: IBAN'ın 5–9. haneleri (banka kodu) → `bank_id` → banka şablonu.
4. Ayrıştırma (§3 MT940, §4 Excel, §5 PDF) → bakiye sürekliliği kontrolü → önizleme (eklenecek / mükerrer / hatalı satır sayısı, dönem toplamları).
5. Onay → `bank_transaction` yazılır, sınıflandırma ve eşleştirme çalışır (`21` §4, §6).

**Dönem hatırlatması:** Banka hesabı bağlı olmayan her `cash_box` (`type=bank_account`, `is_official=true`) için, ayın 1'inden itibaren önceki ayın ekstresi yüklenmemişse mükellefe hatırlatma gider; ayın 15'inde hâlâ yoksa müşavirin "beyana hazır" kontrol listesinde (`GET /accountant/periods/{period}/readiness`) eksik olarak görünür. Yüklenen ekstrenin tarih aralığı ayın tamamını kapsamıyorsa eksik günler gösterilir.

## 3. MT940 / MT942 ayrıştırıcı

### 3.1 Yapı

SWIFT MT940 standardı. Etiketler:

| Etiket | İçerik | Portal |
|---|---|---|
| `:20:` | Ekstre referansı | `raw_payload` |
| `:21:` | İlgili referans (opsiyonel) | `raw_payload` |
| `:25:` | Hesap tanımı: IBAN, ya da `banka kodu-şube kodu/hesap no` | `cash_box` eşleşmesi (IBAN veya `bank_account_number`) |
| `:28C:` (bazı bankalarda `:28:`) | Ekstre no / sayfa no | Sıra kontrolü |
| `:60F:` / `:60M:` | Açılış bakiyesi (F: ilk, M: ara sayfa) | `bank_daily_summary.opening_balance` |
| `:61:` | Hareket satırı | `bank_transaction` (§3.2) |
| `:86:` | Önceki `:61:` satırının açıklaması (çok satırlı) | `description` |
| `:62F:` / `:62M:` | Kapanış bakiyesi | `bank_daily_summary.closing_balance` |
| `:64:` | Kullanılabilir bakiye | `bank_balance_snapshot.available_balance` |
| `:65:` | İleri valörlü bakiye | `raw_payload` |

MT942: aynı yapı, `:34F:` (eşik), `:13D:` (oluşturma zamanı), `:90D:` / `:90C:` (borç/alacak toplamı). `:60F:`/`:62F:` yok.

Bakiye etiketi biçimi: `D|C` + `YYMMDD` + para birimi (3) + tutar (virgül ondalık). Örnek: `:60F:C261101TRY125430,50`.

### 3.2 `:61:` satırı

```
:61:2611011101C1250,00NTRFNONREF//00012345
     │     │   ││      │   │      └─ banka referansı (opsiyonel, "//" sonrası)
     │     │   ││      │   └──────── müşteri referansı (NONREF = yok)
     │     │   ││      └──────────── işlem türü: 1 harf (N/F/S) + 3 karakter kod (TRF, COM, CHG, INT, ...)
     │     │   │└─────────────────── tutar (virgül ondalık)
     │     │   └──────────────────── D / C / RD / RC (RD, RC: ters kayıt)
     │     └──────────────────────── kayıt tarihi MMDD (opsiyonel)
     └────────────────────────────── valör tarihi YYMMDD
```

| Parça | Portal |
|---|---|
| Valör tarihi | `value_date` |
| Kayıt tarihi (varsa, yoksa valör) | `transaction_date`. Yıl, valör yılından alınır; kayıt ayı valör ayından büyükse ve aralık > 6 ay ise yıl −1 (yıl sonu geçişi). |
| `C` / `D` | `A` / `B` → `debit_credit` |
| `RC` / `RD` | Ters kayıt. `RC` (alacak iptali, para çıkar) → `B`. `RD` (borç iptali, para girer) → `A`. `raw_payload.reversal=true`. Aynı tutarlı orijinal hareket bulunursa ikisi birlikte `is_cancelled` işaretlenir. |
| Para birimi kodu harfi (opsiyonel, 1 harf) | Hesabın para birimiyle doğrulanır |
| Tutar | `amount` (virgül → nokta) |
| İşlem kodu (3 karakter) | `mt940_code` (`21` §4.1 tablosu) |
| Müşteri referansı | `raw_payload` (`NONREF` ise boş) |
| `//` sonrası | `bank_reference` (yoksa `receipt_number` adayı) |

`:86:` alanı Türk bankalarında **serbest metin** olarak geliyor; alt alan yapısı bankaya göre değişiyor. Ayrıştırıcı:

1. Satırları birleştirir → `description`.
2. Banka profili varsa (`bank_id`'ye göre) bilinen kalıpları çıkarır: IBAN (`TR\d{24}`), VKN/TCKN (10/11 hane, etiketli olanlar: `VKN`, `TCKN`, `VERGI NO`), karşı taraf adı (`GONDEREN:`, `ALICI:`, `GÖND:` gibi etiketlerden), EFT sorgu no.
3. Çıkarılamayan her şey `description` içinde kalır. Uydurma alan doldurulmaz.

Banka profilleri ilk gerçek dosyalarla oluşturulur (örnek dosya toplama: §6).

### 3.3 Karakter seti ve dosya

- Kodlama sırayla denenir: UTF-8 (BOM'lu/BOM'suz) → Windows-1254 → ISO-8859-9. Türkçe karakter içeren `:86:` satırlarıyla doğrulanır.
- Satır sonu `CRLF` / `LF` ikisi de kabul.
- Tek dosyada birden çok ekstre (birden çok `:20:` bloğu, birden çok hesap) olabilir. Her blok ayrı işlenir.
- SWIFT başlık blokları (`{1:`, `{2:`, `{4:` … `-}`) varsa atılır.
- Açık kaynak MT940 kütüphanesi kullanılabilir. Şartı: §3.2'deki `RC`/`RD`, opsiyonel kayıt tarihi ve virgül ondalığı doğru işlemesi. Kabul testleri (§7) kütüphane seçiminden önce çalıştırılır.

### 3.4 Kontroller

- Her blokta: `:60F:` + Σ(`C`) − Σ(`D`) = `:62F:` (±0,01). Tutmazsa dosya reddedilmez ama `BNK_GAP` uyarısı.
- Ardışık günlerde: önceki günün `:62F:` = bugünün `:60F:`. Değilse arada eksik gün var.
- `:25:` hesabı bulunamazsa: müşavire "bu hesap tanımlı değil, yeni banka hesabı olarak ekle?" sorusu.

### 3.5 Tekil anahtar

MT940'ta hareket başına tekil numara her bankada yok. `source_id`:

```
sha256(iban | transaction_date | value_date | D/C | amount | mt940_code | bank_reference | ekstre içi sıra no)
```

Aynı günün dosyası tekrar gelirse (bankalar düzeltilmiş ekstre gönderebiliyor) aynı anahtar üretilir, mükerrer oluşmaz. Servisten gelen hareketle birleşme `21` §5 madde 2 (bakiye) ve 3 ile.

## 4. Excel / CSV ekstre

`04-excel-ice-aktarma.md` motoru aynen kullanılır. Eklenen veri türü:

| `data_type` | Ortak varlık | Asgari zorunlu kolonlar |
|---|---|---|
| `bank_statement` | `bank_transaction` | Tarih, açıklama, tutar (tek kolon işaretli **ya da** ayrı borç/alacak kolonları). Bakiye kolonu önerilir. |

Ek kurallar:

- **Hesap tespiti:** Banka Excel'lerinde IBAN / hesap no genelde başlık bölgesinde (ilk 10 satır) yazar. Yapı tespiti (`04` §4.1) bu bölgede `TR\d{24}` arar. Bulamazsa yükleme ekranında hesap seçimi zorunlu.
- **Banka tespiti:** IBAN'ın 5–9. haneleri banka kodudur. Banka kodu → `bank_id` tablosu ile `source_system` ve şablon otomatik seçilir.
- **Şablonlar:** Her banka için `global` şablon (`04` §6). Hedef alanlar: `transaction_date`, `value_date`, `description`, `amount` / `debit` / `credit`, `balance_after`, `receipt_number`, `bank_reference`, `channel`, `counterparty_name`.
- **Yön:** Ayrı borç/alacak kolonları varsa dolu olan yön belirler. Tek işaretli kolonda eksi = `B`.
- **Tutar biçimi:** Banka Excel'leri metin olarak `1.234,56` veya sayı olarak gelebilir. `04` §4.4 dönüşümü + bakiye sürekliliği ile çapraz kontrol (yanlış ondalık yorumu bakiye kontrolünü bozar, motor otomatik fark eder).
- **AI eşleme:** `04` §4.3 aynen. Banka ekstresinde AI'a giden örnek satırlarda IBAN, TCKN, VKN ve kişi adları maskelenir.
- **Doğrulama (bank_statement'a özel):** bakiye kolonu varsa satır sırasıyla süreklilik (`20` §7.4) zorunlu. Tutmayan satır `has_errors`'a düşer (genelde sıralama ters, motor önce ters sırayı dener).

## 5. PDF ekstre

1. **Metin tabanlı PDF:** Tablo çıkarma (sayfa bazında, sütun sınırları başlık satırından). Sonuç `04` motoruna "sanal Excel" olarak verilir, aynı şablon mantığıyla eşlenir.
2. **Taranmış PDF / görsel:** OCR. Her satır `capture_method=pdf_import`, `kind_confidence` en fazla 0,6 ve müşavir onayı zorunlu.
3. **Zorunlu kontrol:** İlk satırın önceki bakiyesi + hareketler = son bakiye. Tutmazsa içe aktarma `preview` aşamasına geçmez.
4. E-imzalı PDF ekstrelerde imza bilgisi `attachment` kaydında saklanır (bankanın gerçek belgesi olduğunun kanıtı).

## 6. Toplanacak örnek dosyalar

Şablon ve `:86:` profilleri gerçek dosyayla yapılır. İlk iş: Easytrade'in kendi hesaplarından ve gönüllü mükelleflerden, her banka için:

| Dosya | Bankalar |
|---|---|
| MT940 (1 haftalık, en az 1 gelen EFT, 1 giden EFT, 1 masraf, 1 POS) | Ziraat, Vakıfbank, Halkbank, İş Bankası, Garanti, Yapı Kredi, Akbank, QNB, DenizBank, TEB, ING, Kuveyt Türk |
| İnternet şubesi Excel | Aynı liste + Enpara, Fibabanka, Albaraka, Türkiye Finans, Şekerbank |
| İnternet şubesi PDF | Aynı liste |

Örnek dosyalar kişisel veri içerir. Anonimleştirilmiş kopyaları test verisi olarak saklanır, orijinaller silinir.

## 7. Kabul testleri

1. Örnek MT940 dosyası: hareket adedi, Σ alacak, Σ borç ve kapanış bakiyesi bankanın internet şubesiyle birebir.
2. `RC`/`RD` ters kayıt doğru yönde ve orijinal hareketle bağlanıyor.
3. Yıl sonu geçişi (31 Aralık valör, 2 Ocak kayıt) doğru yıla yazılıyor.
4. Birden çok hesaplı tek MT940 dosyası doğru `cash_box`'lara dağılıyor.
5. Aynı MT940 iki kez yüklenince mükerrer oluşmuyor.
6. Servisten gelmiş bir gün MT940 ile tekrar yüklenince mükerrer oluşmuyor, boş alanlar dolduruluyor.
7. Windows-1254 kodlu dosyada Türkçe karakterler doğru.
8. Banka Excel'inde IBAN başlıktan bulunuyor, banka ve şablon otomatik seçiliyor.
9. Ters sıralı (yeniden eskiye) Excel'de bakiye sürekliliği doğru kuruluyor.
10. Mükellefin kendi `cash_box` IBAN'larında olmayan bir hesabın ekstresi yüklenince dosya işlenmiyor, "yeni hesap olarak ekle?" sorusu çıkıyor.
