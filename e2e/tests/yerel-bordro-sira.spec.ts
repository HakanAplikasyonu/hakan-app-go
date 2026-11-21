import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Bordro ekranında sıra (#710): bordrosu olan mükellefte önce seçili dönemin
 * özeti (net, SGK, işverene maliyet) ve personel dökümü, sonra dönem listesi
 * görünür; "Bordro yükle" formu en altta. Bordrosu olmayan mükellefte form
 * başta durur (iş yüklemektir).
 *
 * Veri: demo ofisi, Atlas Gıda (Ekim ve Eylül 2026 bordroları); yalnız okur.
 */
test("bordro: özet ve dönemler önce, yükleme formu en altta (#710)", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/payroll?period=2026-10`));
  const summary = page.getByRole("region", { name: /Ekim 2026/ }).first();
  const upload = page.getByRole("region", { name: "Bordro yükle" });
  await expect(summary).toBeVisible();
  await expect(upload).toBeAttached();
  const top = async (l: typeof upload) => (await l.boundingBox())!.y;
  await upload.scrollIntoViewIfNeeded();
  const runsTable = page.getByRole("row").filter({ hasText: "Eylül 2026" }).first();
  await expect(runsTable).toBeVisible();
  expect(await top(summary)).toBeLessThan(await top(upload));
  expect(await top(runsTable)).toBeLessThan(await top(upload));
});
