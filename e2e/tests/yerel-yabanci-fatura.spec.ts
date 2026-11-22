import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Yabancı faturalar / KDV2 (#780 A1): ofis mükellef sayfasındaki sekmeden
 * yurt dışı yazılım lisansı faturası ekler (100 USD, kur 34,50, %15 stopaj,
 * brütleştirme); brüt, stopaj, KDV2 ve TL karşılıkları beklenen değerlerde.
 * Kuru girilmemiş fatura toplamda "kur eksik" sayılır, onaylanamaz. Onay ve
 * geri alma; onaylı fatura değişmez. Excel ucu xlsx döner. Ofis ayarı
 * kapalıyken sekme görünmez.
 *
 * Veri: her koşuda yeni mükellef. Ofis geneli ayar (bölüm anahtarı ve
 * lisans türü varsayılanı) testte değişir, sonunda eski hâline döner.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Yabanci Fatura ${run} Ltd. Sti.` };
const LICENCE = `Lisans Yazilim GmbH ${run}`;
const ADS = `Google Ireland Ltd. ${run}`;

type Item = {
  uuid: string;
  vendor: string;
  status: string;
  gross: string;
  stopaj: string;
  kdv2: string;
  amount_try: string | null;
  gross_try: string | null;
  stopaj_try: string | null;
  kdv2_try: string | null;
  missing_rate: boolean;
};
type List = {
  items: Item[];
  totals: { count: number; confirmed: number; missing_rate: number; gross_try: string; kdv2_try: string; stopaj_try: string };
};
type Kind = {
  service_kind: string;
  treatment: string;
  stopaj_rate: string;
  gross_up: boolean;
  vat_rate: string;
  enabled: boolean;
};
type Kinds = { enabled: boolean; items: Kind[] };

/** Bu ay (İstanbul) YYYY-MM. */
const period = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date()).slice(0, 7);

let office: Api;
let engagement = "";
let original: Kinds;
let base = "";

const kindInput = (k: Kind) => ({
  service_kind: k.service_kind,
  treatment: k.treatment,
  stopaj_rate: k.stopaj_rate,
  gross_up: k.gross_up,
  vat_rate: k.vat_rate,
  enabled: k.enabled,
});

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, office, taxpayer, "yabanci");
  base = `/v1/tenant/engagements/${engagement}/foreign-invoices`;
  original = await office.json<Kinds>(await office.get("/v1/tenant/foreign-service-kinds"));
  // Lisans türü: KDV2 + %15 stopaj, brütleştirme (hazır varsayılan); bölüm açık.
  const res = await office.put("/v1/tenant/foreign-service-kinds", {
    enabled: true,
    items: [
      {
        service_kind: "yazilim_lisans",
        treatment: "kdv2_stopaj",
        stopaj_rate: "15",
        gross_up: true,
        vat_rate: "20",
        enabled: true,
      },
    ],
  });
  expect(res.status(), await res.text()).toBe(200);
});

test.afterAll(async () => {
  if (office && original) {
    const lic = original.items.find((k) => k.service_kind === "yazilim_lisans");
    await office.put("/v1/tenant/foreign-service-kinds", {
      enabled: original.enabled,
      items: lic ? [kindInput(lic)] : [],
    });
  }
  await office?.dispose();
});

test("ofis lisans faturası ekler; brüt, stopaj, KDV2 ve TL karşılığı hesaplanır", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/foreign-invoices?period=${period}`));
  await expect(page.getByTestId("engagement-tabs").getByRole("link", { name: "Yabancı faturalar / KDV2" })).toBeVisible();
  await expect(page.getByText("Beyan Luca'da verilir", { exact: false })).toBeVisible();
  await expect(page.getByTestId("foreign-empty")).toBeVisible();

  await page.getByRole("button", { name: "Fatura ekle" }).click();
  const dialog = page.getByRole("dialog", { name: "Yabancı hizmet faturası ekle" });
  // İşlem türü seçilince ofis varsayılanları dolar.
  await dialog.getByLabel("İşlem türü").selectOption("reklam");
  await expect(dialog.getByLabel("Vergi")).toHaveValue("kdv2");
  await expect(dialog.getByLabel("Stopaj oranı (%)")).toHaveCount(0);
  await dialog.getByLabel("İşlem türü").selectOption("yazilim_lisans");
  await expect(dialog.getByLabel("Vergi")).toHaveValue("kdv2_stopaj");
  await expect(dialog.getByLabel("Stopaj oranı (%)")).toHaveValue("15");
  await expect(dialog.getByRole("checkbox", { name: /Brütleştirme/ })).toBeChecked();

  await dialog.getByLabel("Satıcı").fill(LICENCE);
  await dialog.getByLabel("Ülke (2 harf)").fill("DE");
  await dialog.getByLabel("Fatura no").fill(`LIC-${run}`);
  await dialog.getByLabel("Tutar").fill("100");
  await dialog.getByLabel("Döviz").selectOption("USD");
  await dialog.getByLabel("Kur (1 birim = ? TL)").fill("34,50");
  await dialog.getByRole("button", { name: "Ekle", exact: true }).click();
  await expect(dialog).toBeHidden();

  const row = page.getByTestId("foreign-invoice").filter({ hasText: LICENCE });
  await expect(row.getByTestId("foreign-gross")).toHaveText("$ 117,65");
  await expect(row.getByTestId("foreign-stopaj")).toHaveText("$ 17,65");
  await expect(row.getByTestId("foreign-kdv2")).toHaveText("$ 23,53");
  await expect(row.getByTestId("foreign-try")).toContainText("Matrah ₺ 4.058,93");
  await expect(row.getByTestId("foreign-try")).toContainText("KDV2 ₺ 811,79");
  await expect(row.getByTestId("foreign-try")).toContainText("Stopaj ₺ 608,84");
  await expect(page.getByTestId("foreign-total-gross")).toHaveText("₺ 4.058,93");
  await expect(page.getByTestId("foreign-total-kdv2")).toHaveText("₺ 811,79");
  await expect(page.getByTestId("foreign-total-stopaj")).toHaveText("₺ 608,84");

  const list = await office.json<List>(await office.get(`${base}?period=${period}`));
  const it = list.items.find((i) => i.vendor === LICENCE);
  expect(it).toMatchObject({
    status: "draft",
    gross: "117.65",
    stopaj: "17.65",
    kdv2: "23.53",
    amount_try: "3450.00",
    gross_try: "4058.93",
    stopaj_try: "608.84",
    kdv2_try: "811.79",
    missing_rate: false,
  });
});

