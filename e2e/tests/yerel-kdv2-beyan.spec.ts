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
 * Veri: demo ofisi, Atlas Gıda, Ekim 2026. Demoda tevkifatlı alış yok:
 * beyan açılır, "verilmez" der, ödenecek 0. Hesap birim testinde
 * (TestComputeKDV2). Yalnız okur.
 */
test("KDV2: ayrı beyan türü, tevkifatlı alış yoksa verilmez der (#575)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const preview = await api.json<{
    kind: string;
    calendar: { due_date: string };
    kdv2: { rows: unknown[]; totals: { payable: string } };
  }>(
    await api.get(
      `/v1/tenant/engagements/${engagement}/tax-returns/kdv2/2026-10`,
    ),
  );
  await api.dispose();
  expect(preview.kind).toBe("kdv2");
  expect(preview.calendar.due_date).toBe("2026-11-30");
  expect(preview.kdv2.totals.payable).toBe("0.00");

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
  await expect(
    page
      .getByText(
        "Bu dönem tevkifatlı alış faturası yok; 2 No.lu KDV beyannamesi verilmez.",
      )
      .first(),
  ).toBeVisible();
  await expectNoRawText(page.getByRole("main"));
});
