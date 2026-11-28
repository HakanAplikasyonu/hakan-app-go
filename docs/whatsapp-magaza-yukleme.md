# WhatsApp uzantısını Chrome Web Store'a yükleme (#780)

Bu rehber Oğuz için yazıldı: uzantıyı Chrome Web Store'a ilk kez yüklemek, onaydan sonra panele bağlamak ve yeni sürüm göndermek. Komut çalıştırmak gerekmez; her şey tarayıcıdan yapılır.

Mağazaya girince ofisler uzantıyı zip indirip klasöre çıkarmadan, geliştirici modunu açmadan tek tıkla kurar. Yeni sürümler de kendiliğinden gelir.

## Gerekenler

- Bir Google hesabı. Bu hesap mağazada "yayıncı" olur; şirketin ortak bir hesabı daha uygun olur. Hesapta iki adımlı doğrulama açık olmalı.
- GitHub'da depoya erişim (paketi ve görselleri indirmek için).
- Panelin adresi (tarayıcıda panelin açıldığı adres). Aşağıda `<panel adresi>` diye geçer.

## 1. Paketi ve görselleri indirin

Paketi ve görselleri GitHub kendisi hazırlar. Bilgisayarda bir şey derlemeniz gerekmez.

1. GitHub'da depoyu açın, üstte **Actions** sekmesine geçin.
2. Soldan **CI**'ı seçin ve `main` dalındaki en son yeşil çalışmayı açın.
3. Sayfanın altındaki **Artifacts** bölümünden ikisini indirin:
   - `easytrade-whatsapp-magaza`: mağazaya yüklenecek paket.
   - `easytrade-whatsapp-magaza-gorselleri`: mağaza görselleri.
4. İkisi de zip olarak iner. Çift tıklayıp açın:
   - Paket zip'inin içinden `magaza-easytrade-whatsapp-<sürüm>.zip` çıkar. **Mağazaya bu içteki zip yüklenir**, açmayın.
   - Görsel zip'inde `extensions/whatsapp/store-gorseller/` klasörü vardır: `ekran-1-1280x800.png` … `ekran-4-1280x800.png`, `tanitim-440x280.png`, `simge-128.png`. Aynı zip'teki `frontend/...` görselleri kullanıcı kılavuzu içindir, mağazada kullanılmaz.

`easytrade-whatsapp-magaza` listede yoksa GitHub'daki depo değişkeni `WHATSAPP_API_URL` boştur. Depo ayarlarında **Secrets and variables → Actions → Variables** altında API adresinizi (ör. `https://api.ofisiniz.com`) yazın ve CI'ı yeniden çalıştırın. Paket bu adrese bağlıdır; uzantı yalnızca bu sunucuyla konuşur.

Görselleri açıp bakın: adlar ve tutarlar uydurmadır, WhatsApp logosu yoktur.

## 2. Geliştirici hesabını açın

1. Chrome Web Store geliştirici paneline gidin: `https://chrome.google.com/webstore/devconsole`.
2. Google hesabınızla girin ve geliştirici sözleşmesini okuyup kabul edin.
3. Tek seferlik kayıt ücretini kartla ödeyin. **Ücretin güncel tutarını kayıt sayfasında kendiniz kontrol edin**; burada yazmıyoruz çünkü değişebilir.
4. **Hesap** (Account) sayfasını doldurun:
   - Yayıncı adı: mağazada uzantının altında görünür (ör. şirketin adı).
   - İletişim e-postası: Google bu adrese doğrulama e-postası gönderir; e-postadaki bağlantıya tıklayın. Doğrulanmadan yayımlanamaz.
   - Tacir (trader) beyanı sorulursa şirket bilgilerini girin.

## 3. Yeni öğe oluşturun ve paketi yükleyin

1. Panelde **Yeni öğe** (New item) düğmesine basın.
2. 1. adımdaki içteki zip'i (`magaza-easytrade-whatsapp-<sürüm>.zip`) seçin.
3. Yükleme bitince öğenin sayfası açılır. Sol menüde **Mağaza girişi**, **Gizlilik** ve **Dağıtım** sekmeleri vardır.

## 4. Mağaza girişi (Store listing)

Bütün metinler depoda hazır: `extensions/whatsapp/store/`. GitHub'da dosyayı açıp metni kopyalayın.

| Alan | Ne yazılır |
|---|---|
| Açıklama | `ayrintili-aciklama.txt` dosyasının tamamı |
| Kategori | Verimlilik altında **İş akışı ve planlama** (yoksa **Araçlar**) |
| Dil | Türkçe |
| Mağaza simgesi | `simge-128.png` |
| Ekran görüntüleri | `ekran-1` … `ekran-4` (1280×800), sırayla |
| Küçük tanıtım görseli | `tanitim-440x280.png` |
| Resmî URL / ana sayfa | `<panel adresi>` (isteğe bağlı) |
| Destek URL'si | İsteğe bağlı; ofislerin size ulaştığı adres |

Ad ve kısa açıklama paketin içinden gelir, burada yazılmaz (`kisa-aciklama.txt` aynısıdır).

## 5. Gizlilik (Privacy practices)

Bu sekmedeki her şey `extensions/whatsapp/store/magaza-bilgileri.md` dosyasında hazır:

