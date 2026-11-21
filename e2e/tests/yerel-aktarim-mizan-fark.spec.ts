import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Programa aktarımlar ekranında mizan farkı tek tıkla açılır (#669 UX
 * geçişi): "Son aktarımlar" tablosunda "N hesapta fark" yazısı bir
 * bağlantıdır ve o mükellefin Mutabakat ekranını açar (satırın kendisi
 * Fişler'e gider). "Uyumlu" bağlantı değildir.
 *
 * Veri: demo ofisinin aktarım geçmişi (Atlas Gıda Ekim 2026 Luca aktarımı
 * mizan farklı); yalnız okur ve gezinir.
 */
test("aktarımlar: mizan farkı Mutabakat ekranını açar", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/transfers"));
  const diff = page.getByRole("link", { name: /^\d+ hesapta fark: Mutabakatta aç$/ }).first();
  await expect(diff).toBeVisible();
  await expect(page.getByRole("link", { name: /^Uyumlu/ })).toHaveCount(0);

  await diff.click();
  await expect(page).toHaveURL(/\/engagements\/[0-9a-f-]{36}\/reconciliation$/);
  await expect(page.getByRole("heading", { name: "Mutabakat", exact: true }).first()).toBeVisible();
});
