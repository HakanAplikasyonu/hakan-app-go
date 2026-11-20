# Uçtan uca testler (#170, #181)

Playwright ile ofis paneli, mükellef paneli ve çıktı akışları.

| Dosya | Ne sınar | Veri |
|---|---|---|
| `tests/smoke.spec.ts` | Doğrulama raporundaki duman akışı (`docs/dogrulama-raporu.md` §2.2): ofis girişi, realtime ve KVKK uçları, mükellef ekleme ve onay, UBL yükle, öneri ve onay, banka eşleme ve uyarılar, dönem kilidi, yevmiye, Luca / Zirve indirme, mükellef paneli, tenant izolasyonu | Demo ofisi, Atlas Gıda |
| `tests/office-onboarding.spec.ts` | Ofis ana akışı baştan sona | Her koşuda yeni ofis, sahip ve VKN |
| `tests/setup-wizard.spec.ts` | Pilot ofis kurulumu (#273): `scripts/new-office.sh` ofisi açar ve sahibi davet eder; sihirbazda ofis bilgileri, ekip daveti, 200 satırlık mükellef listesi (3 hatalı satır raporlanır), yarıda bırakıp devam, Luca, entegratör adımı atlanır, 197 mükellefe toplu onay e-postası; 10 dakika sınırı | Her koşuda yeni ofis, sahip ve VKN'ler |
| `tests/taxpayer-invite.spec.ts` | Mükellef paneli daveti: davet e-postası → kayıt → kabul; yalnızca kendi şirketini görür | Demo ofisi, Selin Yılmaz'a yeni çalışan |
| `tests/a11y.spec.ts` | Erişilebilirlik (#194): misafir, ofis ve mükellef ana sayfalarında axe-core (WCAG 2.1 A/AA); kritik ve ciddi ihlal testi kırar. Komut paleti klavyeyle (Ctrl+K / Esc) | Yalnızca okur |
| `tests/access.spec.ts` | Rol bazlı erişim reddi: giriş yapmamış kullanıcı, stajyer (roller, davet, ayar, platform, atanmadığı mükellef; arayüz ve API), mükellef kullanıcısının ofis paneli | Yalnızca okur |
| `tests/mukellef-sekmeleri.spec.ts` | Mükellef detayında sekme yapısı (#468): Faturalar sekmesi → fatura detayı → geri (süzgeç korunur), her sekmede başlık, sekme çubuğu ve dönem şeridi, ok tuşlarıyla sekme gezintisi ve axe | Her koşuda yeni mükellef ve üç türetilmiş fatura; sekme gezintisi Atlas Gıda'da (yalnızca okur) |
| `tests/kayit-baglantilari.spec.ts` | Kayıtlar arası bağlantılar (#469): fatura → tahsilat hareketi → fiş → uyarı → kural zinciri mükellef detayında (`?kayit=`), geri tuşuyla dönüş; banka, fiş ve uyarı satırlarından ilişkili kayıtlar (süzgeç korunur); ofis geneli uyarılar ve komuta merkezinden mükellef sekmesine; MUHSGK kontrol listesinden ilgili sekmeye; boş durum ve axe | Her koşuda yeni mükellef; türetilmiş fatura, Ziraat ekstresi, eşleme kabulü, muhasebeleştirme ve fiş taslağı API ile |
| `tests/mutabakat.spec.ts` | Mutabakat panosu (#389): hesap farkları (filtre, ayrıntı, açıkla / yeniden aç), BA/BS beyan satırı ekle / sil, cari mektubu oluştur ve gönder, karşı firmanın yanıt sayfası (itiraz), geçersiz bağlantı | Demo ofisi, Atlas Gıda; benzersiz alıcı ve VKN |
| `tests/beyanname.spec.ts` | Beyannameler (#390): KDV1 düzeltme, kontrol listesi, hazırla, Excel, verildi kaydı; MUHSGK hazırla / Excel / taslağa geri al; Ba / Bs formları | Demo ofisi, Atlas Gıda (beyan dönemi) |
| `tests/bordro-banka.spec.ts` | Bordro (#382, #395): dönem bordrosu, personel ve SGK listeleri, Excel'e aktar; CSV bordro önizle / içe aktar. Banka (#383): IBAN'sız dosyada Türkçe hata, MT940 yükle, hareket filtresi, Excel'e aktar | Atlas Gıda (bordro, 2025-06 dönemi); Selin Yılmaz (her koşuda yeni IBAN'lı hesap) |
| `tests/masraf-z-rapor.spec.ts` | Masraf ve Z raporu (#384): onay ve kategori filtresi, kategoriler, süreklilik denetimi, Excel'e aktar → aynı dosyayı geri yükle (satır eklenmez, #484). Easytrade (#385): bağlantı, ödeme türü özeti, tablo araması. Rapor (#386): aylık / yıllık, PDF, e-postayla gönder, toplu gönderim | Atlas Gıda (masraf, Z, Easytrade; yalnızca okur); Selin Yılmaz (rapor, benzersiz alıcı) |
| `tests/copilot-izgara.spec.ts` | Co-pilot (#392): Sıradaki iş, ⌘K ile aç / kapat, paletten ekrana git, komuttan plan → onayla ve çalıştır. Düzenlenebilir ızgara (#394): ok tuşlarıyla gezinti, yazarak düzenle, geri al / yinele (düğme, Ctrl+Z / Ctrl+Y), Esc ile vazgeç, Ctrl+S ile kaydet | Selin Yılmaz: aylık rapor (yalnızca bu dosya gönderir), her koşuda yeni ETTN'li fatura ve hesap kodu önerisi |
| `tests/whatsapp-aktarim.spec.ts` | WhatsApp uzantısı (#388): eşleştirme kodu, kayıt türü filtresi. Programa aktarım (#391): yevmiyede aktarım geçmişi ve klasör hatası ekranı | Demo ofisi, Atlas Gıda; yalnızca okur |
| `tests/bulut-donem-aktarim.spec.ts` | Dönem aktarımı ve mizan durumu (#566, #574): iki onaylı fiş seçim yapmadan dönemin tamamı olarak Luca'ya aktarılır, masaüstü ajanına gönderilir (ajanın alma / klasöre bırakma adımları API ile), "Aktarıldı" işaretlenir; aktarım satırında "Mizanla karşılaştırılmadı" → "Mizanla karşılaştır" Mutabakat'a götürür; Luca mizanı yüklenip karşılaştırılınca "Luca mizanı kayıtlarla tutuyor", farklı mizanda "1 hesapta fark var" → "Farkları gör". Her adım API'den doğrulanır | Her koşuda yeni mükellef; fişler, mizan (CSV) ve benzersiz adlı masaüstü ajanı API ile (ajan sonunda kaldırılır) |
| `tests/k2-akislar.spec.ts` | EPIC #560 akışları: Mükellefler satırı → detay ve Komuta Merkezi hücresi → hedef ekran (#567); mizan hafızası kimin olduğunu yazar (#572); inceleme kuyruğunda banka hesabı önerisinde "Kodu düzelt" (#569); dönem uyarısı İncele panelinden çözülür (#570); eşleşmeyi geri al → hesaba işle → yeniden eşleştir (#565). Her adımın sonucu API'den doğrulanır | Her koşuda yeni mükellef; türetilmiş fatura ve Ziraat ekstresi API ile |
| `tests/bulut-ofis-aktarimlar.spec.ts` | Ofis geneli programa aktarımlar (#574): bekleyen aktarımlar ucu yeni mükellefin onaylı fişini dönemiyle listeler; `/transfers` sayfasında satır unvan ve dönem metniyle görünür, tıklanınca mükellefin Fişler ekranı dönem seçili açılır | Her koşuda yeni mükellef ve API ile bir onaylı fiş (Ekim 2026); çıktı alınmaz |
| `tests/k2-liste-detay.spec.ts` | Liste-detay deseni (#564): banka listesinde alttaki kayıt yan panelde açılır, satır yerinden oynamaz; panelden önceki / sonraki kayda geçilir (düğme ve j / k); Esc ile kapanınca konum aynı | Atlas Gıda banka hareketleri; yalnızca okur |
| `tests/yerel-fis-geri-al.spec.ts` | Onaylı, henüz aktarılmamış fişin onayı gerekçeyle geri alınır (#566): gerekçesiz istek 400, fiş taslağa döner ve nedeni listede görünür, taslakta geri alma 409, yeniden onaylanır | Demo ofisi, Selin Yılmaz; yalnız kendi açtığı benzersiz açıklamalı fiş, aktarmaz |
| `tests/yerel-inceleme-mukellef.spec.ts` | İnceleme kuyruğu mükellef bazında (#710): `GET /review/taxpayers` toplamı kuyruğun toplamı, mükelleflerin toplamı ona eşit, her mükellefin sayısı kendi kuyruğunun toplamı, çok olan önce, geçersiz durum 400; ekranda "Mükellefe göre" seçimi: "Tümü" sayısı sayaçla aynı, mükellef seçilince kuyruk ve sayaç onun, "Tümü" geri açar | Demo ofisinin bekleyen önerileri; karar verilmez |
| `tests/yerel-donemi-ac.spec.ts` | Kapalı dönemdeki taslak fiş (#669 A): satırda "Dönem kapalı…" ve pasif Onayla; sunucu kapalı dönemde onayı 422, gerekçesiz açmayı 400 ile reddeder; "Dönemi aç" + gerekçe → "Kontrol tamam" → onayla; gerekçe kilit geçmişinde | Her koşuda yeni mükellef; taslak fiş ve dönem kilidi API ile |
| `tests/yerel-bordro-sira.spec.ts` | Bordro ekranında sıra (#710): bordrosu olan mükellefte seçili dönemin özeti ve dönem listesi önce, "Bordro yükle" formu en altta | Demo ofisi, Atlas Gıda; yalnız okur |
| `tests/yerel-bordro-personel.spec.ts` | Bordro personeli elle girilir (#669 J): Personel görünümünde "Personel ekle" ile yeni kişi kaydedilir, listede ve API'de görünür; kalem düğmesiyle görevi değiştirilir, diğer alanlar yerinde kalır | Her koşuda yeni mükellef; personel arayüzden girilir |
| `tests/yerel-kuyruk-sayfalama.spec.ts` | İnceleme kuyruğu sayıları ve sayfalama (#669 B): 100'den fazla bekleyen öneri sayfalarla ("1–100 / N", Sonraki/Önceki 100); başlık, durum dökümü ve API aynı sayı; onaydan sonra "Bekleyen N−1 · Onaylanan 1"; süzgeçliyken ofis toplamı ayrıca | Her koşuda yeni mükellef; 104 hareketli MT940 ekstresi API ile |
| `tests/yerel-talep-eslesme.spec.ts` | İstenen belge gelince talebe bağlanır (#571): ofis belge ister → mükellef belgeyi o talebe yükler (mükellef API) → talep "geldi", dosya talebe bağlı → ofis mükellef özetinde dosyayı indirir ve "Belge tamam" ile isteği kapatır | Her koşuda yeni mükellef ve kullanıcısı; talep ve yükleme API ile |
| `tests/yerel-banka-elle-arama.spec.ts` | Banka hareketi için elle fatura araması tarih ve tutar süzgeciyle (#565): API'de terimsiz süzgeç (tarih + tutar aralığı) yalnız uyan faturayı bulur, hatalı tarih 400; uyarı panelindeki eşleştirmede adaylarda olmayan fatura tutar aralığıyla bulunur ve eşleştirilir; yeniden eşleştirme eskisini değiştirir | Her koşuda yeni mükellef; iki fatura ve Ziraat ekstresi API ile |
| `tests/yerel-banka-bordro.spec.ts` | Banka hareketini bordroya elle bağlama (#710): maaş ödemesinin panelinde hareketin ayı ve önceki iki ayın ödenmemiş bordroları (personel sayısı, açık net); seçilen bordroya bağlanır, bordro "ödendi" olur; tahsilat bordroya bağlanamaz (422); bordro bağlantısı bordro sayfasında o dönemi ve personel dökümünü açar | Her koşuda yeni mükellef; Ekim 2026 bordrosu (CSV) ve Kasım MT940 ekstresi API ile |
| `tests/yerel-banka-sgk.spec.ts` | Banka hareketini SGK primine elle bağlama (#565): prim ödemesinin panelinde hareketin ayı ve önceki üç ayın açık SGK primleri (son ödeme, açık tutar); seçilen prime bağlanır, prim "ödendi" olur ve ödeyen hareketi gösterir; bağlantı kalkınca prim yeniden açık borç olur; tahsilat prime bağlanamaz (422) | Her koşuda yeni mükellef; Ekim 2026 bordrosu (CSV, SGK tahakkuku bordrodan) ve Kasım MT940 ekstresi API ile |
| `tests/yerel-banka-bordro.spec.ts` | Banka hareketini bordroya elle bağlama (#710): maaş ödemesinin panelinde hareketin ayı ve önceki iki ayın ödenmemiş bordroları (personel sayısı, açık net); seçilen bordroya bağlanır, bordro "ödendi" olur; tahsilat bordroya bağlanamaz (422); bordro bağlantısı bordro sayfasında o dönemi ve personel dökümünü açar; personel satırı detayı (brütten nete, işveren payları, bağlı banka ödemesi) | Her koşuda yeni mükellef; Ekim 2026 bordrosu (CSV) ve Kasım MT940 ekstresi API ile |
| `tests/yerel-banka-bakiye.spec.ts` | Banka bakiyesi tutmuyor uyarısı (#710): panelde farkın yönü, neden önemli, adımlar ve "çözüldü"nün anlamı; "Banka hareketlerini aç" uyarının dönemiyle açılır ve hareketler listelenir; hareket hesaptan çıkarılınca bakiye tutar, uyarı kapanır, çıkarılan "bakılacak hareketler"de görünür; yeniden dahil edilince fark ve uyarı geri gelir; gövdesiz istek 400 | Her koşuda yeni mükellef; arada boşluk olan iki MT940 ekstresi API ile |
| `tests/yerel-banka-neden.spec.ts` | Banka eşleştirmede "neden önerildi" ve alakasız aday (#710): her adayın puan parçası karşı tarafla (VKN, IBAN, fatura no, ad) ya da tutarla ilişkili, yalnız tarih penceresindeki fatura listelenmez; otomatik/önerilen eşleşmenin nedeni API'de; hareket panelinde neden Türkçe cümle (kod metni yok); elle eşleşmede neden yok | Her koşuda yeni mükellef; iki fatura ve Ziraat ekstresi API ile |
| `tests/yerel-donem-seridi.spec.ts` | Dönem şeridi seçimi açıkça söyler (#710): tüm dönemlerdeyken hiçbir ay basılı değil ve "Sekmeler tüm dönemleri gösteriyor."; aya basınca o ay basılı ve "… için süzülü."; "Tüm dönemleri göster" seçimi kaldırır | Demo verisi (Atlas); yalnızca okur |
| `tests/yerel-konum-geri.spec.ts` | Konum çubuğunda işlem seviyesi ve geri dönüş (#710): sekme listesinde sayfa adı son kırıntı; fatura ayrıntısında ve `?kayit=` odaklı kayıtta son kırıntı "Kayıt", sayfa adı "… listesine dön" bağlantısı olur ve listeyi açar; dönemle açılan kayıtta dönüş bağlantısı dönemi taşır | Demo verisi (Atlas faturası); yalnızca okur |
| `tests/yerel-hata-yeniden-dene.spec.ts` | Yükleme hatası boşluk ya da "bulunamadı" gibi görünmez (#710): dönem matrisi yüklenemezse mükellef ayrıntısı ve Komuta merkezi "Veriler yüklenemedi" + "Yeniden dene" gösterir (mükellef ayrıntısı "Mükellef bulunamadı" demez); sunucu düzelince yeniden deneme sayfayı açar | Demo verisi; hata tarayıcıda matris isteği 500'e çevrilerek kurulur |
| `tests/yerel-uyari-siradaki.spec.ts` | İncele paneli işlem bitince sıradakine geçer (#570): panelde hareket faturayla eşleştirilir → uyarı kapanır, "Bu uyarı kapandı." görünür, panel kendiliğinden kalan uyarıya (bakiye tutmazlığı) geçer; API'de eşleştirilen uyarı kapalı, öteki açık | Her koşuda yeni mükellef; fatura, Ziraat ekstresi ve bakiyesi tutmayan ikinci ekstre API ile |
| `tests/yerel-uyari-kapatma.spec.ts` | Uyarı İncele panelinden gerekçeyle kapatılır (#570): gerekçesiz "Çözüldü olarak kapat" pasif; gerekçeyle kapatınca açık listeden düşer, API'de `resolved` + not + kapatan; Çözüldü sekmesinde gerekçesiyle görünür | Her koşuda yeni mükellef; Ziraat ekstresi API ile |
| `tests/yerel-talep-otomatik-bagla.spec.ts` | Talebe değil başka yoldan (WhatsApp, görevsiz) gelen belge (#571): açık istek yokken bağlanmaz; tek açık istek varken kendiliğinden bağlanır (istek "geldi"); iki açık istek varken bağlanmaz, ofis WhatsApp kayıtlarında "Talebe bağla" ile seçer; bağlı belge ikinci kez bağlanamaz (409) | Her koşuda yeni mükellef; istekler ve belgeler API ile |
| `tests/yerel-talep-yeniden-iste.spec.ts` | Gelen belge yetersizse ofis ekrandan yeniden ister (#571): "Yeniden iste" + not → istek yeniden açık ("Bekliyor"), API'de not ve istek sayısı 2 → mükellef notu görür, yenisini yükler → satır yeniden "Geldi, kontrol et", iki dosya | Her koşuda yeni mükellef ve kullanıcısı; talep ve yüklemeler API ile |
| `tests/yerel-iliskili-olasi.spec.ts` | İlişkili kayıtlarda yalnız gerçek bağlar (#669): banka masrafı hareketinin onaylanmamış karşı hesap önerisi (780) API'de `possible`, ekranda "Olası kayıtlar (tahmin)" başlığı altında, kesin bağlar arasında değil; başka hareketin faturası listede yok | Her koşuda yeni mükellef; fatura, Ziraat ekstresi ve öneriler API ile |
| `tests/yerel-fis-kontrol-akisi.spec.ts` | #566 kabul akışı: Sıradaki İşler → "Taslak fişleri kontrol et" → kontrol ekranı (doğrudan onay yok) → planda olmayan kodlu satırda "Hesabı düzelt" → "Kontrol tamam" → onayla; sonuç API'den doğrulanır | Her koşuda yeni mükellef; hesap planı ve tek taslak fiş API ile |
| `tests/yerel-karsi-taraf-gecmisi.spec.ts` | Kuyrukta karşı taraf geçmişi sunucudan (#669 L): satır hesabı onaylanınca `GET …/suggestions/history` o kodu 1 kez sayar (counterparty yoksa 400); kuyrukta aynı karşı tarafın öbür satırında "Bu karşı tarafla geçmiş" kodu gösterir | Her koşuda yeni mükellef; iki kalemli Atlas satış faturası türetilir |
| `tests/yerel-fatura-hesap-kodu.spec.ts` | Fatura detayında hesap kodu (#669 F): "Hesap kodu öner" kuyruğa göndermez, öneriler aynı sayfada "Hesap kodları" bölümünde; satırın kodu "Değiştir" → "Kaydet" ile düzeltilir (API'de `corrected`); öneri listesinin `invoice` süzgeci yalnız o faturanın önerilerini döner, geçersiz uuid 400 | Her koşuda yeni mükellef; iki Atlas faturası türetilir |
| `tests/yerel-fis-kabul-akisi.spec.ts` | #566 kabul senaryosu uçtan uca: Sıradaki İşler → "Taslak fişleri kontrol et" → kontrol ekranında kapalı dönem engeli → "Dönemi aç" (gerekçe) → onayla → Sıradaki İşler'e dön → "Aktarımı kontrol et" → aktarım bölümü → seçili fiş Luca'ya aktarılır; düğmeler doğrudan işlem yapmaz, fiş API'de `exported` | Her koşuda yeni mükellef; tek taslak fiş ve dönem kilidi API ile |
| `tests/yerel-uyarili-fis-onayi.spec.ts` | Uyarılı fiş onayı (#669, ofis kararı): eşleşmemiş banka hareketinden fiş "Eksik belge", aynı faturayı kaynak gösteren iki fiş "Mükerrer olabilir"; API bayraksız onaya 409 `JOURNAL_NEEDS_CONFIRMATION`; tekli onayda "Yine de onaylansın mı?" penceresi, gerekçe isteğe bağlı ve fişte kalır (`approval_warnings`, `approval_note`); toplu onay uyarılı fiş sayısını söyler | Her koşuda yeni mükellef; üç taslak fiş API ile |
| `tests/yerel-beyan-son-kontrol.spec.ts` | Beyan son kontrolü (#669, ofis kararı): personel (member) beyanı hazırlar; `check` ve `file` 403; ekranda "Kontrol edildi olarak işaretle" ve "Verildi olarak işaretle" pasif, nedeni yazılı, "Hazırlayan" görünür; sahip kontrol edildi ve verildi işaretler, "Kontrol eden" ve "Veren" görünür | Her koşuda yeni mükellef ve personel rolüyle davet edilen yeni kullanıcı |
| `tests/yerel-ekstre-ai-ayari.spec.ts` | Ekstrede AI kolon önerisi ofis ayarıyla kapatılır (#669): Ayarlar → Yapay zekâ anahtarı kapatılınca `settings.bank_statement_ai=false`; tanınmayan düzendeki CSV analizi kural tabanlı çalışır, AI kaynaklı kolon yok; sonda ayar açığa döner | Demo ofis ayarı; test sonunda eski haline döner |
| `tests/yerel-ekstre-onizleme.spec.ts` | Ekstre önizleme (#669): bilinmeyen düzendeki CSV Excel benzeri tabloda açılır; kolon anlamları seçilir, devreden satırı dışarıda bırakılır, okunamayan tutar yerinde düzeltilir, deneme okumasıyla sayılar tazelenir, içe aktarılır; yükleme sonrası hesap önerisi kendiliğinden çalışır, yüksek güvenliler bildirimden toplu onaylanır ve taslak fiş oluşturulur (API: işletim ücreti 780, `drafted`); aynı dosya yeniden yüklenince eşleme kendiliğinden gelir ve "daha önce yüklenmiş" denir | Selin Yılmaz'a her koşuda sahte IBAN'lı yeni hesap; koşuya özgü kolon başlığı |
| `tests/yerel-vkn-tamamla.spec.ts` | Geçersiz/eksik VKN ofiste tamamlanır (INVALID_TAX_ID): belge yan panelinde "VKN'yi gir"; geçersiz numara 400 `INVALID_TAX_ID`, geçerli numara cariyi bağlar, uyarı `resolved`, kim/ne zaman satırı görünür; ikinci giriş 422 `TAX_ID_SET` | Her koşuda yeni mükellef; atlas-01'den türetilen, alıcı TCKN'si geçersiz satış faturası |
| `tests/yerel-siradaki-is-donem.spec.ts` | "Sıradaki iş" çubuğu sayılan ayı söyler (#710): banka listesi "Tümü"yken çubukta "Tüm dönemler seçili; burada yalnız {ay} sayılır" notu; dönem seçilince (?period=2026-10) not kalkar, çubuk Ekim 2026'yı sayar | Demo ofisi, Atlas Gıda; yalnız okur |
| `tests/yerel-aktarim-mizan-fark.spec.ts` | Programa aktarımlar (#669 UX): "Son aktarımlar"da "N hesapta fark" bağlantısı mükellefin Mutabakat ekranını açar; "Uyumlu" bağlantı değil | Demo ofisinin aktarım geçmişi; yalnız okur |
| `tests/bulut-uyari-tutarlilik.spec.ts` | Komuta Merkezi ↔ Uyarılar (#669 C): eşleme, ekstre bakiye kontrolü tutmayınca `BNK_BALANCE_MISMATCH` engelleyici dönem uyarısını açar (canlı kontrolle aynı fark, tekrar çalışınca tek uyarı); matristeki açık uyarı/engel sayısı uyarı listesinin aynı dönemdeki toplamıyla aynı; Ekim hücresinden gidilen Uyarılar sekmesinde uyarı görünür | Her koşuda yeni mükellef; aynı hesaba arada 500,00 eksik hareket bırakan iki MT940 ekstresi API ile |
| `tests/bulut-belge-isteme.spec.ts` | Belge isteme (#568): aynı başlıkta açık talep varken yenisi açılmaz, var olan 200 ile döner; iptalden sonra yeniden istenebilir (#606). Okunamayan fiş fotoğrafları Masraflar ekranında görünür, "Mükelleften yeniden iste" tek talep açar (ikinci fotoğraf ve sayfa yenileme sonrası tekrar basış yeni talep açmaz), "Kapat" yüklemeyi kapatır (#630). Sonuçlar API'den doğrulanır | Her koşuda yeni mükellef; talepler ve fotoğraflar API ile (AI kapalı ortamda her fotoğraf okunamayan olur; AI açıksa fiş senaryosu atlanır) |
| `tests/bulut-sayfa-durumu.spec.ts` | Sayfa durumu adreste (#669 D, #564): İnceleme kuyruğunda mükellef, öneri türü ve güven kademesi (`?taxpayer=`, `?kind=`, `?tier=`), Fişler'de mükellef ve dönem (`?taxpayer=`, `?period=`) sorgu dizesinde; başka sayfaya gidip geri tuşuyla dönünce aynı süzgeçler ve adres açılır, aynı bağlantı yeni oturumda aynı yeri açar; varsayılan ("Tümü") adrese yazılmaz | Her koşuda yeni mükellef ve API ile bir onaylı fiş (Ekim 2026); çıktı alınmaz |

`office-onboarding.spec.ts` ofis ana akışının adımları:

1. Ofis kaydı: platform yöneticisi ofisi açar, sahibi e-postayla davet eder, sahip davetle kayıt olur.
2. Mükellef ekleme: onay e-postası, mükellefin kaydı ve onayı.
3. UBL yükle: demo setinden yeni VKN ve ETTN ile türetilen faturalar.
4. Hesap önerisi ve kuyruktan onay.
5. Yevmiye: taslak, onay, Luca ve Zirve indirme.
6. Mükellefin kendi paneli.

**Paralellik ve kararlılık:**
- Paralellik dosya düzeyindedir (`workers` varsayılan 3, `E2E_WORKERS` ile değişir).
- Dosya içindeki adımlar sıralıdır.
- Dosyalar birbirinin verisine dokunmaz: e-postalar ve VKN'ler her koşuda benzersiz.
- Yeniden deneme (`retries`) yoktur: kararsız test gizlenmez, düşer.
- Başarısız testte iz (trace), ekran görüntüsü ve video `test-results/` altına yazılır.

Ortam her koşudan önce sıfırdan kurulmalıdır: duman akışı demo verisinin taze olmasına dayanır.

M15 ekranlarının dosyaları (#452) paylaşılan demo verisini bölüşür: Atlas'ın banka verisi ve Ekim fişleri duman testinin, Selin'in aylık raporu co-pilot testinin, Selin'in banka hareketleri bordro-banka testinindir. Dosyanın başındaki not hangi kayda dokunduğunu yazar; yeni bir senaryo başka dosyanın kaydını değiştirmemelidir. Gece e2e (#397, `--grep-invert "@docs|@visual"`) bu dosyaları kendiliğinden koşar.

## Yerelde

Gerekenler: Docker, Go, pnpm, Node 22, `psql`.

```bash
make e2e-up
```

```bash
make e2e-test
```

```bash
make e2e-down
```

- `make e2e-up` (`scripts/e2e-up.sh`): `compose.local.yml` altyapısını temiz birimlerle kurar. Ardından migration'ları sıfırdan uygular, `demo-seed -reset` çalıştırır, bir platform yöneticisi oluşturur (`E2E_SA_EMAIL` / `E2E_SA_PASSWORD`; varsayılan `sa@e2e.local`), Go API'yi ve `next build && next start`'ı başlatır. Env dosyası, pid'ler ve loglar `e2e/.run/` altına yazılır.
- İlk kurulumda `cd e2e && npm ci && npx playwright install chromium` gerekir.
- 5432 portu doluysa `E2E_PG_PORT=55432 make e2e-up` kullanın. Frontend derlemesini atlamak için `E2E_SKIP_BUILD=1`, ek compose dosyası (ör. yerel override) için `E2E_COMPOSE_FILES=a.yml:b.yml`.

Adresler: `E2E_BASE_URL` (frontend, `http://localhost:3000`), `E2E_API_URL` (`http://127.0.0.1:8080`), `E2E_MAIL_URL` (Mailhog, `http://127.0.0.1:8025`).

## Arayüzü olmayan adımlar

Banka hesabı ile ekstre yükleme (#144) ve dönem kilidi için ofis panelinde ekran yok. Bu adımlar API'den yapılır; sonuçları arayüzde (uyarılar, komuta merkezi) ve API'de denetlenir.

## CI

`.github/workflows/ci.yml` içindeki `e2e` işi şu durumlarda koşar: backend ya da frontend değiştiğinde, `e2e/**`, `scripts/e2e-*`, `compose.local.yml` ya da `.env.example` değiştiğinde ve main'de.

Zorunluluk (#194):
- Ortam kurulumu ve **erişilebilirlik adımı** (`--grep @a11y`) `ci` kapısını kırar.
- **Görsel regresyon** (`--grep @visual`, #332) erişilebilirlikten hemen sonra, temiz demo verisinde koşar. Temel görüntüler CI'da doğrulanana kadar zorunlu değildir: fark varsa iş uyarı yazar ve geçer.
- Akış testleri (`--grep-invert "@a11y|@docs|@visual"`) henüz zorunlu değildir: düşerse iş uyarı yazar ve geçer.

## Görsel regresyon (#332)

`tests/gorsel.spec.ts`, ana ekranları dört temada ve üç kırılımda çeker:
- temalar: `m3-light`, `m3-dark`, `n3-light`, `n3-dark`;
- kırılımlar: masaüstü 1440, tablet 1024, telefon 390.

Görüntüler `tests/gorsel.spec.ts-snapshots/` altındaki temel görüntülerle karşılaştırılır. Piksel farkının en fazla %1 olmasına izin verilir.

- **Yeni ekran eklemek:** `snap(page, yol, ad)` çağrısı ekleyin. Ekran tarih, saat ya da sayaç gibi oynak içerik gösteriyorsa o öğeye `data-visual-mask` verin; `time` öğeleri zaten maskelenir.
- **Tasarım bilerek değiştiyse:** temiz ortamda (`make e2e-up`) temel görüntüleri yenileyin ve farkı PR'da gösterin:

  ```bash
  cd e2e && npx playwright test --grep @visual --update-snapshots
  ```

- **CI'da fark çıkarsa:** `e2e-report` artefaktındaki `test-results-visual/` klasöründe her farklı ekran için `-expected`, `-actual` ve `-diff` görüntüleri bulunur. Karşılaştırma yumuşaktır: bir fark diğer çekimleri durdurmaz. Fark yalnızca yazı tipi işlemesinden geliyorsa `-actual` görüntülerini `tests/gorsel.spec.ts-snapshots/` altına (`-chromium-linux.png` adıyla) kopyalayın.

`e2e-report` artefaktında şunlar bulunur:
- HTML rapor,
- başarısız testlerin izi, ekran görüntüsü ve videosu (`test-results/`),
- API ve frontend logları.

İz dosyasını açmak için:

```bash
npx playwright show-trace test-results/<test>/trace.zip
```
