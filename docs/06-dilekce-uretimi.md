# 06 · Dilekçe üretimi: kapsam ve yöntem (#56)

| | |
|---|---|
| Talep | T16 (`kaynak/musavir-entegrasyon-paketi/06-mali-musavir-talepleri.md`): *"Mesela dilekçematik olabilir. Birçok firmanın vergi dairesi, ticaret odası yazışmaları vesaire. Mesela tür değişikliği evrakları hazırlanacak."* Müşavir bunu "ileriki boyutlar" olarak konumladı (G§19). `kaynak/mali-musavir-portali-talepler.md` §13: "Verileri girince hazır çıkan dilekçe ve evraklar". |
| Tarih | 2026-11-16 |
| Durum | İlk dilim yazıldı (`backend/internal/modules/petitions`). **Bütün şablonlar taslak, hiçbiri hukuki kontrolden geçmedi.** |

## Karar

**Şablon tabanlı, ofisin kendi bastığı dilekçe.** Ürün dilekçeyi kuruma göndermez. Hukuki tavsiye de vermez: hangi dilekçenin verileceğine müşavir karar verir.

| | |
|---|---|
| Yöntem | Kodda tutulan, sürümlü şablonlar (`text/template`). Bir metnin her değişikliği kod incelemesinden geçer. Üretilen her dilekçe, şablon anahtarı ve sürümüyle, alan değerleriyle ve o anki tam metniyle saklanır (`petitions` tablosu, ofis ve engagement kapsamı, RLS). |
| Otomatik dolan alanlar | Mükellef: unvan, VKN/TCKN, vergi dairesi (#3). Ofis adı. Hazırlayan: kullanıcının adı ve SMMM oda sicil no (#5). Tarih. |
| Kullanıcının girdiği alanlar | Şablona özgü alanlar: ör. yazının sunulacağı yer, sicil no, genel dilekçede konu ve metin. Zorunluluk ve uzunluk sınırı var. Kullanıcı metni şablon olarak işlenmez, aynen basılır. |
| Çıktı | Ekranda önizleme ve A4 PDF (DejaVu, Türkçe karakter). |
| Taslak işareti | Her şablon `draft: true`. Önizlemede ve PDF'in üst ve alt kısmında *"TASLAK – Bu metin hukuki kontrolden geçmemiştir. Kullanmadan önce gözden geçiriniz."* yazar. Bir şablon ancak hukuk kontrolünden geçince `Draft: false` yapılmalı. |
| Yetki | Okuma ve önizleme `tenant.engagements.read`, kaydetme `tenant.engagements.write`. Yalnızca aktif engagement'ta çalışır. |

### İlk şablonlar (hepsi taslak, sürüm 1)

| Anahtar | Muhatap | Not |
|---|---|---|
| `vd_borc_durum_yazisi` | Vergi dairesi | Borç durum yazısı talebi. Aynı belge e-Devlet'ten de alınabilir (`05` §1). Dilekçe, kurumun elden istediği durumlar için. |
| `vd_mukellefiyet_yazisi` | Vergi dairesi | Mükellefiyet yazısı talebi |
| `ts_faaliyet_belgesi` | Ticaret sicili müdürlüğü | Faaliyet belgesi talebi |
| `genel_dilekce` | Kullanıcı yazar | Konu ve metin serbest. Muhatap, konu ve metin kullanıcıda. |

## Kapsam dışı ve sınırlar

- **Resmi form gerektiren talepler kapsam dışı.** Bazı başvuruların, ilgili tebliğin ekindeki forma uygun yapılması zorunlu. Örnek: 7326 sayılı Kanun'a dayanan talepler için 1 Seri No.lu Genel Tebliğ ekindeki dilekçe formları ([muhasebetr](https://www.muhasebetr.com/yazarlarimiz/mustafaalpaslan/0537/)). Uymayan dilekçe vergi dairesince düzeltme istenmesine yol açabiliyor ([muhasebetr](https://www.muhasebetr.com/yazarlarimiz/ufuk/018/)). Bu talepler için ürün kendi metnini değil resmi formu kullanmalı. İlk dilimde bu tür şablon yok.
- **Tür değişikliği evrakı** (T16'daki örnek): genel kurul kararı, ana sözleşme ve sicil başvurusu gibi bir evrak setidir. Tek bir dilekçe değil. Ayrı ve hukuk kontrollü bir iş olarak ele alınmalı.
- **Elektronik gönderim yok.** İnteraktif Vergi Dairesi ya da DVD üzerinden dilekçe gönderimi `05`'teki sınırlara tabi: resmi API bulunamadı, şifreyle oturum açılmaz.
- **İmza:** PDF'te kaşe ve imza alanı var. e-imza ile imzalama yok.

## Rakipler

- Muhasebe yazarları "dilekçematik" uygulamalarından ve vergi dairesi dilekçe örnek setlerinden söz ediyor ([muhasebetr](https://www.muhasebetr.com/yazarlarimiz/ufuk/018/)).
- Uyumsoft MüşavirPro (ekoSMMM) ve Logo Mali Müşavir kurum entegrasyonlarını öne çıkarıyor ([Uyumsoft](https://uyumsoft.com/haber/mali-musavirler-ekosmmm-ile-dijitallesiyor), [Logo](https://www.logo.com.tr/en/urun/logo-mali-musavir-3)). Dilekçe modülü olup olmadıkları tanıtım sayfalarından **anlaşılamadı**.
- Bu ürünün farkı: dilekçe mükellef ve ofis verisiyle dolu gelir, engagement'a bağlı saklanır ve ofisler arası görünmez (D1b, #6 RLS).

## Karar gerekiyor

1. **Hukuki kontrol:** Dört şablonun metni bir avukata ya da meslek odasına okutulmalı. Okunan şablonlarda `Draft: false` yapılıp `Version` artırılmalı.
2. **Şablon kütüphanesinin genişletilmesi:** Hangi dilekçelerin ekleneceği müşavirden alınmalı. Resmi form isteyen talepler ayrı tutulmalı.
3. **Ofise özel şablon:** Ofisin kendi şablonunu yazması bu dilimde yok. İstenirse hukuki sorumluluk ofiste kalacak biçimde ve "ofis şablonu" etiketiyle eklenebilir.

## Doğrulananlar

- Dört şablon örnek alanlarla işleniyor ve hepsi taslak işaretli. PDF üretiliyor; Türkçe karakterler ve taslak notu PDF'i görüntüye çevirip bakarak kontrol edildi.
- Mükellef, ofis ve hazırlayan alanları engagement'tan doluyor. VKN ve TCKN ayrımı doğru.
- Üretilen dilekçe yalnızca kendi engagement'ında okunuyor. Başka ofis, uuid'i bilse bile okuyamıyor (RLS, `service_db_test.go`).
- Kullanıcı girdisindeki şablon sözdizimi işlenmiyor.
