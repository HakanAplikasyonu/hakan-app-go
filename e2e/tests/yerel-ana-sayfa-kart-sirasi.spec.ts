import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Ana sayfada asıl iş önce (#710): dönem durumu ve sayaçlar en üstte;
 * "Ofis kurulumunu tamamlayın" tek satır olarak onların altında durur ve
 * küçültülebilir. Küçültme bu tarayıcıda hatırlanır, "Göster" geri açar.
 *
 * Veri: demo ofisi (kurulum sihirbazı bitmemiş); yalnız okur, kurulumu
 * değiştirmez.
 */
test("ana sayfa: dönem durumu üstte, kurulum kartı altta ve küçültülür (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());

  const crown = page.getByRole("heading", { name: /Dönem durumu/ }).first();
  const setup = page.getByRole("region", {
    name: "Ofis kurulumunu tamamlayın",
  });
  await expect(crown).toBeVisible();
  await expect(setup).toBeVisible();
  await expect(setup).toContainText(/adımdan .* tamam/);
  await expect(
    setup.getByRole("link", { name: "Kuruluma devam et" }),
  ).toBeVisible();

  const crownBox = await crown.boundingBox();
  const setupBox = await setup.boundingBox();
  expect(crownBox && setupBox && crownBox.y < setupBox.y).toBe(true);
  // Tek satır: kart yüksekliği küçük kalır.
  expect(setupBox!.height).toBeLessThan(90);

  // Küçült → yalnız küçük bir bağlantı kalır; yenilemede de öyle.
  await setup.getByRole("button", { name: "Küçült" }).click();
  await expect(setup).toHaveCount(0);
  const mini = page.getByRole("link", { name: /^Ofis kurulumu: / });
  await expect(mini).toBeVisible();
  await expect(mini).toHaveAttribute("href", /\/setup$/);
  await page.reload();
  await expect(crown).toBeVisible();
  await expect(mini).toBeVisible();
  await expect(setup).toHaveCount(0);

  // Göster → kart geri gelir.
  await page.getByRole("button", { name: "Göster", exact: true }).click();
  await expect(setup).toBeVisible();
  await expect(mini).toHaveCount(0);
});
