# 06 · Mali Müşavir Talepleri ve Portalın Sınırları

Bu dosya portalın **neden** var olduğunu ve müşavirin **gerçekte ne istediğini** anlatır. Her talep müşavirin kendi cümlesine bağlıdır. Müşavirin söylemediği her şey ayrıca işaretlenir. Diğer dosyalar "veri nasıl toplanır" sorusunu cevaplar; bu dosya "hangi ihtiyaç için" sorusunu cevaplar ve paketin hangi talebi karşılayıp hangisini karşılamadığını gösterir.

## 0. Kaynaklar ve etiketler

| Kod | Kaynak |
|---|---|
| **G** | 7 Kasım 2026 yüz yüze görüşme. Müşavir ofisi: Kadir Bey ve kıdemli ortak. Tam döküm: `kaynak/mali-musavir-gorusme-transkript.md`. Atıflar dökümün bölüm numarasıyla verilir: **G§6** = dökümün 6. bölümü. |
| **Y** | 8 Kasım 2026 yazılı görüşme. Mustafa Bey (mali müşavir). |

| Etiket | Anlamı |
|---|---|
| **[Talep]** | Müşavir kendisi istedi veya sorun olarak anlattı |
| **[Onay]** | Easytrade önerdi, müşavir açıkça "evet / aynen öyle" dedi |
| **[Referans]** | Müşavir bugün kullandığı bir programın nasıl çalıştığını anlattı; doğrudan "bunu yapın" demedi ama beklenti olarak okunmalı |
| **[Easytrade]** | Müşavir söylemedi; Easytrade'in önerisi veya Easytrade tarafının görüşmede söylediği |

| Durum (bu paketteki karşılığı) | Anlamı |
|---|---|
| **Var** | Veri toplama katmanında karşılanıyor (dosyası belirtilir) |
| **Kısmen** | Veri toplanıyor ama talebin kendisi (ekran, kural, çıkış) başka bir dokümana kalıyor |
| **Yok** | Bu pakette karşılığı yok |

## 1. Müşavirin ana derdi

İki şey: **mükellefe evrak almaya gitmemek** ve **tek tek elle işlememek.**

> "Normalde faturalar, her şey internetten geliyor, adama gitmeye gerek yok. Ama Z fişlerinin entegrasyonunu çözdüğünüz zaman bu da çok büyük yük kaldırıyor." (G§3)

> "Şimdi fatura almaya gitmiyorsun. E-arşiv fatura almaya gitmiyorsun, onları görüntülüyorsun. Ama Z fişi almaya gidiyorsun. Bir de tek tek işleniyor." (G§3)

> "Z raporunu ortadan kaldırdık diyelim. Masraf fişlerini de kaldırdığın zaman artık gitme ortadan kalkıyor. Mali müşavirler için süper bir şey oluyor." (G§4)

> "Biz şimdi bir haftadır mükelleften banka ekstresi almak için uğraşıyoruz." (G§6)

Bugün bu boşlukları kapatmak için Luca'nın yanında ayrıca ücretli programlar kullanıyorlar ve hepsini tek yerde istiyorlar:

> "Biz de geliştirmediği için MKLF diye bir program aldık, Rahat Aktarım diye bir program aldık. Bunları ben tek bir yerden yapmak isterim. Ama ekstradan bir sürü şeye dijital ücret ödüyorum." (G§12, kıdemli ortak)

E-tebligat takibini de ayrı bir programla (e-Mükellef) yapıyorlar (G§16).

**Bugünkü durum, belge türüne göre (müşavirin anlattığı):**

| Belge | Bugün | Kaynak |
|---|---|---|
| E-fatura, e-arşiv | Mükellefe gidilmiyor; e-fatura sisteminden geliyor, görüntüleniyor | G§3; Y: "kullandığı e fatura sisteminden direk kaynaktan ara program ile direk lucaya entegre ediyoruz" |
| Z raporu | Mükelleften alınıyor veya QR ile tek tek okutuluyor, tek tek işleniyor | G§3 |
| Masraf fişi | Mükelleften alınıyor | G§4 |
| Banka ekstresi | Mükelleften isteniyor, PDF gelince Excel'e çevriliyor; bir kısmında Luca'nın banka bağlantısı var | G§6 |
| Tahsilat makbuzu / tediye | Tek tek elle işleniyor | G§13 |

## 2. Müşavirin bugünkü iş akışı

Ürünün oturacağı zemin bu. Portal bu akışı **değiştirmeye değil, beslemeye** çalışır.

1. **Merkez Luca.** Kayıtlar Luca'ya giriliyor, beyanname Luca'da oluşturulup gönderiliyor:
   > "Herhangi bir yere çıkmıyor. Sadece Luca'nın içerisinde kalıyor. … beyanname formatına uygun şekilde beyannameyi hazırlıyor." (G§9, Kadir Bey)
   > "Luca'ya 'KDV beyannamesi oluştur' diyorsun. … Oluşturuyor, gönderiyor. Bir de e-defter mükellefleri için kayıtlar aylık ya da 3 aylık şekilde e-defter sistemine, maliyeye gönderiliyor." (G§9)
   > "e-defter değilse … işletme defterine göre defter tutanlar için Defter-Beyan sistemi var. Defter-Beyan sistemine de Luca aracılığıyla gönderiyoruz." (G§9)
