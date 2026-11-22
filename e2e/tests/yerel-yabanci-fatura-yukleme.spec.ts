import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Mükelleften yabancı hizmet faturası (#780 KDV2 A2): mükellef panelindeki
 * "Yabancı hizmet faturası gönder" kartından PDF yükler; ofisin Yabancı
 * faturalar / KDV2 sekmesinde "Mükellef panelinden geldi" taslağı görünür,
 * belge açılır, ofis düzenleyip onaylar; mükellef panelinde "alındı" sayısı
 * artar ve durum "İşlendi" olur. Ofis ayarı kapalıyken kart gizlenir ve
 * yükleme reddedilir.
 *
 * Veri: her koşuda yeni mükellef ve kullanıcısı. Ofis geneli bölüm anahtarı
 * testte değişir, sonunda eski hâline döner.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Yabanci Yukleme ${run} Ltd. Sti.` };
const VENDOR = `Meta Platforms Ireland ${run}`;

/** Bu ay (İstanbul) YYYY-MM. */
const period = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date()).slice(0, 7);

/** Her koşuda farklı baytlar: aynı dosya ikinci kez gelirse yeni kayıt açılmaz. */
const pdf = (tag: string) => ({
  name: `meta-reklam-${tag}.pdf`,
  mimeType: "application/pdf",
  buffer: Buffer.from(`%PDF-1.4\n% yabanci fatura ${run} ${tag}\n%%EOF\n`),
});

type Item = { uuid: string; vendor: string; status: string; source_kind: string | null; has_file: boolean; prefilled: boolean };
type TaxpayerList = { enabled: boolean; count: number; items: { uuid: string; vendor: string; status: string }[] };

let office: Api;
let tpApi: Api;
let user: { engagement: string; taxpayerUuid: string; email: string; password: string };
let originalEnabled = true;

const setEnabled = async (enabled: boolean) => {
  const res = await office.put("/v1/tenant/foreign-service-kinds", { enabled });
  expect(res.status(), await res.text()).toBe(200);
};
const taxpayerList = async () =>
  tpApi.json<TaxpayerList>(
    await tpApi.get(`/v1/taxpayer/foreign-invoices?period=${period}`, { "X-Taxpayer": user.taxpayerUuid }),
  );

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  user = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "yabanciyukle");
  tpApi = await Api.login(user.email, user.password);
  const kinds = await office.json<{ enabled: boolean }>(await office.get("/v1/tenant/foreign-service-kinds"));
  originalEnabled = kinds.enabled;
  await setEnabled(true);
});

test.afterAll(async () => {
  if (office) await setEnabled(originalEnabled).catch(() => undefined);
  await tpApi?.dispose();
  await office?.dispose();
});

test("mükellef panelden yabancı fatura gönderir; alındı sayısı artar", async ({ page }) => {
  expect((await taxpayerList()).count).toBe(0);
  await uiLogin(page, user.email, user.password);
  await page.goto(`/m/${user.taxpayerUuid}/uploads`);
  const card = page.getByTestId("foreign-invoice-card");
  await expect(card.getByText("Yabancı hizmet faturası gönder")).toBeVisible();
  await expect(card.getByText("KDV2", { exact: false })).toBeVisible();
  await expect(page.getByTestId("foreign-invoice-count")).toHaveText("Bu ay 0 yabancı faturanız alındı");

  await card.getByLabel("Not (isteğe bağlı)").fill("Bu ayın Instagram reklam faturası");
  await page.getByTestId("foreign-invoice-file").setInputFiles(pdf("panel"));
  await expect(card.getByRole("status")).toContainText("ofisinize gönderildi");
  await expect(page.getByTestId("foreign-invoice-count")).toHaveText("Bu ay 1 yabancı faturanız alındı");
  await expect(page.getByTestId("foreign-invoice-list")).toContainText("Ofiste inceleniyor");

  // Aynı dosya ikinci kez: yeni kayıt açılmaz.
  await page.getByTestId("foreign-invoice-file").setInputFiles(pdf("panel"));
  await expect(card.getByRole("status")).toContainText("daha önce göndermişsiniz");
  await expect(page.getByTestId("foreign-invoice-count")).toHaveText("Bu ay 1 yabancı faturanız alındı");
});

test("ofis listesinde taslak görünür, belge açılır; ofis düzenleyip onaylar", async ({ page }) => {
  const base = `/v1/tenant/engagements/${user.engagement}/foreign-invoices`;
  const list = await office.json<{ items: Item[] }>(await office.get(`${base}?period=${period}`));
  const draft = list.items.find((i) => i.source_kind === "taxpayer_upload");
  expect(draft, JSON.stringify(list.items)).toBeTruthy();
  expect(draft!.status).toBe("draft");
  expect(draft!.has_file).toBe(true);
  // Belge ucu yüklenen PDF'i döner.
  const file = await office.get(`${base}/${draft!.uuid}/file`);
  expect(file.status()).toBe(200);
  expect(file.headers()["content-type"]).toContain("application/pdf");
  // Eksik taslak onaylanmaz.
  expect((await office.post(`${base}/${draft!.uuid}/confirm`)).status()).toBe(400);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${user.engagement}/foreign-invoices?period=${period}`));
  const row = page.locator('[data-testid="foreign-invoice"][data-source="taxpayer_upload"]');
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Mükellef panelinden geldi");
  await expect(row).toContainText("Satıcı girilmedi");
  await expect(row).toContainText("Onaylamak için satıcı, ülke ve tutarı girin");

  const popup = page.waitForEvent("popup");
  await row.getByTestId("foreign-open-file").click();
  await (await popup).close();

  await row.getByRole("button", { name: /düzenle/i }).click();
  const dialog = page.getByRole("dialog", { name: "Faturayı düzenle" });
  await dialog.getByLabel("Satıcı").fill(VENDOR);
  await dialog.getByLabel("Ülke (2 harf)").fill("IE");
  await dialog.getByLabel("Tutar").fill("450");
  await dialog.getByLabel("İşlem türü").selectOption("reklam");
  await dialog.getByLabel("Kur (1 birim", { exact: false }).fill("34,50");
  await dialog.getByRole("button", { name: "Kaydet" }).click();
  await expect(dialog).toBeHidden();

  const done = page.getByTestId("foreign-invoice").filter({ hasText: VENDOR });
  await expect(done).toHaveAttribute("data-status", "draft");
  await done.getByRole("button", { name: /onayla$/i }).click();
  await expect(done).toHaveAttribute("data-status", "confirmed");

  // Mükellef tarafında durum "İşlendi" (processed).
  const mine = await taxpayerList();
  expect(mine.count).toBe(1);
  expect(mine.items[0]).toMatchObject({ vendor: VENDOR, status: "processed" });
});

test("ayar kapalıyken yükleme kartı gizlenir ve yükleme reddedilir", async ({ page }) => {
  try {
    await setEnabled(false);
    const res = await tpApi.postForm("/v1/taxpayer/foreign-invoices", { file: pdf("kapali") }, { "X-Taxpayer": user.taxpayerUuid });
    expect(res.status()).toBe(409);
    expect((await taxpayerList()).enabled).toBe(false);

    await uiLogin(page, user.email, user.password);
    await page.goto(`/m/${user.taxpayerUuid}/uploads`);
    // Sayfanın kendisi (e-fatura yükleme) durur; yabancı fatura kartı yok.
    await expect(page.getByText("Belge yükle").first()).toBeVisible();
    await expect(page.getByTestId("foreign-invoice-card")).toHaveCount(0);
  } finally {
    await setEnabled(true);
  }
});
