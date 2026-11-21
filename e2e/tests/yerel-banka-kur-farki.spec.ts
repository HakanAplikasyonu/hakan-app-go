import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Kur farkı (#565): USD tahsilatı USD faturasını kapatınca müşavir hareketin
 * panelinde ödeme günü kurunu girer. Kapanan tutar × (ödeme kuru − fatura
 * kuru) TL farkı kârsa 646, zararsa 656 olur ve eşleşmede durur. Kur
 * kaldırılınca fark da kalkar; TL hareketinde kur farkı yoktur (409).
 *
 * Veri: demo ofisine her koşuda yeni mükellef. Atlas'ın USD satış faturası
 * türetilir (ATL2026000000106: 1.199,88 USD, kur 34,5678). Yeni IBAN'lı USD
 * hesabına aynı tutarlı Ekim tahsilatı MT940 ile yüklenir. 35,0000 kurla
 * fark 1.199,88 × 0,4322 = 518,59 TL kâr (646).
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kur Farkı ${run} A.Ş.` };

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `00010${"0"}${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Fx = { rate_payment: string | null; diff_try: string | null; account_code: string | null; missing_invoice_rate: boolean };
type Matching = { currency: string; matches: { invoice_number: string }[]; fx: Fx | null };

test("USD tahsilatında ödeme günü kuru girilir, kur farkı 646'ya yazılır (#565)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kurfarki");
  const base = `/v1/tenant/engagements/${engagement}`;

  const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-06-doviz-usd.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);
  const invoices = (await api.json<{ items: { uuid: string; invoice_number: string; currency: string }[] }>(
    await api.get(`${base}/invoices?limit=10`),
  )).items;
  const usd = invoices.find((i) => i.invoice_number === "ATL2026000000106")!;
  expect(usd?.currency).toBe("USD");

  const iban = uniqueIban();
  await api.json(await api.post(`${base}/cash-boxes`, { iban, name: "E2E USD", currency: "USD" }));
  const statement = [
    ":20:E2E565KUR",
    `:25:${iban}`,
    ":60F:C261001USD0,00",
    ":61:2610151015C1199,88NTRFNONREF//K1",
    ":86:EFT GELEN ATL2026000000106",
    ":61:2610161016C100,00NTRFNONREF//K2",
    ":86:EFT GELEN E2E",
    ":62F:C261016USD1299,88",
    "",
  ].join("\r\n");
  const st = await api.upload(`${base}/bank-statements`, {
    name: "kur-farki-2026-10.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(st.status(), await st.text()).toBeLessThan(300);
  const txs = (await api.json<{ items: { uuid: string; amount: string; currency: string }[] }>(
    await api.get(`${base}/bank-transactions?period=2026-10&limit=10`),
  )).items;
  const collection = txs.find((x) => x.amount === "1199.88")!;
  const other = txs.find((x) => x.amount === "100.00")!;
  expect(collection?.currency).toBe("USD");
  const tx = `${base}/bank-transactions/${collection.uuid}`;
  const matched = await api.put(`${tx}/matches`, { invoice_uuids: [usd.uuid] });
  expect(matched.status(), await matched.text()).toBe(200);
  const before = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(before.fx).toEqual({ rate_payment: null, diff_try: null, account_code: null, missing_invoice_rate: false });

  // API: faturası olmayan hareketin kur farkı yok; geçersiz kur 400.
  expect((await api.put(`${base}/bank-transactions/${other.uuid}/fx-rate`, { rate: "35" })).status()).toBe(409);
  expect((await api.put(`${tx}/fx-rate`, { rate: "-1" })).status()).toBe(400);

  // Ekran: hareketin panelinde kuru gir, kâr 646'ya yazılır.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection.uuid}`));
  const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
  const section = panel.getByTestId("bank-transaction-fx");
  await expect(section).toContainText("Kur farkı");
  await section.getByLabel("Ödeme günü kuru (1 USD = TL)").fill("35,0000");
  await section.getByRole("button", { name: "Kur farkını hesapla" }).click();
  const diff = section.getByTestId("bank-transaction-fx-diff");
  await expect(diff).toContainText("Kur farkı kârı:");
  await expect(diff).toContainText("518,59");
  await expect(diff).toContainText("646");

  const after = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(after.fx).toMatchObject({ diff_try: "518.59", account_code: "646" });
  expect(Number(after.fx!.rate_payment)).toBe(35);

  // Kur düşükse zarar (656); kaldırılınca fark kalkar.
  const loss = await api.json<Matching>(await api.put(`${tx}/fx-rate`, { rate: "34.0000" }));
  expect(loss.fx).toMatchObject({ diff_try: "-681.29", account_code: "656" });
  await page.reload();
  await expect(section.getByTestId("bank-transaction-fx-diff")).toContainText("Kur farkı zararı:");
  await section.getByRole("button", { name: "Kuru kaldır" }).click();
  await expect(section.getByLabel("Ödeme günü kuru (1 USD = TL)")).toBeVisible();
  expect((await api.json<Matching>(await api.get(`${tx}/matching`))).fx).toMatchObject({ diff_try: null, rate_payment: null });
  await api.dispose();
});