1. **Tek amaç**: "Tek amaç" başlığının altındaki paragraf.
2. **İzin gerekçeleri**: `storage` ve konak izni (`web.whatsapp.com` ve sunucu adresi) için ayrı kutular çıkar; her birine dosyadaki ilgili paragrafı yapıştırın.
3. **Uzak kod kullanıyor musunuz?**: **Hayır**.
4. **Veri kullanımı**: dosyadaki tabloda "Evet" olan türleri işaretleyin (kişisel olarak tanımlayıcı bilgiler, finansal ve ödeme bilgileri, kimlik doğrulama bilgileri, kişisel iletişimler, web sitesi içeriği). Diğerleri boş kalır.
5. Alttaki üç beyanın üçünü de işaretleyin.
6. **Gizlilik politikası URL'si**: `<panel adresi>/gizlilik/whatsapp-uzantisi`. Göndermeden önce bu adresi gizli pencerede açıp oturum açmadan göründüğünü kontrol edin.

## 6. Dağıtım (Distribution)

- Ücret: **Ücretsiz**.
- Görünürlük: **Liste dışı** (Unlisted). Mağaza aramasında çıkmaz, yalnızca bağlantıyı bilen kurar; panel bu bağlantıyı ofislere gösterir. İleride herkese açmak isterseniz buradan değiştirirsiniz.
- Bölgeler: bütün bölgeler (ya da yalnızca Türkiye).

## 7. İncelemeye gönderin

1. Sağ üstte **İncelemeye gönder** (Submit for review) düğmesine basın.
2. "Onaydan sonra otomatik yayımla" seçeneği çıkarsa açık bırakın.
3. İnceleme birkaç gün sürebilir; sonuç iletişim e-postasına gelir.

Reddedilirse e-postada gerekçe yazar. Sık görülenler:

- **Ad**: "WhatsApp" bir ticari marka. Ad, WhatsApp'ın kendi ürünü sanılacak diye reddedilirse yazılıma haber verin; ad `Easytrade Müşavirim Ofis – WhatsApp Web için` gibi değiştirilip yeni paket hazırlanır.
- **İzin gerekçesi yetersiz**: e-postadaki izni ve gerekçeyi yazılıma iletin.
- **Gizlilik sayfası açılmıyor**: 5. adımdaki adresi gizli pencerede yeniden deneyin.

## 8. Onaydan sonra: panele bağlayın

1. Geliştirici panelinde öğeyi açın. Üstte mağaza bağlantısı (`https://chromewebstore.google.com/detail/...` ile başlar) ve 32 harfli **öğe kimliği** (Item ID) yazar.
2. Sunucunun ortam ayarlarına (Dokploy'da uygulamanın ortamı, ya da sunucudaki `.env.server`) iki satırı girin:
   - `NEXT_PUBLIC_WHATSAPP_STORE_URL` = mağaza bağlantısı;
   - `NEXT_PUBLIC_WHATSAPP_EXTENSION_IDS` = öğe kimliği (panel uzantının kurulu olup olmadığını bununla anlar).
3. Frontend uygulamasını Dokploy'dan yeniden başlatın (yeniden dağıtın).
4. Panelde **WhatsApp** sayfasını açın: kurulum kartında zip yerine **Chrome'a ekle** düğmesi görünmeli. Kendi tarayıcınızda bir kez kurup eşleştirerek deneyin.

Zip ile kurulmuş eski uzantılar çalışmaya devam eder. Ofisler mağazadakini kurduktan sonra eskisini `chrome://extensions` sayfasından kaldırabilir; yeniden eşleştirmek gerekir.

## Yeni sürüm göndermek

1. Yazılım uzantının sürüm numarasını artırır (`extensions/whatsapp/package.json`). Mağaza aynı ya da daha küçük sürümü kabul etmez.
2. `main` güncellenince 1. adımdaki gibi yeni `easytrade-whatsapp-magaza` paketini indirin.
3. Geliştirici panelinde öğeyi açın, **Paket** (Package) sekmesinde **Yeni paket yükle**'ye basın, içteki zip'i seçin ve incelemeye gönderin.
4. İzinler değişmediyse mağaza girişi ve gizlilik sekmelerine dokunmanız gerekmez. Görseller değiştiyse yenilerini aynı yerlere yükleyin.

## Yazılım için notlar

- Paket: `extensions/whatsapp/scripts/store.mjs` (`npm run package:store`), manifest denetimi `scripts/store-manifest.mjs` (testi `tests/store.test.ts`). İsteğe bağlı konak izni pakete gömülü sunucuya daraltılır.
- Görseller: `extensions/whatsapp/scripts/screenshots.mjs`, boyut denetimi `scripts/store-images-check.mjs`; CI işi `extension-whatsapp-gorseller`. Aynı iş kılavuz görüntülerini (`frontend/public/yardim/img/whatsapp-*.png`) de üretir; kılavuz değiştiğinde bunlar yapıttan alınıp commit'lenir.
- Gizlilik sayfası: `frontend/src/features/legal/components/whatsapp-extension-privacy-page.tsx`, metinler `frontend/src/locales/{tr,en}/legal.json` (`wa_privacy_*`). Uzantı yeni bir veri gönderirse bu sayfa ve `extensions/whatsapp/store/magaza-bilgileri.md` birlikte güncellenir.
