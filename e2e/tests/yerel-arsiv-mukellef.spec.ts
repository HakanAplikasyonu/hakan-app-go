import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, uiLogin, uniqueVkn } from "./lib";

/**
 * Belge arşivi → mükellef paneli (#682): ofisin "Mükellef de görsün" diye
 * işaretlediği belge mükellefin "Belgeler" ekranında görünür ve iner;
 * işaretsiz belge ne listede ne indirmede vardır.
 *
 *   ofis iki belge yükler (API): paylaşılan sözleşme + paylaşılmayan iç not →
 *   mükellef API'si yalnız sözleşmeyi listeler, iç notun dosyası 404 →
 *   mükellef giriş yapar, "Belgeler" → "Sözleşme" başlığı altında belge,
 *   "İndir" aynı baytları verir → ofis kullanıcısı mükellef ucuna giremez.
 *
 * Veri: her koşuda yeni mükellef ve kullanıcısı (onay veren sahip).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Arşiv Mükellef ${run} Ltd. Şti.` };
const SHARED = `Kira sözleşmesi ${run}`;
const HIDDEN = `İç not ${run}`;

function pdf(tag: string) {
  return Buffer.from(`%PDF-1.4\n% ${tag} ${run}\n%%EOF\n`, "utf8");
}
const SHARED_PDF = pdf("paylasilan");
const HIDDEN_PDF = pdf("gizli");

type Doc = { uuid: string; title: string; category: string; visible_to_taxpayer: boolean; file_name: string };
type Page = { items: Doc[]; total: number; counts: { total: number } };

let office: Api;
let tpApi: Api;
let user = { email: "", password: "" };
let taxpayerUuid = "";
let shared: Doc;
let hidden: Doc;

const tpHeaders = () => ({ "X-Taxpayer": taxpayerUuid });

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const made = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "arsivmk");
  taxpayerUuid = made.taxpayerUuid;
  user = { email: made.email, password: made.password };
  const base = `/v1/tenant/engagements/${made.engagement}/archive-documents`;

  const up = async (title: string, category: string, visible: boolean, buffer: Buffer) => {
    const res = await office.uploadWith(
      base,
      { name: `${title}.pdf`, mimeType: "application/pdf", buffer },
      { category, title, visible_to_taxpayer: String(visible) },
    );
    expect(res.status(), await res.text()).toBe(201);
    return (await res.json()).data as Doc;
  };
  shared = await up(SHARED, "contract", true, SHARED_PDF);
  hidden = await up(HIDDEN, "tax", false, HIDDEN_PDF);
  expect(shared.visible_to_taxpayer).toBe(true);
  expect(hidden.visible_to_taxpayer).toBe(false);

  tpApi = await Api.login(user.email, user.password);
});

test.afterAll(async () => {
  await tpApi?.dispose();
  await office?.dispose();
});

test("mükellef ucu yalnız paylaşılan belgeyi verir", async () => {
  const page = await tpApi.json<Page>(await tpApi.get("/v1/taxpayer/archive-documents", tpHeaders()));
  expect(page.items.map((d) => d.title)).toEqual([SHARED]);
  expect(page.total).toBe(1);
  expect(page.counts.total).toBe(1);

  const file = await tpApi.get(`/v1/taxpayer/archive-documents/${shared.uuid}/file`, tpHeaders());
  expect(file.status()).toBe(200);
  expect(Buffer.from(await file.body()).equals(SHARED_PDF)).toBeTruthy();

  const denied = await tpApi.get(`/v1/taxpayer/archive-documents/${hidden.uuid}/file`, tpHeaders());
  expect(denied.status()).toBe(404);

  // Ofis kullanıcısı mükellef üyesi değil: mükellef ucuna giremez.
  const outsider = await office.get("/v1/taxpayer/archive-documents", tpHeaders());
  expect(outsider.status()).toBe(403);
});

test("mükellef 'Belgeler' ekranında belgeyi görür ve indirir", async ({ page }) => {
  await uiLogin(page, user.email, user.password);
  await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
  await page.getByRole("link", { name: "Belgeler", exact: true }).first().click();
  await expect(page).toHaveURL(/\/m\/[0-9a-f-]{36}\/archive$/);
  await expect(page.getByText("Paylaşılan belgeler", { exact: true })).toBeVisible();

  const section = page.getByRole("region", { name: "Sözleşme" });
  await expect(section).toContainText(SHARED);
  await expect(page.getByText(HIDDEN)).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Vergi" })).toHaveCount(0);

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    section.getByRole("button", { name: `${SHARED} indir` }).click(),
  ]);
  expect(download.suggestedFilename()).toBe(shared.file_name);
  expect(readFileSync(await download.path()).equals(SHARED_PDF)).toBeTruthy();
});
