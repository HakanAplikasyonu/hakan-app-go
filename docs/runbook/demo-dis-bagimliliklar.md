# Demo: dış bağımlılıklar ve sahte davranış

Pilot demosu (`make demo`, senaryo: `docs/kullanim/demo-senaryosu.md`)
dışarıya hiçbir istek atmadan çalışır. Bu belge her dış bağımlılık için
demodaki davranışı, gerçek bağlantı için gerekenleri ve bilinen eksikleri
toplar.

| Bağımlılık | Demoda | Gerçek bağlantı için |
|---|---|---|
| Easytrade (ön muhasebe) | Kimlik bilgisiz bağlantı, seed ile yazılmış Ekim verisi | Şirket hesabı, `EASYTRADE_*` env, eksik uçlar (aşağıda) |
| Destek penceresi (widget) | Kapalı; istenirse demo kimliğiyle açılır | Gerçek widget kimlikleri ve imza sırrı |
| Banka ekstreleri | Seed ile yazılmış Ekim hareketleri (Excel kaynağı) | Ekstre dosyası yüklemesi ya da banka web servisi |
| Luca / Zirve aktarımı | Kayıtlı ama çalışmayan masaüstü ajanı, örnek aktarım geçmişi | Ofis bilgisayarında masaüstü ajanı ve aktarım klasörü |
| Dosya deposu (S3) | Compose'daki yerel depo; yoksa seed dosyaları bellekte tutar | Üretim deposu (`STORAGE_*`, `S3_*`) |
| E-posta | Yerel MailHog (`http://127.0.0.1:8025`) | SMTP ayarları |

## Easytrade

**Demoda:** `backend/cmd/demo-seed/easytrade.go` Atlas Gıda'ya etkin bir
bağlantı açar. Bağlantının özellikleri:

- `company_id` `demo-42`, Merkez ve Kadıköy Şube.
- 4 saatte bir senkron.
- **Kimlik bilgisi yok.** Bu yüzden worker Easytrade'i hiç çağırmaz.

Veri `internal/demo/easytrade.go`'dan bir senkron yazar gibi
`sources.Write` ile gelir:

- Ekim 2026'nın masrafları
- finansal işlemler (nakit, kart, havale)
- beş Z raporu
- bir kısmi ve birkaç başarılı çalışmalı senkron geçmişi

"Dönemi şimdi senkronla" demoda sonuç üretmez.

**Eksik uçlar:** Easytrade API'sinde bugün olmayan ya da belgelenmemiş
uçların listesi `docs/easytrade-senkron.md` §4'tedir. Öne çıkanlar:

- Z raporu ucu yok. Portal arabirimi ve sahte kaynağı (`ZReportSource`) hazır.
  Uç 404 dönerken kalem `unsupported` sayılır ve çalışma başarısız olmaz.
- Webhook (giden bildirim) yok. Portal tarafı hazır, `EASYTRADE_WEBHOOK_SECRET`
  boşken kapalı.
- Finansal işlemlerde tarih filtresi yok. Pencere tarih sıralı tarama ile
  kesilir.
- Silinen kayıt bildirimi yok.
- Şirket API token'ı ile kimlik doğrulama yok. Bugün kullanıcı girişi ve
  30 günlük token kullanılır.

**Gerçeğe geçiş:**

1. `EASYTRADE_API_BASE_URL` ayarını yapın, gerekirse `EASYTRADE_WEBHOOK_SECRET`'ı
   da.
2. Ofis panelinde mükellefin Easytrade sekmesinden şirket hesabıyla bağlantı
   kurun.

## Destek penceresi (widget kimliği)

**Demoda:** widget kimlikleri boştur, panelde ve mükellef portalında betik
yüklenmez. Bu bilinçli bir seçim: e2e ve görsel testler dış betik yüklemez.
Gösterimde mükellef portalındaki **Destek** düğmesini görmek için:

```bash
DEMO_CLICKFIX_WIDGET_ID=<ofisin widget kimliği> make demo
```

Kimlik demo ofisinin destek ayarına yazılır.

**Gerçeğe geçiş:** `.env` / sunucu env dosyasında aşağıdaki anahtarlar
(şema: `deploy/env/schema`) doldurulur:

| Anahtar | Ne işe yarar |
|---|---|
| `NEXT_PUBLIC_CLICKFIX_WIDGET_ID` | Ofis paneli → yazılım sağlayıcısı |
| `NEXT_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID` | Mükellef portalı → ofis. Ayarında kimlik olmayan ofislerin varsayılanı |
| `NEXT_PUBLIC_CLICKFIX_SCRIPT_URL` | Widget betiğinin adresi |
| `CLICKFIX_IDENTITY_SECRET` | Ofis paneli widget'ının kullanıcı imzası |

Ofisler kendi kimliklerini ofis ayarlarından girer.

## Banka ekstreleri

**Demoda:** `backend/cmd/demo-seed/bank.go` Atlas Gıda'nın Ziraat TL
hesabına (IBAN geçerli ama uydurma) Ekim 2026'nın dokuz hareketini Excel
kaynağıyla (`bank_file`) yazar. Gerçek bir ekstre dosyası yoktur.

- Hareketler Eylül maaş, SGK, KDV ve MUHSGK ödemeleriyle tutarlıdır.
  Tutarlar `backend/cmd/demo-seed/ledger.go`'da tanımlıdır.
- Giden ödemeler onaylanıp fişlenmiş ve Luca'ya aktarılmıştır.
- Bir tahsilat kuyrukta bekler.

**Dosya yüklemeyi göstermek için:**

- **Banka** sekmesinden yeni bir hesap açıp ekstre yükleyin.
- Bankaların örnek ekstre biçimleri: `backend/internal/modules/banking/statement/testdata/banks/`.
- Uçtan uca testin MT940 örneği: `e2e/tests/smoke.spec.ts`.

**Gerçeğe geçiş:**

- Ofis, mükellefin ekstresini (Excel, CSV, MT940) **Banka** sekmesinden
  yükler.
- Banka web servisi kanalı yalnızca uyarlayıcısı yazılmış bankalarda açılır
  (`internal/modules/banking/webservice`). Bu kanal için bankanın servis
  kullanıcısı ve sabit IP izni gerekir. Diğer bankalar ekstre yüklemede kalır.

## Luca / Zirve aktarım yolu

**Demoda:** `backend/cmd/demo-seed/journals.go` iki şey kurar:

- `demo-office-pc-01` adlı bir masaüstü ajanı kaydı. Arkasında çalışan ajan
  yoktur.
- İki örnek aktarım:
  - Luca çıktısı klasöre bırakılmış ve **Programa aktarıldı** olarak
    işaretlenmiş.
  - Zirve çıktısı `FOLDER_MISSING` hatasıyla düşmüş.

Diğer Luca çıktıları (banka, bordro, masraf fişleri) aktarım istenmeden
**Önceki çıktılar**'da durur. Ekim mizanı bu çıktılardan üretilir; yani
Luca'nın içe aktardığı kabul edilir. Gerçek Luca ya da Zirve kurulumu
yoktur, dosyalar programa girmez. Zirve şablonu doğrulanmamıştır
(`TEMPLATE_UNVERIFIED` uyarısı).

**Gerçeğe geçiş:**

1. Ofis bilgisayarına masaüstü ajanını kurun (`docs/kullanim/masaustu.md`).
2. Luca / Zirve'nin içe aktarım klasörünü ajana bağlayın.
3. **Fişler ve aktarım → Programa aktarım → Masaüstü ajanına gönder**.
   Ajan dosyayı klasöre bırakır.
4. Ofis içe aktarımı programda yapar ve sonucu **Aktarıldı** ya da
   **Hata verdi** ile işaretler.

## Demo verisinin kaynağı

| Konu | Dosya |
|---|---|
| Kimlikler (ofis, kullanıcılar, mükellefler) | `backend/internal/demo/demo.go` |
| Atlas Gıda'nın Eylül–Ekim hikâyesi, ortak tutarlar | `backend/cmd/demo-seed/ledger.go` |
| Modül seed'leri | `backend/cmd/demo-seed/*.go` |
| Tutarlılık testi | `backend/cmd/demo-seed/story_db_test.go` |
| Çalışan ortamın duman testi | `scripts/smoke.mjs` (`make smoke`) |
| UBL fatura seti | `backend/internal/demo/ubl/` |

Rakam değiştiren bir seed değişikliğinde iki dosya birlikte güncellenir:

- `ledger.go` sabitleri ve `TestDemoStoryIsConsistent`
- `scripts/smoke.mjs`'teki beklenen değerler
