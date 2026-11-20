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
 * Fişe elle kayıt bağlama (#710): kaynağı olmayan (elle girilmiş) fiş
 * "bu kayda bağlı başka kayıt yok" der; kullanıcı fişin ayındaki faturayı
 * arayıp bağlar. Bağ iki yönde "Elle bağlandı" olarak görünür; fişin
 * satırları değişmez; bağ kaldırılınca liste yine boşalır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ekim 2026'ya bir satış
 * faturası türetilir ve API ile elle bir fiş açılır.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Elle Bağ ${run} A.Ş.` };

type Related = {
  items: { kind: string; uuid: string | null; relation: string }[];
};

test.describe("fişe elle kayıt bağlama (#710)", () => {
  let api: Api;
  let engagement = "";
  let invoice = { uuid: "", invoice_number: "" };
  let draft = "";
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(300_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "ellebag");
    const up = await api.upload(
      `${base()}/ubl-uploads`,
      deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    const items = (
      await api.json<{ items: { uuid: string; invoice_number: string }[] }>(
        await api.get(`${base()}/invoices?period=2026-10&limit=10`),
      )
    ).items;
    expect(items.length).toBe(1);
    invoice = items[0];

    const created = await api.post(`${base()}/journal-drafts`, {
      entry_date: "2026-10-15T00:00:00Z",
      description: `Elle tahsilat ${run}`,
      lines: [
        { account: "100", debit: "492.00", credit: "0" },
        { account: "120", debit: "0", credit: "492.00" },
      ],
    });
    expect(created.status(), await created.text()).toBeLessThan(300);
    draft = (await api.json<{ uuid: string }>(created)).uuid;
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("elle fişe fatura bağlanır, iki yönde görünür, bağ kaldırılır", async ({
    page,
  }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(
        `/engagements/${engagement}/journals?kayit=journal_draft:${draft}`,
      ),
    );
    const panel = page.getByTestId("focused-record");
    await expect(
      panel.getByRole("heading", { name: "Seçili kayıt: Fiş" }),
    ).toBeVisible();
    await expect(panel.getByTestId("related-empty")).toHaveText(
      "Bu kayda bağlı başka kayıt yok.",
    );

    await panel.getByRole("button", { name: "Kayıt bağla" }).click();
    const form = panel.getByRole("region", { name: "Bu fişe kayıt bağla" });
    await expect(
      form.getByRole("button", { name: "Fatura", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await form.getByLabel("Ara").fill(invoice.invoice_number.slice(-6));
    await form
      .getByRole("button", {
        name: `${invoice.invoice_number} kaydını bu fişe bağla`,
      })
      .click();

    await expect(
      panel
        .getByLabel(new RegExp(`^Fatura: ${invoice.invoice_number}`))
        .first(),
    ).toBeVisible();
    await expect(panel.getByTestId("related-definite")).toContainText(
      "Elle bağlandı",
    );

    // Faturadan bakınca fiş görünür; fişin satırları kaynaksız kalır.
    const fromInvoice = await api.json<Related>(
      await api.get(`${base()}/related/invoice/${invoice.uuid}`),
    );
    expect(fromInvoice.items).toContainEqual(
      expect.objectContaining({
        kind: "journal_draft",
        uuid: draft,
        relation: "manual",
      }),
    );
    const detail = await api.json<{ items: { source_kind?: string }[] }>(
      await api.get(`${base()}/journal-drafts/${draft}`),
    );
    expect(detail.items.every((l) => !l.source_kind)).toBe(true);

    // Bağı kaldır: liste yine boş.
    await panel
      .getByRole("button", {
        name: `${invoice.invoice_number} bağını kaldır`,
      })
      .click();
    await expect(panel.getByTestId("related-empty")).toBeVisible();
  });
});
