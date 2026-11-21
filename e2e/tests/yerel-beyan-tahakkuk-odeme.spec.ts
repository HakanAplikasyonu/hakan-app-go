import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Tahakkuk ödeme takibi (#575): verilmiş KDV1'in ödenecek tutarı son günde
 * (beyan son günü) vergi dairesine ödenir.
 *
 *   Ağustos 2026 KDV1, 1.250,00 ilave KDV ile hazırla + verildi (API) →
 *   son gün (28.09.2026) geçti, ödeme yok: "Vadesi geçti, ödenmedi" →
 *   aday listesinde aynı tutarlı vergi ödemesi ("Tutar tutuyor") →
 *   "Bu ödemeyle eşle" → "Ödendi", ödeme günü hareketin tarihi, hareket
 *   360'a işlenir (banka bekleyen işi kalmaz) → ofis matrisinde paid_on →
 *   ödeme kaldırılır, yeniden gecikmiş → banka dışı ödeme günü elle girilir.
 *   Taslak beyana ödeme kaydedilmez (409).
 *
 * Veri: demo ofisine her koşuda yeni mükellef; Eylül 2026 MT940'ı API ile
 * yüklenir (bir vergi ödemesi, bir alakasız ödeme).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Tahakkuk Ödeme ${run} A.Ş.` };
const PERIOD = "2026-08";
const DUE = "2026-09-28";

/** Ziraat banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `000100${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Movement = { uuid: string; transaction_date: string; amount: string; description: string; same_amount: boolean };
type Payment = {
  status: "unpaid" | "paid" | "overdue";
  amount: string;
  due_date: string;
  paid_on: string | null;
  bank_transaction: Movement | null;
  candidates: Movement[];
};
type Preview = { status: string; open_warnings: string[]; payment: Payment | null };
type Matrix = {
  total: number;
  items: { engagement_uuid: string; cells: { period: string; returns: { kind: string; paid_on: string | null }[] }[] }[];
};

let api: Api;
let engagement = "";
let base = "";
const one = (kind = "kdv1") => `${base}/tax-returns/${kind}/${PERIOD}`;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tahakkuk-odeme");
  base = `/v1/tenant/engagements/${engagement}`;

  const iban = uniqueIban();
  await api.json(await api.post(`${base}/cash-boxes`, { iban, name: "Ziraat TL" }));
  const statement = [
    ":20:E2E575ODE",
    `:25:${iban}`,
    ":60F:C260901TRY50000,00",
    ":61:2609100910D480,00NTRFNONREF//B1",
    ":86:KIRTASIYE ODEMESI E2E",
    ":61:2609250925D1250,00NTRFNONREF//B2",
    ":86:GIB VERGI ODEMESI KDV E2E",
    ":62F:C260925TRY48270,00",
    "",
  ].join("\r\n");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "odeme-2026-09.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);

  // Ödeme kaydı yalnız verilmiş beyana: taslakta 409.
  const early = await api.put(`${one()}/payment`, { paid_on: "2026-09-25" });
  expect(early.status()).toBe(409);
  expect(await early.text()).toContain("TAX_RETURN_PAYMENT_NOT_APPLICABLE");

  const saved = await api.put(one(), { adjustments: { additional_vat: "1250.00", carried_in: "0.00" } });
  expect(saved.status(), await saved.text()).toBe(200);
  const draft = await api.json<Preview>(await api.get(one()));
  expect(draft.payment).toBeNull();
  const prepared = await api.json<Preview>(
    await api.post(`${one()}/prepare`, { acknowledged: draft.open_warnings ?? [] }),
  );
  expect(prepared.status).toBe("prepared");
  const filed = await api.json<Preview>(
    await api.post(`${one()}/file`, { filed_on: "2026-09-24", accrual_number: `ODE-${run}` }),
  );
  expect(filed.status).toBe("filed");
});

test.afterAll(async () => {
  await api?.dispose();
});

test("vadesi geçmiş ödenmemiş tahakkuk, aynı tutarlı vergi ödemesi aday", async () => {
  const p = await api.json<Preview>(await api.get(one()));
  expect(p.payment).toMatchObject({ status: "overdue", amount: "1250.00", due_date: DUE, paid_on: null });
  const tax = p.payment!.candidates[0];
  expect(tax).toMatchObject({ transaction_date: "2026-09-25", amount: "1250.00", same_amount: true });
  expect(tax.description).toContain("VERGI");
  // Ba/Bs ödeme taşımaz.
  expect((await api.json<Preview>(await api.get(one("babs")))).payment).toBeNull();
});

test("ekranda aday hareketle eşlenir: Ödendi, hareket 360'a işlenir", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns?period=${PERIOD}`));
  const card = page.getByRole("region", { name: "Tahakkuk ödemesi" });
  await expect(card).toContainText("Vadesi geçti, ödenmedi");
  await expect(card).toContainText("Tutar tutuyor");
  const row = card.getByRole("listitem").filter({ hasText: "GIB VERGI ODEMESI" });
  await row.getByRole("button", { name: "Bu ödemeyle eşle" }).click();
  await expect(page.getByText("Ödeme kaydedildi")).toBeVisible();
  await expect(card).toContainText("Ödendi");
  await expect(card).toContainText("25.09.2026");
  await expect(card.getByRole("button", { name: "Bu ödemeyle eşle" })).toHaveCount(0);

  const p = await api.json<Preview>(await api.get(one()));
  expect(p.payment).toMatchObject({ status: "paid", paid_on: "2026-09-25", candidates: [] });
  expect(p.payment!.bank_transaction).toMatchObject({ amount: "1250.00", same_amount: true });

  // Hareket 360 Ödenecek Vergi ve Fonlar'a işlendi.
  const tx = await api.json<{ posting: { status: string; account_code: string } | null }>(
    await api.get(`${base}/bank-transactions/${p.payment!.bank_transaction!.uuid}/matching`),
  );
  expect(tx.posting?.account_code).toBe("360");
});

test("ofis matrisi ödeme gününü taşır", async () => {
  let paidOn: string | null | undefined;
  for (let offset = 0; paidOn === undefined; offset += 100) {
    const m = await api.json<Matrix>(
      await api.get(`/v1/tenant/periods/matrix?until=${PERIOD}&periods=1&limit=100&offset=${offset}`),
    );
    paidOn = m.items
      .find((r) => r.engagement_uuid === engagement)
      ?.cells.find((c) => c.period === PERIOD)
      ?.returns.find((r) => r.kind === "kdv1")?.paid_on;
    if (paidOn === undefined && offset + 100 >= m.total) break;
  }
  expect(paidOn).toBe("2026-09-25");
});

test("ödeme kaldırılır; banka dışı ödeme günü elle girilir", async () => {
  const cleared = await api.json<Preview>(await api.put(`${one()}/payment`, { paid_on: null }));
  expect(cleared.payment).toMatchObject({ status: "overdue", paid_on: null, bank_transaction: null });

  // Gelecek gün reddedilir.
  const future = await api.put(`${one()}/payment`, { paid_on: "2099-01-01" });
  expect(future.status()).toBe(422);

  const manual = await api.json<Preview>(await api.put(`${one()}/payment`, { paid_on: "2026-09-26" }));
  expect(manual.payment).toMatchObject({ status: "paid", paid_on: "2026-09-26", bank_transaction: null });
});
