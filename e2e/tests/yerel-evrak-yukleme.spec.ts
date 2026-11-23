import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Evrak yüklerken tür ve dönem seçimi (Oğuz'un demo geri bildirimi): ofis
 * kullanıcısı mükellefin Yüklemeler sayfasında türü seçer, belge tarihini
 * yazar; ay tarihten önerilir. Kilitli Eylül 2026'ya giden evrakta üç
 * seçenek sorulur: ekranda "Geçmiş döneme düzeltme olarak işle" seçilir ve
 * düzeltme beyannamesi uyarısı görünür. API ile "Seçilen aya uygula" ve
 * "Reddet" de kaydedilir; seçimsiz yükleme 409 PERIOD_LOCKED alır. Ayar
 * kapalıyken yükleme yapılmaz.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; dönem kilidi API ile.
 * Ofis ayarı test sonunda eski hâline döner.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Evrak Donem ${run} A.Ş.` };
const LOCKED = "2026-09";
const TARGET = "2026-11";

type Upload = {
  uuid: string;
  doc_type: string;
  period: string;
  requested_period: string;
  period_source: string;
  lock_choice: string | null;
  locked_period: string | null;
  correction_needed: boolean;
  status: string;
};

const pdf = (tag: string) => ({
  name: `evrak-${tag}.pdf`,
  mimeType: "application/pdf",
  buffer: Buffer.from(`%PDF-1.4\n% evrak ${run} ${tag}\n%%EOF\n`),
});

test("evrak yüklemede tür ve dönem seçilir; kilitli ayda üç seçenek kaydedilir", async ({
  browser,
  page,
}) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "evrakdonem",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const uploads = `${base}/document-uploads`;
  const lock = await api.post(`${base}/periods/${LOCKED}/lock`);
  expect(lock.status(), await lock.text()).toBe(200);
  const before = await api.json<{ enabled: boolean }>(
    await api.get("/v1/tenant/document-upload-settings"),
  );
  try {
    const on = await api.put("/v1/tenant/document-upload-settings", {
      enabled: true,
    });
    expect(on.status(), await on.text()).toBe(200);

    // Ekran: tür ve belge tarihi seçilir, ay tarihten önerilir.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/upload`));
    const panel = page.getByTestId("doc-upload-panel");
    await expect(panel).toBeVisible();
    await panel.getByTestId("doc-upload-type").selectOption("other");
    await panel.getByTestId("doc-upload-date").fill(`${LOCKED}-15`);
    await expect(panel.getByTestId("doc-upload-period")).toHaveValue(LOCKED);
    await expect(panel.getByTestId("doc-upload-period-source")).toHaveText(
      "Belge tarihinden önerildi.",
    );
    await panel.getByTestId("doc-upload-input").setInputFiles(pdf("ekran"));
    await panel.getByTestId("doc-upload-submit").click();

    // Kilitli ay: üç seçenek; düzeltme seçilince uyarı görünür.
    const locked = panel.getByTestId("doc-upload-locked");
    await expect(locked).toBeVisible();
    await expect(locked.getByText("Reddet", { exact: true })).toBeVisible();
    await expect(
      locked.getByText("Seçilen aya uygula", { exact: true }),
    ).toBeVisible();
    await locked.getByTestId("doc-upload-choice-correction").click();
    await expect(
      panel.getByTestId("doc-upload-correction-warning"),
    ).toContainText("düzeltme beyannamesi");
    await panel.getByTestId("doc-upload-submit").click();
    const result = panel.getByTestId("doc-upload-result");
    await expect(result).toContainText("düzeltme olarak işlendi");
    await expect(panel.getByTestId("doc-upload-list")).toContainText(
      "evrak-ekran.pdf",
    );

    // Uç: seçimsiz kilitli ay 409, seçilen aya uygula ve reddet kaydedilir.
    const form = (tag: string, extra: Record<string, string>) =>
      api.postForm(uploads, {
        file: pdf(tag),
        doc_type: "other",
        period: LOCKED,
        ...extra,
      });
    const blocked = await form("secimsiz", {});
    expect(blocked.status(), await blocked.text()).toBe(409);
    expect(await blocked.text()).toContain("PERIOD_LOCKED");

    const moved = await api.json<Upload>(
      await form("hedef", {
        lock_choice: "target_period",
        target_period: TARGET,
      }),
    );
    expect(moved).toMatchObject({
      period: TARGET,
      requested_period: LOCKED,
      lock_choice: "target_period",
      locked_period: LOCKED,
      status: "processed",
    });
    const sameMonth = await form("ayni", {
      lock_choice: "target_period",
      target_period: LOCKED,
    });
    expect(sameMonth.status(), await sameMonth.text()).toBe(409);

    const rejected = await api.json<Upload>(
      await form("red", { lock_choice: "reject" }),
    );
    expect(rejected).toMatchObject({
      status: "rejected",
      lock_choice: "reject",
      period: LOCKED,
    });

    // Açık ay: belge tarihi ayı önerir, seçim sorulmaz.
    const open = await api.json<Upload>(
      await api.postForm(uploads, {
        file: pdf("acik"),
        doc_type: "bank_statement",
        period: TARGET,
        period_auto: "true",
        document_date: "2026-08-10",
      }),
    );
    expect(open).toMatchObject({
      period: "2026-08",
      period_source: "document_date",
      lock_choice: null,
      status: "processed",
    });

    // Z raporu PDF olamaz: fotoğraftan okunur.
    const zpdf = await api.postForm(uploads, {
      file: pdf("z"),
      doc_type: "z_report",
      period: TARGET,
    });
    expect(zpdf.status(), await zpdf.text()).toBe(415);

    const list = await api.json<{ items: Upload[] }>(
      await api.get(`${uploads}?period=${LOCKED}`),
    );
    const choices = list.items.map((u) => u.lock_choice).sort();
    expect(choices).toEqual(["correction", "reject"]);
    expect(list.items.find((u) => u.lock_choice === "correction")).toMatchObject(
      { correction_needed: true },
    );

    // Ayar kapalı: bölüm gizlenir, yükleme yapılmaz.
    const off = await api.put("/v1/tenant/document-upload-settings", {
      enabled: false,
    });
    expect(off.status(), await off.text()).toBe(200);
    const refused = await form("kapali", { lock_choice: "correction" });
    expect(refused.status(), await refused.text()).toBe(409);
    expect(await refused.text()).toContain("DOCUMENT_UPLOADS_OFF");
    await page.reload();
    await expect(page.getByTestId("ubl-input")).toBeAttached();
    await expect(page.getByTestId("doc-upload-panel")).toHaveCount(0);
  } finally {
    await api.put("/v1/tenant/document-upload-settings", {
      enabled: before.enabled,
    });
    await api.dispose();
  }
});