2. **Faturalar entegratörden geliyor.** Ön muhasebe programından değil:
   > "On muhasebeyi kendi içinde program ile yapanlar var hiç on muhasebe programı kullanmayanlar var, kullanan mükellefler paraşüt, uyumsoft, logo veya daha yerel program kullaniyorlar, fakat biz on muhasebesinden ziyade kullandığı e fatura sisteminden direk kaynaktan ara program ile direk lucaya entegre ediyoruz" (Y)
   > "Büyük çoğunluğu yüzde 80 vardır heralde , işnet entegrator sistem" · "Kalanı uyumsoft, Finansbank, edm, gibi sistemler" (Y)
   Ara program: Rahat Aktarım (Y, G§12).
3. **Hesap planı ve kural ezberi.** Program bir kez öğreniyor, sonra kendisi işliyor (T1).
4. **Kontrol için fatura görüntüsü.** Yevmiye kaydının yanında faturaya bakıyorlar (T2).
5. **Ek programlar:** MKLF, Rahat Aktarım, e-Mükellef (G§12, G§16).

## 3. Talepler

Her talepte: müşavirin sözü, ne istendiği, etiket, bu paketteki karşılığı.

### T1. Hesap planı senkronu, kural ezberi, otomatik cari

> "Programdan direkt Luca'dan kendisi onu çekiyor aslında. Güncelliyor sürekli. Ya da ben Paraşüt'ten bir hesap açarsam aynı anda Luca'ya da bunu entegre ediyor aslında direkt." (G§1)

> "Diyorsunuz ki bu firmanın faturalarını, daha doğrusu kestiğimiz faturaları 600'e işle, 391'e işle. Yani satış matrahını ve KDV'sini işle. Geri kalanında cariler içerisinde TC kimlik numarasına göre, vergi numarasına göre ayarla, bu kaydı at diyorsun." (G§1)

> "Sonra geliyor, 120'yi carilerden bulursa buluyor. Öyle bir cari yoksa otomatikman açıyor kendisi." (G§1)

> "Dolayısıyla bunu ezberlediği zaman … Hiçbir ek işlem yapmadan otomatikman bu satış faturaları muhasebeleştiriliyor." (G§1)

| | |
|---|---|
| Etiket | **[Referans]** (Paraşüt–Luca nasıl çalışıyor). Beklenti açık: portal da en az bu kadar otomatik olmalı. |
| İstenen | Hesap planının muhasebe programından çekilmesi ve güncel tutulması; müşavirin bir kez tanımladığı hesap kuralının (600 / 391 / 120) sonraki bütün kayıtlarda uygulanması; cari yoksa VKN/TCKN ile otomatik açılması |
| Bu paket | **Kısmen.** Veri tarafı hazır: cari VKN/TCKN ile tekilleşiyor (`01` §4.3), kasa/banka için hesap kodu önerisi (`cash_box.suggested_ledger_code`, `01` §4.8), gider kategorileri standart türe eşleniyor (`01` §4.11). Hesap planı çekme, kural ezberi ve Luca'ya kayıt **Luca çıkış dokümanının** işi; bu pakette yok. |
| Not | Görüşmede Easytrade tarafı "Luca ile zaten entegrasyonumuz var. Luca, Mikro, Orka, Zirve, bunların hepsiyle entegrasyonumuz bulunuyor." dedi (G§8, **[Easytrade]**). Luca çıkışı bu mevcut entegrasyon üzerine kurulmalı. |

### T2. Kaydın yanında fatura görüntüsü

> "Direkt bütün hepsinde, yevmiye kaydının yanında ben faturayı da görüyorum. İçeriğine de bakmak için oradan faturanın görüntüsünü görebiliyoruz direkt." (G§2)

> "Fatura görüntüsü için programa gir, programdan şuraya gir, gelen fatura… Yani bir sürü iş var ya." (G§2)

> "Bazen aynı firmadan ya da farklı firmalardan yüklü fatura alan firmalar var. 3.000 tane, 2.000 tane fatura alan da var, 100 tane alan da oluyor. … içeriğinde farklı bir şey almış olabiliyor, demirbaş almış olabiliyor. … Program bize aslında uyarı veriyor … Ama biz yine herhangi bir hata olmasın diye faturanın görüntüsüne bakıyoruz." (G§2)

| | |
|---|---|
| Etiket | **[Talep]** |
| İstenen | Her faturanın orijinal görüntüsü tek tıkla, kaydın yanında |
| Bu paket | **Kısmen.** Görüntü ve UBL toplanıyor: UBL her zaman (`31` §1), PDF kaynak veriyorsa (Easytrade `download-pdf` `09` §3.1, İşNet `GetDocumentViewerLink` `32` §7, Uyumsoft `Get*InvoicePdf` `33` §6, İzibiz `GetInvoiceWithType` `36` §5.4). Saklama: `attachment` (`01` §4.16, `02` §7). Görüntünün kayıt yanında açılması müşavir paneli ve Luca çıkışı tarafında. |

