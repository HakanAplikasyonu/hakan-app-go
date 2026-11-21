import { expect, test } from "@playwright/test";

import { uniqueEmail } from "./lib";

/**
 * Giriş ekranı, yeni müşavir gözüyle (#710): "Passkey", "OTP",
 * "Authenticator", "2FA" gibi İngilizce ya da kısaltma sözcükler görünmez;
 * hatalı şifrede ne olduğu Türkçe yazar ve form yeniden denenebilir.
 *
 * Veri: var olmayan bir e-posta (demo hesaplarının deneme sınırına
 * dokunmaz); giriş yapılmaz.
 */
test("giriş: İngilizce/kısaltma yok, hatalı şifre Türkçe (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/login");
  await expect(
    page.getByRole("button", { name: "Giriş yap", exact: true }),
  ).toBeVisible();
  await expect(page.getByText(/Passkey|OTP|Authenticator|2FA/)).toHaveCount(0);

  await page
    .locator('input[name="email"], input[type="email"]')
    .first()
    .fill(uniqueEmail("girismetin"));
  await page.locator('input[type="password"]').first().fill("Yanlis-Parola-1!");
  await page.getByRole("button", { name: "Giriş yap", exact: true }).click();
  await expect(page.getByText("E-posta veya şifre hatalı")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Giriş yap", exact: true }),
  ).toBeEnabled();
});
