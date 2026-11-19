# Akış denetimi: müşavir gözüyle her ekran (#561)

Tarih: 20.11.2026 · Ortam: yerel demo (`demo-smmm`, 32 mükellef, sahip hesabı) ·
Üst iş: #560

Yöntem: bir serbest muhasebeci mali müşavirin sabahki rutini izlendi
(Genel bakış → Sıradaki iş → mükellef → eksik belge → banka → inceleme →
uyarı → yevmiye → aktarım). Her tıklanabilir görünen öğeye tıklandı;
"nereye gitti, orada iş bitirilebiliyor mu" kaydedildi. Bulgular düzeltme
işlerine (issue) bağlandı; öncelik sırası en sonda.

Gösterim: **Kırık** = çalışmıyor ya da yanlış yere gidiyor ·
**Eksik** = iş o ekranda bitirilemiyor · **Güven** = doğru çalışsa da
kullanıcıda şüphe uyandırıyor.

## 1. Genel bakış (`/t/:slug`)

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 1.1 | "Engel var" yazan mükellef satırları tıklanmıyor. | Kırık | #562 |
| 1.2 | "Engelleri gör" `/warnings`'e süzgeçsiz gidiyor; ne o mükellefe ne döneme ne engelleyicilere daralıyor. | Kırık | #562 |
| 1.3 | "Bekleyen belge görevi" kutucuğu mükellef listesine gidiyor; belge talepleri listesine gitmeli. | Kırık | #562, #563 |
| 1.4 | Türkçe ek hataları: "0'ı'si", "%90'i". | Güven | #576 |
| 1.5 | Üstteki dönem seçici "Tümü"ne dönüyor; mükellef sayfasına geçince seçilen dönem kayboluyor. | Kırık | #562 |

## 2. Mükellefler ve mükellef özeti

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 2.1 | Listede yalnız unvan bağlantı; satırın kendisi tıklanmıyor. | Eksik | #567 |
| 2.2 | Özet sayfasında "Sıradaki iş: 3 belge talebi", yanında "Açık belge isteği 7": aynı şeyin iki farklı sayısı. | Güven | #562, #576 |
| 2.3 | Adım başlıkları (Belgeler, Banka, Fişler…) bağlantı değil. | Eksik | #562 |

## 3. Eksik belge akışı

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 3.1 | "Eksik belgelere bak" açık belge **taleplerine** değil, "Mükellef belgeleri" sayfasına (tüm faturalar, yanlış dönem) gidiyor. Kodda `open_documents` eylemi `engagementDocuments` rotasına bağlı. | Kırık | #563 |
| 3.2 | Geçiş 3 sn'den uzun ve ekranda hiçbir belirti yok; kullanıcı tekrar tıklıyor. | Güven | #576 |
| 3.3 | Varılan sayfada aynı kart tekrar çıkıyor; ikinci tıklama hiçbir şey yapmıyor (aynı adrese gidiyor). | Kırık | #563 |
| 3.4 | Mükellefe hatırlatma, talebi kapatma ya da "gelmeyecek, tahakkuk et" gibi gerçek hayattaki seçenekler yok. | Eksik | #563, #568 |

## 4. Banka

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 4.1 | Başlık "2 hareket bekliyor", sekme "6 eşleşmemiş". | Güven | #565, #576 |
| 4.2 | Satıra tıklayınca `?kayit=` adrese yazılıyor ama "Seçili kayıt" paneli sayfanın en üstünde açılıyor; kullanıcı görmüyor. Eşleşenler sekmesinde alt satırlarda sayfa zıplıyor. | Kırık | #564 |
| 4.3 | Aday listesi "Uyan açık fatura bulunamadı" deyip bitiyor: elle fatura arama, cariye (120/320) ya da başka hesaba (maaş, vergi, kira, virman) eşleştirme yok. | Eksik | #565 |
| 4.4 | Karşı taraf adı açıklamada yazılı olduğu hâlde "Karşı taraf" alanı boş. | Eksik | #565 |
| 4.5 | `BNK_UNDOCUMENTED_PAYMENT` ham kodu kullanıcıya gösteriliyor. | Güven | #576 |
| 4.6 | "Eşleşmeyi geri al" sonrası hareketin nereye döndüğü belirsiz; yeniden eşleştirme aynı panelden yapılamıyor. | Eksik | #565 |

## 5. Sıradaki iş (copilot)

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 5.1 | Sıra gerçekçi değil: fişler bitmeden "Rapor gönderildi" adımı görünüyor. Gerçek sıra: belge tamam → banka → kodlama/inceleme → fiş kontrol → aktarım → beyan → rapor. | Eksik | #562 |
| 5.2 | "Fişleri onayla" ve "Programa aktar" fişleri göstermeden plan penceresinden çalışıyor: müşavir neyi onayladığını görmüyor. | Kırık | #566 |

