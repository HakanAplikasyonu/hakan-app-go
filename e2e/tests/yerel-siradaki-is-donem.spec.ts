import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * "Sıradaki iş" çubuğu hangi ayı saydığını söyler (#710): banka listesi
 * "Tümü"yken çubuk yine tek ayı (beyan dönemi) sayar; "bu dönem banka
 * hareketi yok" listede hareket görünürken çelişkili okunmasın diye çubukta
 * "Tüm dönemler seçili; burada yalnız {ay} sayılır" notu çıkar.
 * Bir dönem seçilince not kalkar, çubuk o dönemi sayar.
 *
 * Veri: demo ofisi, Atlas Gıda; yalnız okur ve gezinir.
 */
test("Sıradaki iş: liste Tümü iken sayılan ay söylenir; dönem seçilince not kalkar (#710)", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  const bar = page.getByRole("region", { name: /^Sıradaki iş · / });
  await expect(bar).toBeVisible();
  const title = (await bar.getAttribute("aria-label"))!.replace("Sıradaki iş · ", "");
  const note = bar.getByTestId("copilot-scope-note");
  await expect(note).toHaveText(`Tüm dönemler seçili; burada yalnız ${title} sayılır.`);

  await page.goto(officePath(`/engagements/${engagement}/bank?period=2026-10`));
  const october = page.getByRole("region", { name: "Sıradaki iş · Ekim 2026" });
  await expect(october).toBeVisible();
  await expect(october.getByTestId("copilot-scope-note")).toHaveCount(0);
});
