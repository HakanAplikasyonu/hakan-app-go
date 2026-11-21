import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Faturasız kira stopajı (#575): şahıstan kiralanan işyerinin kirası bankadan
 * net ödenir; stopaj ödendiği ayın muhtasarına girer.
 *
 *   Ekim 2026 MT940: "ISYERI KIRA ODEMESI" 8.000,00 çıkış + alakasız çıkış →
 *   MUHSGK'da kira adayı ve "Kontrol ettim" isteyen RENT_UNMARKED uyarısı →
 *   ekranda "Kira stopajı ekle" → brüt 10.000,00, %20 stopaj 2.000,00;
 *   muhtasarın "Kira ödemeleri" satırı ve gelir vergisi toplamı → oran %25
 *   ile brüt 10.666,67 → işaret kaldırılır, satır gider → KDV1'e 400,
 *   hazırlanmış muhtasara 409.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; ekstre API ile yüklenir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kira Stopaj ${run} Ltd. Şti.` };
const PERIOD = "2026-10";

/** Ziraat banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `000100${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Rent = { uuid: string; description: string; net: string; marked: boolean; rate: string; gross: string; tax: string };
type Preview = {
  status: string;
  open_warnings: string[];
  checklist: { code: string; count: number; severity: string; amount?: string | null }[];
  muhsgk?: {
    withholdings: { kind: string; rate: string; count: number; base: string; tax: string }[];
    totals: { income_tax: string; tax_payable: string };
    rent_payments?: Rent[];
  };
};

let api: Api;
let engagement = "";
let base = "";
let rentUuid = "";
const one = (kind = "muhsgk") => `${base}/tax-returns/${kind}/${PERIOD}`;
const preview = async () => api.json<Preview>(await api.get(one()));
const rentRow = (p: Preview) => p.muhsgk?.withholdings.find((w) => w.kind === "rent");

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kira-stopaj");
  base = `/v1/tenant/engagements/${engagement}`;
  const iban = uniqueIban();
  await api.json(await api.post(`${base}/cash-boxes`, { iban, name: "Ziraat TL" }));
  const statement = [
    ":20:E2E575KIR",
    `:25:${iban}`,
    ":60F:C261001TRY50000,00",
    ":61:2610051005D8000,00NTRFNONREF//K1",
    ":86:ISYERI KIRA ODEMESI EKIM E2E",
    ":61:2610121012D350,00NTRFNONREF//K2",
    ":86:KIRTASIYE E2E",
    ":62F:C261012TRY41650,00",
    "",
  ].join("\r\n");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "kira-2026-10.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
});

test.afterAll(async () => {
  await api?.dispose();
});

test("kira gibi görünen banka çıkışı muhtasarda aday ve uyarı", async () => {
  const p = await preview();
  const rents = p.muhsgk?.rent_payments ?? [];
  expect(rents).toHaveLength(1);
  expect(rents[0]).toMatchObject({ net: "8000.00", marked: false });
  expect(rents[0].description).toContain("KIRA");
  rentUuid = rents[0].uuid;
  expect(p.checklist.find((i) => i.code === "RENT_UNMARKED")).toMatchObject({ count: 1, severity: "warning" });
  expect(p.open_warnings).toContain("RENT_UNMARKED");
  expect(rentRow(p)).toBeUndefined();
});

test("ekranda kira stopajı eklenir: brüt ve %20 stopaj muhtasara girer", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns?period=${PERIOD}&kind=muhsgk`));
  const card = page.getByRole("region", { name: "Faturasız kira ödemeleri" });
  await expect(card).toContainText("Kira mı? Kontrol edin");
  await card.getByRole("button", { name: "Kira stopajı ekle" }).click();
  await expect(page.getByText("Muhtasar güncellendi")).toBeVisible();
  await expect(card).toContainText("10.000,00");
  await expect(card).toContainText("Stopaj %20");
  await expect(card).toContainText("2.000,00");
  await expect(page.getByText("Kira ödemeleri (GVK 94/5-a)").first()).toBeVisible();

  const p = await preview();
  expect(rentRow(p)).toMatchObject({ rate: "20", count: 1, base: "10000.00", tax: "2000.00" });
  expect(p.muhsgk?.totals.income_tax).toBe("2000.00");
  expect(p.checklist.find((i) => i.code === "RENT_WITHHOLDING")).toMatchObject({ severity: "info", amount: "2000.00" });
  expect(p.checklist.find((i) => i.code === "RENT_UNMARKED")).toBeUndefined();
});

test("oran değişir, işaret kaldırılır", async () => {
  const p25 = await api.json<Preview>(await api.put(`${one()}/rent-withholdings/${rentUuid}`, { rate: "25" }));
  expect(rentRow(p25)).toMatchObject({ rate: "25", base: "10666.67", tax: "2666.67" });

  const bad = await api.put(`${one()}/rent-withholdings/${rentUuid}`, { rate: "100" });
  expect(bad.status()).toBe(422);

  const cleared = await api.json<Preview>(await api.delete(`${one()}/rent-withholdings/${rentUuid}`));
  expect(rentRow(cleared)).toBeUndefined();
  expect(cleared.muhsgk?.rent_payments?.[0]).toMatchObject({ marked: false });
});

test("yalnız taslak MUHSGK: KDV1'e 400, hazırlanmış muhtasara 409", async () => {
  const kdv = await api.put(`${one("kdv1")}/rent-withholdings/${rentUuid}`, {});
  expect(kdv.status()).toBe(400);

  const draft = await preview();
  const prepared = await api.json<Preview>(
    await api.post(`${one()}/prepare`, { acknowledged: draft.open_warnings ?? [] }),
  );
  expect(prepared.status).toBe("prepared");
  const late = await api.put(`${one()}/rent-withholdings/${rentUuid}`, {});
  expect(late.status()).toBe(409);
  expect(await late.text()).toContain("TAX_RETURN_NOT_DRAFT");
});
