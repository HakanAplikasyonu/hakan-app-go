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
 * Damga vergisi beyannamesi ayrı beyan türü (#575): MUHSGK vermeyen
 * mükellef damga vergisini bu beyanla öder. Kağıtlar elle girilir: nispi
 * satırda matrah × binde oran / 1000, maktu satırda girilen vergi.
 *
 * Veri: demo ofisi, Atlas Gıda, Eylül 2026 (başka test damga yazmaz).
 * Taslak baştan ve sonda boşaltılır; test tekrar koşulabilir.
 */
test("Damga: kağıtlar elle girilir, vergi ve toplam hesaplanır (#575)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const url = `/v1/tenant/engagements/${engagement}/tax-returns/damga/2026-09`;
  const reset = async () =>
    expect(
      (await api.put(url, { adjustments: {}, acknowledged: [] })).status(),
    ).toBe(200);
  await reset();
  const empty = await api.json<{
    kind: string;
    calendar: { due_date: string };
    damga: { rows: unknown[]; totals: { payable: string } };
  }>(await api.get(url));
  expect(empty.kind).toBe("damga");
  expect(empty.calendar.due_date).toBe("2026-10-26");
  expect(empty.damga.rows).toHaveLength(0);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=2026-09`),
  );
  const kinds = page.getByRole("group", { name: "Beyan türü" });
  await kinds.getByRole("button", { name: "Damga", exact: true }).click();
  await expect(
    kinds.getByRole("button", { name: "Damga", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Beyan durumu" }),
  ).toContainText("Damga vergisi beyannamesi");
  await expect(
    page.getByRole("heading", { name: "Damga vergisine tabi kağıtlar" }),
  ).toBeVisible();
  await expect(
    page.getByText(/Bu dönem için kağıt girilmedi/).first(),
  ).toBeVisible();

  // Nispi: kira sözleşmesi 120.000 TL, binde 1,89 → 226,80.
  await page.getByRole("button", { name: "Kağıt ekle" }).click();
  const first = page.getByRole("group", { name: "Kağıt 1" });
  await first.getByLabel("Kağıdın türü").fill("Kira sözleşmesi");
  await first.getByLabel("Matrah").fill("120.000");
  await first.getByLabel("Oran (binde)").fill("1,89");
  // Maktu: yalnız vergi tutarı.
  await page.getByRole("button", { name: "Kağıt ekle" }).click();
  const second = page.getByRole("group", { name: "Kağıt 2" });
  await second.getByLabel("Kağıdın türü").fill("Bilanço");
  await second.getByLabel("Maktu vergi").fill("1.106,90");

  const [saved] = await Promise.all([
    page.waitForResponse(
      (r) =>
        r.request().method() === "PUT" &&
        new URL(r.url()).pathname.endsWith("/tax-returns/damga/2026-09"),
    ),
    page.getByRole("button", { name: "Kağıtları kaydet" }).click(),
  ]);
  expect(saved.status()).toBe(200);

  const table = page.getByRole("table").filter({ hasText: "Kira sözleşmesi" });
  const kira = table.getByRole("row").filter({ hasText: "Kira sözleşmesi" });
  await expect(kira).toContainText("226,80");
  await expect(kira).toContainText("‰1,89");
  const bilanco = table.getByRole("row").filter({ hasText: "Bilanço" });
  await expect(bilanco).toContainText("Maktu");
  await expect(bilanco).toContainText("1.106,90");
  await expect(
    table.getByRole("row").filter({ hasText: "Toplam" }),
  ).toContainText("1.333,70");
  await expectNoRawText(page.getByRole("main"));

  const after = await api.json<{
    record: { adjustments: { stamp_lines?: unknown[] } };
    damga: { totals: { count: number; payable: string } };
  }>(await api.get(url));
  expect(after.record.adjustments.stamp_lines).toHaveLength(2);
  expect(after.damga.totals).toMatchObject({ count: 2, payable: "1333.70" });

  await reset();
  await api.dispose();
});
