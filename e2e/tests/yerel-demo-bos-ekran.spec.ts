import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Demo turunda boş / yanıltıcı ekran kalmasın (#780):
 * - Ofis > Faturalar adreste mükellef yokken faturası olan bir mükellefle
 *   açılır, "Bu filtreye uyan fatura yok" ile açılmaz.
 * - İnceleme toplu onay eşiği açılışta %80; öneri varken "öneri yok"
 *   cümlesi kuyruktaki sayıyı da söyler.
 * - Kenar çubuğunda "0 ajan çevrimiçi" yazmaz; ajan yoksa sakin dille
 *   "Masaüstü ajanı bağlı değil".
 * - Atlas Gıda'da Excel aktarımı, dilekçe ve dışa aktarılmış fiş var.
 * Veri: demo ofisi; yalnız okur.
 */
test("demo ekranları boş açılmaz (#780)", async ({ page }) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);

  await page.goto(officePath("/invoices"));
  await expect(page).toHaveURL(/taxpayer=/, { timeout: 30_000 });
  await expect(page.getByText("Bu filtreye uyan fatura yok.")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /faturasını çekmecede aç$/ }).first()).toBeVisible();

  // Kenar çubuğu: "0 ajan çevrimiçi" yok.
  const nav = page.locator("[data-sidebar='sidebar']").first();
  await expect(nav).not.toContainText("0 ajan çevrimiçi");
  await expect(nav).toContainText(/ajan çevrimiçi|Masaüstü ajanı bağlı değil/);

  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${atlas}`;
  for (const path of ["excel-imports", "petitions", "journal-exports"]) {
    const body = await api.json<{ items: unknown[] }>(await api.get(`${base}/${path}`));
    expect(body.items.length, path).toBeGreaterThan(0);
  }
  await api.dispose();

  // İnceleme: toplu onay eşiği %80 ile açılır.
  await page.goto(officePath(`/engagements/${atlas}/review`));
  const slider = page.getByRole("slider", { name: "Güveni en az" });
  await expect(slider).toBeVisible({ timeout: 30_000 });
  await expect(slider).toHaveAttribute("aria-valuenow", "80");
  await expect(page.getByText(/Bu sayfada güveni %\d+ ve üzerinde öneri yok/)).toHaveCount(0);
});
