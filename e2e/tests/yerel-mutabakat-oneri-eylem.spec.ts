import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Mutabakat önerisi tek tıkla işe götürür (#669 UX geçişi): öneri "fiş
 * taslaklarını onaylayıp dışa aktarın" diyorsa satırda "Taslak fişlere git"
 * bağlantısı çıkar ve aynı dönemin Fişler sekmesini açar. Eylemi olmayan
 * öneride (örn. elle açıklanmış satır) bağlantı yok.
 *
 * Veri: demo ofisi, Atlas Gıda'nın demo mutabakatı (100 Kasa farkı
 * aktarılmamış taslak tutarına eşit); yalnız okur ve gezinir.
 */
test("mutabakat: taslak önerisi Fişler sekmesine götürür", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/reconciliation`));
  const row = page.getByRole("row").filter({ hasText: "aktarılmamış fiş taslaklarının" }).first();
  await expect(row).toBeVisible();
  const link = row.getByRole("link", { name: /^Taslak fişlere git: / });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("href", new RegExp(`/engagements/${engagement}/journals\\?period=\\d{4}-\\d{2}$`));

  await link.click();
  await expect(page).toHaveURL(new RegExp(`/engagements/${engagement}/journals\\?period=\\d{4}-\\d{2}`));
});
