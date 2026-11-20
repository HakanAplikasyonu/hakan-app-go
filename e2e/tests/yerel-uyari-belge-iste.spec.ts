import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Uyarı İncele panelinden mükelleften belge isteme (#570): "Faturası olmayan
 * tahsilat" uyarısında "Mükelleften belge iste" belge talebini açar, uyarı
 * isteğin notuyla "Gördüm" olarak kapanır ve açık uyarılardan düşer.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef (k2-akislar.spec.ts ile
 * aynı kurgu): ATL…102 faturası türetilir, Ziraat ekstresi yüklenir, eşleme
 * çalışınca tahsilat öneride kalır ve uyarı açılır. Başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Belge İste ${run} A.Ş.` };
const PERIOD = "2026-10";
const UNINVOICED = "BNK_UNINVOICED_COLLECTION";

type Task = { kind: string; title: string; note: string };
type Warning = { uuid: string; status: string; resolution_note: string | null };

test.describe("uyarı panelinden belge iste (#570)", () => {
  let engagement = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const uninvoiced = async (status: string) =>
    (
      await api.json<{ items: Warning[]; total: number }>(
        await api.get(`${base()}/warnings?status=${status}&code=${UNINVOICED}&period=${PERIOD}&limit=5`),
      )
    ).items;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bist");

    const up = await api.upload(`${base()}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);

    await api.json(await api.post(`${base()}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
    await api.json(
      await api.upload(`${base()}/bank-statements`, {
        name: "atlas-ziraat-2026-10.sta",
        mimeType: "text/plain",
        buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
      }),
    );
    const match = await api.json<{ findings: string[] }>(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
    expect(match.findings).toContain(UNINVOICED);
    expect(await uninvoiced("open")).toHaveLength(1);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Mükelleften belge iste: talep açılır, uyarı gerekçesiyle kapanır", async ({ page }) => {
    const warning = (await uninvoiced("open"))[0];

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
    await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();

    const sheet = page.getByTestId("warning-sheet");
    await expect(sheet).toBeVisible();
    await sheet.getByRole("button", { name: "Mükelleften belge iste" }).click();

    // Veritabanı: uyarı açık değil, "Gördüm" ve isteğin notuyla kapandı.
    await expect.poll(async () => (await uninvoiced("open")).length).toBe(0);
    const decided = (await uninvoiced("acknowledged")).find((w) => w.uuid === warning.uuid);
    expect(decided?.resolution_note).toBe("Mükelleften belge istendi.");

    // Belge talebi mükellefe açıldı; notunda uyarının metni var.
    const tasks = (
      await api.json<{ items: Task[] }>(await api.get(`${base()}/document-tasks?status=all&limit=100`))
    ).items.filter((t) => t.kind === "document_request" && t.title.startsWith("Eksik belge"));
    expect(tasks).toHaveLength(1);
    expect(tasks[0].title).toBe(`Eksik belge ${PERIOD}`);
    expect(tasks[0].note).toBeTruthy();
  });
});
