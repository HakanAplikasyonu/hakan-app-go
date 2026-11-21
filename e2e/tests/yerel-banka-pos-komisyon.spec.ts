import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * POS tahsilatında komisyonun netlenmesi (#565): banka gün sonunda brüt
 * tahsilattan komisyonu düşüp neti yatırır. Hareket panelinde "POS
 * komisyonu" bölümünde komisyon ve gider hesabı (653 / 780) girilir; panel
 * net, komisyon ve brütü gösterir. Banka fişinde banka net borç, komisyon
 * 653 borç, karşı hesap brüt (net + komisyon) alacak olur. Çıkış hareketine
 * komisyon girilemez; fişe girmiş hareketin komisyonu değişmez.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; 12 Kasım 2026'da
 * 9.750,00 TL POS gün sonu tahsilatı ve bir ödeme olan MT940 ekstresi API
 * ile yüklenir; tahsilat API ile 108.01'e işlenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `POS Komisyon ${run} A.Ş.` };
const IBAN = "TR760001000519786457841326";
const PERIOD = "2026-11";

const statement = [
  ":20:E2E565POS",
  `:25:${IBAN}`,
  ":60F:C261101TRY10000,00",
  ":61:2611121112C9750,00NTRFNONREF//S1",
  ":86:POS GUN SONU UYE ISYERI 12.11.2026",
  ":61:2611131113D500,00NTRFNONREF//S2",
  ":86:KIRA ODEMESI E2E",
  ":62F:C261113TRY19250,00",
  "",
].join("\r\n");

type Posting = {
  status: string;
  account_code: string;
  commission_amount: string | null;
  commission_account_code: string | null;
  gross_amount: string | null;
} | null;
type Line = { account: string; debit: string; credit: string };

test("POS tahsilatında komisyon netlenir: banka net, komisyon 653, karşı hesap brüt (#565)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "pos");
  const base = `/v1/tenant/engagements/${engagement}`;

  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  const up = await api.upload(`${base}/bank-statements`, {
    name: "pos-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const pos = txs.items.find((x) => x.amount === "9750.00")!;
  const rent = txs.items.find((x) => x.amount === "500.00")!;
  expect(pos && rent).toBeTruthy();

  // Hesap seçilmeden komisyon girilemez; çıkışa komisyon girilemez.
  const commission = (tx: string) => `${base}/bank-postings/${tx}/commission`;
  for (const tx of [pos.uuid, rent.uuid]) {
    const r = await api.put(`${base}/bank-postings/${tx}`, { account_code: tx === pos.uuid ? "108.01" : "770.01" });
    expect(r.status(), await r.text()).toBe(204);
  }
  const out = await api.put(commission(rent.uuid), { amount: "10.00", account_code: "653" });
  expect(out.status()).toBe(422);
  const badCode = await api.put(commission(pos.uuid), { amount: "250.00", account_code: "770" });
  expect(badCode.status()).toBe(422);

  // Ekran: tahsilatın panelinde komisyonu gir.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${pos.uuid}`));
  const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
  const section = panel.getByTestId("bank-transaction-commission");
  await expect(section).toContainText("POS komisyonu");
  await section.getByLabel("Komisyon:").fill("250,00");
  await section.getByLabel("Gider hesabı").selectOption("653");
  await section.getByRole("button", { name: "Komisyonu kaydet" }).click();
  await expect(section).toContainText("Brüt:");
  await expect(section).toContainText("10.000,00");
  await expect(section).toContainText("250,00");
  await expect(section).toContainText("653");

  const view = await api.json<{ posting: Posting }>(await api.get(`${base}/bank-transactions/${pos.uuid}/matching`));
  expect(view.posting).toMatchObject({
    account_code: "108.01",
    commission_amount: "250.00",
    commission_account_code: "653",
    gross_amount: "10000.00",
  });

  // Fiş: banka net borç, komisyon 653 borç, 108.01 brüt alacak.
  const made = await api.json<{ drafts: { uuid: string }[] }>(
    await api.post(`${base}/bank-postings/drafts`, { period: PERIOD }),
  );
  expect(made.drafts.length).toBeGreaterThan(0);
  let lines: Line[] = [];
  for (const d of made.drafts) {
    const detail = await api.json<{ items: Line[] }>(await api.get(`${base}/journal-drafts/${d.uuid}`));
    if (detail.items.some((l) => l.account === "653")) lines = detail.items;
  }
  const at = (account: string) => lines.find((l) => l.account === account)!;
  expect(Number(at("653").debit)).toBe(250);
  expect(Number(at("108.01").credit)).toBe(10000);
  const bank = lines.find((l) => l.account.startsWith("102"))!;
  expect(Number(bank.debit)).toBe(9750);
  const debit = lines.reduce((s, l) => s + Number(l.debit), 0);
  const credit = lines.reduce((s, l) => s + Number(l.credit), 0);
  expect(debit).toBe(credit);

  // Fişe girmiş hareketin komisyonu değişmez.
  const locked = await api.put(commission(pos.uuid), { amount: null });
  expect(locked.status()).toBe(409);
  await api.dispose();
});
