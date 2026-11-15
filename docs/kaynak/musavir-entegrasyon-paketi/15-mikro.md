# 15 · Mikro Adaptörü

| | |
|---|---|
| Çalışma şekli | Run / Jump / Fly: **kurulu** (mükellef sunucusu, MS SQL). JumpBulut: **bulut** |
| API | MikroAPI (v16 / v17+). Erişim **başvuru ile** veriliyor ✅ |
| Base URL | JumpBulut gateway: `https://jumpbulutapigw.mikro.com.tr/ApiJB/ApiMethods` ✅. Kurulu sürümde MikroAPI servisinin adresi ⚠️ |
| Kimlik doğrulama | `APILogin` + `ApiKey` + Bearer token ✅ |
| 5 kalemin durumu | Hepsi ⚠️: Dokümante API uçları ağırlıklı **yazma** yönünde. Okuma için SQL gerekiyor. |
| Doğrulama kaynağı | apidocs.mikro.com.tr, mikro.com.tr API başvuru formu |

## 0. Özet

- Mikro'nun herkese açık API dokümanı var ✅. Ancak "Alım Satım Evrakı – Fatura" grubundaki yedi uç noktanın hepsi **yazma** işlemi (kaydet, düzelt, sil, siparişten fatura). **Fatura listeleme / okuma ucu yok.**
- Doküman navigasyonunda **"SQL Sorgulama"** adlı bir grup görünüyor ⚠️. İçeriği doğrulanamadı; okuma için kullanılıp kullanılamayacağı bilinmiyor. **Mikro ile ilk görüşmede netleştirilmesi gereken bir numaralı konu bu.** Doğrulanana kadar okuma yolu sadece kurulu sürümde bağlantı ajanı + salt okunur SQL.
- Mikro tablo ve alan adlarını resmi olarak yayınlıyor (`/tablo-alan-adlari/...`) ✅. Bu yüzden SQL ile okuma yolu belgelenmiş bir şema üzerinden kurulabilir.
- Not: Mikro'nun **"Müşavir"** adında ayrı bir ürünü var (API başvuru formundaki ürün listesinde geçiyor ✅). Bu ürün portalın rakibi konumunda.

**Önerilen yol:**

| Kurulum | Okuma yöntemi |
|---|---|
| Run / Jump / Fly (kurulu) | **Bağlantı ajanı + salt okunur SQL** (birincil). MikroAPI "SQL Sorgulama" ucu doğrulanırsa ona geçilir. |
| JumpBulut | MikroAPI. Okuma ucu yoksa entegratör + Excel geçici çözüm. |

## 1. API erişimi

### 1.1 Başvuru ✅

Form: `https://www.mikro.com.tr/mikro-program-api-basvuru/`

İstenen bilgiler: müşteri türü, firma adı, yetkili, telefon, e-posta, açıklama, entegre edilecek ürün (**Run, Jump, Fly, Müşavir**), istenen çözüm, entegre olunacak diğer yazılımlar. Gizlilik sözleşmesi ve **Mikro API Kullanıcı Sözleşmesi** onayı gerekiyor. Ücret bilgisi sayfada yok ⚠️.

Başvuru açıklamasına şunlar yazılmalı:
- Çok kiracılı entegratör senaryosu (birçok Mikro kullanan mükellef)
- Sadece okuma ihtiyacı: satış/alış faturaları, cari hareketleri, kasa/banka hareketleri, masraflar
- "SQL Sorgulama" uç noktasının kapsamı ve izinleri

### 1.2 Login ✅

```
POST {Gateway}/ApiMethods/{LoginAlias}/APILogin
Content-Type: application/json
User-Agent: (Mikro'nun verdiği sabit değer; dokümanda tüm isteklerde zorunlu)

{
  "Alias": "{LoginAlias}",
  "FirmaKodu": "...",
  "CalismaYili": "2026",
  "ApiKey": "...",
  "KullaniciKodu": "SRV",
  "Sifre": "...",
  "FirmaNo": 0,
  "SubeNo": 0
}
```

- `Sifre`: Dokümana göre **günün tarihi + boşluk + şifre** dizgisinin MD5 özeti ✅. Tarih formatı belirtilmemiş ⚠️ (`yyyy-MM-dd` ile başlanmalı).
- Sonraki istekler: `Authorization: Bearer {KeyToken}`. Geçersiz token 401/403 döner ✅.
- `CalismaYili` (çalışma yılı) her yıl değişir. Yıl dönümünde iki yıl paralel taranmalı.
- Kurulu sürümlerde v17 ve üstü gerekebilir ⚠️.

