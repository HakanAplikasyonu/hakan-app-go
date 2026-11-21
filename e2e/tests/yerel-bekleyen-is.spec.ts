import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * "Bekleyen iş" gerçek içeriğe götürür (#710): kilitli Ekim 2026'ya gelen
 * fatura değişiklik talebi açar; dönem rozeti talebe götürür. Mükellefler
 * listesindeki ve Komuta Merkezi'ndeki rozet mükellef ekranını talep listesi
 * AÇIK olarak getirir (yalnız özet değil).
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; dönem kilidi ve fatura
 * API ile.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bekleyen İş ${run} A.Ş.` };

test("bekleyen iş: Mükellefler ve Komuta Merkezi rozeti talep listesini açık getirir (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "bekleyen",
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
  const list = page.locator("#change-requests-list");

  // Mükellefler: beyan dönemi (Ekim) rozeti.
  await page.goto(officePath("/taxpayers"));
  await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.vkn);
  // Kilitli dönem "Onaylandı (kilitli)" yazar; açık talep varken rozet
  // (dönemde engel olsa da) talep listesine götürür.
  const badge = page
    .getByRole("row", { name: new RegExp(taxpayer.title) })
    .locator("a[data-target]");
  await expect(badge).toHaveCount(1);
  await expect(badge).toContainText("Onaylandı (kilitli)");
  await expect(badge).toHaveAttribute("data-target", "change_requests");
  await badge.click();
  await expect(page).toHaveURL(
    new RegExp(`/engagements/${engagement}\\?talepler=ac`),
  );
  await expect(list).toBeVisible();
  await expect(list).not.toContainText("Açık talep kalmadı.");

  // Komuta Merkezi: aynı mükellefin Ekim hücresi.
  await page.goto(officePath("/command"));
  await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.vkn);
  const cell = page.locator('a[data-target="change_requests"]').first();
  await expect(cell).toBeVisible();
  await cell.click();
  await expect(page).toHaveURL(/talepler=ac/);
  await expect(list).toBeVisible();
  await api.dispose();
});
