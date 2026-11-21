import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

type Schedule = {
  enabled: boolean;
  day_of_month: number;
  recipient: string | null;
  next_run: string | null;
};

/**
 * Otomatik aylık rapor ayın 29-31'inde de gönderilebilir (#780, ofis
 * "ayın 29'u" istedi). Önceden gün 1-28 ile sınırlıydı. Ay seçilen günden
 * kısaysa ayın son günü gönderilir.
 *
 * Veri: demo ofisi, Atlas Gıda. Zamanlama baştaki haline geri yazılır.
 */
test("Rapor zamanlaması: ayın 29'u seçilebilir, kısa ayda son gün (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const url = `/v1/tenant/engagements/${engagement}/report/schedule`;
  const before = await api.json<Schedule>(await api.get(url));
  const restore = {
    enabled: before.enabled,
    day_of_month: before.day_of_month,
    ...(before.recipient ? { recipient: before.recipient } : {}),
  };

  // API: 31 kabul edilir; sonraki gönderim o ayın son gününe çekilir.
  const on = await api.json<Schedule>(
    await api.put(url, {
      enabled: true,
      day_of_month: 31,
      recipient: "rapor@demo.hakan.local",
    }),
  );
  expect(on.day_of_month).toBe(31);
  const [y, m, d] = (on.next_run ?? "").split("-").map(Number);
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  expect(
    d === last || on.next_run === new Date().toISOString().slice(0, 10),
  ).toBe(true);
  expect(
    (await api.put(url, { enabled: false, day_of_month: 32 })).status(),
  ).toBe(400);
  await api.put(url, { enabled: false, day_of_month: 1 });

  // Ekran: gün listesinde 29 var; kaydedilince sunucuya 29 yazılır.
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/report`));
  const card = page
    .getByText("Otomatik aylık gönderim", { exact: true })
    .locator("xpath=ancestor::*[.//*[@role='combobox']][1]");
  await card.getByRole("combobox", { name: "Gönderim günü" }).click();
  await page
    .getByRole("option", { name: "Ayın 29. günü (ay kısaysa son gün)" })
    .click();
  await card.getByRole("button", { name: "Kaydet", exact: true }).click();
  await expect(
    page.getByText("Otomatik gönderim ayarı kaydedildi."),
  ).toBeVisible();
  const saved = await api.json<Schedule>(await api.get(url));
  expect(saved.day_of_month).toBe(29);

  await api.put(url, restore);
  await api.dispose();
});
