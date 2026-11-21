import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Ba'ya masraf belgeleri (#575): masraf olarak işlenen alış belgeleri
 * (kâğıt fatura, serbest meslek makbuzu) satıcının VKN / TCKN'siyle Form Ba'ya
 * girer; 5.000 TL sınırı karşı tarafın e-faturaları ve masraf belgeleri
 * birlikte uygulanır. ÖKC / akaryakıt fişi ve gider pusulası girmez.
 * Önceden Ba yalnız e-faturadan hesaplanıyordu.
 *
 * Veri: demo ofisi, Atlas Gıda, Ekim 2026; avukatın serbest meslek makbuzu
 * (Av. Deniz Arslan, 8.550 TL, KDV hariç). Yalnız okur.
 */
test("Ba / Bs: serbest meslek makbuzu satıcı TCKN'siyle Form Ba'da (#575)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const preview = await api.json<{
    babs: {
      ba: {
        rows: {
          counterparty_id: string;
          id_type: string;
          document_count: number;
          expense_count?: number;
          amount: string;
        }[];
      };
    };
  }>(
    await api.get(
      `/v1/tenant/engagements/${engagement}/tax-returns/babs/2026-10`,
    ),
  );
  await api.dispose();
  const lawyer = preview.babs.ba.rows.find(
    (r) => r.counterparty_id === "10000000146",
  );
  expect(lawyer).toMatchObject({
    id_type: "tckn",
    document_count: 1,
    expense_count: 1,
  });
  expect(Number(lawyer!.amount)).toBeGreaterThanOrEqual(5000);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=2026-10`),
  );
  const kinds = page.getByRole("group", { name: "Beyan türü" });
  await kinds.getByRole("button", { name: "Ba / Bs", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: /^Form Ba \(alışlar\)/ }),
  ).toBeVisible();
  const row = page.getByRole("row").filter({ hasText: "Av. Deniz Arslan" });
  await expect(row).toContainText("10000000146");
  await expect(row).toContainText("(1 masraf belgesi)");
});
