import { readFileSync, statSync } from "node:fs";

import { expect, test, type Page } from "@playwright/test";

/**
 * Yevmiye ekranında 3 satırlı dengeli bir fiş kaydeder ve onaylar.
 * `receivable`: alıcı hesabı. Demo ofisinin hesap planında 120.01 grup
 * hesabıdır (#456), çıktı yalnızca muavine izin verir: 120.01.001.
 */
export async function newApprovedEntry(page: Page, description: string, receivable = "120.01") {
  await page.getByPlaceholder(/açıklama/i).first().fill(description);
  const rows = page.locator('tbody tr:has(input[placeholder="770.01"])');
  const lines: [string, string, string, string][] = [
    [receivable, "Yıldız Market", "1200", ""],
    ["600.01", "Satış", "", "1000"],
    ["391.01", "Hesaplanan KDV", "", "200"],
  ];
  while ((await rows.count()) < lines.length) {
    await page.getByRole("button", { name: "Satır ekle" }).click();
  }
  for (let i = 0; i < lines.length; i++) {
    const inputs = rows.nth(i).locator("input");
    for (let j = 0; j < 4; j++) if (lines[i][j]) await inputs.nth(j).fill(lines[i][j]);
  }
  await page.getByRole("button", { name: "Taslağı kaydet" }).click();
  const row = page.locator("tr", { hasText: "Taslak" }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: /Onayla/ }).click();
  await expect(page.locator("tr", { hasText: "Onaylı" }).first()).toBeVisible();
}

/** Onaylı fişleri `driver` programına aktarır ve dosyayı indirip denetler. */
export async function exportAndDownload(page: Page, driver: "luca" | "zirve") {
  await page.locator('select:has(option[value="luca"])').selectOption(driver);
  await page.getByRole("button", { name: "Çıktı al", exact: true }).last().click();
  await expect(page.getByText("1 fiş aktarıldı.")).toBeVisible();
  const file = page
    .getByRole("button", { name: new RegExp(`^${driver}_fis_\\d{4}-\\d{2}\\.xlsx$`) })
    .first();
  const [download] = await Promise.all([page.waitForEvent("download"), file.click()]);
  const saved = test.info().outputPath(download.suggestedFilename());
  await download.saveAs(saved);
  expect(download.suggestedFilename()).toMatch(new RegExp(`^${driver}_fis_`));
  expect(statSync(saved).size).toBeGreaterThan(1000);
  expect(readFileSync(saved).subarray(0, 2).toString()).toBe("PK"); // xlsx = zip
}
