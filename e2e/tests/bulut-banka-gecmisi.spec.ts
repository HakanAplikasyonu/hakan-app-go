import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * #565 Banka hareketi geçmişi: hareket panelinde kim, ne zaman faturaya
 * eşleştirdi, eşleşmeyi geri aldı ya da hesaba işledi görünür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın ATL…102 faturası
 * (5.400,00) bu mükellefe türetilir ve Ziraat ekstresi (fixtures/atlas-ziraat-
 * 2026-10.sta) yüklenir. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Geçmiş Ticaret ${run} A.Ş.` };
const PERIOD = "2026-10";
const INVOICE = "ATL2026000000102";

type HistoryEntry = {
  action: "matched" | "unmatched" | "booked";
  actor_name: string | null;
  invoices: string[];
  account_code: string | null;
};

test.describe("#565 banka hareketi geçmişi", () => {
  let engagement = "";
  let collection = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const history = async () =>
    (
      await api.json<{ items: HistoryEntry[] }>(
        await api.get(`${base()}/bank-transactions/${collection}/history`),
      )
    ).items;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "gecmis");

    const up = await api.upload(
      `${base()}/ubl-uploads`,
      deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title),
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);

    await api.json(await api.post(`${base()}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
    await api.json(
      await api.upload(`${base()}/bank-statements`, {
        name: "atlas-ziraat-2026-10.sta",
        mimeType: "text/plain",
        buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
      }),
    );
    await api.json(await api.post(`${base()}/bank-matching?period=${PERIOD}`));

    const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base()}/bank-transactions?period=${PERIOD}&limit=10`),
    );
    collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("eşleştir → geri al → hesaba işle panelde geçmiş olarak listelenir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    const log = panel.getByTestId("bank-transaction-history");
    await expect(log).toContainText("Hareket geçmişi");
    await expect(log).toContainText("Bu harekette henüz elle yapılmış bir işlem yok.");

    // Eşleştir.
    await panel.getByRole("checkbox", { name: `${INVOICE} faturasını seç` }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    await expect(panel).toContainText("Eşleştiği faturalar");
    await expect(log.getByRole("listitem")).toHaveCount(1);
    await expect(log.getByRole("listitem").first()).toContainText("Faturaya eşleştirdi");
    await expect(log.getByRole("listitem").first()).toContainText(INVOICE);

    // Geri al.
    await panel.getByRole("button", { name: "Eşleşmeyi geri al" }).click();
    await expect(panel.getByText("Eşleşmedi", { exact: true })).toBeVisible();
    await expect(log.getByRole("listitem")).toHaveCount(2);
    await expect(log.getByRole("listitem").first()).toContainText("Eşleşmeyi geri aldı");

    // Hesaba işle.
    const account = panel.getByTestId("bank-transaction-account");
    await account.getByPlaceholder("Hesap ara: kod ya da ad (ör. 320 ya da cari adı)").fill("340.01");
    await account.getByRole("button", { name: "340.01 hesabına işle" }).click();
    await expect(account).toContainText("Hesaba işlendi");
    await expect(log.getByRole("listitem")).toHaveCount(3);
    await expect(log.getByRole("listitem").first()).toContainText("Hesaba işledi");
    await expect(log.getByRole("listitem").first()).toContainText("340.01");
    await expect(log).not.toContainText("Bilinmeyen kullanıcı");

    // Veritabanı: en yenisi önce, her kaydın yapanı belli.
    const items = await history();
    expect(items.map((e) => e.action)).toEqual(["booked", "unmatched", "matched"]);
    expect(items[0].account_code).toBe("340.01");
    expect(items[1].invoices).toEqual([INVOICE]);
    expect(items[2].invoices).toEqual([INVOICE]);
    for (const e of items) expect(e.actor_name).toBeTruthy();

    // Sayfa yenilense de geçmiş aynı.
    await page.reload();
    await expect(log.getByRole("listitem")).toHaveCount(3);
  });
});
