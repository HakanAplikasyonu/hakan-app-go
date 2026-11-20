import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * "Yemeğe çıktı" ne? (#710): menüdeki "Yevmiye ve çıktı" yanlış okunuyordu.
 * Ekranın adı "Fişler ve aktarım"; başlığın altında ne işe yaradığı sade
 * Türkçe yazar. Eski ad hiçbir yerde görünmez.
 *
 * Veri: demo ofisi; yalnız okur.
 */
test("menü ve sayfa: Fişler ve aktarım, ne işe yaradığı yazar (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());
  const link = page.getByRole("link", { name: "Fişler ve aktarım" }).first();
  await expect(link).toHaveAttribute("href", /\/journals/);
  await expect(page.getByText("Yevmiye ve çıktı")).toHaveCount(0);

  await page.goto(officePath("/journals"));
  await expect(
    page.getByRole("heading", { level: 1, name: "Fişler ve aktarım" }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Yevmiye fişlerini hazırlayın ve onaylayın; onaylı fişleri Luca, Zirve ya da Excel dosyası olarak alın veya masaüstü ajanıyla programa aktarın.",
    ),
  ).toBeVisible();
  await expect(page.getByText("Yevmiye ve çıktı")).toHaveCount(0);
});
