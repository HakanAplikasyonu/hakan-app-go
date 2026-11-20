import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka hareketini SGK primine elle bağlama (#565): SGK prim ödemesinin
 * panelinde "SGK primine bağla" bölümü hareketin ayı ve önceki üç ayın açık
 * primlerini (son ödeme günü, açık tutar) listeler. Seçilen prime bağlanır,
 * prim "ödendi" olur ve ödeyen hareketi gösterir; bağlantı kaldırılınca prim
 * yeniden açık borç olur. Gelen para SGK primine bağlanamaz.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ekim 2026 bordrosu (tek
 * personel, SGK tahakkuku bordrodan) ve 20 Kasım'da prim tutarı kadar SGK
 * ödemesi olan MT940 ekstresi API ile yüklenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka SGK ${run} A.Ş.` };
const IBAN = "TR760001000519786457841326";

/** Her koşuda farklı, algoritması geçerli TC kimlik numarası. */
function tckn(seed: number): string {
  const d = `9${String(Date.now() + seed).slice(-8)}`.split("").map(Number);
  const odd = d[0] + d[2] + d[4] + d[6] + d[8];
  const even = d[1] + d[3] + d[5] + d[7];
  d.push((((odd * 7 - even) % 10) + 10) % 10);
  d.push(d.reduce((a, b) => a + b, 0) % 10);
  return d.join("");
}

const csv =
  "Adı Soyadı;TC Kimlik No;SGK Sicil No;Görevi;İşe Giriş Tarihi;Gün;Brüt Ücret;SGK İşçi Payı;" +
  "İşsizlik İşçi Payı;GV İstisnası;Ödenecek Gelir Vergisi;DV İstisnası;Damga Vergisi;Net Ücret;SGK İşveren Payı;" +
  "İşsizlik İşveren Payı;SGK Teşvik\n" +
  `Sultan SGK ${run};${tckn(17)};3400191;Usta;01.06.2025;30;30.000,00;4.200,00;300,00;3.315,70;0,00;197,38;30,32;25.469,68;6.150,00;600,00;1.500,00\n`;

/** MT940 tutarı: 1234.5 → "1234,50". */
const mt = (v: number) => v.toFixed(2).replace(".", ",");

type Premium = {
  uuid: string;
  period: string;
  payable_amount: string;
  paid_amount: string;
  paid_at: string | null;
  status: string;
  bank_transaction_uuid: string | null;
};
type Matching = {
  match_status: string;
  payroll_matchable: boolean;
  sgk_matches: { period: string; amount: string; method: string }[];
  sgk_candidates: { sgk_premium_uuid: string; period: string; open_amount: string; due_date: string }[];
};

test("SGK prim ödemesi prime elle bağlanır; bağlantı kalkınca prim yeniden açılır (#565)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bordro");
  const base = `/v1/tenant/engagements/${engagement}`;

  const imp = await api.uploadWith(
    `${base}/payroll/imports`,
    { name: "bordro-2026-10.csv", mimeType: "text/csv", buffer: Buffer.from(csv, "utf8") },
    { period: "2026-10" },
  );
  expect(imp.status(), await imp.text()).toBe(201);
  const premiums = async () =>
    (await api.json<{ items: Premium[] }>(await api.get(`${base}/payroll/sgk-premiums?year=2026`))).items;
  const october = (await premiums()).find((p) => p.period === "2026-10")!;
  expect(october).toBeTruthy();
  const payable = Number(october.payable_amount);
  expect(payable).toBeGreaterThan(0);

  const statement = [
    ":20:E2E565SGK",
    `:25:${IBAN}`,
    ":60F:C261101TRY100000,00",
    `:61:2611201120D${mt(payable)}NTRFNONREF//S1`,
    ":86:SGK EKIM PRIM ODEMESI",
    ":61:2611211121C1000,00NTRFNONREF//S2",
    ":86:HAVALE GELEN E2E",
    `:62F:C261121TRY${mt(100000 - payable + 1000)}`,
    "",
  ].join("\r\n");
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  const up = await api.upload(`${base}/bank-statements`, {
    name: "sgk-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=2026-11&limit=10`),
  );
  const sgk = txs.items.find((x) => x.amount === payable.toFixed(2))!;
  const incoming = txs.items.find((x) => x.amount === "1000.00")!;
  expect(sgk && incoming).toBeTruthy();
  const tx = `${base}/bank-transactions/${sgk.uuid}`;

  // API: Ekim primi aday (açık tutar = ödenecek); tahsilat prime bağlanamaz.
  const before = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(before.payroll_matchable).toBe(true);
  const cand = before.sgk_candidates.find((c) => c.period === "2026-10")!;
  expect(cand).toMatchObject({ sgk_premium_uuid: october.uuid, open_amount: payable.toFixed(2) });
  const inc = await api.json<Matching>(await api.get(`${base}/bank-transactions/${incoming.uuid}/matching`));
  expect(inc.sgk_candidates).toHaveLength(0);
  const wrong = await api.put(`${base}/bank-transactions/${incoming.uuid}/matches`, { sgk_premium_uuids: [october.uuid] });
  expect(wrong.status()).toBe(422);

  // Ekran: hareket panelinde primi seç, bağla.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${sgk.uuid}`));
  const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
  const section = panel.getByTestId("bank-sgk-candidates");
  await expect(section).toContainText("Ekim 2026 SGK primi");
  await expect(section).toContainText("Son ödeme");
  await section.getByRole("checkbox", { name: "Ekim 2026 SGK primini seç" }).check();
  await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
  const linked = panel.getByTestId("bank-transaction-sgk-matches");
  await expect(linked).toContainText("Ekim 2026 SGK primi");
  await expect(linked).toContainText("Elle");
  await expect(panel.getByTestId("bank-sgk-candidates")).toHaveCount(0);

  // Veritabanı: bağlantı ve primin ödeme durumu.
  const after = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(after.match_status).toBe("matched");
  expect(after.sgk_matches).toEqual([
    expect.objectContaining({ period: "2026-10", amount: payable.toFixed(2), method: "manual" }),
  ]);
  const paid = (await premiums()).find((p) => p.period === "2026-10")!;
  expect(Number(paid.paid_amount)).toBe(payable);
  expect(paid.paid_at).toBe("2026-11-20");
  expect(paid.status).toBe("paid");
  expect(paid.bank_transaction_uuid).toBe(sgk.uuid);

  // Bağlantı kalkınca prim yeniden açık borç olur ve yeniden aday olur.
  const un = await api.delete(`${tx}/matches`);
  expect(un.status(), await un.text()).toBe(200);
  const reopened = (await premiums()).find((p) => p.period === "2026-10")!;
  expect(Number(reopened.paid_amount)).toBe(0);
  expect(reopened.paid_at).toBeNull();
  expect(reopened.bank_transaction_uuid).toBeNull();
  const again = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(again.sgk_matches).toHaveLength(0);
  expect(again.sgk_candidates.map((c) => c.period)).toContain("2026-10");
  await api.dispose();
});
