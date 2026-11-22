import { expect, test } from "@playwright/test";

import { DEMO, expectNoRawText, officePath, uiLogin } from "./lib";

/**
 * Rehber (Oğuz'un demo geri bildirimi): kenar menüde "Rehber" öğesi, ilk
 * kurulum / her ay / kilitli ay / beyanname bölümleri, "Bu ekrana git"
 * bağlantıları ve adım adım tur (İleri / Geri / Kapat).
 *
 * İlk girişteki otomatik tur otomatik tarayıcıda (navigator.webdriver)
 * açılmaz; burada tur Rehber sayfasından elle başlatılır. Yalnız okur.
 */
test("Rehber: menü öğesi, Bu ekrana git ve adım adım tur", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());

  // Otomatik tur e2e'de açılmaz: diğer testler etkilenmez.
  await expect(page.getByTestId("guide-tour")).toHaveCount(0);

  // a) Menü öğesi görünür ve sayfa açılır.
  const menu = page.locator('[data-tour="guide"]');
  await expect(menu).toBeVisible();
  await expect(menu).toContainText("Rehber");
  await menu.click();
  await page.waitForURL((u) => u.pathname === officePath("/guide"));
  await expect(page.getByRole("heading", { level: 1, name: "Rehber" })).toBeVisible();

  for (const section of ["İlk kurulum", "Her ay", "Kilitli aya belge gelirse", "Beyanname"]) {
    await expect(page.getByText(section, { exact: true }).first()).toBeVisible();
  }
  await expectNoRawText(page.locator("main"));

  // Henüz ana dalda olmayan kilitli ay seçenekleri "Yakında", bağlantısız.
  const reject = page.getByTestId("guide-step-reject");
  await expect(reject).toContainText("Yakında");
  await expect(reject.getByRole("link")).toHaveCount(0);

  // b) "Bu ekrana git" gerçek ekrana gider: Sıradaki iş.
  await page
    .getByTestId("guide-step-next-task")
    .getByRole("link", { name: "Bu ekrana git" })
    .click();
  await page.waitForURL((u) => u.pathname === officePath("/copilot"));

  // c) Tur Rehber sayfasından başlar; İleri / Geri / Kapat.
  await page.goto(officePath("/guide"));
  await page.getByTestId("guide-start-tour").click();
  const tour = page.getByRole("dialog");
  await expect(tour).toBeVisible();
  await expect(tour).toContainText("Adım 1 /");
  await expect(tour.getByRole("heading", { name: "Hoş geldiniz" })).toBeVisible();
  await expect(page.getByTestId("guide-tour-prev")).toBeDisabled();

  await page.getByTestId("guide-tour-next").click();
  await expect(tour).toContainText("Adım 2 /");
  await expect(tour.getByRole("heading", { name: "Mükellef ekle" })).toBeVisible();
  // İlgili menü öğesi çerçevelenir.
  await expect(page.getByTestId("guide-tour-highlight")).toBeVisible();

  await page.getByTestId("guide-tour-next").click();
  await expect(tour).toContainText("Adım 3 /");
  await expect(tour.getByRole("heading", { name: "Belge toplama" })).toBeVisible();

  await page.getByTestId("guide-tour-prev").click();
  await expect(tour).toContainText("Adım 2 /");

  await page.getByTestId("guide-tour-close").click();
  await expect(page.getByTestId("guide-tour")).toHaveCount(0);
});
