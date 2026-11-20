import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Komuta Merkezi ↔ Uyarılar tutarlılığı (#669 C).
 *
 * - Bakiye tutmazlığı (`BNK_BALANCE_MISMATCH`) gerçek bir üreticiden gelir:
 *   eşleme çalışınca dönemin ekstre bakiye kontrolü (bank-balance-check)
 *   tutmuyorsa engelleyici dönem uyarısı açılır; uyarı ile canlı kontrol aynı
 *   farkı söyler.
 * - Komuta Merkezi matrisindeki açık uyarı sayısı, mükellefin uyarı
 *   listesinin aynı dönemdeki toplamıyla aynıdır (aynı dönem koşulu).
 * - Komuta Merkezi hücresinden gidilen Uyarılar sekmesinde uyarı görünür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Aynı hesaba Ekim 2026'nın
 * iki MT940 ekstresi yüklenir; ikincinin açılışı birincinin kapanışından
 * 500,00 fazladır (arada eksik hareket var). Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bakiye Kontrol ${run} A.Ş.` };
const PERIOD = "2026-10";
const MISMATCH = "BNK_BALANCE_MISMATCH";
const IBAN = "TR760001000519786457841326";

function statement(lines: string[]): Buffer {
  return Buffer.from([`:20:BAKIYE${run}`, `:25:${IBAN}`, ...lines].join("\n") + "\n", "utf8");
}

// 1.000,00 + 500,00 = 1.500,00 (5 Ekim).
const first = statement([
  ":60F:C261001TRY1000,00",
  ":61:2610051005C500,00NTRFNONREF//B1",
  `:86:EFT GELEN BAKIYE A ${run}`,
  ":62F:C261005TRY1500,00",
]);
// Açılış 2.000,00 (önceki kapanış 1.500,00): 500,00 eksik hareket.
const second = statement([
  ":60F:C261020TRY2000,00",
  ":61:2610211021D100,00NTRFNONREF//B2",
  `:86:EFT GIDEN BAKIYE B ${run}`,
  ":62F:C261021TRY1900,00",
]);

type BalanceCheck = { items: { difference: string | null; balanced: boolean }[] };
type Matrix = {
  items: { engagement_uuid: string; cells: { period: string; open_blockers: number; open_warnings: number }[] }[];
  total: number;
};
type WarningList = { items: { code: string; severity: string; period: string | null }[]; total: number };

test.describe("Komuta Merkezi ↔ Uyarılar (#669 C)", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bakiye");
    await api.json(await api.post(`${base()}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
    for (const [i, buffer] of [first, second].entries()) {
      await api.json(
        await api.upload(`${base()}/bank-statements`, { name: `bakiye-${run}-${i + 1}.sta`, mimeType: "text/plain", buffer }),
      );
    }
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("bakiye tutmazlığı eşlemede üretilir; canlı kontrolle aynı fark", async () => {
    const check = await api.json<BalanceCheck>(await api.get(`${base()}/bank-balance-check?period=${PERIOD}`));
    expect(check.items).toHaveLength(1);
    expect(check.items[0].balanced).toBe(false);
    expect(check.items[0].difference).toBe("500.00");

    const match = await api.json<{ findings: string[] }>(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
    expect(match.findings).toContain(MISMATCH);

    const list = await api.json<WarningList>(
      await api.get(`${base()}/warnings?status=open&code=${MISMATCH}&period=${PERIOD}&limit=5`),
    );
    expect(list.total).toBe(1);
    expect(list.items[0].severity).toBe("blocker");

    // Eşleme yeniden çalışınca ikinci uyarı açılmaz.
    await api.json(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
    const again = await api.json<WarningList>(
      await api.get(`${base()}/warnings?status=open&code=${MISMATCH}&period=${PERIOD}&limit=5`),
    );
    expect(again.total).toBe(1);
  });

  test("matris ve uyarı listesi aynı dönemde aynı sayıyı verir", async () => {
    let cell: Matrix["items"][number]["cells"][number] | undefined;
    for (let offset = 0; !cell; offset += 100) {
      const m = await api.json<Matrix>(await api.get(`/v1/tenant/periods/matrix?until=${PERIOD}&periods=1&limit=100&offset=${offset}`));
      cell = m.items.find((r) => r.engagement_uuid === engagement)?.cells.find((c) => c.period === PERIOD);
      if (!cell && offset + 100 >= m.total) break;
    }
    expect(cell, "matriste mükellefin Ekim hücresi").toBeTruthy();
    const open = await api.json<WarningList>(await api.get(`${base()}/warnings?status=open&period=${PERIOD}&limit=1`));
    const blockers = await api.json<WarningList>(
      await api.get(`${base()}/warnings?status=open&severity=blocker&period=${PERIOD}&limit=1`),
    );
    expect(cell!.open_warnings).toBe(open.total);
    expect(cell!.open_blockers).toBe(blockers.total);
    expect(cell!.open_blockers).toBeGreaterThanOrEqual(1);
  });

  test("Komuta Merkezi hücresinden gidilen Uyarılar sekmesinde uyarı görünür", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/command"));
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.title);
    const cell = page.getByRole("link", { name: new RegExp(`^${taxpayer.title.replace(/[.]/g, "\\.")} · Eki 26:`) });
    await expect(cell).toHaveCount(1);
    await expect(cell).toHaveAttribute("data-target", "blockers");
    await cell.click();
    await page.waitForURL(
      (u) => u.pathname.endsWith(`/engagements/${engagement}/warnings`) && u.searchParams.get("severity") === "blocker",
    );
    await expect(page.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
    await expect(page.getByText("Banka bakiyesi hareketlerle tutmuyor").first()).toBeVisible();
  });
});
