import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, uniqueVkn } from "./lib";

/**
 * Mobil fiş önizlemesi (Oğuz'un geri bildirimi): mükellef fişi çeker, okunan
 * satıcı / tarih / toplam ve KDV oranı satırlarını düzeltip gönderir. Yeni
 * uç yok: yükleme (`POST /v1/taxpayer/receipts`) okunan alanları hemen
 * döner, düzeltmeler mevcut düzeltme ucuna gider. Bu iş, ucun KDV oranı
 * satırlarını (`vat_<oran>_total` / `vat_<oran>_tax`) almasını ekledi.
 *
 * Yapay zekâ e2e'de yoksa fiş "okunamayan" olur ve taslak çıkmaz: o zaman
 * yalnız doğrulama (bilinmeyen oran 400) ve taslaksız fişin 409'u denenir.
 * AI sahtesiyle koşarken düzeltmeler ofisin inceleme kuyruğunda "Mükellef
 * düzeltmesi" olarak görünür.
 */
test.describe.configure({ mode: "serial" });

type Capture = { uuid: string; status: string; draft: { type: string; uuid: string } | null };
type Suggestion = { target_field: string; rationale: string; status: string };

const taxpayer = { vkn: uniqueVkn(), title: "E2E Fiş Önizleme Ltd." };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "fisonizle");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office.dispose();
});

test("mükellef KDV oranı satırlarını düzeltir; ofis Mükellef düzeltmesi olarak görür", async () => {
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };
  const res = await tp.postForm(
    "/v1/taxpayer/receipts",
    {
      image: {
        name: "fis.png",
        mimeType: "image/png",
        buffer: Buffer.concat([
          Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
          Buffer.from(`fis-onizleme-${Date.now()}`),
        ]),
      },
      kind: "expense_receipt",
    },
    { ...headers, "Idempotency-Key": `fisonizle-${Date.now()}` },
  );
  expect(res.status(), await res.text()).toBe(201);
  const capture = (await res.json()).data as Capture;
  const path = `/v1/taxpayer/receipts/${capture.uuid}/corrections`;

  // Bilinmeyen oran ve tanınmayan alan taslağa bakmadan 400.
  for (const field of ["vat_7_total", "vat_20_base"]) {
    const bad = await tp.post(path, { corrections: [{ field, value: "10,00" }] }, headers);
    expect(bad.status(), `${field}: ${await bad.text()}`).toBe(400);
  }

  const corrections = [
    { field: "vendor_name", value: "ÖRNEK MARKET" },
    { field: "total_amount", value: "120.00" },
    { field: "vat_20_total", value: "120.00" },
    { field: "vat_20_tax", value: "20.00" },
    { field: "tax_amount", value: "20.00" },
  ];
  const sent = await tp.post(path, { corrections }, headers);
  if (capture.status !== "draft" || capture.draft?.type !== "expense") {
    // AI yok: okunamayan fişin taslağı yok.
    expect(sent.status(), await sent.text()).toBe(409);
    await tp.dispose();
    return;
  }
  expect(sent.status(), await sent.text()).toBe(202);
  const submitted = ((await sent.json()).data as { submitted: string[] }).submitted;
  // Bir oranın tutarı KDV'sinden önce gider.
  expect(submitted.indexOf("vat_20_total")).toBeLessThan(submitted.indexOf("vat_20_tax"));
  expect(submitted).toHaveLength(corrections.length);
  await tp.dispose();

  const page = await office.json<{ items: Suggestion[] }>(
    await office.get(`/v1/tenant/engagements/${engagement}/suggestions?status=all&kind=receipt_field&limit=100`),
  );
  const vat = page.items.filter((s) => s.target_field.startsWith("vat_20_"));
  expect(vat.map((s) => s.target_field).sort()).toEqual(["vat_20_tax", "vat_20_total"]);
  for (const s of vat) expect(s.rationale).toContain("Mükellef düzeltmesi");
});
