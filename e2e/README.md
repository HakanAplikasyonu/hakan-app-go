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
| `tests/mutabakat.spec.ts` | Mutabakat panosu (#389): hesap farkları (filtre, ayrıntı, açıkla / yeniden aç), BA/BS beyan satırı ekle / sil, cari mektubu oluştur ve gönder, karşı firmanın yanıt sayfası (itiraz), geçersiz bağlantı | Demo ofisi, Atlas Gıda; benzersiz alıcı ve VKN |
| `tests/beyanname.spec.ts` | Beyannameler (#390): KDV1 düzeltme, kontrol listesi, hazırla, Excel, verildi kaydı; MUHSGK hazırla / Excel / taslağa geri al; Ba / Bs formları | Demo ofisi, Atlas Gıda (beyan dönemi) |

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
