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
 * Belge yan panelinde ilişkili kayıtlar (#571): Açık belgeler listesinden
 * açılan faturanın panelinde, tam sayfaya gitmeden, tahsilat hareketi
 * görünür; bağlantı mükellef detayında hareketi seçer.
 *
 * Veri: her koşuda yeni bir mükellef (kayit-baglantilari.spec.ts ile aynı
 * kurgu): Atlas'ın ATL…102 faturası (5.400,00) türetilir, Ziraat ekstresi
 * yüklenir, eşleme önerisi kabul edilir, hareket 120.01'e işlenir ve fiş
 * taslağı oluşur.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Panel Belge ${run} A.Ş.` };
const PERIOD = "2026-10";

test.describe("belge panelinde ilişkili kayıtlar (#571)", () => {
  let engagement = "";
  let invoice = "";

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    const api = await Api.login(DEMO.owner);
    try {
      engagement = await addConsentedTaxpayer(browser, api, taxpayer, "pbl");
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
      await api.json(await api.post(`${base}/bank-matching?period=${PERIOD}`));

      const sug = await api.json<{ items: { uuid: string }[] }>(
        await api.get(`${base}/suggestions?status=pending&kind=bank_match&limit=10`),
      );
      expect(sug.items.length).toBe(1);
      await api.json(await api.post(`${base}/suggestions/${sug.items[0].uuid}/accept`, {}));

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

  test("panelde tahsilat hareketi görünür, bağlantısı çalışır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/documents?belge=${invoice}`));

    const panel = page.getByRole("dialog");
    await expect(panel).toBeVisible();
    const section = panel.getByRole("region", { name: "İlişkili kayıtlar" });
    await expect(section).toBeVisible();
    const related = section.getByTestId("related-records");
    await expect(related).toContainText("Ödeme / tahsilat");
    await expect(related).toContainText("5.400,00");

    // Hareket bağlantısı mükellef detayında, Banka sekmesinde seçili açılır.
    await related.getByRole("link", { name: /^Banka hareketi: EFT GELEN/ }).click();
    await page.waitForURL(
      (u) =>
        u.pathname.endsWith(`/engagements/${engagement}/bank`) &&
        u.searchParams.get("kayit")?.startsWith("bank_transaction:") === true,
    );
    await expect(page.getByTestId("focused-record").getByRole("heading", { name: "Seçili kayıt: Banka hareketi" })).toBeVisible();
  });
});
