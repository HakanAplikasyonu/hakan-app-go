import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Ekranlarda hiyerarşi (#710): üst çubuktaki konum ofis → mükellef → dönem
 * → sayfa → kayıt sırasını gösterir. Fatura ayrıntısında "Faturalar"
 * listeye geri götürür; dönem sayfanın dönemiyle aynıdır.
 *
 * Veri: demo ofisi, Atlas Gıda; yalnız okur ve gezinir. Dönem sabit
 * değildir: Atlas'ın ilk faturasının dönemi kullanılır. Ekim 2026 faturaları
 * duman testinin (smoke.spec.ts) UBL yüklemesinden gelir; e2e paylı koştuğu
 * için (#780) o dosya başka bacakta olabilir.
 */
test("konum: ofis → mükellef → dönem → sayfa → kayıt; kayıttan listeye dönülür (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}`;
  const invoice = (
    await api.json<{ items: { uuid: string; period: string }[] }>(
      await api.get(`${base}/invoices?limit=1`),
    )
  ).items[0];
  expect(invoice, "demo verisinde Atlas faturası").toBeTruthy();
  await api.dispose();
  const period = invoice!.period.slice(0, 7);
  const [year, month] = period.split("-").map(Number);
  const periodLabel = new Intl.DateTimeFormat("tr-TR", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(year, month - 1, 1)));

  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/invoices?period=${period}`),
  );
  const crumbs = page.getByRole("navigation", { name: "Konum" });
  await expect(
    crumbs.getByRole("combobox", { name: /Mükellef değiştir/ }),
  ).toContainText(DEMO.atlas.title);
  await expect(crumbs.getByTestId("crumb-period")).toHaveText(periodLabel);
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");

  // Kayıt ayrıntısı: son halka "Kayıt", sayfa adı listeye götürür.
  await page.goto(
    officePath(`/engagements/${engagement}/invoices/${invoice!.uuid}`),
  );
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Kayıt");
  await expect(crumbs.getByTestId("crumb-period")).toHaveText(periodLabel);
  await crumbs.getByRole("link", { name: "Faturalar" }).click();
  await expect(page).toHaveURL(
    new RegExp(`/engagements/${engagement}/invoices(\\?|$)`),
  );
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");
});
