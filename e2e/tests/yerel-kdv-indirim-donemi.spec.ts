import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * KDV indirim dönemi (#640): alış faturasının KDV'si, belge tarihi
 * değişmeden sonraki bir ayın beyanına alınır. Fatura çekmecesinde
 * "İndirim dönemini değiştir" ile ay seçilir, "şu aya alındı" notu düşer;
 * bildirimdeki "Geri al" belge ayına döndürür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Atlas'ın gelen faturası
 * (atlas-05, Ekim 2026) bu mükellefe türetilir. Başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Dönem ${run} A.Ş.` };

type Detail = { uuid: string; period: string; direction: string; vat_deduction_period: string | null };

test.describe("KDV indirim dönemi (#640)", () => {
  let engagement = "";
  let invoice = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const detail = async () => api.json<Detail>(await api.get(`${base()}/invoices/${invoice}`));

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kdvdonem");
    const up = await api.upload(`${base()}/ubl-uploads`, deriveUbl("atlas-05-iade-alis.xml", taxpayer.vkn, taxpayer.title));
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    invoice = (await api.json<{ items: { uuid: string }[] }>(await api.get(`${base()}/invoices?limit=5`))).items[0].uuid;
    const d = await detail();
    expect(d.direction).toBe("incoming");
    expect(d.period).toBe("2026-10");
    expect(d.vat_deduction_period).toBeNull();
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("çekmeceden KDV sonraki aya alınır, Geri al belge ayına döndürür", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/documents?belge=${invoice}`));

    const box = page.getByRole("region", { name: "KDV indirim dönemi" });
    await expect(box).toBeVisible();
    await expect(box).toContainText("belge ayı Ekim 2026 beyanında");
    await box.getByRole("button", { name: "İndirim dönemini değiştir" }).click();
    const month = box.getByLabel("KDV indirim ayı");
    await expect(month).toHaveAttribute("min", "2026-11");
    await expect(month).toHaveAttribute("max", "2027-12");
    await month.fill("2026-12");
    await box.getByRole("button", { name: "Kaydet" }).click();

    await expect(box.getByTestId("vat-deduction-note")).toContainText("KDV'si Aralık 2026 beyanına alındı");
    await expect(box.getByTestId("vat-deduction-note")).toContainText("Ekim 2026 içinde kalır");
    let d = await detail();
    expect(d.vat_deduction_period).toBe("2026-12");
    expect(d.period).toBe("2026-10");

    // Bildirimdeki "Geri al": belge ayına döner.
    await page.locator("[data-sonner-toast]").getByRole("button", { name: "Geri al" }).click();
    await expect(box.getByTestId("vat-deduction-note")).toHaveCount(0);
    await expect.poll(async () => (await detail()).vat_deduction_period).toBeNull();
    d = await detail();
    expect(d.period).toBe("2026-10");
  });

  test("izleyen yılın Aralık'ından sonrası ve geri ay reddedilir", async () => {
    const put = (period: string) => api.put(`${base()}/invoices/${invoice}/vat-deduction-period`, { period });
    expect((await put("2028-01")).status()).toBe(422);
    expect((await put("2026-09")).status()).toBe(422);
    expect((await put("2027-12")).status()).toBe(200);
    expect((await api.delete(`${base()}/invoices/${invoice}/vat-deduction-period`)).status()).toBe(200);
  });
});
