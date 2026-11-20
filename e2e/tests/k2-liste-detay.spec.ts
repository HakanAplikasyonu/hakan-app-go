import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Liste-detay deseni (#564): listenin altındaki kayda tıklayınca kayıt yan
 * panelde açılır, liste yerinden oynamaz; panelden listedeki önceki kayda
 * geçilir; panel kapanınca kullanıcı aynı yerdedir.
 *
 * Veri: Atlas Gıda'nın demo banka hareketleri; yalnızca okur.
 */
test("banka listesinde alttaki kayıt yan panelde açılır, liste konumu korunur", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  let engagement = "";
  try {
    engagement = await engagementOf(api, DEMO.atlas.title);
  } finally {
    await api.dispose();
  }
  // Kısa pencere: liste ekrana sığmaz, alttaki satır kaydırmayla görünür.
  await page.setViewportSize({ width: 1280, height: 520 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank`));

  const links = page.getByRole("link", { name: /^İlişkili kayıtlar: / });
  await expect(links.first()).toBeVisible();
  expect(await links.count()).toBeGreaterThan(2);
  const last = links.last();
  await last.scrollIntoViewIfNeeded();
  const top = async () => Math.round((await last.boundingBox())!.y);
  const before = await top();

  // Tıkla: panel açılır, satır olduğu yerde kalır (sayfa başa zıplamaz).
  await last.click();
  await page.waitForURL((u) => u.searchParams.get("kayit")?.startsWith("bank_transaction:") === true);
  const panel = page.getByTestId("focused-record");
  await expect(panel.getByRole("heading", { name: "Seçili kayıt: Banka hareketi" })).toBeVisible();
  expect(Math.abs((await top()) - before)).toBeLessThanOrEqual(2);
  const opened = new URL(page.url()).searchParams.get("kayit");

  // Listedeki önceki kayda geç: panel o kayda geçer, liste yine yerinde.
  await panel.getByRole("button", { name: "Listedeki önceki kayıt (k)" }).click();
  await page.waitForURL((u) => {
    const now = u.searchParams.get("kayit");
    return !!now && now.startsWith("bank_transaction:") && now !== opened;
  });
  await expect(panel.getByRole("heading", { name: "Seçili kayıt: Banka hareketi" })).toBeVisible();
  expect(Math.abs((await top()) - before)).toBeLessThanOrEqual(2);

  // Klavyeyle sonraki kayda: ilk açılan kayda dönülür.
  await page.keyboard.press("j");
  await page.waitForURL((u) => u.searchParams.get("kayit") === opened);

  // Kapat: panel gider, kullanıcı aynı yerde.
  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  expect(Math.abs((await top()) - before)).toBeLessThanOrEqual(2);
});
