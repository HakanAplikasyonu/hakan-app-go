import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka hareketinin önerilen hesabı panelden onaylanır (#710, günlük akış
 * gezisi): "Mizan hesabı" bölümü "Öneri, onay bekliyor" dediğinde yanında
 * "Önerilen hesabı onayla (kod)" birincil düğmesi vardır; basınca hareket o
 * hesaba işlenir ("Hesaba işlendi"), API'de öneri onaylıdır. Eskiden onay
 * için inceleme kuyruğunu bulmak gerekiyordu.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; banka masrafı (EFT
 * ücreti) satırı olan MT940 ekstresi API ile.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Hesap Onay ${run} A.Ş.` };
const PERIOD = "2026-10";
const IBAN = "TR760001000519786457841326";
const statement = [
  `:20:ONAY${run}`,
  `:25:${IBAN}`,
  ":60F:C261001TRY1000,00",
  ":61:2610051005D10,53NTRFNONREF//F1",
  ":86:EFT UCRETI",
  ":62F:C261005TRY989,47",
  "",
].join("\r\n");

type Matching = { posting: { status: string; account_code: string } | null };

test("önerilen hesap panelden tek tıkla onaylanır (#710)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "hesap-onay");
  const base = `/v1/tenant/engagements/${engagement}`;
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, {
      name: `onay-${run}.sta`,
      mimeType: "text/plain",
      buffer: Buffer.from(statement, "utf8"),
    }),
  );
  await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const fee = txs.items.find((x) => x.amount === "10.53")!;
  expect(fee).toBeTruthy();
  const matching = async () => api.json<Matching>(await api.get(`${base}/bank-transactions/${fee.uuid}/matching`));
  await expect.poll(async () => (await matching()).posting?.status, { timeout: 60_000 }).toBe("suggested");
  const code = (await matching()).posting!.account_code;

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${fee.uuid}`));
  const account = page.getByTestId("focused-record").getByTestId("bank-transaction-account");
  await expect(account).toContainText("Öneri, onay bekliyor");
  const approve = account.getByRole("button", { name: `Önerilen hesabı onayla (${code})` });
  await approve.click();
  await expect(account).toContainText("Hesaba işlendi");
  await expect(approve).toHaveCount(0);
  expect((await matching()).posting).toMatchObject({ status: "approved", account_code: code });
  await api.dispose();
});
