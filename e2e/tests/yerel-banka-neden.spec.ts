import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Banka eşleştirmede "neden önerildi" görünür, alakasız aday çıkmaz (#710).
 *
 * - Aday ve önerilen/otomatik eşleşmelerin puan parçaları API'de gelir;
 *   ekranda Türkçe cümle olarak yazılır ("açıklamada fatura numarası geçiyor,
 *   tutar birebir aynı").
 * - Hiçbir aday yalnız tarih penceresine düştüğü için listelenmez: her aday
 *   karşı tarafla (VKN, IBAN, fatura no, ad) ya da tutarla ilişkilidir.
 * - Elle eşleşmede neden satırı çizilmez.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Atlas'ın iki satış
 * faturası (ATL…101: 492,00; ATL…102: 5.400,00) ve Ziraat Ekim ekstresi.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka Neden ${run} A.Ş.` };
const PERIOD = "2026-10";
const RELEVANT = ["tax_id", "iban", "invoice_number", "name", "amount_exact", "amount_less_fee"];

type Cand = { invoice_number: string; parts: Record<string, number> };
type Matching = {
  matchable: boolean;
  matches: { invoice_number: string; method: string; parts: Record<string, number> }[];
  candidates: Cand[];
};

test("neden önerildi Türkçe görünür; alakasız aday listelenmez (#710)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "neden");
  const base = `/v1/tenant/engagements/${engagement}`;

  for (const f of ["atlas-01-temel-satis.xml", "atlas-02-ticari-satis.xml"]) {
    const up = await api.upload(`${base}/ubl-uploads`, deriveUbl(f, taxpayer.vkn, taxpayer.title));
    expect(up.status(), await up.text()).toBeLessThan(300);
  }
  await waitForUploads(api, engagement, 2);
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=50`),
  );
  expect(txs.items.length).toBeGreaterThan(0);

  // API: her aday ilişkili; otomatik/önerilen eşleşmenin nedeni var.
  for (const x of txs.items) {
    const m = await api.json<Matching>(await api.get(`${base}/bank-transactions/${x.uuid}/matching`));
    for (const c of m.candidates) {
      expect(
        Object.keys(c.parts).some((k) => RELEVANT.includes(k)),
        `${x.amount} hareketinin adayı ${c.invoice_number}: ${JSON.stringify(c.parts)}`,
      ).toBe(true);
    }
    for (const mm of m.matches) {
      if (mm.method !== "manual") expect(Object.keys(mm.parts).length, mm.invoice_number).toBeGreaterThan(0);
    }
  }

  const collection = txs.items.find((x) => x.amount === "5400.00")!;
  expect(collection).toBeTruthy();
  const tx = `${base}/bank-transactions/${collection.uuid}`;
  const before = await api.json<Matching>(await api.get(`${tx}/matching`));
  const shown = [...before.candidates, ...before.matches].find((c) => c.invoice_number === "ATL2026000000102");
  expect(shown, "ATL…102 aday ya da eşleşme").toBeTruthy();
  expect(shown!.parts).toHaveProperty("invoice_number");
  expect(shown!.parts).toHaveProperty("amount_exact");

  // Ekran: hareket panelinde neden Türkçe yazılır.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();
  const panel = page.getByTestId("warning-sheet").getByTestId("warning-transactions").getByTestId("bank-transaction-matching");
  await expect(panel).toContainText("EFT GELEN ATL2026000000102");
  const reason = panel.getByTestId("bank-match-reason").first();
  await expect(reason).toContainText("Neden önerildi:");
  await expect(reason).toContainText("açıklamada fatura numarası geçiyor");
  await expect(reason).toContainText("tutar birebir aynı");
  // Kod metni (amount_exact, tax_id …) görünmez.
  await expect(panel).not.toContainText("amount_exact");
  await expect(panel).not.toContainText("invoice_number");

  // Elle eşleşmede neden satırı yok.
  const put = await api.put(`${tx}/matches`, {
    invoice_uuids: [
      (await api.json<{ items: { uuid: string; invoice_number: string }[] }>(await api.get(`${base}/invoices?limit=10`))).items.find(
        (i) => i.invoice_number === "ATL2026000000102",
      )!.uuid,
    ],
  });
  expect(put.status(), await put.text()).toBe(200);
  const after = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(after.matches.map((m) => [m.method, Object.keys(m.parts).length])).toEqual([["manual", 0]]);
  await api.dispose();
});