### 1.3 Dokümante uçlar (bilgi amaçlı) ✅

| Grup | Uçlar | Portal için |
|---|---|---|
| Alım Satım Evrakı – Fatura | `AlimSatimEvragiKaydetV2`, `AlimSatimEvragiDuzeltV2`, `AlimSatimEvragiSatirSilV2`, `AlimSatimEvragiSilV2`, `SiparistenFaturaOlusturmaV2`, `FaturaKaydetV2`, `FaturaKaydetV3` | Yazma, kullanılmaz |
| E-Fatura işlemleri | `FaturaToEFaturaV2`, `FaturaPdfV2` | **`FaturaPdfV2`** fatura PDF'i için kullanılabilir ⚠️ |
| Stok | `StokListesiV2` | Okuma (stok), kapsam dışı |
| Cari eşleme | `CariBorcAlacakEslemeKaydetV2`, `...GuidSilV2` | Yazma |
| Oturum / sağlık | `APILogoff`, `APILogoffV2`, `HealthCheck`, `HealthCheck2` | Bağlantı kontrolü |
| SQL Sorgulama | ⚠️ içerik doğrulanamadı | **Okuma için aday** |

## 2. SQL ile okuma (kurulu sürüm, birincil yol)

`02` §6'daki bağlantı ajanı kullanılır. Ajana **salt okunur** bir SQL kullanıcısı tanımlanır (sadece aşağıdaki tablolara `SELECT` yetkisi).

### 2.1 Tablolar

Alan ön ekleri Mikro'nun tablo dokümanından alınır. Doğrulanmış örnek: `E_FATURA_DETAYLARI` (Tablo No 558, ön ek `efd_`) ✅. `efd_fat_uid` faturaya bağlanır, `efd_uuid` ETTN'dir, `efd_pozisyon` e-fatura tipidir, `efd_mVkn` VKN'dir.

Diğer tabloların adı ve alanları Mikro'nun tablo dokümanından kesinleştirilmeli ⚠️. Aşağıdaki adlar **doğrulanmamış** başlangıç listesidir; resmi tablo dokümanıyla karşılaştırılmadan sorgu yazılmaz:

| Veri | Tablo (doğrulanmalı) | Ön ek |
|---|---|---|
| Cari hareketleri (faturalar, tahsilat, ödeme, dekont) | `CARI_HESAP_HAREKETLERI` | `cha_` |
| Stok hareketleri (fatura satırları) | `STOK_HAREKETLERI` | `sth_` |
| Cari kartları | `CARI_HESAPLAR` | `cari_` |
| Kasalar | `KASALAR` ✅ (dokümanda "Diğer Tablolar" altında) | |
| Bankalar | `BANKALAR` ✅ (dokümanda var) | |
| E-fatura detayları | `E_FATURA_DETAYLARI` ✅ | `efd_` |
| Hesap planı | `MUHASEBE_HESAP_PLANI` ⚠️ | `muh_` |
| Masraf / hizmet kartları | `HIZMET_HESAPLARI` ⚠️ | `hiz_` |

### 2.2 Fatura okuma mantığı

Mikro'da fatura, cari hareketinde bir **evrak** olarak durur. Satırlar stok hareketlerinden gelir. Ayrımı yapan alanlar (cari hareket tablosunda, doğrulanmalı ⚠️):

| Alan (beklenen) | Anlamı |
|---|---|
| `cha_evrak_tip` | Evrak tipi (satış faturası, alış faturası, tahsilat makbuzu, tediye makbuzu, dekont...) |
| `cha_cinsi` | Hareket cinsi (toptan, perakende, hizmet, masraf...) |
| `cha_tip` | Borç / alacak |
| `cha_normal_Iade` | Normal / iade |
| `cha_evrakno_seri`, `cha_evrakno_sira` | Evrak seri / sıra |
| `cha_belge_no` | Belge (fatura) no |
| `cha_tarihi`, `cha_belge_tarih` | Tarih |
| `cha_kod` | Cari kodu |
| `cha_meblag` | Tutar |
| `cha_aratoplam` | Ara toplam |
| `cha_vergi1` … `cha_vergiN` | Oran bazında KDV tutarları |
| `cha_iptal` | İptal |
| `cha_Guid` | Kalıcı kimlik → `source_id` |

**Yapılacak iş:** Mikro'nun tablo dokümanından `cha_evrak_tip` ve `cha_cinsi` değer listesi alınıp şu eşleme tablosu doldurulmalı:

