import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Fişe cari bağlama (#710, #723'ün devamı): elle girilmiş fişin "Kayıt
 * bağla" bölümünde "Cari kartı" seçilir, mükellefin müşteri / tedarikçi
 * kartları unvanla aranır, "Bağla" ile bağlanır. Fişin ilişkili
 * kayıtlarında cari kartı türüyle ("Müşteri") ve "Elle bağlandı" olarak
 * görünür; fişin satırları değişmez; "Bağı kaldır" ile geri alınır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ekim 2026'ya bir satış
 * faturası türetilir (alıcısı cari kartı olur) ve API ile elle bir fiş açılır.
 */
test("elle fişe cari kartı bağlanır, türüyle görünür, bağ kaldırılır (#710)", async ({
  page,
  browser,
}) => {
  test.setTimeout(300_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  const run = `${Date.now()}`.slice(-6);
  const taxpayer = { vkn: uniqueVkn(), title: `Cari Bağ ${run} A.Ş.` };
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "caribag",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const up = await api.upload(
    `${base}/ubl-uploads`,
    deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);

  // Faturanın alıcısı mükellefin cari kartıdır.
  const cards = await api.json<{
    items: { uuid: string; name: string; type: string }[];
  }>(await api.get(`${base}/link-candidates/customers`));
  expect(cards.items.length).toBeGreaterThan(0);
  const card = cards.items[0];
  expect(card.type).toBe("customer");

  const created = await api.post(`${base}/journal-drafts`, {
    entry_date: "2026-10-15T00:00:00Z",
    description: `Elle tahsilat ${run}`,
    lines: [
      { account: "100", debit: "492.00", credit: "0" },
      { account: "120", debit: "0", credit: "492.00" },
    ],
  });
  expect(created.status(), await created.text()).toBeLessThan(300);
  const draft = (await api.json<{ uuid: string }>(created)).uuid;

  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(
      `/engagements/${engagement}/journals?kayit=journal_draft:${draft}`,
    ),
  );
  const panel = page.getByTestId("focused-record");
  await expect(panel.getByTestId("related-empty")).toBeVisible();

  await panel.getByRole("button", { name: "Kayıt bağla" }).click();
  const form = panel.getByRole("region", { name: "Bu fişe kayıt bağla" });
  await form.getByRole("button", { name: "Cari kartı", exact: true }).click();
  await expect(
    form.getByRole("button", { name: "Cari kartı", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await form.getByLabel("Ara").fill(card.name.slice(0, 6));
  await expect(form.getByTestId("link-candidates")).toContainText("Müşteri");
  await form
    .getByRole("button", { name: `${card.name} kaydını bu fişe bağla` })
    .click();

  const definite = panel.getByTestId("related-definite");
  await expect(definite).toContainText("Cari kartı");
  await expect(definite).toContainText(card.name);
  await expect(definite).toContainText("Elle bağlandı");
  await expect(definite).toContainText("Müşteri");

  const related = await api.json<{
    items: { kind: string; uuid: string | null; relation: string }[];
  }>(await api.get(`${base}/related/journal_draft/${draft}`));
  expect(related.items).toContainEqual(
    expect.objectContaining({
      kind: "customer",
      uuid: card.uuid,
      relation: "manual",
    }),
  );
  const detail = await api.json<{ items: { source_kind?: string }[] }>(
    await api.get(`${base}/journal-drafts/${draft}`),
  );
  expect(detail.items.every((l) => !l.source_kind)).toBe(true);

  await panel
    .getByRole("button", { name: `${card.name} bağını kaldır` })
    .click();
  await expect(panel.getByTestId("related-empty")).toBeVisible();
  await api.dispose();
});
