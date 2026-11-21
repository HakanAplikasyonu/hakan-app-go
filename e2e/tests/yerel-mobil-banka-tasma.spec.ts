import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Telefon genişliğinde banka listesi taşmaz (#669 UX ikinci tur): kart
 * görünümünde uzun hesap adı ("361 Ödenecek sosyal güvenlik kesintileri")
 * kartın dışına çıkmaz, kısalır ve tamamı ipucunda kalır; "Luca'ya
 * aktarıldı" işareti uzun açıklamada kesilmez. Tablo üstündeki "Mobil
 * görünüm: kart listesi" yalnız ekran okuyucuya söylenir.
 *
 * Veri: demo ofisi, Atlas Gıda Ekim 2026 banka hareketleri; yalnız okur.
 */
test.use({ viewport: { width: 375, height: 812 } });

test("banka: dar ekranda kartlar taşmaz", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?period=2026-10`));
  await expect(page.getByText("Ödenecek sosyal güvenlik kesintileri").first()).toBeAttached();
  await expect(page.getByText("Mobil görünüm: kart listesi")).toHaveClass(/sr-only/);

  // Kaydırma kutusu dışındaki hiçbir öğe ekranın sağından taşmaz.
  const overflowing = await page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    return [...document.querySelectorAll("main *")]
      .filter((e) => {
        const r = e.getBoundingClientRect();
        return (
          r.width > 0 &&
          r.right > vw + 1 &&
          !e.closest('[class*="overflow-x-auto"],[class*="overflow-auto"],[role=tablist],nav,.sr-only')
        );
      })
      .map((e) => (e as HTMLElement).innerText.slice(0, 60));
  });
  expect(overflowing).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
});
