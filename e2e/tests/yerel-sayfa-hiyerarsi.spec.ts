import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Ekranlarda hiyerarşi (#710): üst çubuktaki konum ofis → mükellef → dönem
 * → sayfa → kayıt sırasını gösterir. Fatura ayrıntısında "Faturalar"
 * listeye geri götürür; dönem sayfanın dönemiyle aynıdır.
 *
 * Veri: demo ofisi, Atlas Gıda; yalnız okur ve gezinir.
 */
test("konum: ofis → mükellef → dönem → sayfa → kayıt; kayıttan listeye dönülür (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}`;
  const invoice = (
    await api.json<{ items: { uuid: string }[] }>(
      await api.get(`${base}/invoices?period=2026-10&limit=1`),
    )
  ).items[0];
  expect(invoice).toBeTruthy();
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/invoices?period=2026-10`),
  );
  const crumbs = page.getByRole("navigation", { name: "Konum" });
  await expect(
    crumbs.getByRole("combobox", { name: /Mükellef değiştir/ }),
  ).toContainText(DEMO.atlas.title);
  await expect(crumbs.getByTestId("crumb-period")).toHaveText("Ekim 2026");
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");

  // Kayıt ayrıntısı: son halka "Kayıt", sayfa adı listeye götürür.
  await page.goto(
    officePath(`/engagements/${engagement}/invoices/${invoice!.uuid}`),
  );
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Kayıt");
  await expect(crumbs.getByTestId("crumb-period")).toHaveText("Ekim 2026");
  await crumbs.getByRole("link", { name: "Faturalar" }).click();
  await expect(page).toHaveURL(
    new RegExp(`/engagements/${engagement}/invoices(\\?|$)`),
  );
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");
});
