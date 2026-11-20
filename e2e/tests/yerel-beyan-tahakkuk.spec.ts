import { readFileSync } from "node:fs";

import { expect, test, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Verilmiş beyana GİB tahakkuk PDF'i eklenir ve indirilir (#575).
 *
 * `beyanname.spec.ts` beyanı hazırlayıp "verildi" kaydeder ama tahakkuk
 * PDF'i eklenmez. Bu dosya beyan sürecinin son adımını dener:
 *
 *   KDV1 hazırla + verildi (API) → ekranda "Tahakkuk fişi: Eklenmedi" →
 *   PDF seç → "Tahakkuk fişi eklendi.", dosya adı görünür → API'de kayıtlı
 *   (ad, boyut) → "Tahakkuk fişini indir" aynı baytları verir → PDF
 *   olmayan dosya reddedilir, kayıtlı PDF yerinde kalır.
 *
 * Veri: her koşuda yeni mükellef (faturası yok); dönem ekranın varsayılanı
 * olan beyan dönemi (önceki ay).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Tahakkuk ${run} Ltd. Şti.` };
const ACCRUAL_NO = `TAH-E2E-${run}`;
const FILE_NAME = `kdv1-tahakkuk-${run}.pdf`;

/** En küçük geçerli PDF: başlığı "%PDF-" ile başlar. */
const PDF = Buffer.from(
  [
    "%PDF-1.4",
    "1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj",
    "2 0 obj << /Type /Pages /Kids [] /Count 0 >> endobj",
    "trailer << /Root 1 0 R >>",
    `%% tahakkuk ${run}`,
    "%%EOF",
    "",
  ].join("\n"),
  "utf8",
);

type Preview = {
  kind: string;
  period: string;
  status: string;
  open_warnings: string[];
  record: {
    accrual_number?: string | null;
    accrual_document?: { file_name: string; size_bytes: number } | null;
  } | null;
};

let api: Api;
let office: Page;
let engagement = "";
let period = "";

/** Ekranın varsayılan dönemi: İstanbul saatine göre önceki ay. */
function declarationPeriod() {
  const [y, m] = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul", year: "numeric", month: "2-digit" })
    .format(new Date())
    .split("-")
    .map(Number);
  const d = new Date(Date.UTC(y, m - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

function today() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date());
}

const one = () => `/v1/tenant/engagements/${engagement}/tax-returns/kdv1/${period}`;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tahakkuk");
  period = declarationPeriod();

  // Beyan hazırlanır (açık uyarılar onaylanarak) ve verildi kaydedilir.
  const draft = await api.json<Preview>(await api.get(one()));
  const prepared = await api.json<Preview>(
    await api.post(`${one()}/prepare`, { acknowledged: draft.open_warnings ?? [] }),
  );
  expect(prepared.status).toBe("prepared");
  const filed = await api.json<Preview>(
    await api.post(`${one()}/file`, { filed_on: today(), accrual_number: ACCRUAL_NO }),
  );
  expect(filed.status).toBe("filed");
  expect(filed.record?.accrual_document ?? null).toBeNull();

  office = await (await browser.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  await uiLogin(office, DEMO.owner);
});

test.afterAll(async () => {
  await office?.context().close();
  await api?.dispose();
});

function accrualRegion() {
  return office.getByRole("region", { name: "Tahakkuk fişi" });
}

test("verilmiş beyana tahakkuk PDF'i eklenir", async () => {
  await office.goto(officePath(`/engagements/${engagement}/tax-returns`));
  const status = office.getByRole("region", { name: "Beyan durumu" });
  await expect(status).toContainText("Verildi");
  await expect(status).toContainText(ACCRUAL_NO);

  const region = accrualRegion();
  await expect(region).toContainText("Eklenmedi");
  await expect(region.getByRole("button", { name: "Tahakkuk fişini indir" })).toHaveCount(0);

  await region
    .locator('input[type="file"][aria-label="Tahakkuk PDF\'i seç"]')
    .setInputFiles({ name: FILE_NAME, mimeType: "application/pdf", buffer: PDF });
  await expect(office.getByText("Tahakkuk fişi eklendi.")).toBeVisible();
  await expect(region).toContainText(FILE_NAME);
  await expect(region.getByRole("button", { name: "Değiştir" })).toBeVisible();

  const saved = await api.json<Preview>(await api.get(one()));
  expect(saved.status).toBe("filed");
  expect(saved.record?.accrual_document).toMatchObject({ file_name: FILE_NAME, size_bytes: PDF.length });
});

test("tahakkuk fişi indirilir: aynı dosya", async () => {
  const [download] = await Promise.all([
    office.waitForEvent("download"),
    accrualRegion().getByRole("button", { name: "Tahakkuk fişini indir" }).click(),
  ]);
  expect(download.suggestedFilename()).toBe(FILE_NAME);
  const body = readFileSync(await download.path());
  expect(body.equals(PDF)).toBeTruthy();
});

test("PDF olmayan dosya reddedilir, kayıtlı tahakkuk yerinde kalır", async () => {
  const notPdf = { name: "not.pdf", mimeType: "application/pdf", buffer: Buffer.from("merhaba", "utf8") };
  await accrualRegion().locator('input[type="file"]').setInputFiles(notPdf);
  // Hata metni beyana uygun (bordroya değil).
  await expect(office.getByText("Dosya PDF olmalı.").first()).toBeVisible();
  await expect(office.getByText("Bordro belgeleri PDF olmalı.")).toHaveCount(0);
  await expect(accrualRegion()).toContainText(FILE_NAME);

  const res = await api.uploadPut(`${one()}/accrual-document`, notPdf);
  expect(res.status()).toBe(415);
  const kept = await api.json<Preview>(await api.get(one()));
  expect(kept.record?.accrual_document).toMatchObject({ file_name: FILE_NAME, size_bytes: PDF.length });
});
