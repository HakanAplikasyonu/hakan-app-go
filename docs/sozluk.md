# Sözlük: tek kavram, tek terim

Web arayüzünde (ofis paneli, mükellef paneli) ve kullanım kılavuzlarında
(`docs/kullanim/`) her kavram tek bir adla anılır. Yeni metin yazarken bu
tabloya bakın.

Denetim: `node scripts/check-sozluk.mjs` (ön yüzden `pnpm sozluk:check`). Yasak
varyantları `frontend/src/locales/tr/*.json` değerlerinde ve
`docs/kullanim/*.md`'de arar; CI'da ön yüz işinin "Sözlük" adımında koşar.
Makine okunur liste ve izin verilen istisnalar: [`sozluk.json`](sozluk.json).
Yeni bir yasak varyant eklemek için oraya bir kural, meşru bir kullanım için
`allow` kaydı (dosya, isteğe bağlı satır metni ve gerekçe) ekleyin.

Kapsam yalnızca görünen metindir: API değerleri, enum anahtarları, i18n anahtar
adları, URL yolları ve veritabanı değerleri değişmez.

## Denetlenen terimler

| Kanonik terim | Anlamı | Yasak varyantlar | Not |
|---|---|---|---|
| **şifre** | Kullanıcının hesap şifresi; dosya koruması (şifre korumalı, şifreli); şifre yöneticisi | parola | Masaüstü ajanı ve mobil uygulama kendi arayüzlerinde hâlâ "Parola" der; kılavuzlarda o ekranları anlatan satırlar istisnadır. İki uygulamanın metni ayrı bir işte hizalanmalı. |
| **giriş yap** | Hesaba girmek | oturum aç | "Oturum" yalnız cihaz oturumu içindir: **Oturumu kapat**, Oturumlar listesi. |
| **bağlantı** | Tıklanınca açılan adres (davet bağlantısı, yanıt bağlantısı) | link | |
| **olası mükerrer** | Aynı kaydın ikinci kez geldiği şüphesi | muhtemel mükerrer | Durum adı **Mükerrer**. "Kopyala / kopyalandı" pano işlemidir, mükerrer anlamında kullanılmaz. |
| **karşı taraf** | Faturanın ya da cari hesabın öbür tarafı | karşı firma | |
| **uzantı** | Tarayıcı uzantısı (WhatsApp Web uzantısı) | eklenti | |
| **ofis** | HAKAN'daki muhasebe ofisi (kiracı) | büro, müşavirlik ofisi, mali müşavirliğiniz | Mükellefe dönük metinde "mali müşaviriniz" ya da "mali müşavir ofisi" kullanılabilir. Rol adı **Mali Müşavir** ayrıdır. Demo ofislerinin unvanı (Demir Mali Müşavirlik) özel addır. |
| **e-posta** | Elektronik posta | e-mail, eposta, mail adresi | |
| **ekstre** | Bankanın verdiği hesap hareketleri dosyası (banka ekstresi) | hesap dökümü, banka dökümü, hesap özeti | Profildeki "Hesap özeti" kullanıcı hesabının özetidir, istisnadır. "Bordro dökümü", "personel dökümü" ayrı belgelerdir. |
| **Z raporu** | Yazar kasanın (ÖKC) gün sonu raporu | gün sonu raporu, z raporu, Z Raporu | "Yazar kasa (ÖKC) gün sonu raporları" gibi açıklayıcı cümle serbesttir. WhatsApp uzantısındaki "Z raporu olarak kaydet" bu yazımdadır. |
| **e-Fatura, e-Arşiv, e-Defter** | GİB sistemleri ve belgeleri | e-fatura, E-fatura, e-arşiv, e-defter | GİB yazımı; cümle başında da küçük "e". Ekler aynı kalır: e-Faturaları, e-Defterde. |
| **Vazgeç** | Diyaloğu işlem yapmadan kapatan düğme | İptal (düğme olarak) | "İptal" fatura / aktarım iptali ve **İptal edildi** durumu içindir. Denetim `*cancel` anahtarlı değerlere bakar. |

## Ayrı kalan kavramlar (birleştirilmez)

Aşağıdaki çiftler eş anlamlı değildir; ikisi de doğru yerde kullanılır.

| Terim | Anlamı | Karıştırılmaması gereken | Not |
|---|---|---|---|
| **mükellef** | Ofisin hizmet verdiği işletme (ofisin müşterisi) | müşteri, firma | Ofisin müşterisi için her zaman "mükellef". |
| **müşteri** | Mükellefin kendi müşterisi (alıcı cari, 120) | mükellef | Cari türü: Müşteri / Tedarikçi. |
| **firma** | Karşı taraf şirket ya da dış sistemdeki şirket kaydı (Easytrade firması, mizan hafızasında "aynı firmanın faturaları") | mükellef | Mükellef yerine kullanılmaz. Faturanın öbür tarafı tablo başlıklarında "Karşı taraf". |
| **masraf** | Mükellefin yüklediği harcama belgesi (Masraflar sayfası, masraf fişi) | gider | |
| **gider** | Muhasebe kavramı: gider hesabı, gider pusulası, personel gideri | masraf | |
| **fiş** | Mükellefin gönderdiği alışveriş fişi (fotoğraf) **ve** muhasebe fişi (yevmiye fişi, fiş onayı) | makbuz, belge | İki anlam ürün genelinde yerleşik; bağlamdan ayrılır. Gerekirse "muhasebe fişi" diye açın. |
| **makbuz** | Serbest meslek makbuzu ve benzeri belge türü | fiş | |
| **belge** | Genel üst kavram: fatura, fiş, ekstre, PDF… | | "Önemli belgeler", "belge isteği". |
| **onay bekliyor** | İnceleme / onay kuyruğundaki kayıt | beklemede | **Beklemede**: kabul edilmemiş davet ya da kullanıcı durumu. |
| **okunamadı / okunamayan** | Fotoğraf ya da dosya içeriği çıkarılamadı (OCR, bozuk dosya) | hatalı | **Hatalı**: okunan ama geçersiz olan değer (hatalı satır, hatalı kod, e-posta veya şifre hatalı). |
| **dönem** | Beyan / muhasebe dönemi (aylık ya da 3 aylık) | ay | Takvim ayı söz konusuysa "ay" (ay şeridi, ayın bordrosu). |
| **personel** | Mükellefin bordrolu çalışanları (Personel ve bordro, MUHSGK eki) | çalışan | "Çalışan" mükellef kullanıcısının uygulamadaki yetkisi (Sahip / Çalışan) ve "ofis çalışanı" kılavuz adıdır. Ofis rolü **Personel** (tenant rolü) ayrı bir kavramdır. |
| **yükle** | Dosyayı sisteme almak (fatura yükle, ekstre yükle) | gönder | **Gönder**: bir kişiye iletmek (davet gönder, mükellefin fişini ofise göndermesi, e-posta gönder). |
| **kaydet** | Formdaki değişikliği yazmak | sakla | **Sakla**: tutmak / göstermemek (dosya saklanır, gizle). |
| **içe aktar / dışa aktar** | Dosyadan toplu veri almak / vermek | yükle, indir | Luca'ya **aktarım** ayrı kavramdır. |

## Bu sözlüğün dışında kalanlar

- Masaüstü ajanı (`desktop/`) ve mobil uygulama (`mobile/`) kendi
  arayüzlerinde "Parola" ve "oturum aç" kullanır. Bu iş yalnız web arayüzünü
  kapsar.
- Sunucudan gelen Türkçe hata ve bildirim metinleri
  (`backend/internal/platform/i18n/catalog.go`) bu denetimin dışındadır.
