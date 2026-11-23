import { expect, test } from "@playwright/test";

import { DEMO, expectNoRawText, officePath, uiLogin } from "./lib";

/**
 * Masaüstü ajanını indir (Oğuz'un demo geri bildirimi): Ofis › Masaüstü
 * ajanları sayfasında en yeni `desktop-v*` GitHub Release'inin .dmg ve .msi
 * paketlerine giden "Mac için indir" / "Windows için indir" düğmeleri, sürüm
 * yoksa "Henüz yayınlanmadı", imzasız paket için açma adımları ve Rehber'deki
 * masaüstü ajanı adımının bu bölüme bağlanması.
 *
 * GitHub API yanıtı page.route ile taklit edilir; gerçek sürüme bağlı değil.
 * Yalnız okur.
 */
const RELEASES_API = "https://api.github.com/repos/HakanAplikasyonu/hakan-app-go/releases**";
const DL = "https://github.com/HakanAplikasyonu/hakan-app-go/releases/download";

const RELEASES = [
  {
    tag_name: "v9.9.9",
    published_at: "2026-11-22T10:00:00Z",
    assets: [{ name: "other.dmg", browser_download_url: `${DL}/v9.9.9/other.dmg` }],
  },
  {
    tag_name: "desktop-v0.3.0-beta.1",
    prerelease: true,
    published_at: "2026-11-21T10:00:00Z",
    assets: [
      {
        name: "Hakan_0.3.0-beta.1_universal.dmg",
        browser_download_url: `${DL}/desktop-v0.3.0-beta.1/Hakan_0.3.0-beta.1_universal.dmg`,
      },
    ],
  },
  {
    tag_name: "desktop-v0.2.0",
    prerelease: false,
    published_at: "2026-11-20T10:00:00Z",
    html_url: "https://github.com/HakanAplikasyonu/hakan-app-go/releases/tag/desktop-v0.2.0",
    assets: [
      {
        name: "Hakan_0.2.0_universal.dmg",
        size: 12_000_000,
        browser_download_url: `${DL}/desktop-v0.2.0/Hakan_0.2.0_universal.dmg`,
      },
      {
        name: "Hakan_0.2.0_x64-setup.exe",
        browser_download_url: `${DL}/desktop-v0.2.0/Hakan_0.2.0_x64-setup.exe`,
      },
      {
        name: "Hakan_0.2.0_x64_en-US.msi",
        browser_download_url: `${DL}/desktop-v0.2.0/Hakan_0.2.0_x64_en-US.msi`,
      },
      {
        name: "Hakan_0.2.0_x64_en-US.msi.sig",
        browser_download_url: `${DL}/desktop-v0.2.0/Hakan_0.2.0_x64_en-US.msi.sig`,
      },
    ],
  },
];

test("Masaüstü ajanları: Mac ve Windows için indir düğmeleri en yeni sürüme gider", async ({
  page,
}) => {
  await page.route(RELEASES_API, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RELEASES) }),
  );
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/agents"));

  const panel = page.getByTestId("desktop-download");
  await expect(page.getByRole("heading", { name: "Masaüstü ajanını indir" })).toBeVisible();

  // Kararlı sürüm beta'dan önce gelir; başka etiketler yok sayılır.
  await expect(panel).toContainText("Sürüm 0.2.0");
  await expect(panel).not.toContainText("Deneme sürümü");

  const mac = panel.getByRole("link", { name: /Mac için indir/ });
  await expect(mac).toHaveAttribute("href", `${DL}/desktop-v0.2.0/Hakan_0.2.0_universal.dmg`);
  const win = panel.getByRole("link", { name: /Windows için indir/ });
  await expect(win).toHaveAttribute("href", `${DL}/desktop-v0.2.0/Hakan_0.2.0_x64_en-US.msi`);

  // İmzasız paket için açma adımları.
  await expect(panel).toContainText("İlk açılışta uyarı çıkarsa");
  await expect(panel).toContainText("Yine de aç");
  await expect(panel).toContainText("Ek bilgi");
  await expect(panel).toContainText("Yine de çalıştır");
  await expectNoRawText(page.locator("main"));

  // Rehber'deki masaüstü ajanı adımı indirme bölümüne bağlanır.
  await page.goto(officePath("/guide"));
  const step = page.getByTestId("guide-step-desktop-agent");
  const link = step.getByRole("link", { name: "Bu ekrana git" });
  await expect(link).toHaveAttribute("href", officePath("/agents#indir"));
  await link.click();
  await page.waitForURL((u) => u.pathname === officePath("/agents") && u.hash === "#indir");
  await expect(page.getByTestId("desktop-download")).toBeVisible();
});

test("Masaüstü ajanları: sürüm yoksa Henüz yayınlanmadı yazar", async ({ page }) => {
  await page.route(RELEASES_API, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([RELEASES[0]]),
    }),
  );
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/agents"));

  const panel = page.getByTestId("desktop-download");
  await expect(panel.getByTestId("desktop-download-none")).toContainText("Henüz yayınlanmadı");
  await expect(panel.getByRole("link", { name: /için indir/ })).toHaveCount(0);
  // Açma adımları sürüm olmasa da okunur.
  await expect(panel).toContainText("İlk açılışta uyarı çıkarsa");
  await expectNoRawText(page.locator("main"));
});
