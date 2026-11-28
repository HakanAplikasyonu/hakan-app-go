import { expect, test, type Page } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Profil ekranı cilası (#890 D): ofis kabuğunda başlık, özet kartı (ad,
 * e-posta, rol, ofis), bölümler (kişisel bilgiler, güvenlik, bildirim
 * tercihleri, yetkilerim) ve kaydetme bildirimi; 375 px'te yatay kaydırma yok.
 *
 * Veri: demo ofisi sahibi; ad değiştirilmeden kaydedilir.
 */

const SECTIONS = ["Kişisel bilgiler", "Güvenlik", "Bildirim tercihleri", "Yetkilerim"];

async function openProfile(page: Page) {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/profil"));
  await expect(page.getByRole("heading", { level: 1, name: "Profil" })).toBeVisible();
}

test("profil: özet kartı, bölümler ve kaydet bildirimi", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openProfile(page);

  const summary = page.getByRole("region", { name: "Hesap özeti" });
  await expect(summary.getByText(DEMO.owner)).toBeVisible();
  await expect(summary.getByText("Rol", { exact: true })).toBeVisible();
  await expect(summary.getByText("Ofis", { exact: true })).toBeVisible();

  const nav = page.getByRole("navigation", { name: "Profil bölümleri" });
  for (const name of SECTIONS) {
    await expect(page.getByRole("heading", { level: 2, name, exact: true })).toBeVisible();
    await expect(nav.getByRole("link", { name, exact: true })).toBeVisible();
  }
  // Güvenlik bölümünde mevcut bileşenler.
  const security = page.locator("#guvenlik");
  await expect(security.getByRole("heading", { name: "Şifre değiştir" })).toBeVisible();
  await expect(security.getByRole("heading", { name: "Aktif oturumlar" })).toBeVisible();

  // Ham izin anahtarı görünmez (ör. "tenant.settings.read").
  await expect(page.locator("#yetkiler").getByText(/^tenant\./)).toHaveCount(0);

  // Kaydet: anlaşılır başarı bildirimi.
  await page.locator("#kisisel").getByRole("button", { name: "Kaydet" }).click();
  await expect(page.getByText("Profil güncellendi")).toBeVisible();
});

test("profil: 375 px'te yatay kaydırma yok", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await openProfile(page);
  await expect(page.getByRole("heading", { level: 2, name: "Yetkilerim", exact: true })).toBeAttached();

  const overflowing = await page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    return [...document.querySelectorAll("main *")]
      .filter((e) => {
        const r = e.getBoundingClientRect();
        return (
          r.width > 0 &&
          r.right > vw + 1 &&
          !e.closest('[class*="overflow-x-auto"],[class*="overflow-auto"],[role=tablist],nav,.sr-only')
        );
      })
      .map((e) => (e as HTMLElement).innerText.slice(0, 60));
  });
  expect(overflowing).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
});
