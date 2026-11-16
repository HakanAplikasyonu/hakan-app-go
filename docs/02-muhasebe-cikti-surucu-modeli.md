# 02 · Muhasebe Programı Çıktısı: Sürücü (Driver) Modeli

| | |
|---|---|
| Tarih | 15 Kasım 2026 |
| Karar | İlk sürücüler **Luca** ve **formüllü Excel**. Yapı sürücü mantığında kurulur; Zirve, Mikro ve Orka sonradan tek paket eklenerek gelir. |
| Durum | Tasarım. Kod yok. |

## 1. Luca hakkında bulabildiklerimiz

### 1.1 Luca'nın resmi destek sayfalarından

Kaynak: lucayazilim.freshdesk.com. Makaleler 2021 tarihli, güncellikleri teyit edilmedi.

**Excel Veri Aktarımı (muhasebe fişi)**
- Zorunlu alanlar: **Fiş No, Fiş Tarihi, Hesap Kodu, Borç, Alacak**.
- Fiş No ve Fiş Tarihi aynı olan satırlar aynı fişe girer; biri farklıysa ayrı fiş açılır.
- Bir yüklemede en fazla **50 fiş**. Mahsup fişinde **400 satır** sınırı var; açılış ve kapanış fişinde satır sınırı yok.
- Fiş tipleri: Mahsup, Açılış, Kapanış. Tahsil ve Tediye resmi metinde **yok**.
- Fiş Kes penceresinde Fiş Açıklaması, Fiş Tipi, Fiş Kodu ve Belge Türü seçiliyor. Belge türü için tek örnek "MF" (e-defter).
- Fişteki hesap kodu Luca'da açık değilse "hesap kodu bulunamadı" uyarısı veriliyor.
- Şablon Luca ekranındaki **"Şablon indir"** bağlantısıyla alınıyor. Sütun başlıkları ve sırası makalede yazmıyor.

**Banka ekstreleri**
- Dosya **CSV** olmalı, en fazla **400 satır**. Sütun başlıkları Luca'da tanımlı şablonla birebir aynı olmalı.
- Luca'nın kendi kural motoru var: en fazla 20 kural, "içinde geçen", tutar yönü, 120/320 için sezgisel cari eşleme.

**Hesap planı**
- Excel şablonundan XML üretiliyor, "Hesap Planı XML Aktar" menüsüyle içe alınıyor. Cari (VKN/TCKN, adres) için ayrı bir CSV şablonu var.

**Kayıt aktarımı sayfası:** Bu sayfa başka programdan Luca'ya geçiş içindir (Luca Asistan + başvuru formu), günlük kullanım için değil.

### 1.2 Bulunamayanlar

Aşağıdakiler resmi kaynakta yok. Üçüncü taraf kaynaklar da birbiriyle çelişiyor.
- Fiş şablonunun **sütun başlıkları ve sırası**
- Tarih biçimi ve ondalık ayırıcı
- Belge türü listesi
- .xls / .xlsx ayrımı
- UBL XML ile fatura içe aktarımı
- Luca'nın herkese açık bir API'si

**Gereken:** Bir Luca kullanıcısının indirdiği güncel şablon dosyası (müşavirden istenecek: `00` §6.2 M2) ve Easytrade'in mevcut Luca entegrasyonunun yöntemi (`00` §6.1 E1). İkinci yol olarak Luca desteğine yazılabilir (`aktarim@luca.com.tr`, kayıt aktarımı sayfasında geçiyor). Mesajı Easytrade ya da sen gönderirsin.

### 1.3 Bunun tasarıma etkisi

Sütun adı ve sırası **koda yazılmaz.** Sürücü, şablon tanımını sürümlü bir yapılandırma dosyasından okur (§3.3). Şablon gelince yalnızca bu dosya doldurulur; şablon sonradan değişirse yeni bir sürüm eklenir. Kural her durumda aynıdır: tahmin yok, şablon gelene kadar Luca sürücüsü "doğrulanmamış şablon" uyarısıyla çalışır.

