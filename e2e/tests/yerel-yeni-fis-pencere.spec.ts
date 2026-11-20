import { expect, test } from "@playwright/test";

import { DEMO, officePath, openNewEntry, pickTaxpayer, uiLogin } from "./lib";

/**
 * Yeni fiş (#710, Oğuz demo geri bildirimi): Fişler ekranında üstte boş
 * form ve fazladan satır açılmaz; "Yeni fiş" yalnız fişi açılır pencerede
 * açar, liste aşağı itilmez. Kaydedince pencere kapanır, sayfa en alta ya da
 * başka bir yere atılmaz (kaydırma konumu korunur).
 */
const description = `E2E yeni fiş penceresi ${Date.now()}`;
const amount = `${100 + (Date.now() % 800)},${String(Date.now() % 97).padStart(2, "0")}`;

test("yeni fiş açılır pencerede; kayıttan sonra kaydırma konumu korunur (#710)", async ({
  page,
}) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/journals"));
  await pickTaxpayer(page, DEMO.selin.title);

  // Sayfada boş form yok; yalnız düğme.
  const formRows = page.locator('tbody tr:has(input[placeholder="770.01"])');
  await expect(
    page.getByRole("button", { name: "Yeni fiş", exact: true }),
  ).toBeVisible();
  await expect(formRows).toHaveCount(0);

  // Biraz aşağı kaydır; konum kayıttan sonra aynı kalmalı.
  await page.mouse.wheel(0, 300);
  await expect
    .poll(() => page.evaluate(() => window.scrollY))
    .toBeGreaterThan(0);
  const before = await page.evaluate(() => window.scrollY);

  await openNewEntry(page);
  await expect(formRows).toHaveCount(2);
  await page
    .getByPlaceholder(/açıklama/i)
    .first()
    .fill(description);
  await formRows.nth(0).locator("input").nth(0).fill("770.01");
  await formRows.nth(0).locator("input").nth(2).fill(amount);
  await formRows.nth(1).locator("input").nth(0).fill("100.01");
  await page.getByRole("button", { name: "Farkı kapat" }).click();
  await page.getByRole("button", { name: "Taslağı kaydet" }).click();

  await expect(page.getByRole("dialog", { name: "Yeni fiş" })).toBeHidden();
  await expect(formRows).toHaveCount(0);
  // Liste yenilenir; sayfa yerinde kalır.
  await page.waitForLoadState("networkidle");
  const after = await page.evaluate(() => window.scrollY);
  expect(Math.abs(after - before)).toBeLessThan(40);
});
