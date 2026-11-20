import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Uyarı İncele paneli işlem bitince sıradakine geçer (#570): panelde
 * hareket faturayla eşleştirilince uyarı kendiliğinden kapanır, "Bu uyarı
 * kapandı." kısa süre görünür ve panel kendiliğinden kalan uyarıya geçer.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın ATL…102 faturası
 * türetilir; Ziraat ekstresi (faturası olmayan 5.400,00 tahsilat → "Faturası
 * olmayan tahsilat") ve açılışı öncekinin kapanışını tutmayan ikinci bir
 * ekstre (→ bakiye tutmazlığı) yüklenir. Böylece dönemde iki açık uyarı olur.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Sıradaki Uyarı ${run} A.Ş.` };
const PERIOD = "2026-10";
const IBAN = "TR760001000519786457841326";

// Atlas ekstresi 6.387,50 ile kapanır (6 Ekim); bu ekstre 7.000,00 ile açılır.
const gap = Buffer.from(
  [
    `:20:SIRADAKI${run}`,
    `:25:${IBAN}`,
    ":60F:C261020TRY7000,00",
    ":61:2610211021D100,00NTRFNONREF//S1",
    `:86:EFT GIDEN SIRADAKI ${run}`,
    ":62F:C261021TRY6900,00",
  ].join("\n") + "\n",
  "utf8",
);

type WarningList = { items: { uuid: string; code: string }[]; total: number };

test("panelde eşleştirilen uyarı kapanır, panel kendiliğinden kalan uyarıya geçer (#570)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "sirad");
  const base = `/v1/tenant/engagements/${engagement}`;
  const open = async () =>
    api.json<WarningList>(await api.get(`${base}/warnings?status=open&period=${PERIOD}&limit=20`));

  const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  await api.json(await api.upload(`${base}/bank-statements`, { name: `sirad-${run}.sta`, mimeType: "text/plain", buffer: gap }));
  const match = await api.json<{ findings: string[] }>(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  expect(match.findings).toContain("BNK_UNINVOICED_COLLECTION");
  expect(match.findings).toContain("BNK_BALANCE_MISMATCH");
  const before = await open();
  expect(before.total).toBeGreaterThanOrEqual(2);
  const uninvoiced = before.items.find((w) => w.code === "BNK_UNINVOICED_COLLECTION");
  expect(uninvoiced).toBeTruthy();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();
  const sheet = page.getByTestId("warning-sheet");
  await expect(sheet).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`uyari=${uninvoiced!.uuid}`));

  const panel = sheet.getByTestId("warning-transactions").getByTestId("bank-transaction-matching");
  await panel.getByRole("checkbox", { name: "ATL2026000000102 faturasını seç" }).check();
  await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();

  // Kapandı bildirimi, sonra kendiliğinden kalan uyarıya geçiş.
  await expect(sheet.getByText("Bu uyarı kapandı.")).toBeVisible();
  await expect(sheet.getByTestId("warning-auto-next")).toBeVisible();
  await expect(page).not.toHaveURL(new RegExp(`uyari=${uninvoiced!.uuid}`), { timeout: 10_000 });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByText("Bu uyarı kapandı.")).toHaveCount(0);

  // Veritabanı: eşleştirilen uyarı kapalı, öteki açık kalır.
  const after = await open();
  expect(after.items.some((w) => w.uuid === uninvoiced!.uuid)).toBe(false);
  expect(after.items.some((w) => w.code === "BNK_BALANCE_MISMATCH")).toBe(true);
  await api.dispose();
});
