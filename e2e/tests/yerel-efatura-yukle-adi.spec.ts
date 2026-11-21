import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Teknik ad yok (#710): menüde ve üst çubukta "UBL yükle" yerine
 * "e-Fatura yükle" yazar; yükleme sayfasının dipnotu XSD / Schematron gibi
 * kod adları yerine neyin denetlendiğini sade Türkçe söyler.
 *
 * Veri: demo ofisi; yalnız okur.
 */
test("e-Fatura yükle: menü, üst çubuk ve sayfa sade Türkçe (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/taxpayers"));

  const links = page.getByRole("link", { name: "e-Fatura yükle" });
  await expect(links.first()).toHaveAttribute("href", /\/upload$/);
  await expect(page.getByText("UBL yükle")).toHaveCount(0);

  await page.goto(officePath("/upload"));
  await expect(
    page.getByRole("heading", { level: 1, name: "e-Fatura yükle" }),
  ).toBeVisible();
  await expect(page.getByText(/GİB e-fatura biçimine uygunluk/)).toBeVisible();
  await expect(page.getByText(/Schematron|XSD/)).toHaveCount(0);
});
