import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Mükellef mizanı (#669 E): mükellef ekranının "Mizan" sekmesi. Luca'dan
 * yüklenen mizanlar dönem dönem seçilir, önceki dönemle karşılaştırılır,
 * ana hesap düzeyine indirilir; Excel ve PDF olarak indirilir. Mizan
 * sekmeden de yüklenir.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; iki küçük Luca mizanı
 * (Eylül API ile, Ekim sekmeden) bu mükellefe yüklenir. Başka dosyanın
 * kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Mizan ${run} A.Ş.` };

const mizan = (title: string, rows: string[]) => ({
  name: `mizan-${run}.csv`,
  mimeType: "text/csv",
  buffer: Buffer.from(
    [title, "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye", ...rows].join("\n") + "\n",
    "utf8",
  ),
});

const september = mizan("MİZAN 01.09.2026 - 30.09.2026", [
  "100;KASA;1.000,00;400,00;600,00;",
  "100.01;MERKEZ KASA;1.000,00;400,00;600,00;",
  "320;SATICILAR;0,00;600,00;;600,00",
  "320.01;TEDARİKÇİ A.Ş.;0,00;600,00;;600,00",
]);
const october = mizan("MİZAN 01.10.2026 - 31.10.2026", [
  "100;KASA;2.500,00;1.000,00;1.500,00;",
  "100.01;MERKEZ KASA;2.500,00;1.000,00;1.500,00;",
  "320;SATICILAR;100,00;1.600,00;;1.500,00",
  "320.01;TEDARİKÇİ A.Ş.;100,00;1.600,00;;1.500,00",
]);

test.describe("mükellef mizanı (#669 E)", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "mizan");
    const up = await api.upload(`${base()}/account-plan/imports`, september);
    expect(up.status(), await up.text()).toBe(201);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("sekmeden yüklenir, dönem seçilir, karşılaştırılır, Excel ve PDF iner", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/trial-balance`));
    await expect(page.getByRole("link", { name: "Mizan", exact: true })).toBeVisible();

    // Eylül tek mizan: karşılaştırma yok.
    const table = page.getByRole("table", { name: "Mizan" });
    await expect(table.getByRole("row", { name: /100\.01 MERKEZ KASA/ })).toContainText("600,00");

    // Ekim sekmeden yüklenir; en yeni mizan açılır ve Eylül'le karşılaştırılır.
    await page.getByTestId("trial-balance-file").setInputFiles(october);
    await expect(page.getByText("Karşılaştırılan: Eylül 2026")).toBeVisible();
    const kasa = table.getByRole("row", { name: /100\.01 MERKEZ KASA/ });
    await expect(kasa).toContainText("1.500,00");
    await expect(kasa).toContainText("900,00"); // değişim 1.500 − 600
    await expect(table.getByRole("row", { name: /320\.01 TEDARİKÇİ/ })).toContainText("1.500,00");

    // Ana hesaplar: alt hesaplar gizlenir.
    await page.getByLabel("Düzey").selectOption("1");
    await expect(table.getByRole("row", { name: /100\.01/ })).toHaveCount(0);
    await expect(table.getByRole("row", { name: /^100 KASA/ })).toBeVisible();

    // Dönem seçimi: Eylül'e dönülür.
    const list = await api.json<{ items: { uuid: string; period: string | null }[] }>(
      await api.get(`${base()}/account-plan/trial-balances`),
    );
    const sep = list.items.find((x) => x.period === "2026-09");
    expect(sep).toBeTruthy();
    await page.getByLabel("Dönem (yüklenen mizan)").selectOption(sep!.uuid);
    await expect(table.getByRole("row", { name: /^100 KASA/ })).toContainText("600,00");

    // Excel ve PDF.
    const [xlsx] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Excel" }).click()]);
    expect(xlsx.suggestedFilename()).toBe("mizan_2026-09.xlsx");
    const [pdf] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "PDF" }).click()]);
    expect(pdf.suggestedFilename()).toBe("mizan_2026-09.pdf");
  });

  test("dışa aktarım ucu: biçim denetlenir, dosya başlıkları doğru", async () => {
    const bad = await api.get(`${base()}/account-plan/trial-balances/latest/export?format=docx`);
    expect(bad.status()).toBe(400);
    const res = await api.get(`${base()}/account-plan/trial-balances/latest/export?format=xlsx&compare=none`);
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain("spreadsheetml");
    expect(res.headers()["content-disposition"]).toContain('filename="mizan_2026-10.xlsx"');
    const body = await res.body();
    expect(body.subarray(0, 2).toString()).toBe("PK");
  });
});
