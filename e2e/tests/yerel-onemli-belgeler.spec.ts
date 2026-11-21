import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Önemli belgeler (#780 B): ofis mükellef sayfasındaki "Önemli belgeler"
 * bölümünden belge yükler, arşivdeki bir belgeyi "önemli" işaretler, bir
 * belgede "Mükellef görebilir"i kapatır; mükellef panelinde yalnız açık
 * olanlar görünür, bitişi yakın olan renkli satırdadır.
 *
 * Veri: her koşuda yeni mükellef ve kullanıcısı (onay veren sahip). Ofis
 * geneli ayar değiştirilmez.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Önemli Belge ${run} Ltd. Şti.` };
const LEVHA = `Vergi levhası ${run}`;
const IMZA = `İmza sirküleri ${run}`;
const VEKALET = `Vekâletname ${run}`;

function pdf(name: string, tag: string) {
  return { name, mimeType: "application/pdf", buffer: Buffer.from(`%PDF-1.4\n% ${tag} ${run}\n%%EOF\n`, "utf8") };
}
const dayPlus = (d: number) => {
  const t = new Date();
  t.setDate(t.getDate() + d);
  return t.toISOString().slice(0, 10);
};

type Doc = { uuid: string; title: string; important_kind: string | null; visible_to_taxpayer: boolean; expiry_status: string };

let office: Api;
let tpApi: Api;
let engagement = "";
let taxpayerUuid = "";
let user = { email: "", password: "" };

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const made = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "onemli");
  engagement = made.engagement;
  taxpayerUuid = made.taxpayerUuid;
  user = { email: made.email, password: made.password };
  const base = `/v1/tenant/engagements/${engagement}/archive-documents`;

  // Arşivde sıradan bir belge (sonra arayüzden "önemli" işaretlenecek).
  let res = await office.uploadWith(base, pdf(`imza-${run}.pdf`, "imza"), { category: "trade_registry", title: IMZA });
  expect(res.status(), await res.text()).toBe(201);
  // Önemli, mükellefe açık, 10 gün içinde biten vekâletname.
  res = await office.uploadWith(base, pdf(`vekalet-${run}.pdf`, "vekalet"), {
    category: "general",
    title: VEKALET,
    important_kind: "power_of_attorney",
    valid_until: dayPlus(10),
    visible_to_taxpayer: "true",
  });
  expect(res.status(), await res.text()).toBe(201);
  const d = (await res.json()).data as Doc;
  expect(d).toMatchObject({ important_kind: "power_of_attorney", visible_to_taxpayer: true, expiry_status: "expiring" });

  tpApi = await Api.login(user.email, user.password);
});

test.afterAll(async () => {
  await tpApi?.dispose();
  await office?.dispose();
});

test("ofis önemli belge yükler, işaretler ve mükellefe görünürlüğü kapatır", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const section = page.getByRole("region", { name: "Önemli belgeler" });
  await expect(section).toContainText(VEKALET);
  await expect(section.locator("li[data-urgent]", { hasText: VEKALET })).toContainText("gün kaldı");

  // Bölümden yükleme: tür "Vergi levhası", "Mükellef de görsün" açık gelir.
  await section.getByRole("button", { name: "Önemli belge ekle" }).click();
  const dialog = page.getByRole("dialog", { name: "Arşive belge yükle" });
  await dialog.getByLabel("Dosya").setInputFiles(pdf(`levha-${run}.pdf`, "levha"));
  await expect(dialog.getByLabel("Önemli belge")).toHaveValue("tax_certificate");
  await expect(dialog.getByRole("checkbox", { name: "Mükellef de görsün" })).toBeChecked();
  await dialog.getByLabel("Başlık").fill(LEVHA);
  await dialog.getByRole("button", { name: "Yükle", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(section).toContainText(LEVHA);

  // Arşivdeki belgeyi önemli olarak işaretle.
  await page.goto(officePath(`/engagements/${engagement}/archive`));
  await page.getByRole("button", { name: `${IMZA} belgesini önemli olarak işaretle` }).click();
  const mark = page.getByRole("dialog", { name: "Önemli belge olarak işaretle" });
  await mark.getByLabel("Belge türü").selectOption("signature_circular");
  await mark.getByRole("checkbox", { name: "Mükellef de görsün" }).click();
  await mark.getByRole("button", { name: "İşaretle", exact: true }).click();
  await expect(mark).toBeHidden();
  await expect(page.getByRole("row", { name: new RegExp(IMZA) })).toContainText("Önemli · İmza sirküleri");

  // Vergi levhasında "Mükellef görebilir" kapatılır.
  await page.goto(officePath(`/engagements/${engagement}`));
  await expect(section).toContainText(IMZA);
  const toggle = section.getByRole("switch", { name: `${LEVHA}: mükellef görebilir` });
  await expect(toggle).toBeChecked();
  await toggle.click();
  await expect(toggle).not.toBeChecked();

  const list = await office.json<{ items: Doc[] }>(
    await office.get(`/v1/tenant/engagements/${engagement}/archive-documents?important=true`),
  );
  expect(list.items.map((d) => d.title).sort()).toEqual([IMZA, LEVHA, VEKALET].sort());
  expect(list.items[0].title).toBe(VEKALET); // bitişi yakın olan önce
  expect(list.items.find((d) => d.title === LEVHA)?.visible_to_taxpayer).toBe(false);
});

test("mükellef yalnız görünür önemli belgeleri görür ve indirir", async ({ page }) => {
  const res = await tpApi.get("/v1/taxpayer/important-documents", { "X-Taxpayer": taxpayerUuid });
  expect(res.status()).toBe(200);
  const data = await tpApi.json<{ enabled: boolean; items: Doc[] }>(res);
  expect(data.enabled).toBe(true);
  expect(data.items.map((d) => d.title).sort()).toEqual([IMZA, VEKALET].sort());

  // Ofis kullanıcısı mükellef ucuna giremez.
  const outsider = await office.get("/v1/taxpayer/important-documents", { "X-Taxpayer": taxpayerUuid });
  expect(outsider.status()).toBe(403);

  await uiLogin(page, user.email, user.password);
  await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
  await page.getByRole("link", { name: "Belgeler", exact: true }).first().click();
  await expect(page).toHaveURL(/\/m\/[0-9a-f-]{36}\/archive$/);
  const section = page.getByRole("region", { name: "Önemli belgeler" });
  await expect(section).toContainText(VEKALET);
  await expect(section).toContainText(IMZA);
  await expect(section).not.toContainText(LEVHA);
  await expect(page.getByText(LEVHA)).toHaveCount(0);
  await expect(section.locator("li[data-urgent]", { hasText: VEKALET })).toContainText("gün kaldı");

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    section.getByRole("button", { name: `${VEKALET} indir` }).click(),
  ]);
  expect(download.suggestedFilename()).toBe(`vekalet-${run}.pdf`);
});
