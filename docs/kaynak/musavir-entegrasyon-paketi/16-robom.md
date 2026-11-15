# 16 · Robom Adaptörü

| | |
|---|---|
| Üretici | Mükellef (şirket yönetim platformu) |
| Çalışma şekli | Bulut |
| API | **Herkese açık API bulunamadı** ❌ |
| 5 kalemin durumu | API ile hepsi ❌ |

## 0. Durum

- Robom, Mükellef'in yapay zekâ destekli ön muhasebe ürünü.
- Kapsamı: e-fatura, ön muhasebe, risk skorlama, dijital vergi dairesi ve SGK sorgulama.
- Fatura girişleri, cari işlemler ve banka mutabakatını otomatikleştirdiğini söylüyor.
- Lansmanda yalnızca Mükellef müşterilerine açıktı, tüm KOBİ'lere açılacağı duyurulmuştu.
- Herkese açık geliştirici dokümanı veya API'si bulunamadı.

**Stratejik not:** Robom'un sunduğu dijital vergi dairesi ve SGK sorgulama, otomatik mutabakat gibi özellikler, mali müşavir portalının vaat ettiği özelliklerle örtüşüyor. Robom hem **veri kaynağı** hem **rakip.** Erişim talebi bu dengeyle yapılmalı.

## 1. Erişim yolları (öncelik sırasıyla)

| # | Yol | Not |
|---|---|---|
| 1 | **Mükellef ile iş ortaklığı görüşmesi** | Okuma API'si talep edilir (talep listesi `13-logo-isbasi.md` §1 ile aynı). |
| 2 | **E-belge entegratörü** | Robom'un e-fatura altyapısının hangi entegratör üzerinden çalıştığı öğrenilmeli ⚠️. Satış/alış faturaları oradan alınır. |
| 3 | **Excel dışa aktarım → portal içe aktarma** | Toplu geçmiş veri ve kategoriler için |
| 4 | **Easytrade mobil uygulaması** | Masraf fişleri |
| 5 | **Banka entegrasyonu** | Kasa/banka hareketleri |

## 2. Kalem bazında geçici çözüm

| Kalem | Kaynak |
|---|---|
| Satış ve alış faturaları | Entegratör API'si (UBL XML ile tam KDV kırılımı) |
| Kasa / banka | Banka hesabı bağlama veya PDF/Excel ekstre yükleme (`20`) |
| Tahsilat / ödeme | Banka hareketlerinden eşleme |
| Kategoriler | Portal kural motoru + Excel |
| Masraf fişleri | Easytrade mobil uygulaması |

> **Excel içe aktarma:** Bu programda API ile alınamayan kalemler programın kendi Excel dışa aktarımıyla `04-excel-ice-aktarma.md` akışından alınır. Örnek dışa aktarım dosyaları toplanıp `global` şablon hazırlanmalı.

## 3. Doğrulanacaklar listesi

- [ ] Mükellef'in entegratörlere API açma niyeti ve koşulları
- [ ] Robom'un e-belge altyapısı (hangi özel entegratör)
- [ ] Robom'un Excel dışa aktarım formatları

## Kaynaklar

- Ürün sayfası: https://mukellef.co/robom/
- Lansman haberi: https://www.forbes.com.tr/yapay-zeka/yapay-zeka-destekli-robom-kobi-lerin-finansal-yukunu-hafifletiyor
- Lansman haberi: https://www.bloomberght.com/mukellef-robom-yapay-zekyla-on-muhasebe-cozumleri-sunacak-3746865
