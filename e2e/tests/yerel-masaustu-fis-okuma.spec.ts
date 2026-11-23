import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Masaüstü ajanının izlediği klasöre bırakılan fiş / fatura fotoğrafı
 * (Oğuz'un demo geri bildirimi): ajan `POST /v1/tenant/agents/{a}/documents`
 * ile yükler, sunucu fotoğrafı mobil ve WhatsApp ile aynı fiş okumasına
 * (receipt capture) verir. Mükellef alt klasör adından (unvan ya da VKN)
 * bulunur; bulunamazsa fotoğraf Yerel ajanlar sayfasındaki "Ajandan gelen
 * dosyalar" listesinde "Fiş / fatura fotoğrafı" türüyle eşleştirme bekler.
 *
 * Yapay zekâ CI'da kapalı olabilir: okuma başarısız olsa da fiş kaydı
 * (receipt capture) mükellefe açılır; test okumanın sonucuna değil, aynı
 * yola girildiğine bakar.
 *
 * Veri: kendi açtığı yeni mükellef ve yeni bir ajan kaydı; demo
 * mükelleflerine dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const vkn = uniqueVkn();
const title = `Ajan Fis ${stamp % 100000} Ltd. Şti.`;
let api: Api;
let engagement = "";
let agent = "";

type Doc = {
  uuid: string;
  kind: string;
  status: string;
  match_source: string;
  engagement_uuid: string | null;
  result_uuid: string | null;
  file_name: string;
};

// Her test kendi içeriğini yükler (aynı içerik ofiste bir kez kaydedilir).
const photo = (name: string, tag: string) => ({
  name,
  mimeType: "image/jpeg",
  buffer: Buffer.concat([
    Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00]),
    Buffer.from(`fis-${stamp}-${tag}`, "utf-8"),
    Buffer.from([0xff, 0xd9]),
  ]),
});

test.beforeAll(async ({ browser }) => {
  test.setTimeout(180_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, { vkn, title }, "ajan-fis");
  const reg = await api.json<{ uuid: string }>(
    await api.post("/v1/tenant/agents", {
      machine_id: `e2e-ajan-fis-${stamp}`,
      name: "E2E fiş ajanı",
      platform: "windows",
      version: "1.0.0",
    }),
  );
  agent = reg.uuid;
});

test.afterAll(async () => {
  await api?.dispose();
});

const expectCapture = async (receipt: string | null) => {
  expect(receipt).toBeTruthy();
  const res = await api.get(`/v1/tenant/engagements/${engagement}/receipts/${receipt}`);
  expect(res.status(), await res.text()).toBe(200);
};

test("Mükellef alt klasöründeki fotoğraf fiş okumasına gider", async () => {
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, {
    file: photo("market.jpg", "unvan"),
    folder: `C:\\Fişler\\${title.replace(" Ltd. Şti.", "")}`,
  });
  expect(res.status(), await res.text()).toBe(201);
  const doc = await api.json<Doc>(res);
  expect(doc).toMatchObject({
    kind: "receipt",
    status: "imported",
    match_source: "folder",
    engagement_uuid: engagement,
  });
  await expectCapture(doc.result_uuid);
});

test("VKN adlı alt klasördeki fotoğraf da aynı mükellefe gider", async () => {
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, {
    file: photo("z_raporu.jpg", "vkn"),
    folder: `C:\\Fişler\\${vkn}`,
  });
  expect(res.status(), await res.text()).toBe(201);
  const doc = await api.json<Doc>(res);
  expect(doc).toMatchObject({ kind: "receipt", status: "imported", engagement_uuid: engagement });
  await expectCapture(doc.result_uuid);
});

test("Mükellefi bulunamayan fotoğraf eşleştirme bekler; Eşleştir ile fiş okumasına gider", async ({ page }) => {
  const name = `eslesmeyen-${stamp}.jpg`;
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, {
    file: photo(name, "eslesmeyen"),
    folder: "C:\\Fişler\\Gelen",
  });
  expect(res.status(), await res.text()).toBe(201);
  const doc = await api.json<Doc>(res);
  expect(doc).toMatchObject({ kind: "receipt", status: "needs_match", engagement_uuid: null });

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/agents"));
  await expect(page.getByText(/Ajandan gelen dosyalar/).first()).toBeVisible();
  const row = page.getByRole("row").filter({ hasText: name });
  await expect(row).toBeVisible();
  await expect(row.getByText("Fiş / fatura fotoğrafı")).toBeVisible();

  const resolved = await api.json<Doc>(
    await api.post(`/v1/tenant/agent-documents/${doc.uuid}/resolve`, {
      engagement_uuid: engagement,
      kind: "receipt",
    }),
  );
  expect(resolved).toMatchObject({ status: "imported", match_source: "office", engagement_uuid: engagement });
  await expectCapture(resolved.result_uuid);
});