test("kuru eksik fatura toplamda 'kur eksik' sayılır ve onaylanamaz", async ({ page }) => {
  const res = await office.post(base, {
    period,
    vendor: ADS,
    country: "ie",
    amount: "50.00",
    currency: "USD",
    service_kind: "reklam",
  });
  expect(res.status(), await res.text()).toBe(201);
  const ads = (await res.json()).data as Item;
  expect(ads).toMatchObject({ missing_rate: true, gross: "50.00", kdv2: "10.00", gross_try: null, kdv2_try: null });

  const list = await office.json<List>(await office.get(`${base}?period=${period}`));
  expect(list.totals).toMatchObject({ count: 2, missing_rate: 1, gross_try: "4058.93", kdv2_try: "811.79", stopaj_try: "608.84" });

  const refused = await office.post(`${base}/${ads.uuid}/confirm`);
  expect(refused.status()).toBe(400);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/foreign-invoices?period=${period}`));
  await expect(page.getByTestId("foreign-missing-rate")).toContainText("1 faturada kur eksik");
  const row = page.getByTestId("foreign-invoice").filter({ hasText: ADS });
  await expect(row).toHaveAttribute("data-missing-rate", "true");
  await expect(row.getByRole("button", { name: `${ADS} faturasını onayla` })).toBeDisabled();

  // Kur satırdan girilir; toplam güncellenir.
  await row.getByLabel(`${ADS} faturasının kuru`).fill("34");
  await row.getByRole("button", { name: "Kaydet" }).click();
  await expect(page.getByTestId("foreign-missing-rate")).toHaveCount(0);
  // 4.058,93 + 50 × 34 = 5.758,93; KDV2 811,79 + 340,00 = 1.151,79.
  await expect(page.getByTestId("foreign-total-gross")).toHaveText("₺ 5.758,93");
  await expect(page.getByTestId("foreign-total-kdv2")).toHaveText("₺ 1.151,79");
});

test("onayla ve geri al; onaylı fatura değişmez; Excel ve PDF ucu", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/foreign-invoices?period=${period}`));
  const row = page.getByTestId("foreign-invoice").filter({ hasText: LICENCE });
  await row.getByRole("button", { name: `${LICENCE} faturasını onayla` }).click();
  await expect(row).toHaveAttribute("data-status", "confirmed");
  await expect(row).toContainText("Onaylandı");

  const list = await office.json<List>(await office.get(`${base}?period=${period}`));
  const lic = list.items.find((i) => i.vendor === LICENCE)!;
  expect(lic.status).toBe("confirmed");
  const changed = await office.patch(`${base}/${lic.uuid}`, { amount: "200" });
  expect(changed.status()).toBe(409);
  const removed = await office.delete(`${base}/${lic.uuid}`);
  expect(removed.status()).toBe(409);

  await row.getByRole("button", { name: `${LICENCE} faturasının onayını geri al` }).click();
  await expect(row).toHaveAttribute("data-status", "draft");
  await expect(row).toContainText("Taslak");

  const xlsx = await office.get(`${base}/export?period=${period}&format=xlsx`);
  expect(xlsx.status()).toBe(200);
  expect(xlsx.headers()["content-type"]).toContain("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
  expect(xlsx.headers()["content-disposition"]).toContain(`yabanci-faturalar-${period}.xlsx`);
  expect((await xlsx.body()).subarray(0, 2).toString()).toBe("PK");
  const pdf = await office.get(`${base}/export?period=${period}&format=pdf`);
  expect(pdf.status()).toBe(200);
  expect(pdf.headers()["content-type"]).toContain("application/pdf");

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Excel indir" }).click(),
  ]);
  expect(download.suggestedFilename()).toBe(`yabanci-faturalar-${period}.xlsx`);
});

