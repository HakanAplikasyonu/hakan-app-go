import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { exportAndDownload } from "./journal";
import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Uçtan uca kabul senaryosu (#669): bir mali müşavir ofisinin günlük işi
 * bankadan beyana tek akışta yürür; sistemin baştan sona kullanılabildiğinin
 * kanıtıdır.
 *
 *   yeni mükellef + satış faturası → banka ekstresi → hareketi faturaya
 *   eşleştir ve cariye (120.01) işle → banka fiş taslağı → fişler ekranında
 *   kontrol → kapalı dönemi gerekçeyle aç → onayla → canlı mizanda görünür →
 *   Luca aktarım dosyası → KDV tahmini.
 *
 * Her adımın sonucu API'den (veritabanı) de doğrulanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Atlas'ın ATL…102 satış
 * faturası (5.400,00, KDV 900,00) ve Ziraat ekstresi (fixtures/atlas-ziraat-
 * 2026-10.sta) bu mükellefe yüklenir. Başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Günlük Akış ${run} A.Ş.` };
const PERIOD = "2026-10";
const PERIOD_LABEL = "Eki 26"; // fişler ekranının dönem şeridi
const INVOICE = "ATL2026000000102";
const reason = `Banka fişi eklenecek (${run})`;

test("bankadan beyana günlük akış: ekstre → eşleştir → fiş → dönem aç → onay → mizan → Luca → KDV", async ({
  browser,
  page,
}) => {
  test.setTimeout(420_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "gunluk");
  const base = `/v1/tenant/engagements/${engagement}`;

  await test.step("fatura ve banka ekstresi", async () => {
    const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
    await api.json(
      await api.upload(`${base}/bank-statements`, {
        name: "atlas-ziraat-2026-10.sta",
        mimeType: "text/plain",
        buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
      }),
    );
    await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  });

  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;
  expect(collection).toBeTruthy();

  await uiLogin(page, DEMO.owner);

  await test.step("hareket faturaya eşleşir ve cariye işlenir", async () => {
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    await panel.getByRole("checkbox", { name: `${INVOICE} faturasını seç` }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    await expect(panel).toContainText("Eşleştiği faturalar");

    const account = panel.getByTestId("bank-transaction-account");
    await account.getByPlaceholder("Hesap ara: kod ya da ad (ör. 320 ya da cari adı)").fill("120.01");
    await account.getByRole("button", { name: "120.01 hesabına işle" }).click();
    await expect(account).toContainText("Hesaba işlendi");

    const history = await api.json<{ items: { action: string; account_code: string | null }[] }>(
      await api.get(`${base}/bank-transactions/${collection}/history`),
    );
    expect(history.items.map((e) => e.action)).toEqual(["booked", "matched"]);
    expect(history.items[0].account_code).toBe("120.01");
  });

  type Draft = { uuid: string; status: string; debit: string; issues?: { code: string }[] };
  const drafts = async () =>
    (await api.json<{ items: Draft[] }>(await api.get(`${base}/journal-drafts?period=${PERIOD}&limit=50`))).items;

  await test.step("banka fiş taslağı oluşur; dönem kapalı", async () => {
    const made = await api.json<{ drafts: unknown[] }>(await api.post(`${base}/bank-postings/drafts`, { period: PERIOD }));
    expect(made.drafts.length).toBe(1);
    const [draft] = await drafts();
    // Fiş uçları tutarı sade ondalık verir ("5400"); sayı olarak karşılaştırılır.
    expect(draft.status).toBe("draft");
    expect(Number(draft.debit)).toBe(5400);
    // Dönem kapatılır (müşavirin ay kapanışı); taslak onaylanamaz hâle gelir.
    const locked = await api.post(`${base}/periods/${PERIOD}/lock`);
    expect(locked.status(), await locked.text()).toBe(200);
    expect((await drafts())[0].issues?.map((i) => i.code)).toContain("period_locked");
  });

  await test.step("fişler ekranında kontrol, dönemi aç, onayla", async () => {
    await page.goto(officePath(`/engagements/${engagement}/journals`));
    await page.getByRole("button", { name: new RegExp(`^${PERIOD_LABEL}`) }).click();
    const row = page.locator("tr", { hasText: "Taslak" }).first();
    await expect(row).toContainText("Dönem kapalı; onay için önce dönemi açın");
    await expect(row.getByRole("button", { name: /^Onayla/ })).toBeDisabled();
    await row.getByRole("button", { name: "Dönemi aç" }).click();
    const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
    await dialog.getByRole("textbox").fill(reason);
    await dialog.getByRole("button", { name: "Dönemi aç" }).click();
    await expect(row).toContainText("Kontrol tamam");
    await row.getByRole("button", { name: /^Onayla/ }).click();
    await expect(page.locator("tr", { hasText: "Onaylı" }).first()).toBeVisible();
    expect((await drafts())[0].status).toBe("approved");
  });

  await test.step("canlı mizanda görünür", async () => {
    await page.goto(officePath(`/engagements/${engagement}/trial-balance?kaynak=canli&donem=${PERIOD}`));
    const table = page.getByRole("table", { name: "Mizan" });
    await expect(table.getByRole("row", { name: /^120\.01/ })).toContainText("5.400,00");
    await expect(table.getByRole("row", { name: /^102/ }).first()).toContainText("5.400,00");
    const live = await api.json<{ totals: { debit: string; credit: string }; lines: { code: string; balance: string }[] }>(
      await api.get(`${base}/account-plan/live-trial-balance?period=${PERIOD}`),
    );
    expect(live.totals).toEqual({ debit: "5400.00", credit: "5400.00" });
    expect(live.lines.find((l) => l.code === "120.01")?.balance).toBe("-5400.00");
  });

  await test.step("Luca aktarım dosyası", async () => {
    await page.goto(officePath(`/engagements/${engagement}/journals`));
    await page.getByRole("button", { name: new RegExp(`^${PERIOD_LABEL}`) }).click();
    await exportAndDownload(page, "luca");
    await expect.poll(async () => (await drafts())[0].status).toBe("exported");
  });

  await test.step("KDV tahmini", async () => {
    const vat = await api.json<{ totals: { output_vat: string; invoice_count: number } }>(
      await api.get(`${base}/vat-forecast?period=${PERIOD}`),
    );
    expect(vat.totals.output_vat).toBe("900.00");
    expect(vat.totals.invoice_count).toBe(1);
  });

  await api.dispose();
});
