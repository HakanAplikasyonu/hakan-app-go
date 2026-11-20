import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka bakiyesi tutmuyor uyarısı: ne yapılacağı ve eylemler (#710).
 *
 * - Uyarı panelinde farkın yönü ("eksik giriş ya da iki kez yüklenmiş
 *   çıkış"), neden önemli, adımlar ve "çözüldü"nün anlamı yazar.
 * - "Banka hareketlerini aç" uyarının dönemiyle açılır; hareketler listelenir
 *   (eskiden dönemsiz açılıp boş görünüyordu).
 * - Hareket hesaptan çıkarılır (toplamdan düşer, bakiye tutar, uyarı
 *   kapanır, "bakılacak hareketler"de çıkarılmış görünür) ve yeniden dahil
 *   edilir (fark ve uyarı geri gelir). API ile; ekran düğmeleri aynı ucu
 *   çağırır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; iki MT940 ekstresi
 * (1–5 Ekim: 1.000 → 1.500; 20–21 Ekim: 2.000 → 1.900). Aradaki 500 TL
 * eksik: fark +500.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bakiye Eylem ${run} A.Ş.` };
const PERIOD = "2026-10";
const IBAN = "TR760001000519786457841326";

function statement(lines: string[]): Buffer {
  return Buffer.from([`:20:EYLEM${run}`, `:25:${IBAN}`, ...lines].join("\n") + "\n", "utf8");
}
const first = statement([":60F:C261001TRY1000,00", ":61:2610051005C500,00NTRFNONREF//E1", ":86:HAVALE GELEN EYLEM", ":62F:C261005TRY1500,00"]);
const second = statement([":60F:C261020TRY2000,00", ":61:2610211021D100,00NTRFNONREF//E2", ":86:EFT GIDEN EYLEM", ":62F:C261021TRY1900,00"]);

type Check = {
  items: { difference: string | null; balanced: boolean }[];
  suspects: { uuid: string; amount: string; excluded: boolean }[];
};

test("bakiye uyarısı ne yapılacağını söyler; hareket çıkarılır ve geri alınır (#710)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "eylem");
  const base = `/v1/tenant/engagements/${engagement}`;
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  for (const [i, buffer] of [first, second].entries()) {
    await api.json(await api.upload(`${base}/bank-statements`, { name: `eylem-${run}-${i + 1}.sta`, mimeType: "text/plain", buffer }));
  }
  await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  const check = async () => api.json<Check>(await api.get(`${base}/bank-balance-check?period=${PERIOD}`));
  expect((await check()).items[0].difference).toBe("500.00");
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const out = txs.items.find((x) => x.amount === "100.00")!;
  expect(out).toBeTruthy();
  const exclusion = `${base}/bank-transactions/${out.uuid}/exclusion`;

  // Ekran: uyarı panelinde ne yapılacağı ve dönemli bağlantı.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Banka bakiyesi hareketlerle tutmuyor" }).getByText("Banka bakiyesi hareketlerle tutmuyor").first().click();
  const sheet = page.getByTestId("warning-sheet");
  const guide = sheet.getByTestId("bank-balance-guide");
  await expect(guide).toContainText("Eksik bir giriş ya da iki kez yüklenmiş bir çıkış");
  await expect(guide).toContainText("Neden önemli");
  await expect(guide).toContainText("\"Çözüldü\" ne demek");
  const open = sheet.getByTestId("bank-balance-check").getByRole("link", { name: "Banka hareketlerini aç" });
  await expect(open).toHaveAttribute("href", new RegExp(`period=${PERIOD}`));

  // API: çıkar → toplamdan düşer, bakiye tutar, uyarı kapanır; geri al.
  const ex = await api.put(exclusion, { excluded: true });
  expect(ex.status(), await ex.text()).toBe(200);
  const afterEx = await check();
  expect(afterEx.items[0].balanced).toBe(true);
  expect(afterEx.suspects).toEqual([expect.objectContaining({ uuid: out.uuid, excluded: true })]);
  const openWarnings = async () =>
    (await api.json<{ total: number }>(await api.get(`${base}/warnings?status=open&code=BNK_BALANCE_MISMATCH&period=${PERIOD}&limit=1`))).total;
  expect(await openWarnings()).toBe(0);
  expect((await api.put(exclusion, {})).status()).toBe(400);

  // Yeniden dahil et: fark ve uyarı geri gelir; bankanın iptali değil,
  // yalnız ofisin çıkardığı geri alınır.
  const inc = await api.put(exclusion, { excluded: false });
  expect(inc.status(), await inc.text()).toBe(200);
  const afterInc = await check();
  expect(afterInc.items[0].difference).toBe("500.00");
  expect(afterInc.suspects).toHaveLength(0);
  expect(await openWarnings()).toBe(1);

  // Banka sayfası uyarının dönemiyle açılır, hareketler listelenir.
  await page.goto(officePath(`/engagements/${engagement}/bank?period=${PERIOD}`));
  await expect(page.getByRole("row").filter({ hasText: "EFT GIDEN EYLEM" }).first()).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "HAVALE GELEN EYLEM" }).first()).toBeVisible();
  await api.dispose();
});
