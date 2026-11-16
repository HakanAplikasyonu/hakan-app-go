import { defineConfig, devices } from "@playwright/test";

/**
 * Uçtan uca duman testi (#170). Ortamı scripts/e2e-up.sh kurar; testler o
 * ortama bağlanır, kendisi sunucu başlatmaz.
 *
 *   E2E_BASE_URL  frontend (varsayılan http://localhost:3000)
 *   E2E_API_URL   Go API (varsayılan http://127.0.0.1:8080)
 *   E2E_MAIL_URL  Mailhog HTTP API (varsayılan http://127.0.0.1:8025)
 *
 * Akış tek bir sıralı senaryodur: her adım bir öncekinin ürettiği veriye
 * dayanır (yüklenen fatura → öneri → yevmiye ...). Bu yüzden tek worker.
 */
export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
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
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    acceptDownloads: true,
    actionTimeout: 20_000,
    navigationTimeout: 60_000,
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