### T3. Akıllı uyarılar

> "Fatura içeriğinde, diyor ki, sen satış yapıyorsun, danışmanlık bedeli satışı yapıyorsun, açıklamada bir tane araba faturası gördüm, bunun kaydı farklı olabilir diyor. Veya mal alıyorsun, bunun içerisinde bilgisayar açıklaması var, bu demirbaş olabilir diyor." (G§2)

Easytrade önerdi: "firmanın ortalama alış fatura tutarı 150 bin liradır, bir bakarsınız 2 milyon liralık fatura gelmiş. 'Burada bir problem var mı?' diye algılayabilmesini sağlayabiliriz." Müşavir: "Evet, zaten bizim aktarım programımızda… Her ay yaklaşık 100 milyon fatura oluşuyor, bu sefer 1 milyonluk gelmiş gibi." (G§2)

| Uyarı | Etiket | Kural (öneri) | Uyarı kodu (`05`) |
|---|---|---|---|
| Demirbaş olabilir (mal alımında "bilgisayar" gibi kalem) | **[Talep]** | Gelen faturada `invoice_item.name` sabit kıymet anahtar kelimeleriyle eşleşiyor veya tutar müşavirin belirlediği sınırı aşıyor → `line_type=fixed_asset_candidate` | `FIXED_ASSET_CANDIDATE` |
| Kayıt türü farklı olabilir (danışmanlık satan firmada araç satışı) | **[Talep]** | Kalem, mükellefin olağan satış / alış kalemlerinden belirgin farklı (mükellefin geçmiş kalem adları ve kategorilerine göre) | `UNUSUAL_RECORD_TYPE` |
| Olağandışı tutar | **[Onay]** | Fatura tutarı, aynı karşı tarafın (yoksa mükellefin) son 12 ay ortalamasının müşavirin belirlediği katını aşıyor | `UNUSUAL_AMOUNT` |
| Yeni karşı taraf + yüksek tutar | **[Onay]** | İlk kez görülen karşı taraf ve tutar müşavirin belirlediği sınırın üstünde | `NEW_PARTY_HIGH_AMOUNT` |

| | |
|---|---|
| Bu paket | **Kısmen.** Kuralların beslendiği veri var (kalem adı, karşı taraf, tutar, geçmiş; `01` §4.4–4.5). Uyarı kodları `05`'e eklendi. Eşikler ve anahtar kelime listesi **ürün kararı**; mükellef bazında ayarlanabilir olmalı. |

### T4. Mükerrer fatura kontrolü

> "Faturada da mesela fazla fatura işledi, mükerrer işledi, aynı fatura." (G§14)

> "Luca mesela şunu yapıyor: Gelen faturayı Luca'nın içerisinden işlersem, yanında fiş numarasını veriyor. 'Bu faturayı siz bu fişte işlediniz' diye yazıyor. Ama manuel işlersem, iki defa işlersem, bir ekran daha var: Mükerrer fatura kontrolü. … Fatura numarası ve işlenen cariden buluyor aslında." (G§14)

| | |
|---|---|
| Etiket | Konuyu Easytrade açtı, müşavir sorunu ve Luca'daki çözümü anlattı: **[Talep]** + **[Referans]** |
| İstenen | Aynı faturanın iki kez işlenmesinin yakalanması; fatura numarası + cari ile |
| Bu paket | **Var** (veri tarafı). ETTN, sonra cari VKN + fatura no + tarih, sonra olası mükerrer (`01` §6, uyarı `POSSIBLE_DUPLICATE`). Aynı fatura farklı kaynaklardan gelse de tek kayıt. "Hangi fişte işlendi" bilgisi Luca çıkışı tarafında. |

### T5. Z raporu otomatik aktarımı

> "O Z fişlerinin otomatikman bizim muhasebe programımıza entegre olması." (G§3)

> "Adamın 3 tane şubesi oluyor, 30 tane Z fişi oluyor. Bunları tek tek işlemek… 'Fotoğrafını QR ile okut' diyorlar, 'hallet' diyoruz ama çok pratik olmuyor bence." (G§3)

> "Ya sizden Luca'ya gidecek ya Paygo'dan Luca'ya gidecek. Ama sonuç olarak bu çok büyük bir yük." (G§3)

> "Ek olarak, sizin sistemi kullanıyor ama Paygo kullanmıyor. Farklı anlaşmalar da işe yarar." (G§3)

| | |
|---|---|
| Etiket | **[Talep]**. Bütün markalarla (Beko, Ingenico, Hugin) entegrasyon fikri Easytrade'in (G§3, **[Easytrade]**); müşavir "farklı anlaşmalar da işe yarar" dedi. |
| Müşavirin vurgusu | "çok büyük yük kaldırıyor" |
| Bu paket | **Yok.** Kapsam dışı (`00` §10). Easytrade kullanan mükellefte perakende satışlar (`retail_sale`) çekiliyor (`09` §4) ama Z raporu verisi değil. |
| Müşavirin uyarısı | Yazarkasa tavsiyesi riski, §4 ilke 6 |

