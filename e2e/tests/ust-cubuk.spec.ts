import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Ofis üst çubuğu (#521): bağlam (ofis → mükellef → sayfa), hızlı mükellef
 * değiştirici, kullanıcı menüsü; masaüstü ve telefonda taşma yok. Yalnızca
 * okur ve gezinir.
 */
test.describe("ofis üst çubuğu", () => {
  test("masaüstü: ekmek kırıntısı, mükellef değiştirici, kullanıcı menüsü", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/warnings"));
    const crumbs = page.getByRole("navigation", { name: "Konum" });
    await expect(crumbs).toBeVisible();
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Uyarılar");

    // Mükellef ayrıntısı: mükellef adı değiştiricidir, sekme korunur.
    await page.goto(officePath("/taxpayers"));
    await page.getByRole("link", { name: DEMO.atlas.title }).first().click();
    await expect(page.getByRole("heading", { level: 1, name: DEMO.atlas.title })).toBeVisible();
    await page.getByRole("navigation", { name: "Mükellef bölümleri" }).getByRole("link", { name: "Banka" }).click();
    await expect(page).toHaveURL(/\/engagements\/[^/]+\/bank/);
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Banka");
    await crumbs.getByRole("combobox", { name: /Mükellef değiştir/ }).click();
    await page.getByPlaceholder("Mükellef ara…").fill("Selin");
    await page.getByRole("option", { name: new RegExp(DEMO.selin.title) }).click();
    await expect(page).toHaveURL(/\/engagements\/[^/]+\/bank/);
    await expect(page.getByRole("heading", { level: 1, name: DEMO.selin.title })).toBeVisible();

    // Kullanıcı menüsü: rol ve ofis, yardım, tema, çıkış.
    await page.getByRole("banner").getByRole("button", { name: /^Profil/ }).click();
    const menu = page.getByRole("menu");
    await expect(menu.getByRole("menuitem", { name: "Yardım ve kılavuzlar" })).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: /temaya geç/ })).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: "Çıkış yap" })).toBeVisible();
  });

  test("telefon: tek satır, yatay taşma yok, dokunma hedefleri 44 px", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/warnings"));
    const header = page.getByRole("banner");
    await expect(header.getByRole("navigation", { name: "Konum" })).toBeVisible();
    // Üst çubuk kendi genişliğine sığar: içerik taşmaz, sayfa yana kaymaz.
    const overflow = await header.evaluate((el) => {
      const row = el.firstElementChild as HTMLElement;
      return Math.max(row.scrollWidth - row.clientWidth, el.getBoundingClientRect().right - window.innerWidth);
    });
    expect(overflow).toBeLessThanOrEqual(0);
    for (const name of [/^Ara/, /^Profil/]) {
      const box = await header.getByRole("button", { name }).first().boundingBox();
      expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
  });
});
