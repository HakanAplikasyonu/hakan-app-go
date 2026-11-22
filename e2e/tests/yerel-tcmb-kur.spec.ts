import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * TCMB günlük döviz kurları (#780): kuru gelmeyen dövizli fatura ve yabancı
 * hizmet faturası, faturanın tarihinden önceki son TCMB bülteninin kurunu
 * otomatik alır ve "TCMB, gg.aa.yyyy" etiketiyle görünür. Ofis kuru elle
 * değiştirince etiket "Elle girildi" olur; otomatik kur onu bir daha ezmez.
 * Ofis kur türünü (döviz alış → döviz satış) değiştirince otomatik kur hemen
 * değişir.
 *
 * Veri: demo kur tablosu (son dört ayın USD / EUR / GBP iş günü kurları).
 * Her koşuda yeni mükellef; fatura tarihi 20 gün önce. Ofis kur ayarı testte
 * değişir, finally'de eski hâline döner.
 */
test.describe.configure({ mode: "serial" });

type Settings = { rate_kind: string; auto_fill: boolean };
type Inv = {
  uuid: string;
  invoice_number: string;
  currency: string;
  exchange_rate?: string;
  exchange_rate_source?: string;
  exchange_rate_date?: string;
};
type Foreign = {
  uuid: string;
  vendor: string;
  exchange_rate: string | null;
  exchange_rate_source: string | null;
  exchange_rate_date: string | null;
  missing_rate: boolean;
};

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kur Otomatik ${run} A.Ş.` };
const VENDOR = `Kur Testi Cloud Inc. ${run}`;

/** 20 gün önce (UTC gün), YYYY-MM-DD: demo kurları bunun öncesini kapsar. */
const docDate = new Date(Date.now() - 20 * 24 * 3600 * 1000).toISOString().slice(0, 10);
const trDate = (iso: string) => iso.split("-").reverse().join(".");

let api: Api;
let engagement = "";

test.beforeAll(async ({ browser }) => {
  test.setTimeout(180_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tcmbkur");
});

test.afterAll(async () => {
  await api?.dispose();
});

test("kursuz dövizli fatura TCMB kuruyla gelir; elle girilen kur ezilmez", async ({ page }) => {
  test.setTimeout(240_000);
  const number = `TKR${docDate.slice(0, 4)}${String(Date.now()).slice(-9)}`;
  const ubl = deriveUbl("atlas-06-doviz-usd.xml", taxpayer.vkn, taxpayer.title);
  const xml = ubl.buffer
    .toString("utf8")
    .replace(/<PricingExchangeRate\b[\s\S]*?<\/PricingExchangeRate>\s*/, "")
    .replace(/(<ID\b[^>]*>)ATL\d+(<\/ID>)/, `$1${number}$2`)
    .split("2026-10-01")
    .join(docDate);
  expect(xml).not.toContain("CalculationRate");
  await api.json(
    await api.upload(`/v1/tenant/engagements/${engagement}/ubl-uploads`, {
      ...ubl,
      name: `e2e-tcmb-kur-${number}.xml`,
      buffer: Buffer.from(xml, "utf8"),
    }),
  );
  await waitForUploads(api, engagement, 1);

  let invoice: Inv | undefined;
  await expect
    .poll(
      async () => {
        const list = await api.json<{ items: Inv[] }>(
          await api.get(`/v1/tenant/engagements/${engagement}/invoices?limit=100`),
        );
        invoice = list.items.find((i) => i.invoice_number === number);
        return !!invoice;
      },
      { timeout: 90_000 },
    )
    .toBe(true);
  const url = `/v1/tenant/engagements/${engagement}/invoices/${invoice!.uuid}`;
  const auto = await api.json<Inv>(await api.get(url));
  expect(auto.currency).toBe("USD");
  expect(auto.exchange_rate_source).toBe("tcmb");
  expect(Number(auto.exchange_rate)).toBeGreaterThan(0);
  expect(auto.exchange_rate_date! < docDate).toBe(true);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/documents?belge=${invoice!.uuid}`));
  const box = page.getByRole("region", { name: /^USD kuru: .* TL$/ });
  await expect(box).toBeVisible();
  await expect(box.getByTestId("fx-source")).toHaveText(`TCMB, ${trDate(auto.exchange_rate_date!)}`);

  // Elle değiştir: etiket "Elle girildi", kur 41,25.
  await box.getByRole("textbox", { name: "USD kuru (TL)" }).fill("41,25");
  await box.getByRole("button", { name: "Kuru kaydet" }).click();
  await expect(page.getByText("Kur kaydedildi; KDV ve TL tutarı güncellendi.")).toBeVisible();
  await expect(page.getByTestId("fx-source")).toContainText("Elle girildi");
  const manual = await api.json<Inv>(await api.get(url));
  expect(manual.exchange_rate_source).toBe("manual");
  expect(Number(manual.exchange_rate)).toBe(41.25);

  // Kur ayarını yeniden kaydetmek otomatik kurları yeniler; elle girilen kalır.
  const settings = await api.json<Settings>(await api.get("/v1/tenant/exchange-rate-settings"));
  await api.json(await api.put("/v1/tenant/exchange-rate-settings", settings));
  const after = await api.json<Inv>(await api.get(url));
  expect(after.exchange_rate_source).toBe("manual");
  expect(Number(after.exchange_rate)).toBe(41.25);
  // Elle girilen kur ikinci kez girilemez.
  const again = await api.put(`${url}/exchange-rate`, { rate: "40" });
  expect(again.status()).toBe(409);
});