### T6. Masraf fişi

> "Muhasebe bürolarının en önemli sorunu masraf fişleri. Normalde masraf fişlerinin bir kısmını devlet gider olarak kabul etmiyor. Ama mükellef geliyor, diyor ki 'kardeşim bunları işle.' Geçse de işle, geçmese de işle diyor." (G§4)

> "Diğer firmaların yaptığı QR okuma oluyor … Adam 'ben niye bunları tek tek okutayım' diyor. … piyasa ne yapıyorsa onu yapacaksınız. Piyasa mükellefe cep telefonundan bir ekran açıyor. Mükellef girecek, yapacak, yaptığı zaman mali müşavire düşecek. En azından piyasanın yaptığını yapmanız lazım." (G§4)

> "Bundan sonra teknoloji e-fişe dönerse orada sorun yok." (G§4)

| | |
|---|---|
| Etiket | **[Talep]** |
| Müşavirin vurgusu | "en önemli sorun" |
| İstenen (asgari) | Mükellef telefondan fişi girer, müşavirin önüne düşer |
| Bu paket | **Kısmen.** Easytrade kullanan mükellefte masraflar ve fiş görseli çekiliyor (`09` §6, `expense.receipt_url`). Gider kabulü kararı için alan var (`expense.is_deductible`, varsayılan `unknown`, müşavir karar verir; `01` §4.12). Easytrade kullanmayan mükellef için mobil fiş girişi ve OCR **yok** (kapsam dışı). |
| Not | Easytrade tarafı görüşmede mevcut uygulamada fiş okutma olduğunu, müşavire liste (fiş kodu, KDV oranları, ürün grubu) ve A4'e yerleşmiş görsel olarak verdiğini anlattı (G§4, **[Easytrade]**). |

### T7. Yakıt fişi

> "Benzin fişlerinde Luca'nın bir çalışması var, çekme. Onun sistemini inceleyip, onlar çekebiliyorsa siz de çekmeniz lazım." (G§5)

> "Çünkü masraflar çok yüksek olduğu için bazen yakıt fişleri gelmeyebiliyor. … Luca bunları direkt çekmeye çalışıyor ama çok sağlıklı olmuyor. … Eğer onu da alırsanız Luca'dan daha iyi olabilir yani." (G§5)

> "Çünkü bir adamın benzin fişi için 'geçmiyor' diyemezsin. Plaka var. … Dolayısıyla onları işlersen diğerleri daha hafif kalıyor." (G§5)

