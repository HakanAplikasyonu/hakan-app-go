import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Genel bakış, yeni müşavir gözüyle (#710): "Bugün gelen" sayısının altındaki
 * döküm toplamı tutar (fatura + fiş + belge); dönem kartının başlığı ne
 * olduğunu söyler ("Dönem durumu"); akışta teknik kısaltma (UBL) görünmez.
 *
 * Veri: demo ofisi; yalnız okur. Sayılar paralel testlerle değişebilir, bu
 * yüzden yalnız ekranda yazanların kendi içinde tutarlılığına bakılır.
 */
test("genel bakış: bugün gelen dökümü toplamı tutar, başlıklar sade (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());

  const today = page.getByRole("link").filter({ hasText: "Bugün gelen" });
  await expect(today).toHaveAttribute("href", /\/upload$/);
  await expect(today).toContainText(/\d+ fatura · \d+ fiş · \d+ belge/);
  const text = (await today.innerText()).replace(/\s+/g, " ");
  const m = text.match(
    /Bugün gelen (\d+) (\d+) fatura · (\d+) fiş · (\d+) belge/,
  );
  expect(m, text).toBeTruthy();
  const [total, inv, rec, doc] = m!.slice(1).map(Number);
  expect(inv + rec + doc).toBe(total);

  await expect(page.getByText(/^Dönem durumu · /)).toBeVisible();
  await expect(page.getByText("Dönem tacı")).toHaveCount(0);
  await expect(page.getByText(/UBL/)).toHaveCount(0);
});
