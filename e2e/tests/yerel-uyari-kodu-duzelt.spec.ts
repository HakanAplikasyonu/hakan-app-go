import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Uyarı İncele panelinden kalemin hesap kodunu düzeltme (#570): "Demirbaş
 * olabilecek kalem" uyarısında kalemin bekleyen hesap kodu önerisi panelde
 * düzeltilir (inceleme kuyruğunun düzeltme ucu), uyarı yeni kodla
 * "Çözüldü" olarak kapanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın gelen faturası
 * (atlas-05, Ekim 2026) bu mükellefe türetilir; kalem adı "Dizüstü
 * bilgisayar" yapılır (demirbaş anahtar kelimesi). Hesap kodu önerileri ve
 * müşavir kontrolleri API ile çalıştırılır. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kod Düzelt ${run} A.Ş.` };
const PERIOD = "2026-10";
const ITEM = "Dizüstü bilgisayar";
const CODE = "255.01";

type Warning = { uuid: string; status: string; entity_uuid: string | null; resolution_note: string | null; params: Record<string, unknown> };
type Suggestion = {
  uuid: string;
  status: string;
  target_type: string;
  target_field: string;
  applied_value: unknown;
  provenance: { inputs: Record<string, unknown> };
};

test.describe("uyarı panelinden kodu düzelt (#570)", () => {
  let engagement = "";
  let invoice = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const fixedAsset = async (status: string) =>
    (
      await api.json<{ items: Warning[] }>(
        await api.get(`${base()}/warnings?status=${status}&code=FIXED_ASSET_CANDIDATE&period=${PERIOD}&limit=20`),
      )
    ).items.filter((w) => w.params?.name === ITEM);
  const lineSuggestions = async (status: string) =>
    (
      await api.json<{ items: Suggestion[] }>(
        await api.get(`${base()}/suggestions?status=${status}&kind=account_code&period=${PERIOD}&limit=100`),
      )
    ).items.filter(
      (s) =>
        s.target_type === "invoice_item" &&
        s.target_field === "account_code" &&
        s.provenance.inputs.invoice_uuid === invoice &&
        s.provenance.inputs.item_name === ITEM,
    );

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kod");

    const ubl = deriveUbl("atlas-05-iade-alis.xml", taxpayer.vkn, taxpayer.title);
    const xml = ubl.buffer.toString("utf8");
    expect(xml).toContain("Ofis Koltuğu");
    ubl.buffer = Buffer.from(xml.split("Ofis Koltuğu").join(ITEM), "utf8");
    const up = await api.upload(`${base()}/ubl-uploads`, ubl);
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    invoice = (await api.json<{ items: { uuid: string }[] }>(await api.get(`${base()}/invoices?limit=5`))).items[0].uuid;

    // Kalemin hesap kodu önerisi kuyrukta bekler (kural güveni eşiğin altında).
    await api.json(await api.post(`${base()}/invoices/${invoice}/account-suggestions`, {}));
    await expect.poll(async () => (await lineSuggestions("pending")).length).toBe(1);

    // Müşavir kontrolleri: gelen faturada "bilgisayar" kalemi demirbaş adayı.
    await api.json(await api.post(`${base()}/controls/run`, { period: PERIOD }));
    await expect.poll(async () => (await fixedAsset("open")).length).toBe(1);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Kodu düzelt: öneri düzeltilir, uyarı yeni kodla çözülür", async ({ page }) => {
    const warning = (await fixedAsset("open"))[0];

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
    await page.getByRole("row").filter({ hasText: "Demirbaş olabilecek kalem" }).getByText("Demirbaş olabilecek kalem").first().click();

    const sheet = page.getByTestId("warning-sheet");
    await expect(sheet).toBeVisible();
    const fix = sheet.getByTestId("warning-code-fix");
    await expect(fix).toBeVisible();
    await fix.getByLabel("Doğru hesap kodu").fill(CODE);
    await fix.getByRole("button", { name: "Kodu düzelt" }).click();

    // Veritabanı: öneri yeni kodla düzeltildi, uyarı gerekçesiyle çözüldü.
    await expect.poll(async () => (await fixedAsset("open")).length).toBe(0);
    const corrected = await lineSuggestions("corrected");
    expect(corrected).toHaveLength(1);
    expect(JSON.stringify(corrected[0].applied_value)).toContain(CODE);
    const resolved = (await fixedAsset("resolved")).find((w) => w.uuid === warning.uuid);
    expect(resolved?.resolution_note).toBe(`Kalemin hesap kodu ${CODE} olarak düzeltildi.`);
  });
});
