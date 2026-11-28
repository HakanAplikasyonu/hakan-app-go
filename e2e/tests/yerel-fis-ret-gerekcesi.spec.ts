import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Fiş ret gerekçesi (#780): mükellef fişi telefondan gönderir; ofis panelde
 * Masraflar ekranından reddederken gerekçe penceresinde hazır gerekçe seçer
 * ve açıklama yazar ("Diğer" açıklamasız gönderilemez). Mükellef tarafında
 * Gönderdiklerim (`GET /v1/taxpayer/submissions`) gerekçeyi döner ve
 * "Fişiniz kabul edilmedi" bildirimi gerekçeyi söyler. API'de "diger"
 * açıklamasız 400 alır.
 *
 * Veri: demo ofisine her koşuda yeni mükellef ve kullanıcısı; uydurma PNG
 * başlıklı fotoğraf. AI açıksa fiş masraf taslağı olur (satırdaki Reddet),
 * kapalıysa okunamayan fişlerde kapatılır; ikisi de aynı gerekçe penceresini açar.
 */
test.describe.configure({ mode: "serial" });

type Submission = { uuid: string; status: string; reason: string; reason_code: string };
type SubmissionList = { items: Submission[] };
type Capture = { uuid: string; status: string; draft: { type: string; uuid: string } | null };
type Notification = { title: string; body: string; payload?: Record<string, unknown> };

const taxpayer = { vkn: uniqueVkn(), title: `E2E Fiş Ret ${Date.now().toString().slice(-6)} Ltd.` };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "fisret");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office.dispose();
});

test("ofis gerekçeyle reddeder, mükellef gerekçeyi Gönderdiklerim'de görür (#780)", async ({ page }) => {
  test.setTimeout(180_000);
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };

  const res = await tp.postForm(
    "/v1/taxpayer/receipts",
    {
      image: {
        name: "fis.png",
        mimeType: "image/png",
        buffer: Buffer.concat([Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"), Buffer.from(`fis-ret-${Date.now()}`)]),
      },
      kind: "expense_receipt",
    },
    { ...headers, "Idempotency-Key": `fisret-${Date.now()}` },
  );
  expect(res.status(), await res.text()).toBe(201);
  const capture = (await res.json()).data as Capture;

  // API: "diger" açıklamasız ve bilinmeyen kod 400; fiş reddedilmez.
  const path = `/v1/tenant/engagements/${engagement}/receipts/${capture.uuid}/reject`;
  for (const body of [{ reason_code: "diger" }, { reason_code: "bilinmeyen" }]) {
    const bad = await office.post(path, body);
    expect(bad.status(), await bad.text()).toBe(400);
  }

  // Panel: Masraflar ekranında Reddet → gerekçe penceresi.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/expenses`));
  if (capture.status === "draft" && capture.draft?.type === "expense") {
    await page.getByTestId(`expense-reject-${capture.draft.uuid}`).click();
  } else {
    await page.getByTestId("unreadable-close").first().click();
  }
  const dialog = page.getByTestId("reject-reason-dialog");
  await expect(dialog).toBeVisible();
  // "Diğer" açıklamasız: uyarı, pencere açık kalır.
  await dialog.getByTestId("reject-reason-diger").click();
  await dialog.getByTestId("reject-reason-confirm").click();
  await expect(dialog.getByText("Diğer seçildiğinde açıklama yazın.")).toBeVisible();
  // Hazır gerekçe ve açıklama.
  await dialog.getByTestId("reject-reason-eksik_bilgi").click();
  await dialog.getByTestId("reject-reason-text").fill("Tarih görünmüyor");
  await dialog.getByTestId("reject-reason-confirm").click();
  await expect(dialog).toBeHidden();

  // Mükellef: Gönderdiklerim gerekçeyi döner.
  await expect
    .poll(
      async () =>
        (await tp.json<SubmissionList>(await tp.get("/v1/taxpayer/submissions", headers))).items.find((i) => i.uuid === capture.uuid)
          ?.status,
      { timeout: 30_000 },
    )
    .toBe("rejected");
  const after = await tp.json<SubmissionList>(await tp.get("/v1/taxpayer/submissions", headers));
  const row = after.items.find((i) => i.uuid === capture.uuid)!;
  expect(row.reason_code).toBe("eksik_bilgi");
  expect(row.reason).toBe("Bilgi eksik (VKN, tarih ya da tutar): Tarih görünmüyor");

  // "Fişiniz kabul edilmedi" bildirimi gerekçeyi söyler.
  const tpNotices = async () =>
    (await tp.json<{ items: Notification[] }>(await tp.get("/v1/taxpayer/notifications?limit=50", headers))).items.filter(
      (n) => n.payload?.receipt_uuid === capture.uuid,
    );
  await expect.poll(async () => (await tpNotices()).length, { timeout: 30_000 }).toBe(1);
  const [decision] = await tpNotices();
  expect(decision.title).toBe("Fişiniz kabul edilmedi");
  expect(decision.body).toContain("Gerekçe: Bilgi eksik (VKN, tarih ya da tutar): Tarih görünmüyor.");
  expect(decision.payload?.reason_code).toBe("eksik_bilgi");
  await tp.dispose();
});
