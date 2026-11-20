import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Ofis genelinde programa aktarımlar (#574): onaylanmış ama programa
 * aktarılmamış fişi olan mükellef ve dönem ofis sayfasında listelenir;
 * satır o mükellefin Fişler ekranını dönemiyle açar.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve API ile bir onaylı fiş
 * (Ekim 2026). Başka dosyanın kaydına dokunmaz; çıktı alınmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Aktarım Bekleyen ${run} Ltd. Şti.` };
const description = `E2E ofis aktarımı ${run}`;
const PERIOD = "2026-10";

test.describe("Ofis geneli bekleyen aktarımlar (#574)", () => {
  let engagement = "";
  let api: Api;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "aktarim");
    const base = `/v1/tenant/engagements/${engagement}`;
    const draft = await api.json<{ uuid: string }>(
      await api.post(`${base}/journal-drafts`, {
        entry_date: `${PERIOD}-20T00:00:00Z`,
        description,
        lines: [
          { account: "770.01", debit: "125.40" },
          { account: "100.01", credit: "125.40" },
        ],
      }),
    );
    await api.json(await api.post(`${base}/journal-drafts/${draft.uuid}/approve`));
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("uç onaylı fişi mükellef ve dönemiyle listeler", async () => {
    const page = await api.json<{
      items: { engagement_uuid: string; taxpayer_title: string; period: string; approved: number; in_transfer: number; failed: number }[];
    }>(await api.get("/v1/tenant/journal-transfers/pending?limit=200"));
    const row = page.items.find((r) => r.engagement_uuid === engagement);
    expect(row, "yeni mükellefin bekleyen satırı").toBeTruthy();
    expect(row).toMatchObject({ taxpayer_title: taxpayer.title, period: PERIOD, approved: 1, in_transfer: 0, failed: 0 });
  });

  test("bekleyen satır görünür ve mükellefin aktarım ekranını açar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/transfers"));
    await expect(page.getByRole("heading", { name: "Programa aktarımlar", level: 1 })).toBeVisible();

    // Bekleyenler tablosunda unvanla ara: başka dosyaların mükellefleri de
    // listede olabilir.
    await page.getByPlaceholder("Tabloda ara…").first().fill(taxpayer.title);
    const row = page.getByRole("row").filter({ hasText: taxpayer.title });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText("Ekim 2026");
    await expect(row).toContainText("1 onaylı fiş çıktı bekliyor");
    // Ham kod görünmez: dönem metinle yazılır.
    await expect(row).not.toContainText(PERIOD);

    await row.click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/journals`));
    // Dönem seçili gelir: fiş onaylı olarak listededir.
    const entry = page.locator("tr", { hasText: description }).first();
    await expect(entry).toBeVisible();
    await expect(entry).toContainText("Onaylı");
  });
});