## 2. Ortak çıktı modeli

Sürücüler doğrudan faturayı ya da banka hareketini görmez. Önce ortak bir **yevmiye taslağı** üretilir, sürücü bu taslağı kendi biçimine çevirir. Böylece hesap kodu eşleme, kontroller ve inceleme kuyruğu her sürücü için tek yerde yazılır.

```
fatura / banka / fiş ──► hesap kodu kuralları (T1) ──► JournalDraft ──► inceleme / onay ──► ExportBatch ──► Sürücü ──► dosya(lar)
```

| Varlık | Alanlar (özet) |
|---|---|
| `journal_draft` (fiş) | `engagement_id`, dönem, fiş tipi (`mahsup` / `acilis` / `kapanis` + sürücüye özel eşleme), tarih, açıklama, belge türü, durum (`draft` / `approved` / `exported`), köken (kaynak belgeler, kural, güven) |
| `journal_draft_line` | hesap kodu, açıklama, borç, alacak (`NUMERIC(18,4)`), miktar, para birimi, kur, döviz tutarı, evrak no, evrak tarihi, kaynak belge bağlantısı (ETTN / banka hareketi / fiş) |
| `export_batch` | sürücü, şablon sürümü, kapsam (dönem, fiş listesi), üretilen dosyalar, içerik özetleri (SHA-256), üreten kişi ve zaman |
| `export_batch_item` | fiş ↔ batch; aynı fişin ikinci kez çıkmasını engeller (§4) |

