import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Yabancı faturadan fiş taslağı (#780 KDV2 B): onaylı lisans faturası
 * (100 USD, kur 34,50, %15 stopaj, brütleştirme) listedeki "Fiş oluştur" ile
 * fişe döner. Satırlar: gider 4.058,93 B, 191 811,79 B, 360 KDV2 811,79 A,
 * 360 stopaj 608,84 A, 320 3.450,00 A; brüt döviz tutarı yuvarlandığı için
 * 0,09 TL kuruş farkı ayrı satırda (679 A), fiş dengeli. İkinci kez fiş
 * 409; fiş varken onay geri alınamaz; fiş kaldırılınca geri alınır. Dönem
 * için toplu fiş yalnız fişi olmayan onaylıları alır.
 *
 * Veri: her koşuda yeni (bilanço) mükellef. Ofis ayarı değişmez; hesap
 * kodları ofis ayarından okunur.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Yabanci Fis ${run} Ltd. Sti.` };
const LICENCE = `Fis Lisans GmbH ${run}`;
const ADS = `Fis Google Ireland ${run}`;

type Line = { account: string; description: string; debit: string; credit: string };
type Journal = {
  invoice_uuid: string;
  journal: { uuid: string; entry_no: number; period: string; status: string };
  lines: Line[];
  debit: string;
  credit: string;
  rounding: string;
};
type Item = { uuid: string; vendor: string; status: string; journal: Journal["journal"] | null };
type List = { items: Item[]; totals: { journaled: number; journal_pending: number } };
type Settings = {
  enabled: boolean;
  items: { service_kind: string; expense_account: string }[];
  accounts: {
    kdv2_account: string;
    stopaj_account: string;
    deductible_vat_account: string;
    counter_account: string;
    rounding_income_account: string;
    rounding_expense_account: string;
  };
};

const period = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date()).slice(0, 7);

let office: Api;
let engagement = "";
let base = "";
let settings: Settings;
let licence = "";
let ads = "";

async function addConfirmed(body: Record<string, unknown>): Promise<string> {
  const res = await office.post(base, body);
  expect(res.status(), await res.text()).toBe(201);
  const { uuid } = await office.json<{ uuid: string }>(res);
  const ok = await office.post(`${base}/${uuid}/confirm`);
  expect(ok.status(), await ok.text()).toBe(200);
  return uuid;
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, office, taxpayer, "yabancifis");
  base = `/v1/tenant/engagements/${engagement}/foreign-invoices`;
  settings = await office.json<Settings>(await office.get("/v1/tenant/foreign-service-kinds"));
  test.skip(!settings.enabled, "Yabancı faturalar bölümü ofiste kapalı");
  licence = await addConfirmed({
    period,
    vendor: LICENCE,
    country: "DE",
    invoice_no: `FIS-${run}`,
    amount: "100",
    currency: "USD",
    exchange_rate: "34.50",
    service_kind: "yazilim_lisans",
    treatment: "kdv2_stopaj",
    stopaj_rate: "15",
    gross_up: true,
    vat_rate: "20",
  });
  ads = await addConfirmed({
    period,
    vendor: ADS,
    country: "IE",
    invoice_no: `ADS-${run}`,
    amount: "250",
    currency: "USD",
    exchange_rate: "34.25",
    service_kind: "reklam",
    treatment: "kdv2",
    vat_rate: "20",
  });
});

test.afterAll(async () => {
  await office?.dispose();
});

test("onaylı faturadan fiş: satırlar, tutarlar ve kuruş farkı", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/foreign-invoices?period=${period}`));
  await expect(page.getByTestId("foreign-journal-summary")).toContainText("2 onaylı faturanın fişi henüz yok");
  const row = page.getByTestId("foreign-invoice").filter({ hasText: LICENCE });
  await row.getByRole("button", { name: `${LICENCE} faturasının fişini oluştur` }).click();
  await expect(row.getByTestId("foreign-journal")).toContainText("Fiş taslak");
  await expect(row.getByRole("link", { name: /^Fiş \d+$/ })).toBeVisible();
  // Fiş varken satırda onayı geri al düğmesi yok, neden yazıyor.
  await expect(row.getByText("Onayı geri almak için önce fişi kaldırın.")).toBeVisible();

  const list = await office.json<List>(await office.get(`${base}?period=${period}`));
  const it = list.items.find((i) => i.uuid === licence);
  expect(it?.journal?.status).toBe("draft");
  expect(list.totals.journaled).toBe(1);
  expect(list.totals.journal_pending).toBe(1);

  // Satırlar: ikinci istek 409 döner; satırları fiş ekranından değil,
  // kaldırıp yeniden oluşturarak yanıttan okuruz.
  const del = await office.delete(`${base}/${licence}/journal`);
  expect(del.status(), await del.text()).toBe(204);
  const res = await office.post(`${base}/${licence}/journal`);
  expect(res.status(), await res.text()).toBe(201);
  const j = await office.json<Journal>(res);
  const a = settings.accounts;
  const expense = settings.items.find((k) => k.service_kind === "yazilim_lisans")?.expense_account ?? "770";
  expect(j.lines.map((l) => [l.account, l.debit, l.credit])).toEqual([
    [expense, "4058.93", "0.00"],
    [a.deductible_vat_account, "811.79", "0.00"],
    [a.kdv2_account, "0.00", "811.79"],
    [a.stopaj_account, "0.00", "608.84"],
    [a.counter_account, "0.00", "3450.00"],
    [a.rounding_income_account, "0.00", "0.09"],
  ]);
  expect(j.debit).toBe("4870.72");
  expect(j.credit).toBe("4870.72");
  expect(j.rounding).toBe("0.09");
  expect(j.journal.status).toBe("draft");
});

