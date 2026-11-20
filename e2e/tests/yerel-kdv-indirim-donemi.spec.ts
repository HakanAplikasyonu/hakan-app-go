import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * KDV indirim dönemi (#640): alış faturasının KDV'si, belge tarihi
 * değişmeden sonraki bir ayın beyanına alınır. Fatura çekmecesinde
 * "İndirim dönemini değiştir" ile ay seçilir, "şu aya alındı" notu düşer;
 * kutudaki "Geri al" belge ayına döndürür. Fatura, KDV'sinin alındığı ayın
 * listesinde "… faturası; KDV'si bu ayda" notuyla görünür; belge ayında
 * "KDV'si … beyanına alındı" izi kalır (ofis görüşmesi, #640).
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

    // Kutudaki "Geri al": önceki hâline (belge ayına) döner.
    await box.getByRole("button", { name: "Geri al" }).click();
    await expect(box.getByTestId("vat-deduction-note")).toHaveCount(0);
    await expect.poll(async () => (await detail()).vat_deduction_period).toBeNull();
    d = await detail();
    expect(d.period).toBe("2026-10");
  });

  test("fatura indirim ayının listesinde notla görünür, belge ayında iz kalır", async ({ page }) => {
    const put = await api.put(`${base()}/invoices/${invoice}/vat-deduction-period`, { period: "2026-12" });
    expect(put.status(), await put.text()).toBe(200);
    const listed = async (period: string) =>
      (await api.json<{ items: { uuid: string }[] }>(await api.get(`${base()}/invoices?period=${period}&limit=50`))).items.map(
        (i) => i.uuid,
      );
    // API: hem belge ayında hem indirim ayında listelenir, arada değil.
    expect(await listed("2026-10")).toContain(invoice);
    expect(await listed("2026-12")).toContain(invoice);
    expect(await listed("2026-11")).not.toContain(invoice);

    await uiLogin(page, DEMO.owner);
    // İndirim ayı: "Ekim 2026 faturası; KDV'si bu ayda".
    await page.goto(officePath(`/engagements/${engagement}/invoices?period=2026-12`));
    const note = page.getByTestId("vat-moved-note");
    await expect(note).toHaveCount(1);
    await expect(note).toHaveText("Ekim 2026 faturası; KDV'si bu ayda");
    // Belge ayı: "KDV'si Aralık 2026 beyanına alındı".
    await page.goto(officePath(`/engagements/${engagement}/invoices?period=2026-10`));
    await expect(note).toHaveCount(1);
    await expect(note).toHaveText("KDV'si Aralık 2026 beyanına alındı");

    expect((await api.delete(`${base()}/invoices/${invoice}/vat-deduction-period`)).status()).toBe(200);
    expect(await listed("2026-12")).not.toContain(invoice);
  });

  test("izleyen yılın Aralık'ından sonrası ve geri ay reddedilir", async () => {
    const put = (period: string) => api.put(`${base()}/invoices/${invoice}/vat-deduction-period`, { period });
    expect((await put("2028-01")).status()).toBe(422);
    expect((await put("2026-09")).status()).toBe(422);
    expect((await put("2027-12")).status()).toBe(200);
    expect((await api.delete(`${base()}/invoices/${invoice}/vat-deduction-period`)).status()).toBe(200);
  });
});
