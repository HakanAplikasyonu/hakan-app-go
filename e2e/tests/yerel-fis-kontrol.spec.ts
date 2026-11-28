import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, uniqueVkn } from "./lib";

/**
 * Fiş yapay zekâ kontrolleri (Oğuz'un geri bildirimi): fiş okunurken
 * alıcı mükellef değilse RECEIPT_BUYER_MISMATCH ("Bu fiş başka biri adına
 * kesilmiş"), akaryakıt fişinde plaka araç listesinde yoksa
 * PLATE_NOT_REGISTERED, plaka okunamadıysa PLATE_UNREADABLE. Mükellef
 * çekim önizlemesinde (`checks`), ofis onay ekranında rozet olarak görür.
 * Ofis her kodu uyarı ayarlarından kapatabilir.
 *
 * Mükellefin araç listesi: GET/POST /v1/taxpayer/vehicles, DELETE
 * /v1/taxpayer/vehicles/{uuid}; ofisin listesiyle aynıdır.
 *
 * Yapay zekâ e2e'de yoksa fiş okunamaz ve taslak çıkmaz: o zaman kontrol
 * kodu beklenmez, yalnız alanın biçimi denetlenir.
 */
test.describe.configure({ mode: "serial" });

type Vehicle = { uuid: string; plate: string; description: string };
type Rule = { code: string; effective: { enabled: boolean } };
type Capture = {
  uuid: string;
  status: string;
  plate_check?: string;
  checks?: string[];
  draft: { type: string; uuid: string } | null;
};
type Expense = { uuid: string; checks: string[] };

const CODES = ["RECEIPT_BUYER_MISMATCH", "PLATE_NOT_REGISTERED", "PLATE_UNREADABLE"];
const taxpayer = { vkn: uniqueVkn(), title: "E2E Fiş Kontrol Ltd." };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "fiskontrol");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office.dispose();
});

test("kontrol kodları uyarı ayarlarında; ofis mükellef için kapatabilir", async () => {
  const rules = await office.json<{ items: Rule[] }>(await office.get("/v1/tenant/warning-rules"));
  for (const code of CODES) {
    const rule = rules.items.find((r) => r.code === code);
    expect(rule, code).toBeTruthy();
    expect(rule!.effective.enabled, code).toBe(true);
  }
  const path = `/v1/tenant/engagements/${engagement}/warning-rules/PLATE_UNREADABLE`;
  let res = await office.put(path, { enabled: false });
  expect(res.status(), await res.text()).toBe(200);
  expect((await res.json()).data.effective.enabled).toBe(false);
  // Geri aç: sonraki adımlar varsayılanla koşar.
  res = await office.put(path, { enabled: true });
  expect(res.status(), await res.text()).toBe(200);
});

test("mükellef araç ekler, kaldırır; ofis aynı listeyi görür", async () => {
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };
  try {
    let res = await tp.post("/v1/taxpayer/vehicles", { plate: "34 ek 4242", description: "Binek" }, headers);
    expect(res.status(), await res.text()).toBe(201);
    const added = (await res.json()).data as Vehicle;
    expect(added.plate).toBe("34EK4242");

    // Aynı plaka ikinci kez: 409. Geçersiz plaka: 400.
    res = await tp.post("/v1/taxpayer/vehicles", { plate: "34EK4242" }, headers);
    expect(res.status(), await res.text()).toBe(409);
    res = await tp.post("/v1/taxpayer/vehicles", { plate: "plaka değil" }, headers);
    expect(res.status(), await res.text()).toBe(400);

    const mine = await tp.json<{ items: Vehicle[] }>(await tp.get("/v1/taxpayer/vehicles", headers));
    expect(mine.items.map((v) => v.plate)).toContain("34EK4242");
    const theirs = await office.json<{ items: Vehicle[] }>(
      await office.get(`/v1/tenant/engagements/${engagement}/vehicles`),
    );
    expect(theirs.items.map((v) => v.uuid)).toContain(added.uuid);

    res = await tp.delete(`/v1/taxpayer/vehicles/${added.uuid}`, headers);
    expect(res.status(), await res.text()).toBe(204);
    const after = await tp.json<{ items: Vehicle[] }>(await tp.get("/v1/taxpayer/vehicles", headers));
    expect(after.items.map((v) => v.uuid)).not.toContain(added.uuid);
  } finally {
    await tp.dispose();
  }
});

test("çekim cevabı kontrolleri döner; ofis masraf listesinde rozet alanı", async () => {
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };
  try {
    const res = await tp.postForm(
      "/v1/taxpayer/receipts",
      {
        image: {
          name: "yakit.png",
          mimeType: "image/png",
          buffer: Buffer.concat([
            Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
            Buffer.from(`fis-kontrol-${Date.now()}`),
          ]),
        },
        kind: "fuel_receipt",
      },
      { ...headers, "Idempotency-Key": `fiskontrol-${Date.now()}` },
    );
    expect(res.status(), await res.text()).toBe(201);
    const capture = (await res.json()).data as Capture;
    for (const code of capture.checks ?? []) expect(CODES).toContain(code);

    const page = await office.json<{ items: Expense[] }>(
      await office.get(`/v1/tenant/engagements/${engagement}/expenses?limit=50`),
    );
    for (const x of page.items) expect(Array.isArray(x.checks)).toBe(true);
    if (capture.status === "draft" && capture.draft?.type === "expense") {
      // AI sahtesiyle: taslağın rozeti çekimdeki kontrollerle aynı.
      const x = page.items.find((i) => i.uuid === capture.draft!.uuid);
      expect(x).toBeTruthy();
      expect([...x!.checks].sort()).toEqual([...(capture.checks ?? [])].sort());
    }
  } finally {
    await tp.dispose();
  }
});
