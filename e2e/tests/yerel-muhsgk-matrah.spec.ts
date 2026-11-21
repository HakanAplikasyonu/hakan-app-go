import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * MUHSGK gelir vergisi matrahı (#575): bordrodaki GV matrahı ve stopaj oranı
 * beyana gelir. Demo bordrosunda "GV matrahı" sütunu yoktu; içe aktarıcı artık
 * brütten SGK ve işsizlik işçi paylarını düşerek hesaplıyor. Önceden "GV
 * matrahı ₺0,00" ve oran "—" görünüyordu, kesilen vergi ise doluydu.
 *
 * Veri: demo ofisi, Atlas Gıda, içinde bulunulan beyan dönemi; yalnız okur.
 */
const money = (text: string) =>
  Number(text.replace(/[^\d,-]/g, "").replace(",", "."));

test("MUHSGK: ücret satırında GV matrahı ve oran dolu (#575)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns`));
  const kinds = page.getByRole("group", { name: "Beyan türü" });
  await kinds.getByRole("button", { name: "MUHSGK", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Vergi bildirimi" }),
  ).toBeVisible();

  // Vergi bildirimi: Ücret ödemeleri satırı → matrah > 0, oran %15 (demo tarifesi).
  const wage = page.getByRole("row").filter({ hasText: "Ücret ödemeleri" });
  await expect(wage).toBeVisible();
  const cells = wage.getByRole("cell");
  expect(money(await cells.nth(1).innerText())).toBeGreaterThan(0);
  await expect(cells.nth(2)).toHaveText("%15");

  // Bordro toplamları: GV matrahı brütten küçük, sıfırdan büyük.
  const row = (label: string) =>
    page
      .locator("dl > div")
      .filter({
        has: page.locator("dt", { hasText: new RegExp(`^${label}$`) }),
      })
      .first();
  const base = money(await row("GV matrahı").locator("dd").innerText());
  const gross = money(await row("Brüt ücret").locator("dd").innerText());
  expect(base).toBeGreaterThan(0);
  expect(base).toBeLessThan(gross);
  expect(money(await cells.nth(1).innerText())).toBe(base);
});
