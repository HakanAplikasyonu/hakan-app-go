import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * İncele paneli: uyarı kaydın kendisinde çözülünce panel sıradakine
 * kendiliğinden geçer (#570).
 *
 * "Çözüldü / Gördüm" kararı paneli zaten sıradakine geçiriyordu; uyarı kayıt
 * üzerinde çözüldüğünde (hareket panelde eşleşir) ise panel "Bu uyarı
 * kapandı." deyip bekliyordu. Bu dosya:
 *
 *   fatura + ekstre + eşleme (API) → Uyarılar → "Faturası olmayan tahsilat"
 *   → İncele → hareketi faturayla eşleştir → "Bu uyarı kapandı." → panel
 *   kendiliğinden kalan bir uyarıya geçer, sayaç bir düşer → veritabanında
 *   eşleşme ve kapanan uyarı.
 *
 * Veri: her koşuda yeni mükellef; Atlas'ın ATL…102 faturası (5.400,00) ve
 * Ziraat ekstresi (fixtures/atlas-ziraat-2026-10.sta). Başka dosyanın
 * kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Sıradaki Uyarı ${run} A.Ş.` };
const PERIOD = "2026-10";
const UNINVOICED = "BNK_UNINVOICED_COLLECTION";

type Warning = { uuid: string; code: string; status: string };

let api: Api;
let engagement = "";
let collection = "";

const base = () => `/v1/tenant/engagements/${engagement}`;
const openWarnings = async () =>
  api.json<{ items: Warning[]; total: number }>(await api.get(`${base()}/warnings?status=open&limit=200`));

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "siradaki");

  const up = await api.upload(`${base()}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);

  await api.json(await api.post(`${base()}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base()}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  const match = await api.json<{ findings: string[] }>(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
  expect(match.findings).toContain(UNINVOICED);
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base()}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;
});

test.afterAll(async () => {
  await api?.dispose();
});

test("hareket panelde eşleşince uyarı kapanır ve panel sıradakine geçer", async ({ page }) => {
  const before = await openWarnings();
  const uninvoiced = before.items.find((w) => w.code === UNINVOICED);
  expect(uninvoiced, "faturası olmayan tahsilat uyarısı").toBeTruthy();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();
  const sheet = page.getByTestId("warning-sheet");
  await expect(sheet).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`uyari=${uninvoiced!.uuid}`));
  const position = sheet.getByText(/^\d+ \/ \d+$/);
  await expect(position).toBeVisible();
  // Panelin listesindeki uyarı sayısı (ekrandaki süzgece göre).
  const listed = Number((await position.innerText()).split("/")[1].trim());
  expect(listed, "panelde geçilecek başka uyarı").toBeGreaterThanOrEqual(2);

  const panel = sheet.getByTestId("warning-transactions").getByTestId("bank-transaction-matching");
  await panel.getByRole("checkbox", { name: "ATL2026000000102 faturasını seç" }).check();
  await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();

  // Önce kapandığı söylenir, sonra panel kendiliğinden kalan bir uyarıyı açar.
  await expect(sheet.getByText("Bu uyarı kapandı.")).toBeVisible();
  await expect(sheet.getByText("Sıradaki uyarı açılıyor…")).toBeVisible();
  await expect(page).not.toHaveURL(new RegExp(`uyari=${uninvoiced!.uuid}`), { timeout: 10_000 });
  await expect(sheet.getByText("Bu uyarı kapandı.")).toHaveCount(0);
  await expect(position).toHaveText(new RegExp(`/ ${listed - 1}$`));

  // Açılan uyarı kalan açık uyarılardan biri; kapanan yok.
  const after = await openWarnings();
  expect(after.total).toBe(before.total - 1);
  expect(after.items.some((w) => w.uuid === uninvoiced!.uuid)).toBe(false);
  const opened = new URL(page.url()).searchParams.get("uyari");
  expect(after.items.map((w) => w.uuid)).toContain(opened);

  // Veritabanı: hareket faturaya bağlı.
  const m = await api.json<{ match_status: string; matches: { invoice_number: string }[] }>(
    await api.get(`${base()}/bank-transactions/${collection}/matching`),
  );
  expect(m.match_status).toBe("matched");
  expect(m.matches.map((x) => x.invoice_number)).toEqual(["ATL2026000000102"]);
});
