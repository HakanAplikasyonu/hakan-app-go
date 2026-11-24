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
 * Panelde ham kod yok, talep sade cümle (#710): kilitli Ekim 2026'ya gelen yeni satış
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
  const card = panel.getByTestId("change-request").first();
  // Sade cümle, neden önemli, Uygula / Reddet'in tam etkisi.
  await expect(card).toContainText(
    "Kapalı Ekim 2026 dönemine yeni bir satış faturası geldi:",
  );
  await expect(card).toContainText("₺ 492,00.");
  await expect(card).toContainText("Dönem kapalı olduğu için");
  await expect(card).toContainText(
    "Uygula: fatura Ekim 2026 kayıtlarına eklenir.",
  );
  await expect(card).toContainText("Reddet: gelen fatura kaydedilmez");
  await expect(card).toContainText("Kaynak: Elle yükleme · UBL dosyası");
  await expect(card).toContainText("Ödenecek tutar");
  await expect(card).toContainText("Satış");
  await expect(card).toContainText("₺ 492,00");
  await expectNoRawText(panel);

  // Döneme git: Ekim seçilir, faturalar açılır; kapalı dönem notu salt
  // okunur fişler / faturalar / beyan bağlantılarını gösterir.
  await card
    .getByRole("button", { name: "Ekim 2026 faturalarını gör" })
    .click();
  await expect(page).toHaveURL(/\/engagements\/[^/]+\/invoices/);
  const note = page.getByTestId("locked-period-note");
  await expect(note).toContainText("Ekim 2026 kapalı");
  await expect(note).toContainText("salt okunur");
  for (const name of ["Fişler", "Faturalar", "Beyan"]) {
    await expect(note.getByRole("link", { name })).toBeVisible();
  }

  // Panel her sekmede; liste kapandıysa yeniden açılır.
  if (!(await card.isVisible())) {
    await panel.getByRole("button", { name: "İncele" }).click();
  }

  // Uygulanan fatura: ayrıntıda kaynak Türkçe.
  // #816 kartı "Seçilen aya uygula" düğmesi de taşır; doğrudan uygulama "Uygula".
  await card.getByRole("button", { name: "Uygula", exact: true }).click();
  await expect(panel).toBeHidden({ timeout: 15_000 });
  const inv = (
    await api.json<{ items: { uuid: string }[] }>(
      await api.get(`${base}/invoices?limit=5`),
    )
  ).items[0];
  await page.goto(officePath(`/invoices/${inv.uuid}?taxpayer=${engagement}`));
  await expect(page.getByText("Elle yükleme").first()).toBeVisible();
  await expect(page.getByText("manual_upload")).toHaveCount(0);

  // Kapalı dönemin beyanı da açılır (salt okunur notla).
  await page.goto(officePath(`/engagements/${engagement}`));
  await page
    .getByTestId("locked-period-note")
    .getByRole("link", { name: "Beyan" })
    .click();
  await expect(page).toHaveURL(/\/tax-returns/);
  await expect(page.getByTestId("locked-period-note")).toContainText(
    "Ekim 2026 kapalı",
  );
  await api.dispose();
});
