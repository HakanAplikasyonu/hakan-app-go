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
 * İlişkili kayıtlarda yalnız gerçek bağlar (#669): Banka masrafı hareketinin
 * (EFT UCRETI, 12,50) sistemce önerilen karşı hesabı (780) onaylanmadığı için
 * kesin bağ sayılmaz; "Olası kayıtlar (tahmin)" başlığı altında, ayrı ve
 * etiketli görünür. Aynı ekstredeki başka hareketin faturası (5.400,00
 * tahsilatın faturası) masraf hareketinin listesine girmez.
 *
 * Veri: her koşuda yeni mükellef; Atlas'ın ATL…102 faturası türetilir,
 * Ziraat ekstresi yüklenir, eşleme ve karşı hesap önerileri üretilir
 * (kabul edilmez).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Olası Bağ ${run} A.Ş.` };
const PERIOD = "2026-10";

type Related = { items: { kind: string; uuid: string | null; relation: string; label: string }[] };

test.describe("ilişkili kayıtlarda tahmin ayrı gösterilir (#669)", () => {
  let engagement = "";
  let feeTx = "";
  let invoice = "";

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    const api = await Api.login(DEMO.owner);
    try {
      engagement = await addConsentedTaxpayer(browser, api, taxpayer, "obg");
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
      const sug = await api.post(`${base}/bank-postings/suggest`, { period: PERIOD });
      expect(sug.status(), await sug.text()).toBeLessThan(300);

      const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
        await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
      );
      const fee = txs.items.find((x) => x.amount === "12.50");
      expect(fee).toBeTruthy();
      feeTx = fee!.uuid;
    } finally {
      await api.dispose();
    }
  });

  test("API: onaylanmamış karşı hesap 'possible', başka hareketin faturası yok", async () => {
    const api = await Api.login(DEMO.owner);
    try {
      const rel = await api.json<Related>(await api.get(`/v1/tenant/engagements/${engagement}/related/bank_transaction/${feeTx}`));
      const accounts = rel.items.filter((x) => x.kind === "account");
      expect(accounts.length).toBeGreaterThan(0);
      for (const a of accounts) expect(a.relation).toBe("possible");
      expect(accounts.some((a) => a.label.startsWith("780"))).toBe(true);
      expect(rel.items.some((x) => x.relation === "counterparty_account")).toBe(false);
      // Alakasız kayıt: tahsilat hareketinin faturası masraf hareketine bağlı değildir.
      expect(rel.items.some((x) => x.kind === "invoice" || x.uuid === invoice)).toBe(false);
    } finally {
      await api.dispose();
    }
  });

  test("ekran: tahmin 'Olası kayıtlar' başlığı altında, kesin bağlar arasında değil", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${feeTx}`));

    const focused = page.getByTestId("focused-record");
    await expect(focused.getByRole("heading", { name: "Seçili kayıt: Banka hareketi" })).toBeVisible();
    await expect(focused.getByTestId("related-possible-heading")).toContainText("Olası kayıtlar (tahmin)");
    await expect(focused.getByTestId("related-possible").filter({ hasText: "780" })).toHaveCount(1);
    await expect(focused.getByTestId("related-definite").filter({ hasText: "780" })).toHaveCount(0);
    await expect(focused.getByRole("link", { name: /^Fatura: / })).toHaveCount(0);
  });
});
