import { defineConfig, devices } from "@playwright/test";

/**
 * Uçtan uca duman testi (#170). Ortamı scripts/e2e-up.sh kurar; testler o
 * ortama bağlanır, kendisi sunucu başlatmaz.
 *
 *   E2E_BASE_URL  frontend (varsayılan http://localhost:3000)
 *   E2E_API_URL   Go API (varsayılan http://127.0.0.1:8080)
 *   E2E_MAIL_URL  Mailhog HTTP API (varsayılan http://127.0.0.1:8025)
 *
 * Paralellik dosya düzeyindedir: her dosya ayrı bir worker'da koşar.
 * Dosya içindeki akışlar `serial`dır (adım bir öncekinin verisini kullanır).
 * Dosyalar birbirinin verisine dokunmaz (bkz. her dosyanın başındaki not).
 * Yeniden deneme yok: kararsız (flaky) test gizlenmez, düşer.
 */
export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: process.env.E2E_WORKERS ? Number(process.env.E2E_WORKERS) : 3,
  retries: 0,
  forbidOnly: !!process.env.CI,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never" }]]
    : [["list"]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    locale: "tr-TR",
    timezoneId: "Europe/Istanbul",
    // Başarısızlıkta iz, ekran görüntüsü ve video (CI artefaktı: e2e-report-<bacak>).
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    acceptDownloads: true,
    actionTimeout: 20_000,
    navigationTimeout: 60_000,
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
