import { readFileSync } from "node:fs";

import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

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
 * Kayıtlar arası bağlantılar (#469): fatura → tahsilat hareketi → oluşan fiş
 * → hareketin uyarısı → ilgili kural zinciri mükellef detayında, sekme
 * çubuğu kaybolmadan; tarayıcının geri tuşu zinciri geri sarar.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın ATL…102 faturası
 * (5.400,00) bu mükellefe türetilir; aynı ekstre (fixtures/atlas-ziraat-
 * 2026-10.sta) yüklenir, eşleme önerisi kabul edilir, hareket 120.01'e
 * muhasebeleştirilir ve fiş taslağı oluşur. Eşleme çalışırken banka kontrolü
 * dönem uyarısı (BNK_UNINVOICED_COLLECTION) açar; uyarı hareketi anar.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bağ Ticaret ${run} A.Ş.` };
const PERIOD = "2026-10";

test.describe("kayıtlar arası bağlantılar", () => {
  let engagement = "";
  let invoice = "";

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    const api = await Api.login(DEMO.owner);
    try {
      engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bag");
      const base = `/v1/tenant/engagements/${engagement}`;

      const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
      expect(up.status(), await up.text()).toBeLessThan(300);
      await waitForUploads(api, engagement, 1);
      invoice = (await api.json<{ items: { uuid: string }[] }>(await api.get(`${base}/invoices?limit=5`))).items[0].uuid;

      await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
      await api.json(
        await api.upload(`${base}/bank-statements`, {
          name: "atlas-ziraat-2026-10.sta",
          mimeType: "text/plain",
          buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
        }),
      );
      const match = await api.json<{ findings: string[] }>(await api.post(`${base}/bank-matching?period=${PERIOD}`));
      expect(match.findings).toContain("BNK_UNINVOICED_COLLECTION");

      // Eşleme önerisi kabul → hareket faturaya bağlanır.
      const sug = await api.json<{ items: { uuid: string }[] }>(
        await api.get(`${base}/suggestions?status=pending&kind=bank_match&limit=10`),
      );
      expect(sug.items.length).toBe(1);
      await api.json(await api.post(`${base}/suggestions/${sug.items[0].uuid}/accept`, {}));

      // Tahsilat 120.01'e muhasebeleştirilir; fiş taslağı oluşur.
      const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
        await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
      );
      const collection = txs.items.find((x) => x.amount === "5400.00");
      expect(collection).toBeTruthy();
      const put = await api.put(`${base}/bank-postings/${collection!.uuid}`, { account_code: "120.01", account_name: "Alıcılar" });
      expect(put.status(), await put.text()).toBe(204);
      const drafts = await api.json<{ drafts: unknown[] }>(await api.post(`${base}/bank-postings/drafts`, { period: PERIOD }));
      expect(drafts.drafts.length).toBeGreaterThan(0);
    } finally {
      await api.dispose();
    }
  });

  const tabs = (page: Page) => page.getByRole("navigation", { name: "Mükellef bölümleri" });
  const focused = (page: Page) => page.getByTestId("focused-record");

  async function expectShell(page: Page, tab: string) {
    await expect(page.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
    await expect(tabs(page)).toBeVisible();
    await expect(tabs(page).getByRole("link", { name: tab, exact: true })).toHaveAttribute("aria-current", /page|true/);
  }

  test("fatura → banka hareketi → fiş → uyarı → kural ve geri", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/invoices/${invoice}`));
    await expectShell(page, "Faturalar");

    // Fatura detayı: İlişkili kayıtlar bölümünde tahsilat (5.400,00).
    const related = page.getByTestId("related-records");
    await expect(page.getByRole("heading", { name: "İlişkili kayıtlar" })).toBeVisible();
    await expect(related).toContainText("Ödeme / tahsilat");
    await expect(related).toContainText("5.400,00");
    await related.getByRole("link", { name: /^Banka hareketi: EFT GELEN/ }).click();

    // Banka sekmesi, hareket seçili: fatura, fiş, uyarı ve karşı hesap.
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/bank`) && u.searchParams.get("kayit")?.startsWith("bank_transaction:") === true);
    await expectShell(page, "Banka");
    await expect(focused(page).getByRole("heading", { name: "Seçili kayıt: Banka hareketi" })).toBeVisible();
    await expect(focused(page).getByRole("link", { name: /^Fatura: ATL2026000000102/ })).toBeVisible();
    await expect(focused(page)).toContainText("120.01");
    await focused(page).getByRole("link", { name: /^Fiş: / }).click();

    // Fişler sekmesi, fiş seçili: kaynağı olan banka hareketi.
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/journals`) && u.searchParams.get("kayit")?.startsWith("journal_draft:") === true);
    await expectShell(page, "Fişler");
    await expect(focused(page).getByRole("heading", { name: "Seçili kayıt: Fiş" })).toBeVisible();
    await expect(focused(page).getByRole("link", { name: /^Banka hareketi: EFT GELEN/ })).toBeVisible();

    // Geri: hareket; oradan hareketin uyarısına.
    await page.goBack();
    await page.waitForURL((u) => u.pathname.endsWith("/bank"));
    await expectShell(page, "Banka");
    await focused(page).getByRole("link", { name: "Uyarı: BNK_UNINVOICED_COLLECTION" }).click();

    // Uyarılar sekmesi, uyarı seçili: andığı hareket ve ilgili kural.
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/warnings`) && u.searchParams.get("kayit")?.startsWith("warning:") === true);
    await expectShell(page, "Uyarılar");
    await expect(focused(page).getByRole("link", { name: /^Banka hareketi: EFT GELEN/ })).toBeVisible();
    const axe = await new AxeBuilder({ page }).include('[data-testid="focused-record"]').withTags(["wcag2a", "wcag2aa"]).analyze();
    expect(axe.violations.filter((v) => v.impact === "critical" || v.impact === "serious")).toEqual([]);
    await focused(page).getByRole("link", { name: "Kural: BNK_UNINVOICED_COLLECTION" }).click();
    await page.waitForURL((u) => u.pathname.endsWith("/warnings") && u.searchParams.get("code") === "BNK_UNINVOICED_COLLECTION");
    await expectShell(page, "Uyarılar");

    // Geri tuşu zinciri sarar: uyarı → hareket → fatura detayı.
    await page.goBack();
    await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("warning:") === true);
    await page.goBack();
    await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("bank_transaction:") === true);
    await expectShell(page, "Banka");
    // Seçimi kaldır: panel kapanır, sekme yerinde.
    await focused(page).getByRole("link", { name: "Seçimi kaldır" }).click();
    await expect(focused(page)).toHaveCount(0);
    await expectShell(page, "Banka");
  });

  test("liste satırlarından ilişkili kayıtlar: banka, fiş, uyarı", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const base = `/engagements/${engagement}`;

    // Banka sekmesi: satırın bağlantısı hareketi seçili açar, süzgeç korunur.
    await page.goto(officePath(`${base}/bank?match=matched`));
    await expectShell(page, "Banka");
    await page.getByRole("link", { name: /^İlişkili kayıtlar: EFT GELEN/ }).first().click();
    await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("bank_transaction:") === true);
    expect(new URL(page.url()).searchParams.get("match")).toBe("matched");
    await expect(focused(page).getByRole("link", { name: /^Fatura: ATL2026000000102/ })).toBeVisible();

    // Fişler sekmesi: fişin satırı → kaynağı olan hareket.
    await tabs(page).getByRole("link", { name: "Fişler", exact: true }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/journals`));
    // Fişler tek dönemlik: dönem şeridinden Ekim seçilir.
    await page.getByRole("button", { name: /^Eki 26/ }).click();
    await page.getByRole("link", { name: /^İlişkili kayıtlar: #\d+$/ }).first().click();
    await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("journal_draft:") === true);
    await expectShell(page, "Fişler");
    await expect(focused(page).getByRole("link", { name: /^Banka hareketi: EFT GELEN/ })).toBeVisible();

    // Uyarılar sekmesi: uyarı kartından → andığı hareket ve kural.
    await tabs(page).getByRole("link", { name: "Uyarılar", exact: true }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/warnings`));
    await page.getByRole("link", { name: "İlişkili kayıtlar: BNK_UNINVOICED_COLLECTION" }).first().click();
    await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("warning:") === true);
    await expectShell(page, "Uyarılar");
    await expect(focused(page).getByRole("link", { name: "Kural: BNK_UNINVOICED_COLLECTION" })).toBeVisible();
  });

  test("ilişkisi olmayan kayıtta boş durum metni", async ({ page }) => {
    // Hiçbir kayda bağlanmamış ücret hareketi (12,50): yalnız karşı hesabı yok.
    const api = await Api.login(DEMO.owner);
    let fee = "";
    try {
      const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
        await api.get(`/v1/tenant/engagements/${engagement}/bank-transactions?period=${PERIOD}&limit=10`),
      );
      fee = txs.items.find((x) => x.amount === "12.50")!.uuid;
    } finally {
      await api.dispose();
    }
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${fee}`));
    await expectShell(page, "Banka");
    await expect(focused(page).getByTestId("related-empty")).toHaveText("Bu kayda bağlı başka kayıt yok.");
  });
});
