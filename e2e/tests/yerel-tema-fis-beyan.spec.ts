import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Ortak tema fiş ve beyan ekranlarında (#710, #708 dili): durumlar sade
 * rozetle (ince çerçeve, küçük renkli nokta, metin) görünür; anlamı metin
 * taşır. Fiş: taslak gri, onaylı yeşil. Beyan: taslak gri.
 *
 * Veri: her koşuda yeni mükellef; fiş ve onay API ile.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Tema Fiş ${run} Ltd. Şti.` };
const PERIOD = "2026-10";

test("fiş ve beyan durumları sade rozetle görünür (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tema");
  const base = `/v1/tenant/engagements/${engagement}`;
  const draft = async (description: string) =>
    api.json<{ uuid: string }>(
      await api.post(`${base}/journal-drafts`, {
        entry_type: "mahsup",
        entry_date: `${PERIOD}-15T00:00:00Z`,
        description,
        lines: [
          { account: "120.01.001", debit: "600.00", credit: "0" },
          { account: "600.01", debit: "0", credit: "500.00" },
          { account: "391.01", debit: "0", credit: "100.00" },
        ],
      }),
    );
  await draft(`E2E tema taslak ${run}`);
  const approved = await draft(`E2E tema onaylı ${run}`);
  const ok = await api.post(`${base}/journal-drafts/${approved.uuid}/approve`);
  expect(ok.status(), await ok.text()).toBeLessThan(300);

  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/journals?taxpayer=${engagement}&period=${PERIOD}`),
  );
  const pill = (description: string) =>
    page
      .locator("tr", { hasText: description })
      .first()
      .locator("[data-tone]")
      .first();
  await expect(pill(`E2E tema taslak ${run}`)).toHaveText("Taslak");
  await expect(pill(`E2E tema taslak ${run}`)).toHaveAttribute(
    "data-tone",
    "neutral",
  );
  await expect(pill(`E2E tema onaylı ${run}`)).toHaveText("Onaylı");
  await expect(pill(`E2E tema onaylı ${run}`)).toHaveAttribute(
    "data-tone",
    "ok",
  );

  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=${PERIOD}`),
  );
  const status = page.getByRole("region", { name: "Beyan durumu" });
  await expect(status.locator("[data-tone]").first()).toHaveText("Taslak");
  await expect(status.locator("[data-tone]").first()).toHaveAttribute(
    "data-tone",
    "neutral",
  );
  await api.dispose();
});
