import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Ortak tema mükellef sekmelerinde (#710, tasarım PR-2): sekme sayfası
 * ortak başlıkla (h2 + kısa açıklama) açılır ve tek birincil eylemi vardır;
 * öteki eylemler ikincildir. Boş sekmede de başlık görünür, boş durum ne
 * yapılacağını söyler.
 *
 * - Banka: "Banka hareketleri" başlığı; "Ekstre yükle" birincil, "Excel'e
 *   aktar" ikincil.
 * - Mizan (boş): "Mizan" başlığı, boş durum ve birincil "Mizan yükle".
 *
 * Veri: her koşuda yeni (boş) mükellef; yalnızca okur ve gezinir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Tema Sekme ${run} A.Ş.` };

test("banka ve mizan sekmeleri ortak başlık ve tek birincil eylemle açılır (#710)", async ({ browser, page }) => {
  test.setTimeout(180_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tema-sekme");
  await api.dispose();
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);

  // Gizli dosya girişi (input type=file) de "button" rolündedir ve aynı
  // adı taşır: görünür düğme data-slot ile seçilir.
  const button = (scope: ReturnType<typeof page.locator>, name: string) =>
    scope.locator('[data-slot="button"]', { hasText: name });
  const primaries = (scope: ReturnType<typeof page.locator>) =>
    scope.locator('[data-slot="button"][data-variant="default"]:visible');

  // Banka: ortak başlık, tek birincil eylem.
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  const bankHeading = page.getByRole("heading", { level: 2, name: "Banka hareketleri" });
  await expect(bankHeading).toBeVisible();
  const bankHeader = bankHeading.locator("xpath=ancestor::div[contains(@class,'justify-between')][1]");
  await expect(button(bankHeader, "Ekstre yükle")).toHaveAttribute("data-variant", "default");
  await expect(button(bankHeader, "Excel'e aktar")).toHaveAttribute("data-variant", "outline");
  await expect(primaries(bankHeader)).toHaveCount(1);

  // Mizan boş: başlık görünür, boş durum ve birincil yükleme.
  await page.goto(officePath(`/engagements/${engagement}/trial-balance`));
  await expect(page.getByRole("heading", { level: 2, name: "Mizan" })).toBeVisible();
  const upload = button(page.getByRole("main"), "Mizan yükle");
  await expect(upload).toBeVisible();
  await expect(upload).toHaveAttribute("data-variant", "default");
});
