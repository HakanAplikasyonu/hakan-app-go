# 12 · BizimHesap Adaptörü (B2B API)

| | |
|---|---|
| Çalışma şekli | Bulut, REST, JSON |
| Base URL | `https://bizimhesap.com/api/b2b` |
| Kimlik doğrulama | Hesaba özel `token` header'ı |
| 5 kalemin durumu | Satış ❌ · Alış ❌ · Kasa/Banka ⚠️ (sadece cari ekstre) · Kategori ❌ · Masraf ❌ |
| Doğrulama kaynağı | Topluluk SDK'ları (`tcgunel/bizimhesap-b2b`, `vizyontech/bizimhesap-laravel`). **Resmi herkese açık doküman yok.** |

> ⚠️ Bu dosyadaki uç nokta ve alanların hepsi topluluk SDK'larından alındı. BizimHesap'tan resmi doküman alınmadan veya gerçek bir mükellef token'ıyla denenmeden üretime alınmaz.

## 0. Özet: API tek başına yetmez

BizimHesap'ın herkese açık B2B API'si **e-ticaret senaryosu** için tasarlanmış. Fatura **yazmaya** odaklı. Okuma tarafında sadece cari listesi, cari ekstresi, ürün, depo ve stok var.

**Satış faturası, alış faturası, kasa/banka hareketi ve gider kategorisi listeleme uçları yok.** Bilinen kısıtlar:
- Sadece GET ve POST desteklenir.
- Güncelleme ve silme uçları yok.
- Yanıt formatı dokümantasyonu sınırlı.
- Resmi rate limit bilgisi yok.
- Bilinen doküman 5 yıldan eski.

Bu yüzden BizimHesap mükellefi için veri **üç kaynaktan birleştirilir:**

| Kalem | Birincil kaynak | İkincil kaynak |
|---|---|---|
| Satış / alış faturası | **E-belge entegratörü API'si** (e-fatura / e-arşiv) | BizimHesap Excel dışa aktarımı (portal içe aktarma) |
| KDV kırılımı | Entegratörden gelen UBL XML | Excel |
| Kasa / banka | **Banka entegrasyonu** (banka hesabı bağlama / PDF-Excel ekstre, `20`) | BizimHesap cari ekstresi (aşağıda) |
| Tahsilat / ödeme | BizimHesap cari ekstresi | |
| Cari listesi ve bakiye | BizimHesap API | |
| Gider kategorisi | Portalın kendi kural motoru | Excel |
| Masraf fişi | **Easytrade mobil uygulaması** (OCR) | Excel |

**Easytrade'in yapması gereken:** BizimHesap'a iş ortaklığı başvurusu yapıp **okuma uçlarını** talep etmek (aşağıda §5).

> **Excel içe aktarma:** Bu programda API ile alınamayan kalemler programın kendi Excel dışa aktarımıyla `04-excel-ice-aktarma.md` akışından alınır. Örnek dışa aktarım dosyaları toplanıp `global` şablon hazırlanmalı.

## 1. Erişim

### 1.1 Bilgiler

| Bilgi | Açıklama |
|---|---|
| `token` | Mükellefin BizimHesap hesabından ürettiği API token'ı. Fatura iptal çağrısında `FirmId` olarak da geçiyor. |
| Firma ID | Topluluk paketlerinde ayrı alan olarak isteniyor ⚠️ |
| Sabit API anahtarı | Bir topluluk paketi sabit bir B2B anahtarı kullanıyor ⚠️. **Bu değer koda gömülmemeli.** BizimHesap'tan Easytrade'e özel anahtar istenmeli. |

Mükellef token'ı BizimHesap panelinden alıp portala girer. Topluluk entegrasyonlarında akış "BizimHesap sisteminden API bilgisi alınır, ilgili sisteme girilir" şeklinde.

### 1.2 İstek biçimi

```
GET https://bizimhesap.com/api/b2b/customers
token: MUKELLEF_TOKEN
```

## 2. Okunabilen uçlar

| Uç | Yöntem | İçerik |
|---|---|---|
| `/customers` | GET | Cari listesi |
| `/abstract/{customerId}` | GET | Tek carinin hesap ekstresi |
| `/products` | GET | Ürün kartları |
| `/warehouses` | GET | Depolar |
| `/inventory/{warehouseId}` | GET | Depo stok durumu |

Yazma uçları (portal kullanmaz): `POST /addinvoice`, `POST /cancelinvoice`.

### 2.1 Cari listesi

```
GET /customers
→ { "data": { "customers": [ ... ] } }
```

| BizimHesap alanı | Ortak model (`customer`) |
|---|---|
| `id` | `source_id` |
| `code` | ek alan |
| `title` | `trade_name` |
| `taxno` | `tax_number / identity_number` |
| `taxoffice` | `tax_office` |
| `address` | `address` |
| `phone`, `email` | aynen |
| `authorized` | yetkili kişi (ek alan) |
| `balance` | bakiye (kontrol amaçlı) |

