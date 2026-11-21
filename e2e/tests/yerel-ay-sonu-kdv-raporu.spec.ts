import { expect, test } from "@playwright/test";

import {
  Api,
  DEMO,
  engagementOf,
  expectNoRawText,
  officePath,
  uiLogin,
} from "./lib";

type Settings = {
  report_enabled: boolean;
  report_day: number;
  report_ready_on: string;
};

/**
 * Ay sonu KDV raporu (#780 B, 21.11 ofis görüşmesi): Raporlar sayfasında
 * bütün mükelleflerin bu ayki satış ve alış KDV'si, devreden KDV ve
 * ödenecek / devredecek tahmini tek tabloda. Devreden KDV'yi ofis satırdan
 * girer. Ofis ayarından kapatılır; varsayılan açık, ayın 29'u.
 *
 * Veri: demo ofisi, Atlas Gıda; içinde bulunulan ay. Girilen devir ve ayar
 * sonda geri alınır.
 */
test("Ay sonu KDV raporu: tüm mükellefler, devir girişi, ayardan kapanır (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const settingsUrl = "/v1/tenant/vat-tracking-settings";
  const settings = await api.json<Settings>(
    await api.put(settingsUrl, { report_enabled: true, report_day: 29 }),
  );
  expect(settings.report_day).toBe(29);
  const period = settings.report_ready_on.slice(0, 7);
  const [y, m] = period.split("-").map(Number);
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  expect(settings.report_ready_on).toBe(
    `${period}-${String(Math.min(29, last)).padStart(2, "0")}`,
  );
  const opening = `/v1/tenant/engagements/${engagement}/vat-forecast/openings/${period}`;
  await api.delete(opening);
  expect(
    (
      await api.put(settingsUrl, { report_enabled: true, report_day: 0 })
    ).status(),
  ).toBe(400);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/reports"));
  const table = page.getByRole("table", { name: "Ay sonu KDV raporu" });
  await expect(table).toBeVisible();
  for (const col of [
    "Satış KDV'si",
    "Alış KDV'si",
    "Devreden KDV",
    "Ödenecek KDV",
  ]) {
    await expect(
      table.getByRole("columnheader", { name: col, exact: true }),
    ).toBeVisible();
  }
  const row = table.getByRole("row").filter({ hasText: DEMO.atlas.title });
  await expect(row).toHaveCount(1);
  await expect(
    table.getByRole("row").filter({ hasText: "Toplam" }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("status").filter({ hasText: /rapor hazır|hazır sayılır/ }),
  ).toBeVisible();

  // Devreden KDV'yi ofis girer: satırdaki tek tık.
  await row
    .getByRole("button", {
      name: `Devri gir: ${DEMO.atlas.title}`,
    })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Devreden KDV (TL)").fill("12.500,00");
  await dialog
    .getByLabel("Not (isteğe bağlı)")
    .fill("Önceki muhasebeciden devir");
  await dialog.getByRole("button", { name: "Kaydet", exact: true }).click();
  await expect(page.getByText("Devreden KDV kaydedildi.")).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(row.getByRole("cell").nth(3)).toContainText("12.500,00");
  await expectNoRawText(page.getByRole("main"));

  const fc = await api.json<{ totals: { carried_in: string } }>(
    await api.get(
      `/v1/tenant/engagements/${engagement}/vat-forecast?period=${period}`,
    ),
  );
  expect(fc.totals.carried_in).toBe("12500.00");

  // Ofis ayarından kapatılır: rapor görünmez, ayarlar sayfasında anahtar.
  await api.put(settingsUrl, { report_enabled: false, report_day: 29 });
  await page.reload();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(table).toHaveCount(0);
  await page.goto(officePath("/settings"));
  const section = page.getByRole("region", { name: "KDV takibi" });
  const toggle = section.getByRole("switch", {
    name: "Raporlar sayfasında ay sonu KDV raporunu göster",
  });
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect(toggle).toBeChecked();
  const back = await api.json<Settings>(await api.get(settingsUrl));
  expect(back.report_enabled).toBe(true);

  await api.delete(opening);
  await api.dispose();
});
