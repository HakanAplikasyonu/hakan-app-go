import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Düzeltme beyannamesi (#575). Ofis kararı: verilmiş dönemde satış
 * değişirse düzeltme beyannamesi verilir.
 *
 *   Ekim 2026 KDV1 satışsız hazırla + verildi (API) → sonradan satış
 *   faturası (4.500,00 + %20 KDV) yüklenir → verilmiş beyanda "düzeltme
 *   gerekebilir", verilen / güncel farkı (matrah 0 → 4.500,00) → ekranda
 *   "Düzeltme beyannamesi başlat" → 2. sürüm taslak, önceki sürüm arşivde
 *   (verildi günü, tahakkuk no), öncekiyle fark → hazırla + verildi →
 *   verilmiş 2. sürüm düzeltme istemez, farkı önceki sürümle gösterir.
 *   Taslağa düzeltme açılmaz (409).
 *
 * Veri: demo ofisine her koşuda yeni mükellef; fatura Atlas UBL'inden
 * türetilir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Düzeltme ${run} A.Ş.` };
const PERIOD = "2026-10";
const ACCRUAL_NO = `DZT-${run}`;

type Figure = { key: string; previous: string; current: string; difference: string };
type Version = { version: number; filed_on: string; accrual_number: string | null };
type Preview = {
  status: string;
  open_warnings: string[];
  amendment: {
    version: number;
    needed: boolean;
    previous: Version | null;
    differences: Figure[];
    versions: Version[];
  } | null;
};

let api: Api;
let engagement = "";
let base = "";
const one = () => `${base}/tax-returns/kdv1/${PERIOD}`;
const preview = async () => api.json<Preview>(await api.get(one()));
const fig = (p: Preview, key: string) => p.amendment?.differences.find((d) => d.key === key);

function today() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date());
}

async function prepareAndFile(accrual: string) {
  const draft = await preview();
  const prepared = await api.json<Preview>(
    await api.post(`${one()}/prepare`, { acknowledged: draft.open_warnings ?? [] }),
  );
  expect(prepared.status).toBe("prepared");
  return api.json<Preview>(await api.post(`${one()}/file`, { filed_on: today(), accrual_number: accrual }));
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "duzeltme");
  base = `/v1/tenant/engagements/${engagement}`;
  expect((await preview()).amendment).toBeNull();

  const filed = await prepareAndFile(ACCRUAL_NO);
  expect(filed.status).toBe("filed");
  expect(filed.amendment).toMatchObject({ version: 1, needed: false, versions: [] });

  // Taslak / verilmemiş beyana düzeltme açılmaz: MUHSGK hiç verilmedi.
  const notFiled = await api.post(`${base}/tax-returns/muhsgk/${PERIOD}/amend`, {});
  expect(notFiled.status()).toBe(409);
  expect(await notFiled.text()).toContain("TAX_RETURN_NOT_FILED");

  // Verildikten sonra satış faturası gelir.
  const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);
});

test.afterAll(async () => {
  await api?.dispose();
});

test("verilmiş beyanda satış değişince düzeltme gerekebilir", async () => {
  const p = await preview();
  expect(p.status).toBe("filed");
  expect(p.amendment).toMatchObject({ version: 1, needed: true, previous: null });
  expect(fig(p, "base_total")).toMatchObject({ previous: "0.00", current: "4500.00", difference: "4500.00" });
  expect(fig(p, "calculated_vat")).toMatchObject({ previous: "0.00", current: "900.00", difference: "900.00" });
});

test("ekranda düzeltme beyannamesi başlatılır: 2. sürüm, öncekiyle fark", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns?period=${PERIOD}`));
  const card = page.getByRole("region", { name: "Düzeltme beyannamesi" });
  await expect(card).toContainText("Düzeltme beyannamesi gerekebilir");
  await expect(card.getByRole("table", { name: "Öncekiyle fark" })).toContainText("4.500,00");
  await card.getByRole("button", { name: "Düzeltme beyannamesi başlat" }).click();
  await expect(page.getByText("Düzeltme beyannamesi taslağı açıldı")).toBeVisible();

  const v2 = page.getByRole("region", { name: "Düzeltme beyannamesi" });
  await expect(v2).toContainText("Düzeltme beyannamesi · 2. sürüm");
  await expect(v2).toContainText("Önceki: 1. sürüm");
  await expect(v2).toContainText(ACCRUAL_NO);
  await expect(v2.getByRole("button", { name: "Düzeltme beyannamesi başlat" })).toHaveCount(0);

  const p = await preview();
  expect(p.status).toBe("draft");
  expect(p.amendment).toMatchObject({ version: 2, needed: false, previous: { version: 1, accrual_number: ACCRUAL_NO } });
  expect(p.amendment?.versions).toHaveLength(1);
  expect(fig(p, "base_total")).toMatchObject({ previous: "0.00", current: "4500.00", difference: "4500.00" });
});

test("düzeltme beyannamesi verilir; öncekiyle farkı kalır", async () => {
  const filed = await prepareAndFile(`${ACCRUAL_NO}-2`);
  expect(filed.status).toBe("filed");
  expect(filed.amendment).toMatchObject({ version: 2, needed: false, previous: { version: 1 } });
  expect(fig(filed, "calculated_vat")).toMatchObject({ difference: "900.00" });

  // Taslağa düzeltme açılmaz; verilmiş 2. sürüm yeniden düzeltilebilir.
  const again = await api.json<Preview>(await api.post(`${one()}/amend`, {}));
  expect(again.amendment).toMatchObject({ version: 3, previous: { version: 2 } });
  const draft = await api.post(`${one()}/amend`, {});
  expect(draft.status()).toBe(409);
});
