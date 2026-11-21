import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Mükellef panelinde anlık KDV durumu ve satış raporu (#780). Mükellef
 * kullanıcısı ana sayfasında bu ayın KDV kartını ve satış raporunu görür;
 * ofis Ayarlar → Mükellef paneli anahtarını kapatınca bölüm kaybolur ve uç
 * nokta 403 döner. Sonda ayar eski haline (açık) döner.
 */
type Settings = { name: string; settings: Record<string, unknown> };
const KEY = "taxpayer_vat_sales_panel";

async function setPanel(api: Api, on: boolean) {
  const cur = await api.json<Settings>(await api.get("/v1/tenant/settings"));
  const res = await api.patch("/v1/tenant/settings", { settings: { ...cur.settings, [KEY]: on } });
  expect(res.status(), await res.text()).toBeLessThan(300);
}

test("mükellef panelinde KDV kartı ve satış raporu; ofis ayarıyla kapanır", async ({ browser, page }) => {
  test.setTimeout(120_000);
  const api = await Api.login(DEMO.owner);
  const taxpayerApi = await Api.login(DEMO.taxpayerUser);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  const tp = await api.json<{ uuid: string }>(await api.get(`/v1/tenant/engagements/${atlas}/taxpayer`));
  const office = await (await browser.newContext()).newPage();
  try {
    await setPanel(api, true);

    // Açık: uç nokta yanıt verir, ana sayfada kart ve rapor görünür.
    const ok = await taxpayerApi.get("/v1/taxpayer/vat-sales", { "X-Taxpayer": tp.uuid });
    expect(ok.status(), await ok.text()).toBe(200);
    const body = await taxpayerApi.json<{ sales: { trend: unknown[] }; calendar: { due_date: string } }>(ok);
    expect(body.sales.trend).toHaveLength(6);
    expect(body.calendar.due_date).toMatch(/^\d{4}-\d{2}-\d{2}$/);

    await uiLogin(page, DEMO.taxpayerUser);
    await page.goto(`/m/${tp.uuid}`);
    await expect(page.getByRole("heading", { name: "Bu ayın KDV durumu" })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText("Satış KDV'si").first()).toBeVisible();
    await expect(page.getByText(/Son ödeme günü/).first()).toBeVisible();
    await expect(page.getByRole("heading", { name: "Satış raporu" })).toBeVisible();
    await expect(page.getByText("En çok satış yaptığın alıcılar")).toBeVisible();
    await expect(page.getByText("Son 6 ayın satışı")).toBeVisible();

    // Ofis ayardan kapatır.
    await uiLogin(office, DEMO.owner);
    await office.goto(officePath("/settings"));
    const toggle = office.getByRole("switch", { name: "Mükellef panelinde KDV ve satış raporu göster" });
    await expect(toggle).toBeChecked();
    await toggle.click();
    await expect(toggle).not.toBeChecked();
    await expect(office.getByText("Ayarlar kaydedildi.").first()).toBeVisible();
    await expect
      .poll(async () => (await api.json<Settings>(await api.get("/v1/tenant/settings"))).settings[KEY])
      .toBe(false);

    // Kapalı: uç nokta 403, bölüm görünmez.
    const off = await taxpayerApi.get("/v1/taxpayer/vat-sales", { "X-Taxpayer": tp.uuid });
    expect(off.status()).toBe(403);
    expect((await off.json()).error.code).toBe("TAXPAYER_VAT_PANEL_DISABLED");
    await page.reload();
    await expect(page.getByRole("heading", { name: "Müşavirinin istedikleri" })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByRole("heading", { name: "Bu ayın KDV durumu" })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Satış raporu" })).toHaveCount(0);
  } finally {
    // Eski haline: açık.
    await setPanel(api, true);
    await office.context().close();
    await taxpayerApi.dispose();
    await api.dispose();
  }
});
