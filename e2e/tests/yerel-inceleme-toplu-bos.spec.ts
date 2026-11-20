import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Toplu onay şeridi sıfırken ne yapılacağını söyler (#710, günlük akış
 * gezisi): eşiğin üstünde öneri yokken "0 öneriyi toplu onayla" yerine
 * "Toplu onaylanacak öneri yok" (pasif) ve "Bu sayfada güveni %99 ve
 * üzerinde öneri yok. Eşiği düşürebilir ya da tek tek inceleyebilirsiniz."
 * Öneri varsa sayı ve "Bu sayfada" ile. Yalnızca okur; onaylamaz.
 */
test("toplu onay şeridi sıfırken açık cümle ve pasif düğme gösterir (#710)", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/review"));
  const slider = page.getByRole("slider", { name: "Güveni en az" });
  await expect(slider).toBeVisible();
  await slider.focus();
  await page.keyboard.press("End"); // eşik %99
  const strip = slider.locator("xpath=ancestor::div[contains(@class,'px-4') and contains(@class,'py-3')][1]");
  const button = strip.getByRole("button").last();
  const label = (await button.innerText()).trim();
  if (label === "Toplu onaylanacak öneri yok") {
    await expect(button).toBeDisabled();
    await expect(strip).toContainText("Bu sayfada güveni %99 ve üzerinde öneri yok.");
    await expect(strip).not.toContainText("0 öneriyi toplu onayla");
  } else {
    expect(label).toMatch(/^[1-9]\d* öneriyi toplu onayla$/);
    await expect(strip).toContainText(/Bu sayfada [1-9]\d* öneri güven %99 ve üzerinde\./);
  }
});
