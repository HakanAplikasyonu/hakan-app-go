# 13 · Logo İşbaşı Adaptörü

| | |
|---|---|
| Çalışma şekli | Bulut (Logo'nun KOBİ ön muhasebe ürünü) |
| API | **Herkese açık API dokümanı yok.** Bir üçüncü taraf entegrasyon firmasının sayfasındaki "API erişimi ayarlanıyor" ifadesi dışında API'nin varlığına ve kapsamına dair doğrulanmış bilgi yok. |
| Kimlik doğrulama | Bilinmiyor ⚠️ |
| 5 kalemin durumu | API ile hepsi **yok** (doğrulanmış bir okuma ucu bulunamadı). Şimdilik Excel. |
| Doğrulama kaynağı | Üçüncü taraf entegratörlerin ürün sayfaları |

## 0. Durum

Üçüncü taraf entegrasyon firmaları İşbaşı'na sipariş ve fatura aktarıyor. Kurulumda "kullanılan ürünün API erişimi ayarlanıyor". Erişim için Logo tarafında başvuru gerekiyorsa süre bu onaya bağlı.

Bu ifadeden çıkan, doğrulanmamış sonuçlar:
- Doküman herkese açık değil.
- Erişim büyük ihtimalle **Logo iş ortaklığı / başvuru** ile veriliyor.
- Bilinen kullanımlar yazma yönünde (sipariş → fatura). Okuma kapsamı bilinmiyor.

Bu dosya, erişim alındığında doldurulacak **iskelet ve talep listesidir.** Erişim yokken uygulanacak geçici çözüm §3'te.

## 1. Logo'ya iletilecek talep listesi

Logo iş ortaklığı ekibine (İşbaşı) yazılırken şunlar istenmeli:

1. Güncel API dokümanı ve sandbox hesabı
2. Kimlik doğrulama modeli: OAuth mu, mükellef API anahtarı mı. **Çok kiracılı entegratör senaryosu** desteklenmeli (bir entegratör, birçok İşbaşı firması).
3. Okuma uçları:
   - Satış faturaları (iade, iptal dahil). Satırlar, oran bazında KDV, tevkifat, istisna kodu, ETTN, e-belge PDF.
   - Alış faturaları (aynı alanlar)
   - Gelen e-fatura kutusu (alışa dönüşmemiş olanlar dahil)
   - Kasa ve banka hesapları ile hareketleri
   - Tahsilat ve ödemeler (fatura eşlemesiyle)
   - Gider kayıtları / masraflar ve kategorileri
   - Cari kartlar (VKN/TCKN dahil)
4. Tarih aralığı filtresi ve `updated_since` veya webhook
5. Sayfalama ve rate limit
6. Ücret / lisans koşulu (mükellef başına ücret var mı)

## 2. Adaptör iskeleti (erişim alındığında)

`02-adaptor-mimarisi.md` sözleşmesi aynen uygulanır. Doldurulacak eşleme tabloları:

| Kalem | İşbaşı uç noktası | Ortak model |
|---|---|---|
| Satış faturaları | _(Logo'dan)_ | `invoice` (`direction=outgoing`) |
| Alış faturaları | _(Logo'dan)_ | `invoice` (`direction=incoming`) |
| Kasa/banka hareketleri | _(Logo'dan)_ | `financial_transaction` |
| Kategoriler | _(Logo'dan)_ | `expense_category` |
| Masraflar | _(Logo'dan)_ | `expense` |

Logo Tiger/GO'daki alan adları (`14-logo-tiger-go.md`) İşbaşı'nda **farklı olabilir.** İşbaşı ayrı bir bulut ürünü. Tiger eşlemesi kopyalanmamalı.

## 3. Erişim yokken geçici çözüm

| Kalem | Kaynak |
|---|---|
| Satış ve alış faturaları | **E-belge entegratörü** (İşbaşı'nın e-dönüşüm altyapısının hangi entegratör üzerinden çalıştığı netleştirilmeli ⚠️) |
| Kâğıt faturalar, fişler | Easytrade mobil uygulaması (OCR) |
| Kasa / banka | Banka entegrasyonu veya ekstre yükleme |
| Toplu geçmiş veri | İşbaşı Excel dışa aktarımı → portal içe aktarma ekranı |

> **Excel içe aktarma:** Bu programda API ile alınamayan kalemler programın kendi Excel dışa aktarımıyla `04-excel-ice-aktarma.md` akışından alınır. Örnek dışa aktarım dosyaları toplanıp `global` şablon hazırlanmalı.

## 4. Doğrulanacaklar listesi

- [ ] API'ye kimler, hangi koşulla erişebiliyor
- [ ] Okuma uçlarının kapsamı
- [ ] E-belge altyapısı (Logo'nun kendi entegratörü mü)
- [ ] Excel dışa aktarım formatları (örnek dosya)

## Kaynaklar

- Üçüncü taraf entegrasyon notu: https://www.zunapro.com/turkey/tr/entegrasyon/logo
