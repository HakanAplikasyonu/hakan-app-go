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
 * Luca'ya aktarılmış fişin tamamen iptali (#710, ofis kararı A.4): e-defter
 * verilmeden önce düzeltme fişi yapılmaz. "Aktarımı iptal et" gerekçe ister
 * ve programda hangi fişin silineceğini söyler; fiş aktarımdan çıkar ve
 * taslağa döner (neden fişte görünür), düzeltilip yeniden onaylanır ve
 * yeniden aktarılır. Aktarılmamış fiş iptal edilemez (409).
 *
 * Veri: her koşuda yeni mükellef; fiş, onay ve Luca çıktısı API ile.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Aktarım İptal ${run} Ltd. Şti.` };
const PERIOD = "2026-10";
const description = `E2E aktarım iptali ${run}`;
const reason = "Tutar yanlış; Luca'da silinecek";

type Draft = {
  uuid: string;
  entry_no: number;
  status: string;
  description: string;
  approval_reverted?: { reason: string } | null;
};

test.describe("aktarılmış fişin iptali (#710)", () => {
  let api: Api;
  let engagement = "";
  let draft: Draft;
  const base = () => `/v1/tenant/engagements/${engagement}`;
  const find = async () =>
    api.json<Draft>(await api.get(`${base()}/journal-drafts/${draft.uuid}`));

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "aktiptal");
    draft = await api.json<Draft>(
      await api.post(`${base()}/journal-drafts`, {
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
    // Aktarılmamış fiş iptal edilemez.
    const early = await api.post(
      `${base()}/journal-drafts/${draft.uuid}/cancel-export`,
      { reason },
    );
    expect(early.status()).toBe(409);
    const approved = await api.post(
      `${base()}/journal-drafts/${draft.uuid}/approve`,
    );
    expect(approved.status(), await approved.text()).toBeLessThan(300);
    const exported = await api.post(`${base()}/journal-exports`, {
      driver: "luca",
      drafts: [draft.uuid],
    });
    expect(exported.status(), await exported.text()).toBeLessThan(300);
    expect((await find()).status).toBe("exported");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Aktarımı iptal et: fiş taslağa döner, yeniden onaylanıp aktarılır", async ({
    page,
  }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/journals?taxpayer=${engagement}&period=${PERIOD}`),
    );
    const row = page.locator("tr", { hasText: description }).first();
    await expect(row).toContainText("Aktarıldı");

    await row.getByRole("button", { name: "Aktarımı iptal et" }).click();
    const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
    await expect(dialog).toContainText(
      `${draft.entry_no} numaralı fişin aktarımını iptal et`,
    );
    await expect(dialog).toContainText("Düzeltme fişi oluşturulmaz.");
    await expect(dialog).toContainText(
      `15.10.2026 tarihli ${draft.entry_no} numaralı fişi programda siz silin`,
    );
    await dialog.getByRole("textbox").fill(reason);
    await dialog.getByRole("button", { name: "Aktarımı iptal et" }).click();

    await expect(
      page.getByText(
        `Fiş taslağa döndü. Luca'daki ${draft.entry_no} numaralı fişi silmeyi unutmayın.`,
      ),
    ).toBeVisible();
    await expect(row).toContainText("Taslak");
    await expect(row.getByTestId("approval-reverted")).toContainText(reason);

    const cancelled = await find();
    expect(cancelled.status).toBe("draft");
    expect(cancelled.approval_reverted?.reason).toBe(reason);

    // Düzeltilip yeniden onaylanır ve yeniden aktarılır.
    const again = await api.post(
      `${base()}/journal-drafts/${draft.uuid}/approve`,
    );
    expect(again.status(), await again.text()).toBeLessThan(300);
    const reexported = await api.post(`${base()}/journal-exports`, {
      driver: "luca",
      drafts: [draft.uuid],
    });
    expect(reexported.status(), await reexported.text()).toBeLessThan(300);
    expect((await find()).status).toBe("exported");
  });
});