test("ofis ayarı kapalıyken sekme görünmez", async ({ page }) => {
  try {
    const off = await office.put("/v1/tenant/foreign-service-kinds", { enabled: false });
    expect(off.status(), await off.text()).toBe(200);
    const created = await office.post(base, { period, vendor: "Kapali", country: "US", amount: "1", service_kind: "bulut" });
    expect(created.status()).toBe(409);

    await uiLogin(page, DEMO.owner);
    // Ayar okunduktan sonra bakılır (sekme ayar gelene kadar zaten gizli).
    await Promise.all([
      page.waitForResponse((r) => r.url().includes("/foreign-service-kinds") && r.ok()),
      page.goto(officePath(`/engagements/${engagement}`)),
    ]);
    const tabs = page.getByTestId("engagement-tabs");
    await expect(tabs.getByRole("link").first()).toBeVisible();
    await expect(tabs.getByRole("link", { name: "Yabancı faturalar / KDV2" })).toHaveCount(0);
  } finally {
    const on = await office.put("/v1/tenant/foreign-service-kinds", { enabled: true });
    expect(on.status()).toBe(200);
  }
  await page.goto(officePath(`/engagements/${engagement}`));
  await expect(page.getByTestId("engagement-tabs").getByRole("link", { name: "Yabancı faturalar / KDV2" })).toBeVisible();
});