test("kur türü döviz satışa geçince yabancı faturanın otomatik kuru değişir", async ({ page }) => {
  test.setTimeout(180_000);
  const base = `/v1/tenant/engagements/${engagement}/foreign-invoices`;
  const period = docDate.slice(0, 7);
  const created = await api.post(base, {
    period,
    vendor: VENDOR,
    country: "us",
    invoice_date: docDate,
    amount: "100.00",
    currency: "USD",
    service_kind: "bulut",
  });
  expect(created.status(), await created.text()).toBe(201);
  const item = (await created.json()).data as Foreign;
  expect(item.exchange_rate_source).toBe("tcmb");
  expect(item.missing_rate).toBe(false);
  const buying = Number(item.exchange_rate);
  expect(buying).toBeGreaterThan(0);

  const original = await api.json<Settings>(await api.get("/v1/tenant/exchange-rate-settings"));
  try {
    if (original.rate_kind !== "forex_buying" || !original.auto_fill) {
      await api.json(
        await api.put("/v1/tenant/exchange-rate-settings", { rate_kind: "forex_buying", auto_fill: true }),
      );
    }
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/settings"));
    const kind = page.getByLabel("Hangi kur kullanılsın?");
    await expect(kind).toBeEnabled();
    await kind.selectOption("forex_selling");
    await expect(page.getByText("Kur ayarı kaydedildi.")).toBeVisible();

    const list = await api.json<{ items: Foreign[] }>(await api.get(`${base}?period=${period}`));
    const now = list.items.find((i) => i.uuid === item.uuid)!;
    expect(now.exchange_rate_source).toBe("tcmb");
    expect(now.exchange_rate_date).toBe(item.exchange_rate_date);
    expect(Number(now.exchange_rate)).toBeGreaterThan(buying);

    await page.goto(officePath(`/engagements/${engagement}/foreign-invoices?period=${period}`));
    const row = page.getByTestId("foreign-invoice").filter({ hasText: VENDOR });
    await expect(row.getByTestId("foreign-rate-source")).toHaveText(`TCMB, ${trDate(item.exchange_rate_date!)}`);
  } finally {
    await api.json(await api.put("/v1/tenant/exchange-rate-settings", original));
  }
});
