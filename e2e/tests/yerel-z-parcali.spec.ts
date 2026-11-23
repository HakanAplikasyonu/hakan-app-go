import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayerWithUser,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Parçalı Z raporu (Oğuz'un demo geri bildirimi): uzun Z raporu tek karede
 * çekilemez; mükellef (mobil) ya da ofis (web) fotoğrafları parça 1/2/3
 * olarak tek yüklemede gönderir (`image`, `image_2`, …). Sunucu tek yakalama
 * ve tek Z raporu tutar; parçalar sırasıyla saklanır ve `?part=N` ile
 * görüntülenir. Ofis ayarı (`/v1/tenant/z-report-parts-settings`) kapalıyken
 * çok fotoğraflı yükleme 409 `Z_REPORT_PARTS_OFF` olur, tek fotoğraf çalışır.
 *
 * Yapay zekâ e2e'de yoksa fotoğraf "okunamayan" olur (Z raporu taslağı
 * çıkmaz); parçalar yine de yakalamada sıralı durur. AI sahtesiyle koşarken
 * Z raporu satırında "2 parça" ve yan panelde parçalar da denetlenir.
 */
test.describe.configure({ mode: "serial" });

type Capture = {
  uuid: string;
  status: string;
  kind: string;
  draft: { type: string; uuid: string } | null;
  parts?: { part_no: number; file_name: string }[];
};
type RetailSale = {
  uuid: string;
  receipt_uuid: string | null;
  part_count: number;
};

/** Ayırt edilebilir, PNG olarak tanınan küçük bir "fotoğraf". */
function photo(tag: string): Buffer {
  return Buffer.concat([
    Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
    Buffer.from(tag),
  ]);
}
const png = (name: string, tag: string) => ({
  name,
  mimeType: "image/png",
  buffer: photo(`${tag}-${Date.now()}`),
});

const taxpayer = { vkn: uniqueVkn(), title: "E2E Parçalı Z Ltd." };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;
const base = () => `/v1/tenant/engagements/${engagement}`;
const SETTINGS = "/v1/tenant/z-report-parts-settings";

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(
    browser,
    office,
    taxpayer,
    "zparca",
  );
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
  // Ayar varsayılan açık; önceki koşudan kapalı kalmış olabilir.
  await office.json(await office.put(SETTINGS, { enabled: true }));
});

test.afterAll(async () => {
  await office.put(SETTINGS, { enabled: true });
  await office.dispose();
});

test("mükellef mobilden iki parça gönderir: tek yakalama, sıralı parçalar, parça görseli", async () => {
  const settings = await office.json<{ enabled: boolean }>(
    await office.get(SETTINGS),
  );
  expect(settings.enabled).toBe(true);

  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const res = await tp.postForm(
    "/v1/taxpayer/receipts",
    {
      image: png("z-parca-1.png", "parca-1"),
      image_2: png("z-parca-2.png", "parca-2"),
      kind: "z_report",
    },
    { "X-Taxpayer": taxpayerUuid, "Idempotency-Key": `zparca-${Date.now()}` },
  );
  expect(res.status(), await res.text()).toBe(201);
  const capture = (await res.json()).data as Capture;
  await tp.dispose();

  expect(capture.parts?.map((p) => p.part_no)).toEqual([1, 2]);
  expect(capture.parts?.[1].file_name).toBe("z-parca-2.png");

  // Ofis tek yakalamayı parçalarıyla görür; parça 2'nin görseli gelir.
  const got = await office.json<Capture>(
    await office.get(`${base()}/receipts/${capture.uuid}`),
  );
  expect(got.uuid).toBe(capture.uuid);
  const img = await office.get(`${base()}/receipts/${capture.uuid}/image?part=2`);
  expect(img.status()).toBe(200);
  expect(img.headers()["content-type"]).toBe("image/png");
  expect(
    (await office.get(`${base()}/receipts/${capture.uuid}/image?part=3`)).status(),
  ).toBe(404);
  expect(
    (await office.get(`${base()}/receipts/${capture.uuid}/image?part=9`)).status(),
  ).toBe(400);

  if (capture.status === "draft" && capture.draft?.type === "retail_sale") {
    // AI sahtesiyle: tek Z raporu, iki parça.
    const sale = await office.json<RetailSale>(
      await office.get(`${base()}/retail-sales/${capture.draft.uuid}`),
    );
    expect(sale.receipt_uuid).toBe(capture.uuid);
    expect(sale.part_count).toBe(2);
    const list = await office.json<{ items: RetailSale[] }>(
      await office.get(`${base()}/retail-sales?limit=100`),
    );
    expect(list.items.filter((x) => x.receipt_uuid === capture.uuid)).toHaveLength(1);
  } else {
    test.info().annotations.push({
      type: "not",
      description: `Yapay zekâ bu ortamda yok (yükleme: ${capture.status}); Z raporu taslağı denetlenmedi.`,
    });
    expect(capture.status).toBe("failed");
  }
});

