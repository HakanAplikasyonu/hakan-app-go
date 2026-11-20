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
 * Beyan sayfasında bölüm özeti (#710): Z raporları, faturalar, banka
 * hareketleri ve KDV'li fişler; her biri "x / y işlendi", yok sayılan ve
 * bekleyen sayısıyla. Bölüm adı o dönemin listesine götürür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ekim 2026'ya iki TL satış
 * faturası türetilir. Z raporu, banka ve fiş yok.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bölüm Özeti ${run} A.Ş.` };

type Section = {
  key: string;
  total: number;
  processed: number;
  ignored: number;
};

test.describe("beyan bölüm özeti (#710)", () => {
  let api: Api;
  let engagement = "";

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(300_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bolum");
    for (const file of [
      "atlas-01-temel-satis.xml",
      "atlas-02-ticari-satis.xml",
    ]) {
      const up = await api.upload(
        `/v1/tenant/engagements/${engagement}/ubl-uploads`,
        deriveUbl(file, taxpayer.vkn, taxpayer.title),
      );
      expect(up.status(), await up.text()).toBeLessThan(300);
    }
    await waitForUploads(api, engagement, 2);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Ekim KDV1: faturalar 2 / 2 işlendi, diğer bölümlerde kayıt yok", async ({
    page,
  }) => {
    const preview = await api.json<{ sections: Section[] }>(
      await api.get(
        `/v1/tenant/engagements/${engagement}/tax-returns/kdv1/2026-10`,
      ),
    );
    expect(preview.sections.map((s) => s.key)).toEqual([
      "retail",
      "invoices",
      "bank",
      "expenses",
    ]);
    expect(preview.sections[1]).toEqual({
      key: "invoices",
      total: 2,
      processed: 2,
      ignored: 0,
    });

    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/engagements/${engagement}/tax-returns?period=2026-10`),
    );
    const box = page.getByRole("region", { name: "Bölüm özeti" });
    await expect(box).toBeVisible();
    await expect(box.getByTestId("tax-return-section-invoices")).toContainText(
      "2 / 2 işlendi",
    );
    for (const key of ["retail", "bank", "expenses"]) {
      await expect(box.getByTestId(`tax-return-section-${key}`)).toContainText(
        "Bu ay kayıt yok",
      );
    }
    await expect(box).not.toContainText("bekliyor");

    // Bölüm adı o dönemin fatura listesine götürür.
    await box.getByRole("link", { name: "Faturalar", exact: true }).click();
    await expect(page).toHaveURL(
      new RegExp(`/engagements/${engagement}/invoices\\?period=2026-10`),
    );
  });
});
