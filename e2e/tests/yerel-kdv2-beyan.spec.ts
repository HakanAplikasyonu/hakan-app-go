import { expect, test } from "@playwright/test";

import {
  Api,
  DEMO,
  engagementOf,
  expectNoRawText,
  officePath,
  uiLogin,
} from "./lib";

/**
 * 2 No.lu KDV beyannamesi ayrı beyan türü (#575): alışlarda tevkif edilen
 * KDV'yi sorumlu sıfatıyla beyan eder. Önceden yalnız KDV1'de "ödendiği
 * varsayılarak" indiriliyordu; ayrı beyan yoktu.
 *
 * Veri: demo ofisi, Atlas Gıda. Ekim 2026'da demo yabancı hizmet faturaları
 * (Google Ads, SolidCAD; #804) KDV2 doğurur: ödenecek sıfırdan büyük, işlem
 * tablosu dolu. Eylül 2026'da tevkifatlı alış yok: beyan açılır, "verilmez"
 * der, ödenecek 0. Hesap birim testinde (TestComputeKDV2). Yalnız okur.
 */
test("KDV2: ayrı beyan türü, tevkifatlı alış yoksa verilmez der (#575)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  type Preview = {
    kind: string;
    calendar: { due_date: string };
    kdv2: { rows: unknown[]; totals: { payable: string } };
  };
  const preview = (period: string) =>
    api.get(`/v1/tenant/engagements/${engagement}/tax-returns/kdv2/${period}`).then((r) => api.json<Preview>(r));
  const october = await preview("2026-10");
  expect(october.kind).toBe("kdv2");
  expect(october.calendar.due_date).toBe("2026-11-30");
  expect(Number(october.kdv2.totals.payable)).toBeGreaterThan(0);
  expect(october.kdv2.rows.length).toBeGreaterThan(0);
  const september = await preview("2026-09");
  await api.dispose();
  expect(september.kind).toBe("kdv2");
  expect(september.kdv2.totals.payable).toBe("0.00");
  expect(september.kdv2.rows).toHaveLength(0);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=2026-10`),
  );
  const kinds = page.getByRole("group", { name: "Beyan türü" });
  await kinds.getByRole("button", { name: "KDV2", exact: true }).click();
  await expect(
    kinds.getByRole("button", { name: "KDV2", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  const status = page.getByRole("region", { name: "Beyan durumu" });
  await expect(status).toContainText("2 No.lu KDV beyannamesi");
  await expect(
    page.getByRole("heading", { name: "Tevkifat uygulanan işlemler" }),
  ).toBeVisible();
  const empty = page.getByText(
    "Bu dönem tevkifatlı alış faturası yok; 2 No.lu KDV beyannamesi verilmez.",
  );
  await expect(empty).toHaveCount(0);
  await expectNoRawText(page.getByRole("main"));

  // Eylül: tevkifatlı alış yok, beyan verilmez.
  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=2026-09`),
  );
  await kinds.getByRole("button", { name: "KDV2", exact: true }).click();
  await expect(status).toContainText("2 No.lu KDV beyannamesi");
  await expect(empty.first()).toBeVisible();
  await expectNoRawText(page.getByRole("main"));
});
