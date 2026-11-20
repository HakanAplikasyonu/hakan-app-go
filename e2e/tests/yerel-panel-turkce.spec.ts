import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  expectNoRawText,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Panelde ham kod yok (#710): kilitli Ekim 2026'ya gelen yeni satış
 * faturasının değişiklik talebi alan adlarını, değerleri ve kaynağı Türkçe
 * gösterir (`payable_amount`, `SATIS`, `TRY`, `manual_upload` görünmez).
 * Uygulanan faturanın ayrıntısında kaynak da Türkçe.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; dönem kilidi ve fatura
 * API ile.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Türkçe Panel ${run} A.Ş.` };

test("kilitli dönem değişiklik talebi Türkçe; ham alan / kaynak adı görünmez (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "turkce",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const locked = await api.post(`${base}/periods/2026-10/lock`);
  expect(locked.status(), await locked.text()).toBe(200);

  const up = await api.upload(
    `${base}/ubl-uploads`,
    deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  await expect
    .poll(
      async () =>
        (
          await api.json<{ items: unknown[] }>(
            await api.get(`${base}/change-requests?status=open`),
          )
        ).items.length,
      { timeout: 60_000 },
    )
    .toBe(1);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const panel = page.locator("section", {
    has: page.locator("#change-requests-title"),
  });
  await panel.getByRole("button", { name: "İncele" }).click();
  const card = panel.locator("article").first();
  await expect(card).toContainText("Ekim 2026");
  await expect(card).toContainText("Kaynak: Elle yükleme · UBL dosyası");
  await expect(card).toContainText("Ödenecek tutar");
  await expect(card).toContainText("Satış");
  await expect(card).toContainText("₺ 492,00");
  await expectNoRawText(panel);

  // Uygulanan fatura: ayrıntıda kaynak Türkçe.
  await card.getByRole("button", { name: "Uygula" }).click();
  await expect(panel).toBeHidden({ timeout: 15_000 });
  const inv = (
    await api.json<{ items: { uuid: string }[] }>(
      await api.get(`${base}/invoices?limit=5`),
    )
  ).items[0];
  await page.goto(officePath(`/invoices/${inv.uuid}?taxpayer=${engagement}`));
  await expect(page.getByText("Elle yükleme").first()).toBeVisible();
  await expect(page.getByText("manual_upload")).toHaveCount(0);
  await api.dispose();
});
