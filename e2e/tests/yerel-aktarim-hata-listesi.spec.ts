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
 * Luca aktarımında hata verenler ve aktarılamayanlar ayrı listede (#710).
 * Vazgeçilen (iptal edilen) aktarım "programa gitmedi" listesinde nedeniyle
 * durur; listedeki "Yeniden dene" aynı çıktıyı yeniden kuyruğa alır ve
 * liste kalkar. Hata veren aktarımın yolu `yerel-aktarim-yeniden-dene`de.
 *
 * Veri: her koşuda yeni mükellef; onaylı fiş, Luca çıktısı ve aktarım API
 * ile; aktarım ajan almadan iptal edilir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Aktarım Liste ${run} Ltd. Şti.` };
const PERIOD = "2026-10";

type Transfer = { uuid: string; batch_uuid: string; status: string };

test.describe("aktarım hata listesi (#710)", () => {
  let api: Api;
  let engagement = "";
  let cancelled = "";
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "aktliste");
    const draft = await api.json<{ uuid: string }>(
      await api.post(`${base()}/journal-drafts`, {
        entry_type: "mahsup",
        entry_date: `${PERIOD}-15T00:00:00Z`,
        description: `E2E aktarım listesi ${run}`,
        lines: [
          { account: "120.01.001", debit: "600.00", credit: "0" },
          { account: "600.01", debit: "0", credit: "500.00" },
          { account: "391.01", debit: "0", credit: "100.00" },
        ],
      }),
    );
    const approved = await api.post(
      `${base()}/journal-drafts/${draft.uuid}/approve`,
    );
    expect(approved.status(), await approved.text()).toBeLessThan(300);
    const batch = await api.json<{ uuid: string }>(
      await api.post(`${base()}/journal-exports`, {
        driver: "luca",
        drafts: [draft.uuid],
      }),
    );
    const transfer = await api.json<Transfer>(
      await api.post(`${base()}/journal-exports/${batch.uuid}/transfers`, {}),
    );
    cancelled = transfer.uuid;
    const cancel = await api.post(
      `${base()}/journal-transfers/${cancelled}/cancel`,
    );
    expect(cancel.status(), await cancel.text()).toBeLessThan(300);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("vazgeçilen aktarım ayrı listede; listeden yeniden denenir", async ({
    page,
  }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/journals?taxpayer=${engagement}&period=${PERIOD}`),
    );
    const problems = page.getByRole("region", {
      name: "Hata verenler ve aktarılamayanlar",
    });
    await expect(problems).toBeVisible();
    // Hata veren yok; yalnız aktarılamayan.
    await expect(problems.getByRole("alert")).toHaveCount(0);
    const notSent = problems.getByTestId("transfer-not-transferred");
    await expect(notSent).toContainText(
      "1 aktarımdan vazgeçildi; programa gitmedi",
    );
    await expect(notSent).toContainText(
      "Aktarım iptal edildi, fişler programa aktarılmadı.",
    );

    await notSent
      .getByRole("button", { name: `Luca · ${PERIOD} aktarımını yeniden dene` })
      .click();
    await expect(problems).toHaveCount(0);

    const list = (
      await api.json<{ items: Transfer[] }>(
        await api.get(`${base()}/journal-transfers?limit=20`),
      )
    ).items;
    expect(list).toHaveLength(2);
    expect(list.find((t) => t.uuid !== cancelled)?.status).toBe("queued");
  });
});