test("ofis web'den parçalı Z raporu yükler: Parça 1/2 sırası, tek yükleme", async ({
  page,
}) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/z-reports`));
  await expect(page.getByRole("heading", { name: "Z raporları" })).toBeVisible();

  await page.getByTestId("zreport-parts-open").click();
  const panel = page.getByTestId("zreport-parts-upload");
  await expect(panel).toBeVisible();
  await page.getByTestId("zreport-parts-input").setInputFiles([
    png("ust.png", "ust"),
    png("alt.png", "alt"),
  ]);
  await expect(page.getByTestId("zreport-parts-item-1")).toContainText("Parça 1");
  await expect(page.getByTestId("zreport-parts-item-1")).toContainText("ust.png");
  // Sıra değiştirilir: alt.png parça 1 olur.
  await page
    .getByTestId("zreport-parts-item-2")
    .getByRole("button", { name: "2. parçayı yukarı taşı" })
    .click();
  await expect(page.getByTestId("zreport-parts-item-1")).toContainText("alt.png");

  const posted = page.waitForResponse(
    (r) => r.url().endsWith(`${base()}/receipts`) && r.request().method() === "POST",
  );
  await page.getByTestId("zreport-parts-submit").click();
  const res = await posted;
  expect(res.status()).toBe(201);
  const capture = (await res.json()).data as Capture;
  expect(capture.kind).toBe("z_report");
  expect(capture.parts?.map((p) => [p.part_no, p.file_name])).toEqual([
    [1, "alt.png"],
    [2, "ust.png"],
  ]);
  await expect(panel).toHaveCount(0);

  if (capture.status === "draft" && capture.draft?.type === "retail_sale") {
    // AI sahtesiyle: listede "2 parça", yan panelde parçalar sırasıyla.
    await expect(page.getByTestId("zreport-part-count").first()).toContainText(
      "2 parça",
      { timeout: 15_000 },
    );
  }
});

test("ofis ayarı kapalıyken çok fotoğraf 409, tek fotoğraf çalışır; web'de bölüm gizlenir", async ({
  page,
}) => {
  try {
    await office.json(await office.put(SETTINGS, { enabled: false }));
    const many = await office.postForm(`${base()}/receipts`, {
      image: png("a.png", "kapali-1"),
      image_2: png("b.png", "kapali-2"),
      kind: "z_report",
    });
    expect(many.status()).toBe(409);
    expect((await many.json()).error.code).toBe("Z_REPORT_PARTS_OFF");

    const one = await office.postForm(`${base()}/receipts`, {
      image: png("tek.png", "kapali-tek"),
      kind: "z_report",
    });
    expect(one.status(), await one.text()).toBe(201);
    expect(((await one.json()).data as Capture).parts ?? []).toEqual([]);

    // Ayarlar → Belgeler'de anahtar kapalı görünür; Z raporları sayfasında
    // parçalı yükleme düğmesi yoktur.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/settings"));
    await expect(page.getByTestId("z-parts-setting")).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await page.goto(officePath(`/engagements/${engagement}/z-reports`));
    await expect(page.getByRole("heading", { name: "Z raporları" })).toBeVisible();
    await expect(page.getByTestId("zreport-parts-open")).toHaveCount(0);
  } finally {
    await office.put(SETTINGS, { enabled: true });
  }
});