test("ikinci kez fiş 409; fiş varken onay geri alınamaz", async () => {
  const again = await office.post(`${base}/${licence}/journal`);
  expect(again.status()).toBe(409);
  expect((await again.json()).error.code).toBe("FOREIGN_INVOICE_JOURNAL_EXISTS");

  const undo = await office.post(`${base}/${licence}/unconfirm`);
  expect(undo.status()).toBe(409);
  expect(await undo.text()).toContain("önce fişi kaldırın");
  const patch = await office.patch(`${base}/${licence}`, { note: "değişmemeli" });
  expect(patch.status()).toBe(409);
});

test("dönem için toplu fiş yalnız fişi olmayan onaylıyı alır; fiş kaldırılınca onay geri alınır", async () => {
  const res = await office.post(`${base}/journal?period=${period}`);
  expect(res.status(), await res.text()).toBe(200);
  const out = await office.json<{ created: Journal[]; skipped: number; failed: unknown[] }>(res);
  expect(out.created.map((c) => c.invoice_uuid)).toEqual([ads]);
  expect(out.skipped).toBe(1);
  expect(out.failed).toEqual([]);
  const adLines = out.created[0].lines;
  expect(adLines.map((l) => [l.debit, l.credit])).toEqual([
    ["8562.50", "0.00"],
    ["1712.50", "0.00"],
    ["0.00", "1712.50"],
    ["0.00", "8562.50"],
  ]);

  const again = await office.post(`${base}/journal?period=${period}`);
  const second = await office.json<{ created: Journal[]; skipped: number }>(again);
  expect(second.created).toEqual([]);
  expect(second.skipped).toBe(2);

  const del = await office.delete(`${base}/${licence}/journal`);
  expect(del.status(), await del.text()).toBe(204);
  const undo = await office.post(`${base}/${licence}/unconfirm`);
  expect(undo.status(), await undo.text()).toBe(200);
  const none = await office.delete(`${base}/${licence}/journal`);
  expect(none.status()).toBe(404);
});
