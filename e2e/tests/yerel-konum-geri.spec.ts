import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Konum çubuğunda işlem seviyesi ve geri dönüş yolu (#710): ofis →
 * mükellef → sayfa → işlem. Sekmenin listesindeyken sayfa adı son
 * kırıntıdır; bir kayda inince (alt yol ya da `?kayit=`) sayfa adı listeye
 * dönüş bağlantısı olur (dönem korunur), son kırıntı "Kayıt"tır. Yalnızca
 * okur ve gezinir.
 */
test("kayıttan sayfa listesine konum çubuğuyla dönülür; dönem korunur (#710)", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  const inv = await api.json<{ items: { uuid: string }[] }>(
    await api.get(`/v1/tenant/engagements/${atlas}/invoices?limit=1`),
  );
  await api.dispose();
  test.skip(inv.items.length === 0, "demo verisinde Atlas faturası yok");

  await uiLogin(page, DEMO.owner);
  const crumbs = page.getByRole("navigation", { name: "Konum" });

  // Liste: sayfa adı son kırıntı, bağlantı değil.
  await page.goto(officePath(`/engagements/${atlas}/invoices`));
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");
  await expect(crumbs.getByRole("link", { name: "Faturalar listesine dön" })).toHaveCount(0);

  // Fatura ayrıntısı: son kırıntı "Kayıt", sayfa adı listeye döner.
  await page.goto(officePath(`/engagements/${atlas}/invoices/${inv.items[0].uuid}`));
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Kayıt");
  await crumbs.getByRole("link", { name: "Faturalar listesine dön" }).click();
  await expect(page).toHaveURL(new RegExp(`/engagements/${atlas}/invoices$`));
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Faturalar");

  // Odaklı kayıt (?kayit=) dönemle açıldıysa dönüş bağlantısı dönemi taşır.
  await page.goto(officePath(`/engagements/${atlas}/bank?period=2026-10&kayit=bank_transaction:00000000-0000-0000-0000-000000000000`));
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Kayıt");
  const back = crumbs.getByRole("link", { name: "Banka listesine dön" });
  await expect(back).toHaveAttribute("href", /\/bank\?period=2026-10$/);
  await back.click();
  await expect(page).toHaveURL(/\/bank\?period=2026-10$/);
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Banka");
});
