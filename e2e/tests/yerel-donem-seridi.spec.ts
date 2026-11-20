import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Dönem şeridi ne gösterildiğini söyler (#710, günlük akış gezisi): "Tüm
 * dönemler" seçiliyken hiçbir ay seçili görünmez ve şerit "Sekmeler tüm
 * dönemleri gösteriyor." der (eskiden beyan ayı seçili duruyordu, liste ise
 * bütün dönemleri gösteriyordu). Bir aya basınca o ay seçili olur, şerit
 * "Sekmeler … için süzülü." der; "Tüm dönemleri göster" seçimi kaldırır.
 * Yalnızca okur ve gezinir.
 */
test("dönem şeridi seçimi açıkça söyler; tek tıkla tüm dönemlere döner (#710)", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${atlas}/warnings`));

  const strip = page.getByRole("region", { name: "Dönem şeridi" });
  const scope = strip.getByTestId("period-strip-scope");
  const months = strip.getByRole("listitem").getByRole("button");

  // Başlangıçta tüm dönemler (ofis seçicisi varsayılanı): hiçbir ay basılı değil.
  await expect(scope).toBeVisible();
  if (await strip.getByRole("button", { name: "Tüm dönemleri göster" }).isVisible()) {
    await strip.getByRole("button", { name: "Tüm dönemleri göster" }).click();
  }
  await expect(scope).toHaveText("Sekmeler tüm dönemleri gösteriyor.");
  await expect(strip.locator('[aria-pressed="true"]')).toHaveCount(0);

  // Bir aya basınca o ay seçili, satır süzgeci söyler.
  const last = months.last();
  await last.click();
  await expect(last).toHaveAttribute("aria-pressed", "true");
  await expect(scope).toContainText("için süzülü.");

  // Tek tıkla tüm dönemlere dönülür.
  await strip.getByRole("button", { name: "Tüm dönemleri göster" }).click();
  await expect(scope).toHaveText("Sekmeler tüm dönemleri gösteriyor.");
  await expect(strip.locator('[aria-pressed="true"]')).toHaveCount(0);
});