## 6. İnceleme kuyruğu

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 6.1 | "Kodu düzelt" çoğu öneride pasif ve nedeni yazmıyor; fiş alanı (fiş no/toplam) önerisinde değer düzeltilemiyor, yalnız onay/ret var. | Kırık | #569 |
| 6.2 | Tablo görünümü "0 öneri bekliyor" ve boş; kuyruk aynı anda 107 diyor. | Kırık | #569 |
| 6.3 | Öneri kartında belge görüntüsü yok: "Fiş no · gider fişi = 7776" doğru mu, kıyaslanacak bir şey yok. Tarih, tutar, satıcı da yok. | Eksik | #569 |
| 6.4 | "OCR alan güveni 0.41000000000000000", "demo-ocr" gibi ham metin. | Güven | #576 |
| 6.5 | Güven %40 iken başlıkta "Yüksek güven". | Güven | #576 |
| 6.6 | Cari öneri "120" ana hesap; gerçekte alt hesap (120.01.xxx) açılmalı ya da seçilmeli. | Eksik | #569, #572 |

## 7. Uyarılar

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 7.1 | Kenar çubuğu "Uyarılar 2", sayfa "Açık 209 · Engelleyici 3". | Güven | #576 |
| 7.2 | Engelleyici "Banka bakiyesi hareketlerle tutmuyor" panelinde fark yok: açılış, hareket toplamı, ekstre kapanışı ve fark gösterilmiyor; ekstreye bağlantı yok. Panel "kaydı burada düzelt" diyor ama düzeltilecek bir şey yok. | Eksik | #570 |
| 7.3 | Hiçbir şey düzeltilmeden engelleyici "Çözüldü" yapılabiliyor; gerekçe zorunlu değil. Denetim izi açısından en ciddi bulgu. | Kırık | #570 |
| 7.4 | "İlişkili kayıtlar" tablosunda `Kural BNK_BALANCE_MISMATCH` satırı, tarih ve tutar "—". | Güven | #570, #576 |

## 8. Komuta merkezi

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 8.1 | Dönem hücreleri ("Bekleyen iş var · 7", "Hazır · 6") tıklanmıyor; o dönemin işine gidilemiyor. | Kırık | #567 |
| 8.2 | Unvan mükellef özetine gidiyor ama seçili dönemi taşımıyor. | Eksik | #567 |
| 8.3 | Uyarısı olan dönem "Hazır · 7"; ayı bitmemiş Kasım "Hazır"; Ağustos onaylı, Eylül "Hazır", Ekim bekliyor gibi gerçekte olmayacak sıralar. | Güven | #567, #568, #576 |

## 9. Mizan hafızası ve yevmiye

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 9.1 | Sayfa ilk açılışta rastgele bir mükellefi (Selin Yılmaz) seçiyor; kullanıcı hangi mükellefin hafızasına baktığını fark etmiyor. Veri tarafında kurallar mükellefe bağlı (`account_rules.engagement_id`), ama ofis şablonu kopyası nedeniyle "karışık" görünüyor. | Güven | #572 |
| 9.2 | Mükellefin hesap planı yüklü değilken "Planda eksik hesap yok" diyor; plan yokken bu cümle yanlış güven veriyor. | Güven | #572 |
| 9.3 | Yevmiye de rastgele mükellefle açılıyor; üst çubuk "Tümü", sayfa "Kasım 2026": iki ayrı dönem göstergesi. | Kırık | #573 |
| 9.4 | Yeni fiş satırları "770.01" ile dolu geliyor; kullanıcı bunu kendi girdiği sanabilir. | Güven | #573 |

## 10. Demo verisi

| # | Bulgu | Tür | İş |
|---|---|---|---|
| 10.1 | Tüm mükelleflerde aynı kalıp: her kuyruk önerisi "gider fişi", aynı uyarı türleri. Geç gelen belge, yanlış kayıt ve düzeltme fişi, iade, avans, kasa farkı, çek/senet, SGK/muhtasar gibi rutin dışı işlemler yok. | Eksik | #568 |
| 10.2 | Çalışan (müşavir/stajyer) dağılımı ve iş ataması görünmüyor. | Eksik | #568 |

## Öncelik sırası

1. **#563, #562**: yanlış yere giden yönlendirmeler. Az emek, en çok görünen hata.
2. **#576**: güven hataları. Çoğu küçük, birlikte büyük etki.
3. **#570**: engelleyici uyarı panelinin farkı göstermesi ve gerekçesiz kapatmanın kalkması.
4. **#564, #565**: banka eşleştirme ve yan panel deseni.
5. **#566, #569**: fiş kontrol ekranı, inceleme kuyruğunda düzeltme.
6. **#567, #571, #572, #573**: kalan ekranlar.
7. **#568**: gerçekçi demo verisi. Bulguların düzeldiği gerçek senaryolarda doğrulanır.
8. **#574, #575**: Luca ve beyanname uçtan uca (sonraki aşama).
