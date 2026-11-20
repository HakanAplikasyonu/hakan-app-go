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
 * Uyarı İncele panelinde ilişkili kayıt aynı panelde açılır (#564, PR #635):
 * "İlişkili kayıtlar"daki banka hareketi sekmeye gitmez, panelin içinde
 * açılır; "Uyarıya dön" uyarıya geri getirir; kendi sayfası ikincil bağlantı
 * olarak kalır; hareket panelin içinden faturasıyla eşleştirilir.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın ATL…102 faturası
 * (5.400,00) bu mükellefe türetilir, Ziraat ekstresi (fixtures/atlas-ziraat-
 * 2026-10.sta) yüklenir ve eşleme çalıştırılır: "Faturası olmayan tahsilat"
 * dönem uyarısı açılır ve tahsilat hareketini anar. Başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `İlişki Ticaret ${run} A.Ş.` };
const PERIOD = "2026-10";

test.describe("uyarı panelinde ilişkili kayıt (#564)", () => {
  let engagement = "";
  let collection = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "iliski");

    const up = await api.upload(`${base()}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
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
    const match = await api.json<{ findings: string[] }>(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
    expect(match.findings).toContain("BNK_UNINVOICED_COLLECTION");

    const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base()}/bank-transactions?period=${PERIOD}&limit=10`),
    );
    collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("ilişkili banka hareketi panelde açılır, uyarıya dönülür, panelden eşleştirilir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const warningsPath = `/engagements/${engagement}/warnings`;
    await page.goto(officePath(`${warningsPath}?view=list`));
    await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();

    const sheet = page.getByTestId("warning-sheet");
    await expect(sheet).toBeVisible();
    await expect(sheet.getByTestId("warning-transactions")).toBeVisible();

    // İlişkili kayıtlar: hareket panelde açılır; kendi sayfası ikincil bağlantı.
    const related = sheet.getByTestId("related-records");
    const own = related.getByRole("link", { name: /^Banka hareketi: EFT GELEN.*kendi sayfasında aç$/ });
    await expect(own).toHaveAttribute("href", new RegExp(`/engagements/${engagement}/bank\\?.*kayit=bank_transaction(:|%3A)${collection}`));
    await related.getByRole("button", { name: /^Banka hareketi: EFT GELEN/ }).click();

    const inPanel = sheet.getByTestId("warning-related-record");
    await expect(inPanel).toBeVisible();
    await expect(inPanel.getByRole("heading", { name: /^İlişkili kayıt: Banka hareketi · EFT GELEN/ })).toBeVisible();
    await expect(inPanel.getByTestId("bank-transaction-matching")).toContainText("EFT GELEN ATL2026000000102");
    // Sayfa değişmedi: hâlâ Uyarılar, uyarı ayrıntısı yerini ilişkili kayda bıraktı.
    expect(new URL(page.url()).pathname.endsWith(warningsPath)).toBe(true);
    await expect(sheet.getByTestId("warning-transactions")).toHaveCount(0);

    // Uyarıya dön: uyarının kendi ayrıntısı geri gelir.
    const back = inPanel.getByTestId("warning-related-back");
    await expect(back).toHaveText(/Uyarıya dön/);
    await expect(back).toBeFocused();
    await back.click();
    await expect(sheet.getByTestId("warning-related-record")).toHaveCount(0);
    await expect(sheet.getByTestId("warning-transactions")).toBeVisible();

    // İş panelde biter: hareket, ilişkili kayıt görünümünden faturasıyla eşleşir.
    await sheet.getByTestId("related-records").getByRole("button", { name: /^Banka hareketi: EFT GELEN/ }).click();
    const matching = sheet.getByTestId("warning-related-record").getByTestId("bank-transaction-matching");
    await matching.getByRole("checkbox", { name: "ATL2026000000102 faturasını seç" }).check();
    await matching.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    await expect
      .poll(async () => (await api.json<{ match_status: string }>(await api.get(`${base()}/bank-transactions/${collection}/matching`))).match_status)
      .toBe("matched");
  });
});