| | |
|---|---|
| Etiket | **[Talep]** + **[Referans]** (Luca'nın çekme özelliği) |
| Bu paket | **Yok.** Luca'nın yakıt fişini nereden ve nasıl çektiği bilinmiyor; açık iş (§7). Alanlar hazır: `expense.receipt_type=fuel_receipt`, `vehicle_plate` (`01` §4.12). |

### T8. Plaka kontrolü

Easytrade sordu: "Bu adamın üzerine bu plaka kayıtlı mı değil mi? Çünkü kayıtlı değilse aslında geçerli değil. Doğru mu?" Müşavir:

> "Evet, evet. Böyle bir imkânınız varsa süper olur. Genelde şöyle oluyor: Kayıtlı olanı da olmayanı da alıyor, işliyor. Sonra 'aa, bu kayıtlı değilmiş' oluyor." (G§5)

> "Adam arabayı satıyor, fatura kesmiyor. Veya alıyor, fatura almıyor. Dolayısıyla siz 'kayıtlı değil' diyeceksiniz, adam 'niye kayıtlı değil' diye kendi sistemine bakacak. Bu aslında bir çözüm olacak. Mali müşaviri büyük bir yükten kurtaracak." (G§5)

| | |
|---|---|
| Etiket | **[Onay]** |
| Bu paket | **Kısmen.** Plaka normalize ediliyor (`expense.vehicle_plate`, `01` §4.12). Mükellef adına kayıtlı araç listesinin **nereden alınacağı** belirsiz; bu çalışmada otomatik bir kaynak bulunmadı. |
| Öneri **[Easytrade]** | İlk sürümde araç listesi müşavir / mükellef tarafından portalda tanımlanır (plaka, ruhsat sahibi VKN/TCKN, geçerlilik tarihleri); fişteki plaka listede yoksa veya fiş tarihi geçerlilik dışındaysa uyarı `PLATE_NOT_REGISTERED`. Otomatik kaynak bulunursa liste oradan beslenir. |

### T9. Banka

Müşavir üç seviye anlattı:

**Seviye 1, ekstre dönüştürme:**

> "Bir PDF'i, Excel'i yüklüyorsun. PDF'i Excel'e çeviriyor, yani Luca'nın anlayacağı dile çeviriyor. Öteki türlü, biz Excel istiyoruz ama mükellef PDF gönderiyor. Onu çevirmesi bir sürü işimizi alıyor." (G§6)

> "Sadece 102 hesabını bir kere tanımlıyorsun, karşı hesabı yine ezberden alıyor. Dolayısıyla manuel işlemekten yarı otomatiğe dönüyor." (G§6)

**Seviye 2, Luca'nın banka bağlantısı:**

> "'Al bu sözleşmeleri bankalara götür' diyor. … 'Günlük olarak ben sana bunu göndereceğim' diyor." (G§6)

> "Bu da tamam ama bazen hatalı çekebiliyor." · "Şu çok zor oluyor: Çoğu mükellef için alıp bankaya bile götürtemiyorsun adamı." · "Bankaya gidiyor, evrakı veriyor, bize kopyasını göndermiyor. … Diğerine göre çok iyi mi? İyi. Ama yine de bir yükü var." (G§6)

**Seviye 3, onay linki (müşavirin "ideal" dediği):**

> "Bunun daha ideali şu. Bir programda bunu yapıyoruz, adam link atıyor. Tabii bütün bankalarda olmadığı için çok kullanışlı değil. Sallıyorum, Garanti Bankası'nda kesin var. 3 bankayla anlaşması var herhalde. Mükellefe link atıyoruz. Mükellef bir kere girip linki onaylıyor. 3 ay boyunca bana banka bilgileri, ekstre görüntüleri geliyor." (G§6)

> "Direkt görüntü geliyor. PDF'i geliyor. … Program kendi içinde o PDF'i işlenebilir bir şeye çözümlüyor." (G§6)

> "Adama 'Abi WhatsApp'tan gir, şuna onay ver' diyorsun, olay bitiyor, banka geliyor." (G§6)

| Seviye | Etiket | Bu paket |
|---|---|---|
| 1. PDF / Excel ekstre → muhasebe programının anlayacağı biçim | **[Talep]** | **Kısmen.** Yükleme ve ayrıştırma var (PDF, Excel, MT940; `23`), hareketler sınıflanıyor ve faturalarla eşleşiyor (`21`). Luca'nın anlayacağı Excel'e çıkış ve 102 hesap eşlemesi **Luca çıkış dokümanında.** |
| 2. Şube talimatı + bankanın servisi | **[Referans]** | **Var.** Portalın "hesap bağlama" yolu bu (`20`, `22`). **Müşavirin anlattığı zorluk bizde de aynen geçerli:** mükellefin şubeye talimat götürmesi gerekiyor. Portal bunu talimat yazısını hazırlayarak ve adım adım takip ederek hafifletir (`20` §5), ortadan kaldırmaz. |
| 3. Onay linki | **[Talep]** ("ideal") | **Yok.** Bu akış açık bankacılık veya aracı hizmet sağlayıcı yapısına benziyor. Easytrade'in mevcut kararı "aracı firma ve açık bankacılık kullanılmaz" (`20` §2) bu seviyeyi dışarıda bırakıyor. **Karar gerekli** (§6). Müşavir örnek linki gönderecek (§7). |

### T10. KDV erken uyarısı

> "Adam ne yapıyor? Evrakı bize takip eden ayın 20'sinde gönderiyor. Adama KDV ödemesi çıkıyor. Diyor ki 'keşke önceden haberim olsaydı.'" (G§7)

> "Çünkü bazı programlarda faturayı kesiyorsunuz, alışı yapıyorsunuz ama o durum ekranda gözükmüyor. … 100 bin, 200 bin ödeme çıkınca bir anda ortam geriliyor." (G§7)

> "Ödemeye döndüğü zaman programın adama ışık yakması lazım. 'KDV ödemen 100 lira oldu, 200 lira oldu' diye, aralarda sıkmayacak şekilde ışık yakmak lazım. Ya da mail atması lazım." (G§7)

> "SMS göndermesi lazım vesaire. Ya da, atıyorum, ay sonuna geldi. … Programın adama demesi lazım ki 'Ayın 28'i geldi, 500 lira KDV ödemen var. Eğer fatura bulmak istiyorsan son 2 gün.'" (G§7)

| | |
|---|---|
| Etiket | **[Talep]**. "Müşavir eşiği mükellef bazında bir kez söyler" fikri Easytrade'in (G§8: "Mali müşavir bana 'bu adamın KDV'si 300'ün üzerine çıkarsa uyarı ver' der", **[Easytrade]**). |
| İstenen | Mükellefe dönem içinde tahmini KDV durumu; tutar arttıkça uyarı; ay sonuna doğru (ör. 28'i) hatırlatma; SMS / e-posta / bildirim; sıkmayacak sıklıkta |
| Bu paket | **Kısmen.** Hesap için veri var: satış ve alış faturalarının oran bazında KDV'si (`invoice_tax_summary`, `01` §4.6), masraf KDV'si (`01` §4.12), kaynağın KDV raporu (`09` §8). Devreden KDV Easytrade'de var; diğer kaynaklarda yok, müşavir girer **[Easytrade]**. Hesap kuralı, bildirim kanalları ve sıklık **ürün dokümanında.** |
| Dikkat | Erken uyarı tahminidir; dönem içinde gelmemiş faturalar ve kâğıt belgeler hesapta yok. Mükellefe gösterilen metin bunu söylemeli **[Easytrade]**. |

### T11. Tahsilat makbuzu ve ödeme türü tutarlılığı

> "Tahsilat makbuzları mesela. Mükellef Easytrade kullanıyorsa, oradaki tahsilatların ve tediyelerin sisteme girilip muhasebeye aktarılması önemli. Diğer programlarda bunu yapmıyorum, tek tek işliyorum." (G§13)

Easytrade sordu: mükellef EFT'yi nakit girerse ne olur? Müşavir:

> "Mutlaka. O iş içinden çıkılmaz bir hal alır. Veri sağlıklı gelmezse adam bir daha oradan aktarmaz. Çünkü nakit işlenirse 100'e, banka işlenirse 102'ye, çek işlenirse 101'e işleniyor. … Manuel düzelteceğine manuel işler adam." (G§13)

Easytrade önerdi: banka hareketleriyle eşleştirip "burada bir hata var" demek. Müşavir: "Aynen öyle. … O hatayı da ortaya çıkarırsanız süper olur." · "'Bu bankadan gelmiş' diye bir uyarı da verirseniz o da artı bir güzellik olur." (G§13)

| | |
|---|---|
| Etiket | Aktarım **[Talep]**; tutarlılık kontrolü **[Onay]** |
| Bu paket | Tahsilat / tediye verisi **Var** (`financial_transaction`, `01` §4.9; Easytrade `09` §5, Paraşüt, KolayBi). Ödeme türü kontrolü **Var**: `BNK_PAYMENT_METHOD_MISMATCH` (`21` §6.3). Luca'ya aktarım **Luca çıkış dokümanında.** |

### T12. Dijital vergi dairesi ve SGK

> "Şimdi dijital vergi dairesinde bazı hizmetler var. Borcu yoktur yazısı, mükellefiyet yazısı. SGK sisteminde SGK borcu yoktur yazısı. … Ama Easytrade üzerine dijital vergi dairesi ve SGK entegrasyonu kurulsa… Mesela bir şey lazım oldu, girdi, 'borcu yoktur yazısı' istedi. Dijital vergi dairesinde birçok veri var, o verileri mükellef de görsün." (G§15)

Etiket **[Talep]** · Bu paket **Yok.**

### T13. E-tebligat

> "E-tebligatı kaçırmak çok ciddi sorunlar demek. … Mükellefe 'tebligat senin sorumluluğunda' diyoruz. Ama benim programım e-tebligatı geceleri sorguluyor. Gündüz bana rapor atıyor." (G§16)

> "Siz bunu dijitale bağladığınız zaman, Easytrade'in ön muhasebe programında tebligat geldiğinde ekrana düşsün. Bu da çok yük alır. Ciddi yük alır." (G§16)

Etiket **[Talep]** · Bu paket **Yok.** Sorumluluk sınırı: §4 ilke 7.

### T14. Müşavire özel mobil uygulama

> "Uyumsoft mali müşavirlere has bir cep telefonu uygulaması yazmış. … uygulamada menülerden çok basit şekilde mizan çekme, fatura listesi, bordro, ücret pusulası var. Mükellefin sizi arayıp isteyebileceği her şeyi mobilden hızlıca halledebiliyorsunuz." (G§17)

> "Ben sadece onu satsalardı alacaktım, Luca ile entegrasyonu olsaydı." (G§17)

Etiket **[Talep]** + **[Referans]** · Bu paket **Yok** (ürün tarafı). Not: bordro ve ücret pusulası verisi bu pakette toplanmıyor; personel ödemeleri sadece kaynak veriyorsa (`01` §4.13).

### T15. AI destekli finansal yorum ve sunum

> "'Firmanın mali verilerini, beyannamesini, mizanını programa at, yapay zekâ bunu sana yorumlasın' diyor. … Ben size fikir veriyorum, siz bunu yapın." (G§18)

> "Atıyorum, adamın 5 yıllık verisini yüklediniz. Grafiklerle satış eğrisi, kârlılığı vesaire, çok güzel bir sunumla." (G§18)

> "Adamın 100 tane mükellefi var. Mükellefi arayıp 'bak kardeşim, senin kasanda bu var, şurada bu var' diyemiyor." · Mükellef istiyor mu: "İstiyor." (G§18)

Easytrade sordu: otomatik e-postayla, ofise özel gönderim? Müşavir: "Evet. Standart olarak." · "Evet, ofise özel gitsin." (G§18)

Etiket fikir **[Talep]**, otomatik ve ofise özel gönderim **[Onay]** · Bu paket **Yok** (ürün tarafı; veri tabanı bu paketin topladığı veri).

### T16. Mesleğe özel küçük programlar, dilekçe ve evrak

> "mali müşavirlere has programlar yazılabilir mi, işte mükellef takibi vesaire, onun üzerinde de düşünmeniz lazım." (G§18)

> "Mesela dilekçematik olabilir. Birçok firmanın vergi dairesi, ticaret odası yazışmaları vesaire. Mesela tür değişikliği evrakları hazırlanacak." · "Onlar tabii ileriki boyutlar. Temel boyutlar bunlar." (G§19)

Etiket **[Talep]**, müşavir "ileriki boyutlar" dedi · Bu paket **Yok.**

### T17. Luca'yı bırakmak mı, Luca'ya entegre kalmak mı

Easytrade sordu: Luca'ya göndermek mi, Luca'yı bırakıp doğrudan buradan mı?

**Kadir Bey:**

> "Ben tabii ki ön muhasebe programından direkt göndermek isterim. Ama Luca'da şöyle bir avantaj var: Luca bozulduğunda maliye uzatma verebiliyor." (G§10)

> "Luca'nın şu anda çok gelişmiş olduğunu düşünmüyorum zaten. … Ama bize sağladığı avantaj bu, çok fazla kullanım olduğu için." (G§10)

> "Luca aylar öncesinde il dışındakileri yeni sisteme, e-beyan sistemine direkt otomatik gönderebiliyordu. Bu şekilde kendini güncel tutabiliyor." (G§10)

**Kıdemli ortak:**

> "Luca'ya geçme sebebimiz neydi? Güvenli olması ve verilerin bulutta tutulması." (G§12)

> "Luca biraz da özgüvenle, 'nasıl olsa bana muhtaçlar' diye geride kaldı. Birçok şeyde geride kaldı. Destek zor alıyorsun. Buna rağmen iyi mi? İnsan alıştığı için iyi." (G§12)

> "Dolayısıyla sizin oraya çok güçlü girmeniz gerekiyor. … Üstesinden gelemezseniz sıkıntı da büyük olur." (G§12)

**Easytrade'in görüşmede söylediği** **[Easytrade]**: "Her ne olursa olsun, çıkışımız Luca ile entegre olacak." (G§12)

| | |
|---|---|
| Sonuç | Portal Luca'nın yerine geçmez, **Luca'yı besler.** Beyanname, e-defter, Defter-Beyan Luca'da kalır. |
| Bu paket | Veri toplama tarafı bu kararla uyumlu. Luca çıkışı ayrı doküman. |
| Luca maliyeti (müşavirin söylediği) | "yıllık 12 bin liraymış" (G§11). Öncesinde "Üzerine yeni 10 tane mükellef gelince de 1.500–2.000 lira gibi bir rakam alıyordu" dedi; ek ücretin yapısı net değil. |

## 4. Müşavirin uyarıları ve bu paketteki karşılıkları

| # | İlke | Müşavirin sözü | Pakette nasıl karşılandı |
|---|---|---|---|
| 1 | **Veri yanlışsa aktarım kullanılmaz** | "Veri sağlıklı gelmezse adam bir daha oradan aktarmaz. … Manuel düzelteceğine manuel işler adam." (G§13) | Fatura içeriği UBL'den (`31`); bütün tutarsızlıklar kodlu uyarı (`05`); her adaptörde kaynakla birebir sayı / tutar kabul testi (`02` §11, `30` §9) |
| 2 | **Güçlü girin, yarım iş zarar** | "sizin oraya çok güçlü girmeniz gerekiyor. … Üstesinden gelemezseniz sıkıntı da büyük olur." (G§12) | Bilgisi olmayan entegrasyon yazılmaz, "yok" denir (`00` §5, `CLAUDE.md` §5) |
| 3 | **Mevzuat değişikliği sisteme kendiliğinden yansımalı** | "Mesela bir bilgi geliyor, o bilginin programda güncellenmesi lazım. Bunu otomatik bir sistemle güncellemeniz lazım." (G§12) | Vergi, istisna, tevkifat kod listeleri kodda sabit değil, tablodan (`31` §5.3, `01` §1.1) |
| 4 | **Yoruma açık konuda belirleyici olan** | "Siz 'böyle' diyorsunuz, Luca 'ben bunu böyle yorumladım' diyor. Devletin lehine yorumluyor. Ve onun dediği kabul ediliyor." (G§12) | Gider kabulü, iade yorumu gibi kararlar müşaviredir (`expense.is_deductible`, `RETURNED` uyarısı); sistem karar vermez |
| 5 | **Mükellef kullanmazsa çalışmaz** | "Adam 'ben niye bunları tek tek okutayım' diyor." (G§4) | Mükellefe iş yükü bindiren adımlar en aza: entegratörden otomatik çekim, ekstre hatırlatması (`23` §2) |
| 6 | **Tavsiye ettiğin ürünün arkasında dur** | "Luca dedi ki … 'Vera'yı Luca ile entegre edeceğiz.' Tavsiye etti, biz de herkese tavsiye ettik. Entegre etmedi. … Çok mahcup olduk." · "Değiştiren de pişman olmamalı." (G§3) | Desteklenmeyen kaynak "desteklenir" diye gösterilmez; durum tabloları açık (`00`) |
| 7 | **Sorumluluk sınırı** | "Tabii biz bunu mükellef adına üstlenemiyoruz, yükümlülük altına girmiyoruz." (G§16) | Bu pakette ilgili özellik yok; e-tebligat yapılırsa bildirim mükellefe gider |
| 8 | **Geçiş ofise yük bindirmemeli** | "Her dönem yoğun dönem. 'Gel programı değiştir' deseniz, adam 'çok yoğunum' der, değiştirmez." (G§12) | Portal mevcut düzeni (Luca) değiştirmez, besler (T17) |

## 5. Karşılama özeti

Vurgu sütunu müşavirin kendi ifadelerinden alındı; Easytrade'in iç önceliği değildir.

| # | Talep | Müşavirin vurgusu | Bu paket | Kalan iş hangi dokümanda |
|---|---|---|---|---|
| T1 | Hesap planı, kural ezberi, otomatik cari | Temel beklenti (G§1) | Kısmen | Luca çıkışı |
| T2 | Kayıt yanında fatura görüntüsü | "bir sürü iş var" (G§2) | Kısmen | Müşavir paneli, Luca çıkışı |
| T3 | Akıllı uyarılar | Bugün kullandığı programda var (G§2) | Kısmen | Ürün: eşikler, kelime listesi |
| T4 | Mükerrer fatura | (G§14) | Var | Luca çıkışı: "hangi fişte" |
| T5 | Z raporu | "çok büyük yük kaldırıyor" (G§3) | Yok | Ayrı doküman |
| T6 | Masraf fişi | "en önemli sorunu" (G§4) | Kısmen (Easytrade kullanana) | Mobil giriş / OCR dokümanı |
| T7 | Yakıt fişi çekme | (G§5) | Yok | Luca'nın yöntemi incelenince |
| T8 | Plaka kontrolü | "büyük bir yükten kurtaracak" (G§5) | Kısmen | Ürün: araç listesi |
| T9 | Banka: 1 ve 2 | (G§6) | Var / Kısmen | Luca çıkışı (Excel, 102) |
| T9 | Banka: 3, onay linki | "ideali" (G§6) | Yok | Karar (§6) |
| T10 | KDV erken uyarısı | (G§7) | Kısmen | Ürün: hesap, kanal, sıklık |
| T11 | Tahsilat aktarımı + ödeme türü kontrolü | "iş içinden çıkılmaz bir hal alır" (G§13) | Var | Luca çıkışı |
| T12 | Dijital VD + SGK | (G§15) | Yok | Ürün |
| T13 | E-tebligat | "çok ciddi sorunlar" (G§16) | Yok | Ürün |
| T14 | Müşavir mobil uygulaması | "Ben sadece onu satsalardı alacaktım" (G§17) | Yok | Ürün |
| T15 | AI finansal yorum, ofise özel otomatik gönderim | (G§18) | Yok | Ürün |
| T16 | Mesleğe özel programlar, dilekçe | "ileriki boyutlar" (G§19) | Yok | İleri faz |

**Okuma:** Bu paket müşavirin en temel derdinin **veri tarafını** çözüyor: faturalar (T2, T4), tahsilat (T11) ve banka (T9 seviye 1–2) evraksız ve elle işlemeden toplanıyor. Müşavirin asıl gördüğü değer **Luca'ya otomatik ve doğru aktarım** (T1, T9, T11) ve bunun dokümanı henüz yok. En yüksek vurgulu iki talep (Z raporu T5, masraf fişi T6) bu paketin dışında veya kısmen içinde.

## 6. Karar bekleyenler

| # | Karar | Neden şimdi |
|---|---|---|
| 1 | **Banka onay linki (T9 seviye 3).** Müşavirin "ideal" dediği akış, "aracı firma ve açık bankacılık kullanılmaz" kararıyla (`20` §2) dışarıda kalıyor. Müşavirin göndereceği örnek link incelenip karar verilmeli. | Müşavir şube talimatının (bizim hesap bağlama yolumuz) mükellefe zor geldiğini açıkça söyledi |
| 2 | **Luca çıkışının yöntemi.** Easytrade'in mevcut Luca entegrasyonu mu kullanılacak, Luca'nın Excel içe aktarımı mı, ikisi mi? Hesap planı ve kural ezberi (T1) bunun üzerine kurulur. | Müşavirin gördüğü asıl değer burada |
| 3 | **Z raporu (T5)** ve **mobil masraf fişi (T6)** ne zaman, hangi kapsamla | Müşavirin en yüksek vurguyla söylediği iki talep |
| 4 | **Akıllı uyarı eşikleri (T3), plaka listesi kaynağı (T8), KDV uyarısı kuralları (T10)** | Ürün kararı; kod tarafı alanları hazır |
| 5 | **TÜRMOB tercihli entegratör kullananlarda çift kayıt** (`30` §1) | Luca'ya otomatik aktarım zaten varsa portalın aktarımı çift kayıt üretir |

## 7. Görüşmeden kalan açık işler

| # | İş | Kimde | Kaynak |
|---|---|---|---|
| 1 | Banka onay linki örneği | Müşavir ofisi | G§6: "Ona bakarız." |
| 2 | e-Mükellef ekran görüntüleri, belki video | Müşavir ofisi | G§16, G§20: "Olur, atarız." |
| 3 | Luca'nın yakıt fişi çekme yöntemini incelemek | Easytrade | G§5 |
| 4 | Proje taslağı ve ekranlar; yaklaşık 15 gün sonra demo randevusu | Easytrade | G§20 |
| 5 | Ara dönemde müşavire soru sorulabilir | — | G§20: "Olur." |