Müşteri ve tedarikçi ayrımı bu uçta yok ⚠️. `type = both` yazılır, hareketlerden düzeltilir.

### 2.2 Cari ekstresi

```
GET /abstract/{customerId}
→ { "data": { "abstractUser": { title, email, phone, balance, debitSum, creditSum, link,
                                  abstract: [ { trxdate, type, note, payment, debit, credit, balance } ] } } }
```

| Alan | Kullanım |
|---|---|
| `trxdate` | Hareket tarihi |
| `type` | Hareket türü (fatura, tahsilat, ödeme vb.) ⚠️ değer listesi bilinmiyor, ilk senkronda loglanmalı |
| `note` | Açıklama |
| `payment` | Ödeme şekli ⚠️ (nakit/banka ayrımı için kritik, değerleri doğrulanmalı) |
| `debit`, `credit` | Borç / alacak |
| `balance` | Yürüyen bakiye |
| `link` (üst düzey) | Ekstrenin web görünümü |

Bu ekstreden üretilebilenler:
- **Tahsilat ve ödemeler** → `financial_transaction` (`type` tahsilat/ödeme olan satırlar). `payment` alanından `payment_method_type`.
- **Fatura varlık kontrolü:** Ekstrede fatura satırı var ama entegratörde karşılığı yoksa (ör. kâğıt fatura) "eksik belge" uyarısı.

Üretilemeyenler: KDV kırılımı, fatura satırları, fatura numarası (⚠️ `note` içinde olabilir).

**İstek bütçesi:** Ekstre cari başına ayrı çağrıdır. 500 carili bir mükellefte tam tarama 500 istektir. Bu yüzden:
- Sadece bakiyesi değişen cariler taranır (`/customers` içindeki `balance` önceki senkronla karşılaştırılır).
- Tam tarama günde 1 kez, gece yapılır.

## 3. Kalem bazında yol haritası

### Kalem 1–2: Satış ve alış faturaları ❌ API

1. **Birincil:** Mükellefin e-belge entegratörü (BizimHesap'ın kullandığı entegratör ⚠️ öğrenilmeli) üzerinden gelen/giden e-fatura ve e-arşiv.
2. **İkincil:** BizimHesap'ın fatura listesi Excel dışa aktarımı. Portalda "BizimHesap Excel yükle" ekranı yapılır. Şablon kolonları ilk örnek dosyadan çıkarılır.
3. **Kâğıt fatura:** Easytrade mobil uygulamasından fotoğraf + OCR.

### Kalem 3: Finansal işlemler ⚠️

- Cari ekstresinden tahsilat ve ödemeler (§2.2).
- Kasa ve banka bakiyesi için uç yok. Banka verisi banka hesabı bağlama veya PDF/Excel ekstre yükleme ile alınır (`20`).

### Kalem 4: Gelir/gider türleri ❌

Uç yok. Kategoriler Excel'den veya portalın kural motorundan (açıklama + karşı taraf VKN'si) atanır.

### Kalem 5: Masraf fişleri ❌

Uç yok. Easytrade mobil uygulaması birincil kanal.

## 4. Senkron planı

| Adım | Çağrı | Sıklık |
|---|---|---|
| 1 | `/customers` | 4 saatte 1 |
| 2 | Bakiyesi değişen cariler için `/abstract/{id}` | 4 saatte 1 |
| 3 | Tüm cariler için `/abstract/{id}` | Gece 1 |
| 4 | Entegratör adaptörü (`30`–`36`) | 60 dakikada 1 |

## 5. BizimHesap'a iletilecek talep listesi

İş ortaklığı görüşmesinde şu okuma uçları istenmeli:

1. `GET /invoices?type=sales|purchase&from=&to=`: satırlar, oran bazında KDV, tevkifat ve ETTN dahil
2. `GET /cash-accounts` ve `GET /cash-accounts/{id}/transactions?from=&to=`
3. `GET /expenses?from=&to=` (gider/masraf kayıtları ve kategorileri)
4. `GET /categories`
5. Cari listesinde müşteri/tedarikçi ayrımı
6. `updated_since` filtresi veya webhook
7. Entegratöre özel anahtar ve rate limit bilgisi
8. Güncel API dokümanı

## 6. Doğrulanacaklar listesi

- [ ] Easytrade'e özel API anahtarı mı, mükellef token'ı yeterli mi
- [ ] Ekstre `type` ve `payment` alanlarının değer listesi
- [ ] Ekstre satırında fatura numarasının yeri
- [ ] Rate limit
- [ ] BizimHesap'ın e-belge altyapısını hangi entegratörden aldığı
- [ ] Fatura listesi Excel dışa aktarımının kolon yapısı (örnek dosya istenmeli)

## Kaynaklar

- Topluluk SDK'sı (uç noktalar ve modeller): https://packagist.org/packages/tcgunel/bizimhesap-b2b
- Topluluk SDK'sı (kısıtlar): https://packagist.org/packages/vizyontech/bizimhesap-laravel
- Entegrasyon akış örneği: https://www.zunapro.com/turkey/tr/entegrasyon/bizimhesap
