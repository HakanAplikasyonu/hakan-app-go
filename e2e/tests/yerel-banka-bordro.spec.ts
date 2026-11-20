import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka hareketini bordroya elle bağlama (#710): toplu maaş ödemesi hareket
 * panelinde "Bordroya bağla" bölümünde, hareketin ayı ve önceki iki ayın
 * ödenmemiş bordrolarıyla (personel sayısı, açık net) listelenir. Seçilen
 * bordroya bağlanır; panelde personel dökümü (kişi başı net, bordro
 * toplamı, bağlanan tutar, fark) açılır; bordro bağlantısı bordro
 * sayfasında o dönemi ve personel dökümünü açar; bordro "ödendi" olur.
 * Personel satırına basınca detay açılır: brütten nete, işveren payları,
 * bağlı banka ödemesi. Gelen para (tahsilat)
 * bordroya bağlanamaz.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ekim 2026 bordrosu (iki
 * personel, net 25.469,68 + 25.469,68) ve 5 Kasım'da 50.939,36 TL maaş
 * ödemesi olan MT940 ekstresi API ile yüklenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka Bordro ${run} A.Ş.` };
const IBAN = "TR760001000519786457841326";
const PEOPLE = [`Sultan Deneme ${run}`, `Kerem Deneme ${run}`];

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
  PEOPLE.map(
    (p, i) =>
      `${p};${tckn(i * 7919)};340019${i};Usta;01.06.2025;30;30.000,00;4.200,00;300,00;3.315,70;0,00;197,38;30,32;25.469,68;6.150,00;600,00;1.500,00\n`,
  ).join("");

const statement = [
  ":20:E2E710",
  `:25:${IBAN}`,
  ":60F:C261101TRY100000,00",
  ":61:2611051105D50939,36NTRFNONREF//S1",
  ":86:EKIM MAAS ODEMESI TOPLU",
  ":61:2611061106C1000,00NTRFNONREF//S2",
  ":86:HAVALE GELEN E2E",
  ":62F:C261106TRY50060,64",
  "",
].join("\r\n");

type Matching = {
  match_status: string;
  payroll_matchable: boolean;
  payroll_matches: { period: string; amount: string; method: string }[];
  payroll_candidates: { payroll_run_uuid: string; period: string; open_amount: string; employee_count: number }[];
};

test("maaş ödemesi bordroya elle bağlanır; bordro bağlantısı dönemi açar (#710)", async ({ browser, page }) => {
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

  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  const up = await api.upload(`${base}/bank-statements`, {
    name: "bordro-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const txs = await api.json<{ items: { uuid: string; amount: string; debit_credit: string }[] }>(
    await api.get(`${base}/bank-transactions?period=2026-11&limit=10`),
  );
  const salary = txs.items.find((x) => x.amount === "50939.36")!;
  const incoming = txs.items.find((x) => x.amount === "1000.00")!;
  expect(salary && incoming).toBeTruthy();
  const tx = `${base}/bank-transactions/${salary.uuid}`;

  // API: Ekim bordrosu aday; tahsilat bordroya bağlanamaz.
  const before = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(before.payroll_matchable).toBe(true);
  const cand = before.payroll_candidates.find((c) => c.period === "2026-10")!;
  expect(cand).toMatchObject({ open_amount: "50939.36", employee_count: 2 });
  const inc = await api.json<Matching>(await api.get(`${base}/bank-transactions/${incoming.uuid}/matching`));
  expect(inc.payroll_matchable).toBe(false);
  expect(inc.payroll_candidates).toHaveLength(0);
  const wrong = await api.put(`${base}/bank-transactions/${incoming.uuid}/matches`, { payroll_run_uuids: [cand.payroll_run_uuid] });
  expect(wrong.status()).toBe(422);

  // Ekran: hareket panelinde bordroyu seç, bağla.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${salary.uuid}`));
  const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
  const payroll = panel.getByTestId("bank-payroll-candidates");
  await expect(payroll).toContainText("Ekim 2026 bordrosu");
  await expect(payroll).toContainText("2 personel");
  await payroll.getByRole("checkbox", { name: "Ekim 2026 bordrosunu seç" }).check();
  await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
  const linked = panel.getByTestId("bank-transaction-payroll-matches");
  await expect(linked).toContainText("Ekim 2026 bordrosu");
  await expect(linked).toContainText("Elle");

  // Personel dökümü: kişi başı net, bordro toplamı, bu ödeme ve fark.
  const breakdown = linked.getByTestId("bank-payroll-breakdown");
  await breakdown.getByRole("button", { name: "Personel dökümü" }).click();
  for (const p of PEOPLE) {
    await expect(breakdown.getByRole("row").filter({ hasText: p })).toContainText("25.469,68");
  }
  await expect(breakdown.getByRole("row").filter({ hasText: "Bordro net toplamı (2 kişi)" })).toContainText("50.939,36");
  await expect(breakdown.getByRole("row").filter({ hasText: "Bu ödemeden bağlanan" })).toContainText("50.939,36");
  await expect(breakdown.getByTestId("bank-payroll-diff")).toContainText("Fark yok");

  // Veritabanı: bağlantı ve bordronun ödeme durumu.
  const after = await api.json<Matching>(await api.get(`${tx}/matching`));
  expect(after.match_status).toBe("matched");
  expect(after.payroll_matches).toEqual([expect.objectContaining({ period: "2026-10", amount: "50939.36", method: "manual" })]);
  const runs = await api.json<{ items: { period: string; payment_status: string }[] }>(await api.get(`${base}/payroll/runs`));
  expect(runs.items.find((r) => r.period === "2026-10")?.payment_status).toBe("paid");

  // Bordro bağlantısı Ekim bordrosunu ve personel dökümünü açar.
  await linked.getByRole("link", { name: "Ekim 2026 bordrosu" }).click();
  await page.waitForURL(/\/payroll\?/);
  const region = page.getByRole("region", { name: /Ekim 2026/ }).first();
  for (const p of PEOPLE) await expect(region.getByRole("row").filter({ hasText: p })).toBeVisible();

  // Personel satırı → detay: brütten nete, işveren payları, bağlı ödeme.
  await region.getByRole("row").filter({ hasText: PEOPLE[0] }).first().click();
  const sheet = page.getByTestId("payroll-line-sheet");
  await expect(sheet.getByRole("heading", { name: PEOPLE[0] })).toBeVisible();
  await expect(sheet).toContainText("Ekim 2026 bordrosu");
  const calc = sheet.getByTestId("payroll-line-calc");
  await expect(calc).toContainText("30.000,00");
  await expect(calc).toContainText("4.200,00");
  await expect(calc).toContainText("30,32");
  await expect(calc.getByRole("definition").last()).toContainText("25.469,68");
  await expect(sheet.getByTestId("payroll-line-employer")).toContainText("6.150,00");
  const pay = sheet.getByTestId("payroll-line-payment");
  await expect(pay).toContainText("Ödendi");
  await expect(pay.getByRole("link", { name: /Banka ödemesi/ })).toBeVisible();
  await expect(pay).toContainText("50.939,36");
  await api.dispose();
});
