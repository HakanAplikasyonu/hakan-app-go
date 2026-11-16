# Uçtan uca duman testi (#170)

Doğrulama raporundaki akışı (`docs/dogrulama-raporu.md` §2.2) Playwright ile koşar:

ofis girişi → mükellef ekleme (onay e-postası, kayıt, onay) → UBL yükle → fatura detayından hesap kodu önerisi ve kuyruktan onay → banka eşleme ve uyarılar → dönem kilidi → yevmiye taslağı ve onayı → Luca / Zirve dosyası indirme → mükellef web paneli → tenant izolasyonu.

Testler sırayla koşar: her adım bir öncekinin verisini kullanır. Bu yüzden ortam her koşudan önce sıfırdan kurulmalıdır.

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

- `make e2e-up` (`scripts/e2e-up.sh`): `compose.local.yml` altyapısını temiz birimlerle kurar. Ardından migration'ları sıfırdan uygular, `demo-seed -reset` çalıştırır, Go API'yi ve `next build && next start`'ı başlatır. Env dosyası, pid'ler ve loglar `e2e/.run/` altına yazılır.
- İlk kurulumda `cd e2e && npm ci && npx playwright install chromium` gerekir.
- 5432 portu doluysa `E2E_PG_PORT=55432 make e2e-up` kullanın. Frontend derlemesini atlamak için `E2E_SKIP_BUILD=1`, ek compose dosyası (ör. yerel override) için `E2E_COMPOSE_FILES=a.yml:b.yml`.

Adresler: `E2E_BASE_URL` (frontend, `http://localhost:3000`), `E2E_API_URL` (`http://127.0.0.1:8080`), `E2E_MAIL_URL` (Mailhog, `http://127.0.0.1:8025`).

## Arayüzü olmayan adımlar

Banka hesabı ile ekstre yükleme (#144) ve dönem kilidi için ofis panelinde ekran yok. Bu adımlar API'den yapılır; sonuçları arayüzde (uyarılar, komuta merkezi) ve API'de denetlenir.

## CI

`.github/workflows/ci.yml` içindeki `e2e` işi şu durumlarda koşar: backend ya da frontend değiştiğinde, `e2e/**`, `scripts/e2e-*`, `compose.local.yml` ya da `.env.example` değiştiğinde ve main'de.

İlk aşamada **zorunlu değil**: `ci` kapısında yok ve `continue-on-error` ile iş düşse de iş akışı kırmızıya dönmez. Rapor, iz (trace) ve loglar `e2e-report` artefaktında.