| `cha_evrak_tip` | `cha_cinsi` | Ortak model |
|---|---|---|
| _(dokümandan)_ |  | `invoice` / `sales` / `invoice` |
| _(dokümandan)_ |  | `invoice` / `sales` / `return` |
| _(dokümandan)_ |  | `invoice` / `purchase` / `invoice` |
| _(dokümandan)_ |  | `invoice` / `purchase` / `return` |
| _(dokümandan)_ |  | `financial_transaction` / `collection` |
| _(dokümandan)_ |  | `financial_transaction` / `payment` |
| _(dokümandan)_ |  | `expense` |

**Oran bazında KDV:** `cha_vergi1..N` alanlarının sabit oran sıralarına karşılık geldiği varsayılıyor ⚠️. Hangi sıranın hangi orana denk geldiği **firma parametrelerinden** okunmalı ⚠️. Bu eşleme yanlış kurulursa KDV beyanı hatalı çıkar. Pilot mükellefte Mikro'nun KDV raporuyla birebir doğrulanmadan üretime alınmamalı.

### 2.3 Kasa / banka

Kasa ve banka hareketleri de cari hareket tablosunda, karşı hesap türü kasa veya banka olan satırlar olarak durur ⚠️. Kasa ve banka kartları `KASALAR` ve `BANKALAR` tablolarından okunur.

### 2.4 Sorgu kuralları

- Ajan sadece önceden tanımlı, parametreli sorgular çalıştırır (dönem başlangıç ve bitiş tarihi).
- Okuma `WITH (NOLOCK)` ile yapılır, gün içi tarama küçük pencereli tutulur, tam tarama gece yapılır.
- Değişiklik tespiti: Mikro tablolarında oluşturma/güncelleme tarih alanları varsa (`*_create_date`, `*_lastup_date` ⚠️) artımlı senkron bunlarla yapılır.

## 3. JumpBulut (bulut sürüm)

- SQL'e doğrudan erişim yok. Sadece MikroAPI.
- "SQL Sorgulama" ucu JumpBulut'ta açıksa okuma onunla yapılır.
- Değilse geçici çözüm:
  - E-belge entegratörü (satış/alış)
  - Mikro'dan Excel dışa aktarım → portal içe aktarma
  - Banka entegrasyonu
  - Easytrade mobil uygulaması (masraf)

> **Excel içe aktarma:** Bu programda API ile alınamayan kalemler programın kendi Excel dışa aktarımıyla `04-excel-ice-aktarma.md` akışından alınır. Örnek dışa aktarım dosyaları toplanıp `global` şablon hazırlanmalı.

## 4. Kalem özeti

| Kalem | Kurulu (SQL) | JumpBulut |
|---|---|---|
| Satış faturaları | `cha_` + `sth_` + `efd_` | ⚠️ SQL Sorgulama ucu veya entegratör |
| Alış faturaları | aynı | aynı |
| Kasa / banka | `cha_` + `KASALAR` + `BANKALAR` | ⚠️ |
| Kategoriler | Hesap planı + masraf kartları | ⚠️ |
| Masraf fişleri | Masraf / hizmet hareketleri | ⚠️ |
| Fatura PDF | `FaturaPdfV2` API ⚠️ | `FaturaPdfV2` ⚠️ |

## 5. Doğrulanacaklar listesi

- [ ] API başvurusu sonucu ve ücret koşulu
- [ ] "SQL Sorgulama" uç noktasının kapsamı (JumpBulut ve kurulu sürüm)
- [ ] Kurulu sürümde MikroAPI servisinin adresi ve portu
- [ ] `Sifre` MD5 dizgisindeki tarih formatı
- [ ] `User-Agent` sabit değeri (Mikro'dan alınacak)
- [ ] `CARI_HESAP_HAREKETLERI`, `STOK_HAREKETLERI`, `CARI_HESAPLAR` tablo sayfaları ve alan listeleri
- [ ] `cha_evrak_tip`, `cha_cinsi`, `cha_tip` değer listeleri
- [ ] `cha_vergi1..N` ↔ KDV oranı eşlemesinin kaynağı
- [ ] Güncelleme tarih alanları (artımlı senkron)

## Kaynaklar

- MikroAPI login dokümanı: https://apidocs.mikro.com.tr/jbapi
- Fatura uçları: https://apidocs.mikro.com.tr/apis/alim-satim-evraki-fatura
- E-fatura uçları: https://apidocs.mikro.com.tr/apis/e-fatura-islemleri
- Tablo dokümanı örneği: https://apidocs.mikro.com.tr/tablo-alan-adlari/e_fatura_detaylari
- API başvuru formu: https://www.mikro.com.tr/mikro-program-api-basvuru/
