import { expect, test, type Page } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Mükellef özeti, dörtlü düzen (Oğuz'un demo geri bildirimi): en üstte
 * Fatura, Z raporu, Masraf ve Banka kartları dönemin sayısıyla; karta
 * tıklayınca o ayın listesi açılır. Yanında "Luca'ya aktar" ve "Luca'nın
 * API'si yok" açıklaması; işletme defterinde düğme yerine neden yazar.
 * Kartların altında diğer bölümler; "Önemli belgeler"de tek düğme ve tek
 * kısa açıklama.
 *
 * Veri: her koşuda yeni bir mükellef ve bir UBL faturası. Ofis geneli ayar
 * değiştirilmez; defter türü testin kendi mükellefinde değişir ve geri alınır.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Dörtlü Özet ${run} Ltd. Şti.` };

let api: Api;
let engagement = "";
const base = () => `/v1/tenant/engagements/${engagement}`;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "dortlu");
  const up = await api.upload(
    `${base()}/ubl-uploads`,
    deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);
});

test.afterAll(async () => {
  await api?.dispose();
});

/** Kartın bağlantısındaki dönem (`?period=YYYY-MM`). */
async function cardPeriod(page: Page, kind: string): Promise<string> {
  const href = await page.getByTestId(`month-card-${kind}`).getAttribute("href");
  const period = new URL(href!, "http://x").searchParams.get("period");
  expect(period).toMatch(/^\d{4}-\d{2}$/);
  return period!;
}

async function total(path: string): Promise<number> {
  const body = await api.json<{ total?: number; summary?: { total: number } }>(
    await api.get(path),
  );
  return body.total ?? body.summary?.total ?? -1;
}

test("dört kart sayıyla görünür ve tıklayınca o ayın listesi açılır", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));

  const summary = page.getByTestId("month-summary");
  await expect(summary).toBeVisible({ timeout: 30_000 });
  await expect(summary.getByRole("heading", { level: 2 })).toContainText("özeti");

  const kinds = [
    ["invoices", "Fatura", "invoices"],
    ["zreports", "Z raporu", "retail-sales"],
    ["expenses", "Masraf", "expenses"],
    ["bank", "Banka", "bank-transactions"],
  ] as const;
  for (const [kind, title, endpoint] of kinds) {
    const card = page.getByTestId(`month-card-${kind}`);
    await expect(card).toBeVisible();
    await expect(card).toContainText(title);
    const period = await cardPeriod(page, kind);
    const want = await total(`${base()}/${endpoint}?period=${period}&limit=1`);
    expect(want).toBeGreaterThanOrEqual(0);
    await expect(card.getByTestId("month-card-count")).toHaveText(String(want));
    // Durum satırı yükleniyor yazısında kalmaz.
    await expect(card).not.toContainText("Yükleniyor");
  }

  // Diğer bölümler kartların altında; sekmeler yerinde.
  const others = page.getByRole("navigation", { name: "Diğer bölümler" });
  for (const name of ["Excel aktarımı", "Geçmiş dönem", "Beyannameler", "Dilekçeler"])
    await expect(others.getByRole("link", { name: new RegExp(`^${name}`) })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Mükellef bölümleri" })).toBeVisible();

  // Fatura kartı o ayın fatura listesini açar.
  const invPeriod = await cardPeriod(page, "invoices");
  await page.getByTestId("month-card-invoices").click();
  await page.waitForURL(
    (u) =>
      u.pathname.endsWith(`/engagements/${engagement}/invoices`) &&
      u.searchParams.get("period") === invPeriod,
  );

  // Z raporu kartı da kendi listesini açar.
  await page.goto(officePath(`/engagements/${engagement}`));
  await page.getByTestId("month-card-zreports").click();
  await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/z-reports`));
});

test("Luca'ya aktar: düğme, açıklama; işletme defterinde neden yazar", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));

  const luca = page.getByTestId("luca-export");
  await expect(luca).toBeVisible({ timeout: 30_000 });
  await expect(luca.getByRole("heading", { name: "Luca'ya aktar" })).toBeVisible();
  await expect(luca.getByTestId("luca-hint")).toContainText("Luca'nın API'si yok");
  await expect(luca.getByTestId("luca-hint")).toContainText("Luca'ya siz yüklersiniz");
  const button = luca.getByRole("button", { name: "Luca'ya aktar" });
  await expect(button).toBeEnabled();

  // Yeni mükellefin onaylı fişi yok: dosya yerine Fişler sayfasına yol gösterir
  // (ya da fiş varsa dosya iner).
  await button.click();
  await expect(
    luca
      .getByRole("link", { name: "Fişler sayfasında aç" })
      .or(luca.getByText(/dosyaya yazıldı/)),
  ).toBeVisible({ timeout: 30_000 });

  // İşletme defteri: yevmiye fişi yok, düğme yerine neden.
  const set = await api.put(`${base()}/ledger-type`, { ledger_type: "isletme" });
  expect(set.status(), await set.text()).toBe(200);
  try {
    await page.reload();
    await expect(luca.getByTestId("luca-single-entry")).toContainText("yevmiye fişi yok", {
      timeout: 30_000,
    });
    await expect(luca.getByRole("button", { name: "Luca'ya aktar" })).toHaveCount(0);
    await expect(luca.getByRole("link", { name: "İşletme hesabını aç" })).toBeVisible();
  } finally {
    const back = await api.put(`${base()}/ledger-type`, { ledger_type: "bilanco" });
    expect(back.status(), await back.text()).toBe(200);
  }
});

test("Önemli belgeler: tek düğme, tek kısa açıklama", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));

  const section = page.getByRole("region", { name: "Önemli belgeler" });
  await expect(section).toBeVisible({ timeout: 30_000 });
  await expect(section.getByRole("button", { name: "Önemli belge ekle" })).toHaveCount(1);
  await expect(section.getByTestId("important-docs-hint")).toContainText("Vergi levhası");
  // Arşivden işaretleme bağlantısı yalnız bir kez (boş durumda tekrar yok).
  await expect(
    section.getByRole("link", { name: "Arşivdeki bir belgeyi önemli olarak işaretle" }),
  ).toHaveCount(1);
  await expect(section.getByText("Henüz önemli belge yok")).toBeVisible();

  // Düğme yükleme penceresini açar.
  await section.getByRole("button", { name: "Önemli belge ekle" }).click();
  const dialog = page.getByRole("dialog", { name: "Arşive belge yükle" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("Önemli belge")).toHaveValue("tax_certificate");
});
