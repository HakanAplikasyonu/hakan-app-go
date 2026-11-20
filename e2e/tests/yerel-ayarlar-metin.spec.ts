import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Ayarlar, yeni müşavir gözüyle (#710): sayfa "Ofis ayarları" der
 * (uygulamanın geri kalanı gibi "ofis"; "organizasyon" değil); destek kartı
 * "widget" yerine "destek penceresi" der ve kimliğin nereden alınacağını
 * söyler.
 *
 * Veri: demo ofisi; yalnız okur, kaydetmez.
 */
test("ayarlar: ofis dili, destek kartında İngilizce sözcük yok (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/settings"));

  await expect(
    page.getByRole("heading", { level: 1, name: "Ofis ayarları" }),
  ).toBeVisible();
  await expect(page.getByText("Organizasyon ayarları")).toHaveCount(0);
  await expect(
    page.getByText(/Ofisinizin adı; menüde ve mükellef portalında görünür\./),
  ).toBeVisible();

  await expect(page.getByLabel("Destek penceresi kimliği")).toBeVisible();
  await expect(page.getByText(/widget kimliği|widget'ı/)).toHaveCount(0);
});