Kurallar:
- Fiş dengesi (Σ borç = Σ alacak) **sürücüye gelmeden** doğrulanır. Dengesiz fiş çıktıya giremez (blocker).
- Hesap kodu, ofisin yüklediği **Luca hesap planında** yoksa uyarı `ACCOUNT_NOT_IN_TARGET_PLAN` üretilir. Böylece Luca'daki "hesap kodu bulunamadı" hatası çıktı öncesinde yakalanır.
  - Denetim kodu önce normalize eder (`320 01 002` → `320.01.002`) ve satıra planın yazımını koyar; alt hesabı olan **ana hesap** da aynı uyarıyla reddedilir (`reason: group_account`), çünkü Luca ana hesaba fiş almaz (#319).
  - Sonuç dönemin uyarısı olarak kaydedilir (`control:target_plan`); kod düzelince uyarı kendiliğinden kapanır. Engelleme için hem çıktı ayarı (`block_on_unknown_account`) hem uyarı kodunun ayarı (etkin, `blocker`) gerekir.
- Her satır kaynak belgesini (ETTN vb.) taşır. Çift kayıt kontrolü buna dayanır (`00` E9).

## 3. Sürücü modeli

### 3.1 Arayüz

`database/sql` sürücü kalıbı: her sürücü kendi paketinde `init()` ile kendini kaydeder, uygulama sürücüyü adıyla açar. Yeni bir program eklemek için tek paket yazılır ve import edilir; çekirdeğe dokunulmaz.

```go
// package accountingexport

type Driver interface {
    Info() DriverInfo                       // ad, sürüm, desteklenen fiş tipleri, dosya türleri, limitler
    Validate(ctx context.Context, b Batch) []Issue        // sürücüye özel kurallar (limit, alan uzunluğu, tip eşleme)
    Export(ctx context.Context, b Batch, w FileWriter) (Result, error)
}

type DriverInfo struct {
    Name          string        // "luca", "excel_formula", ileride "zirve", "mikro", "orka"
    TemplateVer   string        // kullanılan şablon sürümü
    Limits        Limits        // MaxEntriesPerFile (Luca: 50), MaxLinesPerEntry (Luca mahsup: 400) …
    EntryTypes    map[EntryType]string // ortak fiş tipi → programın değeri
    Verified      bool          // şablon gerçek dosyayla doğrulandı mı
}

// Kayıt defteri
func Register(name string, factory func(cfg Config) (Driver, error))
func Open(name string, cfg Config) (Driver, error)

// Batch: onaylı JournalDraft'lar + hedef hesap planı + mükellef / ofis ayarları
```

Kurallar:
- **Bölme sürücünün işidir.** Luca'da 50 fişi aşan batch birden fazla dosyaya bölünür, 400 satırı aşan mahsup fişi birden fazla fişe ayrılır. Ayrılan fişlerin her biri ayrıca dengeli olmalıdır; dengeli bölünemiyorsa blocker.
- **Sürücü sadece dosya üretir.** Programa doğrudan yazma (API, masaüstü ajanı) ayrı bir `Uploader` arayüzüyle sonra eklenir. Masaüstü Go sidecar (`01` §4.2) burada devreye girebilir.
- `Verified=false` olan sürücünün çıktısında arayüz "şablon doğrulanmadı" uyarısını gösterir.

### 3.2 İlk sürücüler

| Sürücü | Çıktı | Durum |
|---|---|---|
| `luca` | Luca Excel Veri Aktarımı fiş şablonu (.xlsx; .xls gerekirse şablona göre). Fiş No + Fiş Tarihi gruplaması, 50 fiş/dosya, 400 satır/mahsup. | Şablon dosyası gelene kadar `Verified=false` |
| `luca_bank_csv` (opsiyonel) | Luca'nın banka ekstresi CSV şablonu. Müşavir Luca'nın kendi kural motorunu kullanmak isterse. | Şablon gelene kadar beklemede |
| `excel_formula` | **Formüllü kontrol ve çalışma Excel'i** (§3.4). Programdan bağımsız. | Şablon gerektirmez, ilk gün çalışır |

Zirve, Mikro ve Orka aynı arayüzle ayrı paket olarak gelir. Her biri için ilk iş, o programın içe aktarım şablonunu gerçek dosyayla almaktır.

### 3.3 Şablon tanımı (kod değil, veri)

```yaml
# exports/luca/templates/fis_v1.yaml   (örnek yapı; değerler şablon dosyası gelince doldurulur)
driver: luca
template: fis
version: "unverified-1"
verified: false
file: { format: xlsx, sheet: null, header_row: 1 }
limits: { max_entries_per_file: 50, max_lines_per_mahsup: 400 }
columns:            # sıra = dosyadaki sıra
  - { key: entry_no,    header: "Fiş No",      required: true }
  - { key: entry_date,  header: "Fiş Tarihi",  required: true, format: "<şablondan>" }
  - { key: account,     header: "Hesap Kodu",  required: true }
  - { key: debit,       header: "Borç",        required: true, decimal: "<şablondan>" }
  - { key: credit,      header: "Alacak",      required: true, decimal: "<şablondan>" }
  # diğer sütunlar şablon dosyasından eklenir
entry_types: { mahsup: "<şablondan>", acilis: "<şablondan>", kapanis: "<şablondan>" }
```

Yukarıdaki beş başlık, resmi metinde geçen zorunlu alan **adlarıdır**. Dosyadaki gerçek başlık yazımı ve sırası şablondan teyit edilir.

### 3.4 `excel_formula`: formüllü Excel

Amaç iki yönlü: müşavir çıktıyı Excel'de **kontrol edebilsin**, gerekirse elle düzeltip yine kullanabilsin. Excel boilerplate'teki `excelize` ile üretilir; formüller hücreye yazılır, değerler ön hesaplanmaz.

| Sayfa | İçerik | Formüller |
|---|---|---|
| `Fişler` | Satır başına bir hesap hareketi (fiş no, tarih, hesap, açıklama, borç, alacak, kaynak belge, güven) | Her fiş grubunun altında `SUMIFS` ile borç ve alacak toplamı, `=borç-alacak` farkı. Fark ≠ 0 ise koşullu biçim kırmızı. |
| `Fiş Özeti` | Fiş başına tek satır | Borç, alacak ve fark `SUMIFS(Fişler!…)` ile; denge durumu `IF(ABS(fark)<0,005;"Dengeli";"DENGESİZ")` |
| `Mizan (dönem)` | Hesap kodu başına | `SUMIFS` ile borç, alacak, bakiye; ana hesap (ilk 3 hane) ara toplamı |
| `KDV` | Oran bazında matrah ve KDV (191 / 391) | `SUMIFS` ile; ödenecek / devreden KDV formülü. Portalın kendi KDV özetiyle yan yana gösterilir (fark sütunu). |
| `Uyarılar` | Açık uyarılar (`05` kodları), kaynak bağlantısı | — |
| `Ayarlar` | Dönem, mükellef, şablon sürümü, üretim zamanı, batch özeti | Formüllerin başvurduğu tolerans hücresi (ör. 0,005) |

Kurallar:
- Formüllerde Türkçe yerel ayar sorunu olmasın diye formüller excelize'de standart (İngilizce, `,` ayraçlı) yazılır. Excel bunları kullanıcının diline göre gösterir.
- Tutar hücreleri sayı tipinde, `#,##0.00` biçiminde yazılır; metin olarak yazılmaz.
- Kaynak belge sütunu portalda o belgeyi açan bir bağlantıdır.

## 4. Çift kayıt ve izlenebilirlik

- Bir fiş en fazla bir `export_batch`'te `exported` olabilir. Yeniden çıktı almak için önce "geri al" kaydı açılır; kim, ne zaman ve neden alanları zorunludur.
- TÜRMOB tercihli entegratör mükelleflerinde (`00` E9) faturalar Luca'ya zaten gidiyor olabilir. Bu mükelleflerde fatura kaynaklı fişler varsayılan olarak çıktıya **girmez**, müşavir açıkça seçerse girer. Mükellef ayarı: `invoices_reach_luca_directly`.
- Batch dosyalarının SHA-256 özetleri saklanır. "Luca'ya giden dosya buydu" sorusunun cevabı her zaman verilebilir.

## 5. Kabul testleri

1. Dengeli 3 fişlik batch → Luca dosyası. Fiş No + Tarih gruplaması doğru.
2. 120 fişlik batch → 3 dosya (50 / 50 / 20).
3. 450 satırlık mahsup → dengeli iki fişe bölünüyor; bölünemeyen örnekte blocker.
4. Hedef hesap planında olmayan kod → `ACCOUNT_NOT_IN_TARGET_PLAN`, çıktı engelleniyor (ayara bağlı).
5. Aynı fişin ikinci kez çıktıya alınması reddediliyor.
6. `excel_formula`: dosya Excel ve LibreOffice'te açıldığında formüller hesaplanıyor; dengesiz fiş kırmızı görünüyor; KDV sayfası portal özetiyle aynı.
7. **Gerçek Luca'ya yükleme:** şablon gelince bir test mükellefinde (müşavirin onayıyla) yükleme yapılıyor. `Verified=true` ancak bundan sonra işaretleniyor.

## Kaynaklar

- Luca destek, Excel Veri Aktarımı: https://lucayazilim.freshdesk.com/support/solutions/articles/67000267580-01-02-09-excel-veri-aktar-m-
- Luca destek, Banka Ekstreleri: https://lucayazilim.freshdesk.com/support/solutions/articles/67000267579-01-02-08-banka-ekstreleri
- Luca destek, Hesap Planı XML / CSV Excel Aktar: https://lucayazilim.freshdesk.com/support/solutions/articles/67000267449-01-01-04-hesap-plan-xml-csv-excel-aktar
- Luca kayıt aktarımı: https://www.luca.com.tr/Sayfa/luca-kayit-aktarimi/64
- Üçüncü taraf (çelişkili, esas alınmadı): https://dcloudsoftware.com/tr/blog/luca-excel-fis-aktarimi-sablon-hazirligi-sik-hatalar , https://www.fisle.co/blog/luca-formati-nedir
