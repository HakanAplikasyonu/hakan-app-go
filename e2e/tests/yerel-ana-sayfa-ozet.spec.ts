import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Ofis ana sayfasının beyan özeti sade Türkçe (#710, günlük akış gezisi):
 * başlık "Dönem tacı" değil "Beyana hazırlık · <dönem>"; hazır olmayan
 * mükelleflerin hepsinde engel varsa "5 mükellefin 5 tanesinde" yerine
 * "hepsinde" der. Yalnızca okur.
 */
test("ana sayfa beyan özeti sade: başlık ve 'hepsinde' (#710)", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(""));
  const main = page.getByRole("main");
  await expect(main.getByText(/Beyana hazırlık · /).first()).toBeVisible();
  await expect(main.getByText(/Dönem tacı/i)).toHaveCount(0);
  const text = await main.innerText();
  expect(text).not.toMatch(/(\d+) mükellefin \1 tanesinde/);
});
