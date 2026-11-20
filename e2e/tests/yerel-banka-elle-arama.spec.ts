import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Banka hareketi için elle fatura araması tarih ve tutar süzgeciyle (#565):
 * fatura no ya da unvan bilinmeden, "şu tarihlerde, şu kadar" diye aranan
 * fatura bulunur ve hareketle eşleştirilir; yeniden eşleştirme eskisini
 * değiştirir.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın iki satış
 * faturası türetilir (ATL…101: 492,00; ATL…102: 5.400,00; ikisi de
 * 1 Ekim). Ziraat ekstresindeki 5.400,00 tahsilatın aday listesinde yalnız
 * ATL…102 vardır; ATL…101 ancak arama ile bulunur.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Elle Arama ${run} A.Ş.` };
const PERIOD = "2026-10";

type Matching = { match_status: string; matches: { invoice_number: string | null; amount: string }[] };

test("elle arama tarih ve tutarla faturayı bulur, eşleştirir; yeniden eşleştirme değiştirir (#565)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "elle");
  const base = `/v1/tenant/engagements/${engagement}`;

  for (const f of ["atlas-01-temel-satis.xml", "atlas-02-ticari-satis.xml"]) {
    const up = await api.upload(`${base}/ubl-uploads`, deriveUbl(f, taxpayer.vkn, taxpayer.title));
    expect(up.status(), await up.text()).toBeLessThan(300);
  }
  await waitForUploads(api, engagement, 2);
  const invoices = (await api.json<{ items: { uuid: string; invoice_number: string }[] }>(await api.get(`${base}/invoices?limit=10`))).items;
  const inv102 = invoices.find((i) => i.invoice_number === "ATL2026000000102")!;
  expect(inv102).toBeTruthy();

  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`));
  const collection = txs.items.find((x) => x.amount === "5400.00")!;
  expect(collection).toBeTruthy();
  const tx = `${base}/bank-transactions/${collection.uuid}`;

  // API: süzgeçler terimsiz çalışır; tutar aralığı dışı bulunmaz.
  const byRange = await api.json<{ items: { invoice_number: string }[] }>(
    await api.get(`${tx}/invoice-search?from=2026-09-25&to=2026-10-03&min=400&max=500`),
  );
  expect(byRange.items.map((i) => i.invoice_number)).toEqual(["ATL2026000000101"]);
  const none = await api.json<{ items: unknown[] }>(await api.get(`${tx}/invoice-search?min=6000`));
  expect(none.items).toHaveLength(0);
  expect((await api.get(`${tx}/invoice-search?from=01.10.2026`)).status()).toBe(400);

  // Ekran: uyarı panelindeki eşleştirmede tutar aralığıyla ara, eşleştir.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();
  const sheet = page.getByTestId("warning-sheet");
  const panel = sheet.getByTestId("warning-transactions").getByTestId("bank-transaction-matching");
  await expect(panel).toContainText("EFT GELEN ATL2026000000102");
  // Aday listesinde ATL…101 yok; ancak süzgeçle bulunur.
  await expect(panel.getByRole("checkbox", { name: "ATL2026000000101 faturasını seç" })).toHaveCount(0);
  const range = panel.getByTestId("bank-invoice-search-range");
  await range.getByLabel("En az tutar").fill("400");
  await range.getByLabel("En çok tutar").fill("500");
  await panel.getByRole("checkbox", { name: "ATL2026000000101 faturasını seç" }).check();
  await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();

  await expect
    .poll(async () => (await api.json<Matching>(await api.get(`${tx}/matching`))).matches.map((m) => m.invoice_number))
    .toEqual(["ATL2026000000101"]);

  // Yanlış fatura düzeltilir: yeniden eşleştirme eskisini değiştirir.
  const put = await api.put(`${tx}/matches`, { invoice_uuids: [inv102.uuid] });
  expect(put.status(), await put.text()).toBe(200);
  const after = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(after.match_status).toBe("matched");
  expect(after.matches.map((m) => m.invoice_number)).toEqual(["ATL2026000000102"]);
  await api.dispose();
});
