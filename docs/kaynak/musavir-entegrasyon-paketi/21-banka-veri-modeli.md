# 21 · Banka Veri Modeli, Sınıflandırma ve Eşleştirme

Bütün banka kanalları (web servisi, banka API'si, PDF, Excel, MT940) veriyi **bu şemaya** yazar. Kontrol katmanı sadece bu tabloları okur.

İşaretler `01` ile aynı: (yok) Easytrade'de aynı adla var · **⊕** portal uzantısı · **⚠️** doğrulanmalı.

Easytrade API'de banka ekstresi kavramı yok. Easytrade'de banka hesabı `cash_box` (`type=bank_account`), para hareketi `financial_transaction`'dır. Bu yüzden:

- Banka hesabı = **`cash_box`** (Easytrade alanları + birkaç ⊕ alan).
- Bankadan gelen ham hareket = **`bank_transaction` ⊕**. Bu bir kanıt kaydıdır, muhasebe kaydı değildir.
- Eşleştirme sonucunda `bank_transaction`, Easytrade adlarıyla tanımlı varlıklara (`financial_transaction`, `expense`, `cash_box_transfer`, `personnel_transaction`, `check`, `invoice`) bağlanır veya onlar için **öneri** üretir.

## 1. Varlıklar

```
company
 ├── source_connection (source_system = 'bank_<bank_id>', 'bank_file')
 ├── cash_box (type = bank_account | credit_card | valor)
 │     ├── bank_transaction ⊕ ──► bank_transaction_match ⊕ ──► financial_transaction / expense / cash_box_transfer / ...
 │     ├── bank_balance_snapshot ⊕
 │     └── bank_daily_summary ⊕
 ├── bank_rule ⊕ (mükellefe özel öğrenilmiş sınıflandırma kuralları)
 └── attachment (kind = bank_receipt_pdf | bank_statement_file)
```

### 1.1 `cash_box` alan eklemeleri (`01` §4.8'e ek)

| Alan | Easytrade | Açıklama |
|---|---|---|
| bank_id | `bank_id.value` | Easytrade enum. Enum'da olmayan banka için ⊕ değer (`teb`, `ziraat-katilim`, `vakif-katilim`, `sekerbank`, `odeabank`, `anadolubank`, `emlak-katilim`, `hsbc`, `other`). |
| iban | `iban` | Boşluksuz, büyük harf. **Banka hareketini hesaba bağlayan ana anahtar.** |
| currency | `currency` | |
| bank_account_number | ⊕ | Bankanın hesap no'su (bazı servisler IBAN yerine hesap no ister: Ziraat 12 hane, Vakıfbank `00158…` 17 hane, TEB 8 hane + şube) |
| bank_branch_code | ⊕ | |
| bank_account_suffix | ⊕ | Ek no (Ziraat `EkNo`, Kuveyt Türk / Vakıf Katılım `AccountSuffix`) |
| bank_customer_no | ⊕ | Bankadaki müşteri no |
| bank_account_kind | ⊕ | `demand` (vadesiz), `term` (vadeli), `credit` (KMH / kredi hesabı), `investment`, `gold`, `other` |
| bank_connection_id | ⊕ | Hangi banka bağlantısından besleniyor (`source_connection.id`) |
| bank_sync_from | ⊕ | Banka servisinden çekilen en eski tarih. Öncesi dosya ile tamamlanır. |

### 1.2 `bank_balance_snapshot` ⊕

| Alan | Açıklama |
|---|---|
| cash_box_id | |
| captured_at | TIMESTAMPTZ |
| balance | Hesap bakiyesi |
| available_balance | Kullanılabilir bakiye |
| blocked_amount | Bloke tutar |
| credit_limit | KMH / kredi limiti |
| available_with_credit | Limit dahil kullanılabilir |
| source | `api`, `mt940`, `excel_import`, `pdf_import` |

Dikkat: Bazı bankalarda yanıttaki hesap seviyesi "Bakiye" alanı sorgu aralığının son bakiyesidir, güncel bakiye değildir (Halkbank'ta güncel bakiye `KullanilabilirBakiye`; Vakıfbank'ta hesap bakiyeleri sorgu anına aittir). Hangi alanın ne olduğu `22`'de banka bazında.

### 1.3 `bank_daily_summary` ⊕

Banka gün sonu özeti veriyorsa (Vakıfbank `GetirHareketOzet`, Ziraat `SorgulaOzetWS`) veya MT940 `:60F:`/`:62F:` varsa.

| Alan | Açıklama |
|---|---|
| cash_box_id, date | |
| opening_balance, closing_balance | |
| total_credit, total_debit | |
| transaction_count | |

Bakiye sürekliliği kontrolünde kullanılır (`20` §7.4).

## 2. `bank_transaction` ⊕

| Alan | Tip | Açıklama |
|---|---|---|
| id | UUID | |
| company_id | UUID | |
| cash_box_id | UUID | IBAN / hesap no ile bulunur |
| source_system | ENUM | `bank_ziraat`, `bank_vakifbank`, …, `bank_file` |
| source_id | TEXT | Bankanın tekil hareket anahtarı (`22`'de her bankada). Yoksa hash (`20` §7.3). |
| capture_method | ENUM | `api`, `mt940`, `excel_import`, `pdf_import` |
| booked_at | TIMESTAMPTZ | İşlem tarihi + saati (`Europe/Istanbul`). Saat yoksa 00:00. |
| has_time | BOOL | Banka saat verdi mi. `false` ise sıralama ve süreklilik kontrolünde banka yanıtındaki sıra kullanılır. |
| bank_sequence | INT | Banka yanıtındaki / dosyadaki sıra (aynı gün içi sıralama için) |
| transaction_date | DATE | İşlem (muhasebe) tarihi. **Dönem bu tarihe göre atanır.** |
| value_date | DATE | Valör |
| debit_credit | CHAR(1) | `A` (hesaba giriş, alacak) / `B` (hesaptan çıkış, borç). Bankaların servislerinde kullandığı gösterge (`BorcAlacak`, `BA`). |
| amount | NUMERIC(18,4) | **Pozitif.** Yön `debit_credit` ile. |
| currency | CHAR(3) | `TRY`, `USD`, `EUR`, … (`TL` → `TRY`) |
| balance_after | NUMERIC(18,4) | İşlem sonrası bakiye (banka veriyorsa). Eksi olabilir (KMH). |
| balance_before | NUMERIC(18,4) | Banka veriyorsa (Vakıfbank `IslemOncesiBakiye`) |
| description | TEXT | Bankanın ekstre açıklaması, olduğu gibi |
| description_extra | TEXT | İkinci açıklama alanı (Halkbank `Aciklama`, Türkiye Finans `DescriptionTwo`, TEB `ISLEM_ACK`) |
| bank_reference | TEXT | Banka referans no |
| receipt_number | TEXT | Dekont no |
| bank_txn_code | TEXT | Bankanın kendi işlem kodu, ham (Ziraat `IslemTipi`, Garanti `TransactionId`, TEB `ISLEM_TUR`, Vakıfbank `IslemKodu`) |
| mt940_code | VARCHAR(4) | MT940 işlem kodu (banka veriyorsa: `TRF`, `COM`, `CHG`, `INT`, `LDP`, `CHK`, …) |
| channel | TEXT | İşlem kanalı (internet, ATM, şube, mobil), banka veriyorsa |
| counterparty_name | TEXT | Karşı taraf adı / unvanı |
| counterparty_iban | VARCHAR(34) | |
| counterparty_tax_number | VARCHAR(10) | VKN |
| counterparty_identity_number | VARCHAR(11) | TCKN |
| counterparty_bank_code | TEXT | |
| eft_query_number | TEXT | EFT sorgu no |
| payment_purpose | VARCHAR(3) | EFT ödeme amacı kodu, banka döndürürse (Vakıfbank `Detaylar.EftIslemTuru`: `M01`–`M09`, `K14`), §3.3 |
| is_cancelled | BOOL | İptal edilmiş hareket (Halkbank `Iptal=E`, Ziraat `kayitDurumu=I`) |
| kind | ENUM | **Merkezi işlem türü** (§3.1) |
| kind_detail | ENUM | Beyan için alt tür (§3.2) |
| kind_source | ENUM | `bank_code`, `mt940_code`, `payment_purpose`, `counterparty`, `keyword`, `company_rule`, `ai`, `manual` |
| kind_confidence | NUMERIC(3,2) | 0–1 |
| match_status | ENUM | `unmatched`, `suggested`, `matched`, `ignored` |
| period | CHAR(7) | `YYYY-MM` |
| raw_payload | JSONB | Ham banka kaydı (alan seviyesinde şifreli, `20` §8) |
| content_hash | TEXT | |
| created_at, updated_at | TIMESTAMPTZ | |

`bank_transaction` hareketin **tek** kaydıdır. Aynı hareket birden fazla kanaldan gelebildiği için (servis + Excel + PDF) kaynak bilgisi ayrı tabloda tutulur:

**`bank_transaction_source` ⊕**

| Alan | Açıklama |
|---|---|
| bank_transaction_id | Bağlı olduğu hareket |
| source_system | `bank_vakifbank`, …, `bank_file` |
| capture_method | `api`, `mt940`, `excel_import`, `pdf_import` |
| source_id | Bu kanaldaki tekil anahtar |
| import_id | Dosya yüklemesiyse yükleme kaydı (`04` / `23`). Geri alma bununla yapılır. |
| raw_payload | Bu kanaldan gelen ham kayıt |
| received_at | |

Benzersiz anahtar: `bank_transaction_source (company_id, cash_box_id, source_system, source_id)`. `bank_transaction` üzerindeki `source_system`, `source_id`, `capture_method`, `raw_payload` alanları **birincil kaynağın** kopyasıdır (§5 öncelik sırası). Bir dosya yüklemesi geri alınırsa sadece o yüklemenin `bank_transaction_source` satırları silinir; başka kaynağı kalmayan `bank_transaction` silinir, kalan varsa birincil kaynak yeniden seçilir.

### 2.1 Normalizasyon kuralları

| Konu | Kural |
|---|---|
| Tutar biçimi | Bankaya göre ondalık ayırıcı farklı. Halkbank `+1234,56` (virgül ondalık, işaretli), Garanti `27,299.33` (virgül binlik, nokta ondalık), Ziraat `8208.00`, Fibabanka `-555`. **Genel bir "akıllı" ayrıştırıcı kullanılmaz.** Her adaptör kendi bankasının biçimini sabit kuralla çevirir (`22`). |
| Yön | Banka ayrı yön alanı veriyorsa (`BorcAlacak`, `BA`, `HareketTipi`) o esas alınır. Vermiyorsa tutarın işareti. Yön kodun kendisinden çıkarılmaz (Ziraat dokümanı da bunu söylüyor: aynı kod hem borç hem alacak olabilir). |
| Tarih | Bankaların tarih biçimleri farklı (`yyMMdd`, `dd.MM.yyyy`, `dd/MM/yyyy`, `yyyyMMdd`, `yyyy-MM-dd HH:mm:ss`, 20 haneli zaman damgası). Adaptör sabit biçimle ayrıştırır. Saat dilimi `Europe/Istanbul`. |
| IBAN | Boşluksuz, büyük harf, `TR` + 24 hane doğrulaması (mod 97). Maskeli veya eksik IBAN `counterparty_iban`'a yazılmaz, `raw_payload`'da kalır. |
| VKN / TCKN | Sadece rakam. 10 hane → `counterparty_tax_number`, 11 hane → `counterparty_identity_number`. `0000000000` gibi doldurma değerleri `NULL`. |
| Açıklama | Olduğu gibi saklanır. Sınıflandırma için ayrıca normalize edilmiş kopya üretilir: önce **Türkçe yerel ayarla** büyük harfe (`i→İ`, `ı→I`), sonra ASCII'ye (`İ→I`, `Ş→S`, `Ğ→G`, `Ü→U`, `Ö→O`, `Ç→C`), noktalama boşluğa, fazla boşluk tek boşluğa. |
| Para birimi | `TL`, `YTL`, `TRL` → `TRY`. Akbank gibi bankaya özel sayısal kodlar (`888` = TRY) adaptörde çevrilir. |

## 3. Merkezi işlem türü

### 3.1 `kind`: ÖHVPS işlem türü listesi

Merkezi dil olarak **TCMB'nin açık bankacılık düzenlemesi için BKM'nin yayımladığı standarttaki işlem türü listesi** (`TR.OHVPS.DataCode.IslemTuru`, ÖHVPS v2.0.1 Ekler) birebir kullanılır. Kendi listemizi icat etmiyoruz; bankaların hesap hareketlerini sınıflandırmak için düzenleyicinin belirlediği liste bu. Portal açık bankacılık servisini **kullanmıyor**, sadece listeyi adlandırma standardı olarak alıyor. Her bankanın kendi işlem kodu bu listeye çevrilir (§4.1).

| `kind` | Anlamı |
|---|---|
| `HAVALE` | Banka içi transfer |
| `EFT` | Bankalar arası EFT |
| `FAST` | FAST transfer |
| `PARA_YATIRMA` | Nakit yatırma |
| `PARA_CEKME` | Nakit çekme |
| `YABANCI_PARA_HAVALE` | Döviz transferi (SWIFT dahil) |
| `YATIRIM_HESABINA_AKTARIM` | Yatırım hesabına aktarım |
| `YATIRIM_HESABINDAN_AKTARIM` | Yatırım hesabından aktarım |
| `KURUM_FATURA_ODEMESI` | Elektrik, su, doğalgaz, telefon vb. fatura ödemesi |
| `CEK` | Çek |
| `SENET` | Senet |
| `SIGORTA_ODEMESI` | Sigorta |
| `UCRET_KOMISYON_FAIZ` | Ücret, komisyon, faiz |
| `SGK_ODEMESI` | SGK |
| `VERGI_ODEMESI` | Vergi |
| `DOVIZ_ALIM` | Döviz alımı |
| `DOVIZ_SATIM` | Döviz satımı |
| `KREDI_ODEMESI` | Kredi geri ödemesi |
| `KREDI_KULLANIM` | Kredi kullanımı |
| `KK_ODEMESI` | Kredi kartı borç ödemesi |
| `KK_NAKIT_AVANS` | Kredi kartı nakit avans |
| `SANS_OYUNU` | Şans oyunu |
| `UYE_ISYERI_ISLEMLERI` | POS / üye işyeri |
| `HGS_OGS_ISLEMLERI` | HGS / OGS |
| `DOGRUDAN_BORCLANDIRMA_SISTEMI` | DBS |
| `DIGER` | Diğer |

### 3.2 `kind_detail` ⊕: beyan için alt tür

ÖHVPS listesi bazı beyan ayrımlarını tek başlıkta topluyor (ör. ücret + komisyon + faiz). Portal bunları ayırır:

| `kind_detail` | Ait olduğu `kind` | Beyandaki yeri |
|---|---|---|
| `customer_collection` | HAVALE, EFT, FAST, CEK, SENET | Tahsilat → satış faturası eşleşmesi |
| `supplier_payment` | HAVALE, EFT, FAST, CEK, SENET, DBS | Ödeme → alış faturası eşleşmesi |
| `own_account_transfer` | HAVALE, EFT, FAST, PARA_YATIRMA, PARA_CEKME, DOVIZ_* , YATIRIM_* , KK_ODEMESI | Virman (gelir/gider değil) |
| `bank_fee` | UCRET_KOMISYON_FAIZ | Banka masrafı (gider) |
| `commission` | UCRET_KOMISYON_FAIZ | Komisyon (EFT, teminat mektubu, çek vb.) |
| `bsmv` | UCRET_KOMISYON_FAIZ | BSMV (gider, KDV değil) |
| `interest_expense` | UCRET_KOMISYON_FAIZ, KREDI_ODEMESI | Kredi / KMH faizi (finansman gideri) |
| `interest_income` | UCRET_KOMISYON_FAIZ | Mevduat / katılım payı geliri |
| `withholding_tax` | UCRET_KOMISYON_FAIZ, VERGI_ODEMESI | Faiz gelirinden kesilen stopaj |
| `loan_principal` | KREDI_ODEMESI | Anapara (gider değil) |
| `loan_disbursement` | KREDI_KULLANIM | Kredi girişi (gelir değil) |
| `tax_payment` | VERGI_ODEMESI | Vergi ödemesi (KDV, muhtasar, geçici vergi, MTV, damga vb.) |
| `sgk_payment` | SGK_ODEMESI | SGK primi |
| `salary_payment` | HAVALE, EFT, FAST | Maaş ödemesi |
| `rent_payment` | HAVALE, EFT, FAST | Kira ödemesi (stopaj kontrolü) |
| `utility_payment` | KURUM_FATURA_ODEMESI | Fatura ödemesi |
| `pos_settlement` | UYE_ISYERI_ISLEMLERI | POS tahsilatının hesaba geçişi |
| `pos_commission` | UYE_ISYERI_ISLEMLERI | POS komisyonu |
| `insurance_payment` | SIGORTA_ODEMESI | |
| `vehicle_toll` | HGS_OGS_ISLEMLERI | |
| `fx_trade` | DOVIZ_ALIM, DOVIZ_SATIM | Kur farkı için |
| `cash_deposit`, `cash_withdrawal` | PARA_YATIRMA, PARA_CEKME | Kasa ↔ banka |
| `private_or_non_business` | SANS_OYUNU, DIGER | Müşavire "işletmeyle ilgisiz olabilir" uyarısı |
| `unknown` | hepsi | Sınıflanamadı, müşavir bakacak |

### 3.3 Ödeme amacı kodları

İncelenen banka servisleri içinde ödeme amacını ayrı alan olarak veren tek doküman **Vakıfbank** (`Detaylar.EftIslemTuru`). Diğer bankalarda bu bilgi yok; varsa açıklama metninin içindedir.

| Kod (Vakıfbank dokümanı) | Anlamı | Portal |
|---|---|---|
| `M01` | Hesaba ödeme | — |
| `M02` | İsme ödeme | — |
| `M03` | Kamu ödemesi | `tax_payment` adayı |
| `M04` | Konut kirası | `rent_payment` |
| `M05` | İşyeri kirası | `rent_payment` |
| `M06` | Diğer kira | `rent_payment` |
| `M07` | Maaş | `salary_payment` |
| `M08` | Kredi kartı ödemesi | `KK_ODEMESI` |
| `M09` | Ödemenin iadesi | — |
| `K14` | Muhabirlik ödemesi | — |

## 4. Sınıflandırma

Her `bank_transaction` için sırayla çalışır. İlk kesin sonuç kazanır. AI sadece en sonda ve sadece öneri olarak.

| Adım | Kaynak | Örnek | `kind_source` | Güven |
|---|---|---|---|---|
| 1 | Karşı taraf IBAN'ı mükellefin kendi `cash_box` IBAN'larından biri | Kendi Ziraat → kendi Garanti | `counterparty` | 1,00 |
| 2 | Bankanın işlem kodu tablosu (§4.1) | Ziraat `218F` → masraf · Fibabanka `LDP` → kredi anapara | `bank_code` / `mt940_code` | 0,95 |
| 3 | Ödeme amacı (§3.3, sadece banka veriyorsa) | `M05` → işyeri kirası | `payment_purpose` | 0,95 |
| 4 | Mükellefe özel öğrenilmiş kural (`bank_rule`) | Bu mükellefte "XYZ LTD" → tedarikçi ödemesi | `company_rule` | 0,90 |
| 5 | Karşı taraf VKN/TCKN veya IBAN'ı bir `customer` kaydında | `customer.type=supplier` → `supplier_payment` | `counterparty` | 0,90 |
| 6 | Anahtar kelime kuralları (§4.2) | `BSMV` → `bsmv` | `keyword` | 0,80 |
| 7 | AI önerisi | Kalanlar | `ai` | Modelin verdiği, en fazla 0,70 |

- Güveni 0,80'in altındaki sınıflandırma müşavire "kontrol et" olarak gösterilir.
- Müşavirin düzelttiği her sınıflandırma, aynı karşı taraf / açıklama kalıbı için **`bank_rule`** olarak kaydedilir (mükellef kapsamı). Aynı düzeltme 3+ mükellefte yapılırsa ofis kapsamına önerilir.
- AI'a giden veride IBAN, TCKN, VKN maskelenir. AI sadece `kind` / `kind_detail` önerir. Tutar, tarih, eşleştirme kararı AI'a bırakılmaz.

### 4.1 Banka işlem kodu tabloları

Bankaların dokümanlarında verilen kodlar. Tablo kodda **veri olarak** tutulur (`bank_code_map`), banka başına genişletilir. Dokümanda olmayan kodlar ilk senkronlarda loglanır ve tabloya eklenir.

**MT940 işlem kodları.** Tam liste sadece Fibabanka dokümanında var. Halkbank (`IslemKod`) ve Akbank (`MT940` alanı) dokümanları da MT940 kodu döndürdüğünü söylüyor ama sadece örnek veriyor (Halkbank: `MSC`, `TRF`, `FEX`; Akbank: `EFT`, `TRF`, `VRM`, `CCP`). Bu bankalarda Fibabanka listesinde olmayan kod gelirse `DIGER` yazılır ve loglanır:

| Kod | Fibabanka dokümanındaki anlamı | `kind` | `kind_detail` |
|---|---|---|---|
| `TRF` | Havale, giden EFT, gelen EFT | yöne göre `HAVALE` / `EFT` | karşı tarafa göre |
| `COM` | Komisyonlar (ilgili vergiler dahil) | `UCRET_KOMISYON_FAIZ` | `commission` (açıklamada BSMV varsa `bsmv`) |
| `CHG` | Hesap işletim / yıllık ücret, komisyon vergi ve masrafları | `UCRET_KOMISYON_FAIZ` | `bank_fee` |
| `INT` | KMH veya kredi faiz tahakkuku / tahsilatı | `UCRET_KOMISYON_FAIZ` | `B` → `interest_expense`, `A` → `interest_income` |
| `LDP` | Kredi anapara ödemeleri | `KREDI_ODEMESI` | `loan_principal` |
| `CHK` | Çek | `CEK` | yöne göre |
| `BOE` | Senet / poliçe (bordro) | `SENET` | yöne göre |
| `PRO` | Senet | `SENET` | yöne göre |
| `BND` | Tahvil alım satım | `YATIRIM_HESABINA_AKTARIM` / `YATIRIM_HESABINDAN_AKTARIM` | |
| `EXP` | İhracat akreditif, vesaik | `YABANCI_PARA_HAVALE` | `customer_collection` |
| `IMC`, `IMP` | İthalat | `YABANCI_PARA_HAVALE` | `supplier_payment` |
| `FEX` | Döviz (Halkbank örneği) | `DOVIZ_ALIM` / `DOVIZ_SATIM` | `fx_trade` |
| `MSC` | Çeşitli (Halkbank örneği) | `DIGER` | anahtar kelimeye bırakılır |
| `EFT` | EFT (Akbank örneği) | `EFT` | karşı tarafa göre |
| `VRM` | Virman (Akbank örneği) | `HAVALE` | adım 1 ile kontrol |
| `CCP` | ⚠️ Akbank örneği, anlamı dokümanda yok | `DIGER` | ilk senkronda doğrulanacak |

**Ziraat Bankası `IslemTipi`** (Ziraat dokümanı):

| Kod | Dokümandaki açıklama | `kind` |
|---|---|---|
| `211A` | Havale (alacak) | `HAVALE` |
| `211M` | ATM/IVR/internet/düzenli havale, virman, EFT iade, gelen EFT otomatik aktarım (alacak) | açıklamaya göre `HAVALE` / `EFT` |
| `212T` | Havale, EFT, virman, düzenli talimat, ATM/internet (borç) | açıklamaya göre `HAVALE` / `EFT` |
| `212C` | Takas provizyonu, çek ödeme/iptal | `CEK` |
| `212X` | Tanımsız işlem tipi | `DIGER` (anahtar kelimeye bırakılır) |
| `214A` | İthalat akreditif, poliçe, transfer | `YABANCI_PARA_HAVALE` |
| `218F` | Serbest masraf alma | `UCRET_KOMISYON_FAIZ` → `bank_fee` |
| `221K` | Kredi kullandırım ve geri ödemeleri, taksit, faiz tahsilatı | `A` → `KREDI_KULLANIM`, `B` → `KREDI_ODEMESI` |
| `300D` | Döviz/efektif alış-satış | `DOVIZ_ALIM` / `DOVIZ_SATIM` |
| `300K` | Teminat mektubu giriş ve komisyon | `UCRET_KOMISYON_FAIZ` → `commission` |
| `300V` | Vadesiz hesap kapama | `DIGER` |
| `300Y` | Nakil hesap açma/kapama, vadeli faiz tahakkuk | açıklamada faiz varsa `interest_income` |
| `400I` | İhracat bedeli alış | `YABANCI_PARA_HAVALE` → `customer_collection` |

**Vakıf Katılım `TransactionType`** (Vakıf Katılım dokümanı): `1` gelen EFT · `2` muhabirden gelen EFT · `3` gelen SWIFT · `4` havale · `5` nakit yatırma · `6` gelen PTT havale · `7` bağış tahsilatı · `8` giden EFT · `9` nakit çekme · `10` giden SWIFT · `11` muhabire giden EFT. Kuveyt Türk aynı çekirdek bankacılık altyapısını (BOA) kullanıyor, aynı tablo ⚠️ doğrulanarak uygulanır.

**TEB `ISLEM_TUR`**: dokümanda 15 kod var (`22`, `23`, `24`, `25`, `28`, `63`, `227`, `260`, `378`, `418`, `624`, `626`, `633`, `688`, `1207`). Bilinenler: `22` şubeler arası TL havale, `1207` DBS tahsilatı. Diğerleri TEB dokümanından birebir alınıp tabloya girilir (`22` §6).

**Garanti `TransactionId`** (işlem kodu, ör. `EL07`, `ELMP`, `WPSO`, `AT03`): Dokümanda anlam listesi yok. İlk senkronlarda açıklama ile birlikte loglanır, tablo öğrenilerek doldurulur.

### 4.2 Anahtar kelime kuralları

Normalize açıklama (§2.1) üzerinde. Kurallar kodda veri olarak tutulur, sıra önemlidir (ilk eşleşen). **Eşleşme kelime bazında yapılır, alt dize ile değil** (`POS` kelimesi `POSTA` içinde, `ASKI` kelimesi `MASKI` içinde eşleşmez). Çok kelimeli kalıplar ardışık kelime dizisi olarak aranır. Bu tablo başlangıç setidir; gerçek ekstrelerle (`23` §6) doğrulanıp genişletilir.

| Kalıp (normalize metinde) | `kind` | `kind_detail` |
|---|---|---|
| `BSMV` | UCRET_KOMISYON_FAIZ | `bsmv` |
| `STOPAJ`, `GV KESINTI`, `GELIR VERGISI KESINTISI` | UCRET_KOMISYON_FAIZ | `withholding_tax` |
| `FAIZ` + yön `A` | UCRET_KOMISYON_FAIZ | `interest_income` |
| `FAIZ`, `KMH FAIZ`, `TEMERRUT` + yön `B` | UCRET_KOMISYON_FAIZ | `interest_expense` |
| `KAR PAYI` + yön `A` (katılım bankaları) | UCRET_KOMISYON_FAIZ | `interest_income` |
| `KOMISYON`, `EFT UCRETI`, `HAVALE UCRETI` | UCRET_KOMISYON_FAIZ | `commission` |
| `HESAP ISLETIM`, `MASRAF`, `UCRET`, `KART AIDATI`, `SMS UCRETI` | UCRET_KOMISYON_FAIZ | `bank_fee` |
| `POS`, `UYE ISYERI`, `BLOKE COZUM`, `GUN SONU` + yön `A` | UYE_ISYERI_ISLEMLERI | `pos_settlement` |
| `POS KOMISYON`, `UYE ISYERI KOMISYON` | UYE_ISYERI_ISLEMLERI | `pos_commission` |
| `SGK`, `SSK`, `SOSYAL GUVENLIK`, `PRIM BORCU` | SGK_ODEMESI | `sgk_payment` |
| `VERGI`, `GIB`, `GELIR IDARESI`, `KDV`, `MUHTASAR`, `GECICI VERGI`, `MTV`, `DAMGA` | VERGI_ODEMESI | `tax_payment` |
| `MAAS`, `UCRET ODEMESI`, `PERSONEL` | (transfer türü) | `salary_payment` |
| `KIRA` | (transfer türü) | `rent_payment` |
| `ELEKTRIK`, `DOGALGAZ`, `SU IDARESI`, `ISKI`, `ASKI`, `IGDAS`, `TELEKOM`, `TURKCELL`, `VODAFONE`, `INTERNET` | KURUM_FATURA_ODEMESI | `utility_payment` |
| `HGS`, `OGS` | HGS_OGS_ISLEMLERI | `vehicle_toll` |
| `SIGORTA`, `KASKO`, `POLICE` | SIGORTA_ODEMESI | `insurance_payment` |
| `KREDI KARTI ODEME`, `KK ODEME`, `KART BORCU` | KK_ODEMESI | kendi kartıysa `own_account_transfer` |
| `NAKIT AVANS` | KK_NAKIT_AVANS | |
| `KREDI TAKSIT`, `TAKSIT TAHSIL` | KREDI_ODEMESI | `loan_principal` (faiz ayrımı müşavir) |
| `KREDI KULLANDIRIM`, `KREDI ACILIS` | KREDI_KULLANIM | `loan_disbursement` |
| `CEK` | CEK | yöne göre |
| `SENET` | SENET | yöne göre |
| `DBS` | DOGRUDAN_BORCLANDIRMA_SISTEMI | `supplier_payment` |
| `DOVIZ ALIS`, `DOVIZ SATIS`, `EFEKTIF` | DOVIZ_ALIM / DOVIZ_SATIM | `fx_trade` |
| `ATM` + yön `B`, `NAKIT CEKIM` | PARA_CEKME | `cash_withdrawal` |
| `ATM` + yön `A`, `NAKIT YATIR` | PARA_YATIRMA | `cash_deposit` |
| `FAST` | FAST | karşı tarafa göre |
| `EFT` | EFT | karşı tarafa göre |
| `HAVALE`, `VIRMAN` | HAVALE | karşı tarafa göre |
| `IDDAA`, `MILLI PIYANGO`, `SANS OYUN` | SANS_OYUNU | `private_or_non_business` |


## 5. Kanallar arası mükerrer

Aynı hareket birden fazla kanaldan gelebilir (servis + MT940 + Excel yüklemesi).

1. Aynı `cash_box_id` + aynı kanal + aynı `source_id` → aynı kayıt (`bank_transaction_source` üzerinden).
2. Değilse: aynı `cash_box_id` + `transaction_date` + `debit_credit` + `amount` + `balance_after` → aynı kayıt (yürüyen bakiye hareketi tekil kılar).
3. Bakiye yoksa: aynı `cash_box_id` + `transaction_date` + `debit_credit` + `amount` + normalize açıklamanın ilk 30 karakteri → **olası mükerrer**, müşavir onayı.

Eşleşen kayıt yeni bir `bank_transaction` oluşturmaz, mevcut harekete yeni bir `bank_transaction_source` satırı ekler. Birincil kaynak önceliği: `api` > `mt940` > `excel_import` > `pdf_import`. Düşük öncelikli kanaldaki ek bilgi (ör. Excel'deki ayrı açıklama sütunu) boş alanlara yazılır, dolu alanı ezmez.

## 6. Beyan varlıklarıyla eşleştirme

### 6.1 Ne ile eşleşir

| `kind_detail` | Aranan kayıt | Eşleşme yoksa portalın önerisi (Easytrade alan adlarıyla) |
|---|---|---|
| `customer_collection` | `financial_transaction` (`transaction_type=income`) veya açık `invoice` (`direction=outgoing`) | `financial_transaction`: `transaction_type=income`, `payment_method_type=bank_transfer`, `cash_box_id`, `customer_id` (VKN/IBAN'dan), `amount`, `transaction_date`, `reference_document_no=receipt_number` + **"faturasız tahsilat" uyarısı** |
| `supplier_payment` | `financial_transaction` (`expense`) veya açık `invoice` (`direction=incoming`) | `financial_transaction` (`transaction_type=expense`) + **"belgesiz ödeme" uyarısı** |
| `own_account_transfer`, `cash_deposit`, `cash_withdrawal` | `cash_box_transfer` | `cash_box_transfer`: `from_cash_box_id`, `to_cash_box_id`, `payment_method_type=bank_transfer`, `amount`, `transaction_date` |
| `bank_fee`, `commission`, `bsmv`, `pos_commission` | `expense` | `expense`: `expense_category_id` (canonical `bank_fee`), `tax_rate=0_kdv`, `receipt_type=bank_fee` ⊕, `payment_method=bank_transfer`, `cash_box_id`, `is_paid=true`, `amount`, `expense_date` |
| `interest_expense` | `expense` | `expense` (kategori canonical `finance_expense`, `tax_rate=0_kdv`) |
| `interest_income`, `withholding_tax` | — | Gelir kaydı önerisi (`expense_category.scope=income`, canonical `interest_income`) + stopaj tutarı ayrı satır |
| `tax_payment` | `expense` (`receipt_type=tax_payment`) veya `financial_transaction` (`transaction_subtype=tax_payment`) | `financial_transaction`: `transaction_type=expense`, `transaction_subtype=tax_payment` ⊕ |
| `sgk_payment` | Aynı | `transaction_subtype=tax_payment`, kategori canonical `social_security` |
| `salary_payment` | `personnel_transaction` (`salary_payment`) | Karşı taraf TCKN/IBAN personel kartında varsa `personnel_transaction` önerisi (`transaction_type=salary_payment`, `payment_method_type=bank_transfer`) |
| `rent_payment` | `expense` / alış faturası | Kira gideri önerisi + stopaj kontrolü (§6.3) |
| `utility_payment` | Alış faturası (`incoming`) | Fatura eşleşmezse "fatura eksik" uyarısı |
| `pos_settlement` | `financial_transaction` (`credit_card`) / `retail_sale` ödemeleri / `cash_box` `valor` | Valörlü POS kasasından bankaya geçiş: `cash_box_transfer` (`from_cash_box_id`=valor kasası) |
| `loan_disbursement`, `loan_principal` | — | Gelir/gider **değil**. Müşavire bilgi. |
| CEK / SENET | `check` / `promissory_note` | Durum güncelleme önerisi |

Portal öneriyi **kendisi kaydetmez.** Müşavir onaylar. Onaylanan öneri portal veritabanına `capture_method=bank_match` ⊕ ile yazılır. Mükellefin ön muhasebe programına geri yazma bu dokümanın kapsamında değil.

### 6.2 Eşleştirme puanı

Aday kayıtlar aynı şirkette, aynı yönde aranır. Puan bileşenleri:

| Bileşen | Puan |
|---|---|
| Tutar birebir (±0,01) | 40 |
| Tutar, aday tutar − olası banka masrafı aralığında (EFT ücreti kesintili gelen ödemeler için, ofis ayarı) | 25 |
| Karşı taraf VKN/TCKN = `customer.tax_number` / `identity_number` | 30 |
| Karşı taraf IBAN = `customer.bank_accounts[].iban` | 30 (VKN ile birlikte en fazla 30) |
| Açıklamada fatura numarası geçiyor (GİB fatura no kalıbı `\b[A-Z0-9]{3}20\d{2}\d{9}\b`) ve aday faturayla aynı | 30 |
| Tarih: aynı gün 10 · ±3 gün 6 · ±15 gün 3 · vadeye (`payment_due_date`) ±3 gün 8 | en fazla 10 |
| Karşı taraf adı benzerliği (normalize, token bazlı) ≥ 0,8 | 10 |

- **Otomatik eşleşme:** puan ≥ 80 **ve** tek aday **ve** ikinci adayla fark ≥ 20.
- **Öneri:** 50–79 veya birden fazla güçlü aday.
- **Eşleşmedi:** < 50.
- Bir banka hareketi birden çok faturayı kapatabilir (toplu ödeme). Aday faturaların tutar kombinasyonu birebir tutuyorsa (en fazla 5 fatura, aynı karşı taraf) öneri olarak sunulur.
- Eşleşme kaydı: `bank_transaction_match` ⊕ (`bank_transaction_id`, `target_type`, `target_id`, `amount`, `score`, `method` = `auto` | `suggested_accepted` | `manual`, `matched_by`, `matched_at`).

### 6.3 Müşavir kontrolleri

Kontrol katmanına eklenen kurallar. Eşik değerleri **ofis ayarıdır**, kodda sabit değildir.

| Kod | Kural | Neden önemli |
|---|---|---|
| `BNK_UNINVOICED_COLLECTION` | Dönemde `customer_collection` olup hiçbir satış faturası / tahsilat kaydıyla eşleşmeyen tutarlar (karşı taraf bazında toplanır) | Faturasız satış riski |
| `BNK_UNDOCUMENTED_PAYMENT` | `supplier_payment` olup alış faturası / masraf kaydı olmayan | Belgesiz gider, indirilemeyen KDV |
| `BNK_FEES_NOT_BOOKED` | Dönemin `bank_fee` + `commission` + `bsmv` toplamı, ön muhasebedeki banka masrafı giderinden büyük | Kaydedilmemiş gider |
| `BNK_INTEREST` | Faiz geliri / gideri ve stopaj dönem toplamları | Geçici vergi, gelir/kurumlar |
| `BNK_TAX_PAID` | Dönemin `tax_payment` ve `sgk_payment` hareketlerinin listesi ve toplamı. Tahakkuk verisi bu dokümanların kapsamında **yok**; ilk sürümde liste müşavire gösterilir, tahakkukla eşleştirmeyi müşavir yapar. | Ödenmemiş / geç ödenmiş beyan |
| `BNK_RENT_WITHHOLDING` | `rent_payment` karşı tarafı TCKN (gerçek kişi) ve işyeri kirası (`M05` veya müşavir onayı) | Muhtasarda stopaj beyanı |
| `BNK_SALARY_VS_PAYROLL` | `salary_payment` dönem toplamı ↔ `personnel_transaction` `salary_payment` toplamı (sadece personel verisi veren kaynaklarda: Easytrade, Paraşüt) | Muhtasar / SGK tutarlılığı |
| `BNK_PAYMENT_METHOD_MISMATCH` | Ön muhasebede `payment_method_type=cash` girilmiş tahsilat/ödeme, aynı gün ±1 ve aynı tutarda banka hareketiyle eşleşiyor | Müşavirin görüşmede istediği kontrol (`01` §4.9) |
| `BNK_CASH_LIMIT` | Aynı karşı tarafa / taraftan nakit (kasa) ile yapılmış ve tevsik zorunluluğu sınırını aşan tahsilat/ödeme. Sınır ofis ayarı (güncel VUK tebliğine göre müşavir girer). | Banka ile tevsik zorunluluğu |
| `BNK_POS_VS_SALES` | Dönem `pos_settlement` toplamı (+ komisyon) ↔ `credit_card` ile tahsil edilmiş satışlar. POS valör ve bloke süreleri yüzünden ay sonu kayması olur; karşılaştırma ofis ayarındaki gün toleransıyla yapılır. | Kartlı satışların faturalanması |
| `BNK_BALANCE_MISMATCH` | Dönem sonu banka bakiyesi (`bank_balance_snapshot` / `bank_daily_summary`) ↔ `cash_box.balance` | Mizan doğruluğu |
| `BNK_GAP` | Bakiye sürekliliği bozuk (`20` §7.4) | Eksik veri |
| `BNK_NON_BUSINESS` | `private_or_non_business` hareketleri | Gider olarak yazılmamalı |
| `BNK_UNCLASSIFIED` | `kind_detail=unknown` hareket sayısı ve tutarı | Veri kalitesi |

Her uyarı `GET /accountant/warnings` (`02` §8) üzerinden, ilgili `bank_transaction` id'leriyle döner.

## 7. Portal uç noktaları (Easytrade kalıbında)

| Uç | İçerik |
|---|---|
| `GET /accountant/bank-connections` | Mükellefin banka bağlantıları ve durumları (`X-Company-ID`) |
| `POST /accountant/bank-connections` | Yeni bağlantı (banka seçimi) → `draft` |
| `GET /accountant/bank-connections/{id}/instruction-form` | Önceden doldurulmuş talimat PDF'i |
| `PUT /accountant/bank-connections/{id}/credentials` | Banka bilgileri (sadece kasaya yazılır, yanıt maskeli). `Idempotency-Key`. |
| `POST /accountant/bank-connections/{id}/verify` | `healthCheck` + `listAccounts` |
| `POST /accountant/bank-connections/{id}/sync` | Anlık çekim (5 dakikada 1) |
| `GET /accountant/bank-transactions` | Filtreler: `filters[transaction_date][start\|end]`, `filters[cash_box_id]`, `filters[kind]`, `filters[kind_detail]`, `filters[match_status]`, `filters[debit_credit]` |
| `GET /accountant/bank-transactions/{id}` | Hareket + ham alanlar (yetkiye göre) + eşleşmeler + adaylar |
| `POST /accountant/bank-transactions/{id}/match` | Eşleştir (hedef tür + id) |
| `POST /accountant/bank-transactions/{id}/classify` | Sınıflandırmayı düzelt (`bank_rule` üretir) |
| `POST /accountant/bank-transactions/{id}/accept-suggestion` | Öneriyi kabul et |
| `POST /accountant/bank-transactions/{id}/ignore` | Yok say (gerekçe zorunlu) |
| `GET /accountant/bank-reconciliation?period=` | Hesap bazında dönem özeti: açılış, toplam A/B, kapanış, ön muhasebe bakiyesi, fark, eşleşme oranı |
| `POST /accountant/bank-statements/import` | Ekstre yükleme: PDF, Excel, MT940 (`23`). Mükellef de yükleyebilir. |

Yanıt zarfı, sayfalama ve hata yapısı `02` §8 ile aynı.
