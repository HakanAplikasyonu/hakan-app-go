import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Senaryo B kabul testi (#780): mükellef telefonda fişi çekip gönderir.
 * Mobil uygulamanın kullandığı uçlarla: fiş `POST /v1/taxpayer/receipts`
 * ile gider; Gönderdiklerim (`GET /v1/taxpayer/submissions`) fişi ofisin
 * adıyla "received" gösterir; ofise uygulama içi bildirim gider; fiş ofis
 * panelinde onay listesinde görünür (AI açıksa mükellefin Gelen kutusunda
 * masraf olarak, AI kapalıysa Masraflar'daki okunamayan fişlerde); ofis
 * reddedince Gönderdiklerim "rejected" olur ve mükellefe bildirim gider.
 *
 * Veri: demo ofisine her koşuda yeni mükellef ve kullanıcısı; uydurma PNG
 * başlıklı fotoğraf.
 */
test.describe.configure({ mode: "serial" });

type Submission = { uuid: string; source: string; status: string; office_name: string; kind: string };
type SubmissionList = { office_name: string; items: Submission[] };
type Capture = { uuid: string; status: string; draft: { type: string; uuid: string } | null };
type Notification = { title: string; body: string; action_url?: string | null; payload?: Record<string, unknown> };

const taxpayer = { vkn: uniqueVkn(), title: `E2E Mobil Fiş ${Date.now().toString().slice(-6)} Ltd.` };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "mobilfis");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office.dispose();
});

test("mobil fiş ofisin onay listesine düşer, ofis bildirimi alır; ret mükellefe döner (#780 Senaryo B)", async ({ page }) => {
  test.setTimeout(180_000);
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };

  // Gönderim onayı ekranının ofis adı.
  const before = await tp.json<SubmissionList>(await tp.get("/v1/taxpayer/submissions", headers));
  expect(before.office_name).not.toBe("");
  expect(before.items).toHaveLength(0);

  // 1-2) Fotoğraf çekilir ve gönderilir.
  const res = await tp.postForm(
    "/v1/taxpayer/receipts",
    {
      image: {
        name: "fis.png",
        mimeType: "image/png",
        buffer: Buffer.concat([Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"), Buffer.from(`mobil-fis-kabul-${Date.now()}`)]),
      },
      kind: "expense_receipt",
    },
    { ...headers, "Idempotency-Key": `mobilfis-${Date.now()}` },
  );
  expect(res.status(), await res.text()).toBe(201);
  const capture = (await res.json()).data as Capture;

  // 3) Gönderdiklerim: ofise ulaştı (AI kapalıysa okunamadı), ofisin adıyla.
  const mine = await tp.json<SubmissionList>(await tp.get("/v1/taxpayer/submissions", headers));
  const row = mine.items.find((i) => i.uuid === capture.uuid);
  expect(row).toBeTruthy();
  expect(row).toMatchObject({ source: "receipt", office_name: before.office_name, kind: "expense_receipt" });
  expect(["received", "unreadable"]).toContain(row!.status);

  // 4a) Ofise uygulama içi bildirim (mobil push'a da yansır).
  const officeNotices = async () =>
    (await office.json<{ items: Notification[] }>(await office.get("/v1/notifications?limit=50&channel=inapp"))).items.filter(
      (n) => n.payload?.receipt_uuid === capture.uuid,
    );
  await expect.poll(async () => (await officeNotices()).length, { timeout: 30_000 }).toBe(1);
  const [notice] = await officeNotices();
  expect(notice.title).toMatch(/^Mükelleften masraf fişi geldi$/);
  expect(notice.body).toContain(taxpayer.title);
  expect(notice.payload?.kind).toBe("receipt_submitted");

  // 4b) Ofis panelinde onay listesi.
  const receipts = await office.json<{ items: { uuid: string }[] }>(
    await office.get(`/v1/tenant/engagements/${engagement}/receipts?limit=100`),
  );
  expect(receipts.items.map((i) => i.uuid)).toContain(capture.uuid);
  await uiLogin(page, DEMO.owner);
  if (capture.status === "draft" && capture.draft?.type === "expense") {
    const expense = await office.json<{ period: string | null }>(
      await office.get(`/v1/tenant/engagements/${engagement}/expenses/${capture.draft.uuid}`),
    );
    const period = expense.period!;
    const inbox = await office.json<{ items: { uuid: string; type: string }[] }>(
      await office.get(`/v1/tenant/inbox?period=${period}&engagement=${engagement}&type=expense`),
    );
    expect(inbox.items.map((i) => i.uuid)).toContain(capture.draft.uuid);
    await page.goto(officePath(`/engagements/${engagement}/review?view=inbox&period=${period}`));
    await expect(page.getByTestId("inbox-table").locator('tbody tr[data-type="expense"]')).toHaveCount(1);
  } else {
    // AI kapalı: fiş okunamayan fişler bölümünde, ofis bakıp yenisini ister.
    await page.goto(officePath(`/engagements/${engagement}/expenses`));
    await expect(page.getByText(/Okunamayan fiş fotoğrafları \(1\)/)).toBeVisible();
  }

  // Ofis reddeder → Gönderdiklerim "rejected", mükellefe bildirim.
  const rejected = await office.post(`/v1/tenant/engagements/${engagement}/receipts/${capture.uuid}/reject`);
  expect(rejected.status(), await rejected.text()).toBe(200);
  const after = await tp.json<SubmissionList>(await tp.get("/v1/taxpayer/submissions", headers));
  expect(after.items.find((i) => i.uuid === capture.uuid)?.status).toBe("rejected");
  const tpNotices = async () =>
    (await tp.json<{ items: Notification[] }>(await tp.get("/v1/taxpayer/notifications?limit=50", headers))).items.filter(
      (n) => n.payload?.receipt_uuid === capture.uuid,
    );
  await expect.poll(async () => (await tpNotices()).length, { timeout: 30_000 }).toBe(1);
  const [decision] = await tpNotices();
  expect(decision.title).toBe("Fişiniz kabul edilmedi");
  expect(decision.action_url).toBe(`/m/${taxpayerUuid}/gonderdiklerim`);

  // Başka şirketin kullanıcısı bu listeye ulaşamaz.
  const other = await tp.get("/v1/taxpayer/submissions", { "X-Taxpayer": "00000000-0000-4000-8000-000000000000" });
  expect(other.status()).toBe(403);
  await tp.dispose();
});
