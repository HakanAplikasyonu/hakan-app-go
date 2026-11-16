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
- Akış testleri (`--grep-invert @a11y`) henüz zorunlu değildir: düşerse iş uyarı yazar ve geçer.

`e2e-report` artefaktında şunlar bulunur:
- HTML rapor,
- başarısız testlerin izi, ekran görüntüsü ve videosu (`test-results/`),
- API ve frontend logları.

İz dosyasını açmak için:

```bash
npx playwright show-trace test-results/<test>/trace.zip
```
