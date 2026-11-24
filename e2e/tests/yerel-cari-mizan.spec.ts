import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, expectNoRawText, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Cari mizanı (#780 C): Mizan sekmesinde "Cari mizanı" kaynağı. Her carinin
 * borcu, alacağı ve bakiyesi ana hesabının altında; ana hesap toplamı canlı
 * mizandaki satırla aynı, pay sütunu toplamın hangi carilerden oluştuğunu
 * gösterir. Ters bakiyeli cari ve ana hesaba yazılmış (cariye ayrılmamış)
 * tutar işaretlenir; arama, "yalnız ters bakiyeliler" ve taslaklar.
 *
 * Veri: her koşuda yeni bir mükellef. Ekim Luca mizanı (cari adları için)
 * API ile yüklenir; Ekim'de beş onaylı ve bir taslak fiş API ile yazılır.
 * Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Cari Mizan ${run} A.Ş.` };

const plan = {
  name: `cari-mizan-${run}.csv`,
  mimeType: "text/csv",
  buffer: Buffer.from(
    [
      "MİZAN 01.10.2026 - 31.10.2026",
      "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye",
      "120;ALICILAR;0,00;0,00;;",
      "120.01;YURTİÇİ ALICILAR;0,00;0,00;;",
      "120.01.001;ALFA GIDA LTD. ŞTİ.;0,00;0,00;;",
      "100;KASA;0,00;0,00;;",
      "100.01;MERKEZ KASA;0,00;0,00;;",
      "120.01.002;BETA MARKET A.Ş.;0,00;0,00;;",
      "320;SATICILAR;0,00;0,00;;",
      "320.01;YURTİÇİ SATICILAR;0,00;0,00;;",
      "320.01.001;GAMA AMBALAJ LTD. ŞTİ.;0,00;0,00;;",
      "600;YURTİÇİ SATIŞLAR;0,00;0,00;;",
      "600.01;YURTİÇİ SATIŞLAR;0,00;0,00;;",
      "770;GENEL YÖNETİM GİDERLERİ;0,00;0,00;;",
      "770.01;GENEL YÖNETİM GİDERLERİ;0,00;0,00;;",
    ].join("\n") + "\n",
    "utf8",
  ),
};

const entry = (date: string, description: string, debit: string, credit: string, amount: string) => ({
  entry_date: `${date}T09:00:00Z`,
  description,
  lines: [
    { account: debit, debit: amount, credit: "0" },
    { account: credit, debit: "0", credit: amount },
  ],
});

type CariReport = {
  from_period: string;
  groups: {
    main_code: string;
    debit: string;
    credit: string;
    balance: string;
    count: number;
    reverse_count: number;
    accounts: { code: string; name: string; balance: string; share: string | null; reverse: boolean; unassigned: boolean }[];
  }[];
  totals: {
    debit: string;
    credit: string;
    debit_balance: string;
    credit_balance: string;
    count: number;
    reverse_count: number;
    unassigned_count: number;
  };
};

test.describe("cari mizanı (#780 C)", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "carimizan");
    const up = await api.upload(`${base()}/account-plan/imports`, plan);
    expect(up.status(), await up.text()).toBe(201);

    // Alfa 1.000 borç; Beta 500 satış, 700 tahsilat (200 ters bakiye);
    // Gama 300 alacak; ana hesaba (120) 50. Taslak: Alfa'ya 100 daha.
    for (const [date, debit, credit, amount, approve] of [
      ["2026-10-02", "120.01.001", "600.01", "1000.00", true],
      ["2026-10-05", "120.01.002", "600.01", "500.00", true],
      ["2026-10-09", "100.01", "120.01.002", "700.00", true],
      ["2026-10-12", "770.01", "320.01.001", "300.00", true],
      ["2026-10-14", "120", "600.01", "50.00", true],
      ["2026-10-20", "120.01.001", "600.01", "100.00", false],
    ] as const) {
      const res = await api.post(`${base()}/journal-drafts`, entry(date, `Cari mizan ${run} ${date}`, debit, credit, amount));
      expect(res.status(), await res.text()).toBe(201);
      if (!approve) continue;
      const draft = await api.json<{ uuid: string }>(res);
      const ok = await api.post(`${base()}/journal-drafts/${draft.uuid}/approve`, { confirm_warnings: true });
      expect(ok.status(), await ok.text()).toBe(200);
    }
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("cari bazında bakiye, pay, ters bakiye, arama ve taslaklar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/trial-balance`));
    await page.getByLabel("Kaynak").selectOption("cari");
    await expect(page).toHaveURL(/kaynak=cari/);

    await page.goto(officePath(`/engagements/${engagement}/trial-balance?kaynak=cari&donem=2026-10`));
    await expect(page.getByText(/2026-01 – 2026-10 arası onaylı fişlerden/)).toBeVisible();
    const table = page.getByRole("table", { name: "Cari mizanı" });

    // Ana hesap satırı canlı mizandaki 120 ile aynı: borç 1.550, alacak 700, bakiye 850.
    const alicilar = table.getByTestId("cari-group-120");
    await expect(alicilar).toContainText("ALICILAR");
    // Cari sayısı ana hesaba yazılmış tutarı saymaz (Alfa, Beta).
    await expect(alicilar).toContainText("2 cari");
    await expect(alicilar).toContainText("1.550,00");
    await expect(alicilar).toContainText("850,00");

    const alfa = table.getByTestId("cari-row-120.01.001");
    await expect(alfa).toContainText("ALFA GIDA LTD. ŞTİ.");
    await expect(alfa).toContainText("%95,2");
    const beta = table.getByTestId("cari-row-120.01.002");
    await expect(beta).toContainText("BETA MARKET A.Ş.");
    await expect(beta).toContainText("Ters bakiye");
    await expect(beta).toContainText("200,00");
    const unassigned = table.getByTestId("cari-row-120");
    await expect(unassigned).toContainText("Cariye ayrılmamış tutar");
    await expect(unassigned).toContainText("Ana hesaba yazılmış");
    await expect(table.getByTestId("cari-row-320.01.001")).toContainText("%100,0");

    const summary = page.getByTestId("cari-balances-summary");
    await expect(summary).toContainText("Cari sayısı");
    await expect(summary).toContainText("1.050,00");
    await expect(summary).toContainText("500,00");
    await expectNoRawText(page.locator("main"));

    // Arama: unvanla yalnız Gama kalır.
    await page.getByLabel("Cari ara").fill("gama");
    await expect(table.getByTestId("cari-row-320.01.001")).toBeVisible();
    await expect(table.getByTestId("cari-row-120.01.001")).toHaveCount(0);
    await page.getByLabel("Cari ara").fill("");

    // Yalnız ters bakiyeliler: Beta.
    await page.getByRole("checkbox", { name: "Yalnız ters bakiyeliler" }).click();
    await expect(table.getByTestId("cari-row-120.01.002")).toBeVisible();
    await expect(table.getByTestId("cari-row-120.01.001")).toHaveCount(0);
    await page.getByRole("checkbox", { name: "Yalnız ters bakiyeliler" }).click();

    // Taslaklar da katılınca Alfa 1.100.
    await page.getByRole("checkbox", { name: "Taslakları da kat" }).click();
    await expect(alfa).toContainText("1.100,00");

    // Canlı mizana geçiş aynı dönemle.
    await page.getByRole("button", { name: "Canlı mizanda gör" }).click();
    await expect(page).toHaveURL(/kaynak=canli/);
    await expect(page).toHaveURL(/donem=2026-10/);
  });

  test("cari mizanı ucu: genel mizanla uyum, pay, ters bakiye, dönem denetimi", async () => {
    const rep = await api.json<CariReport>(await api.get(`${base()}/account-plan/cari-balances?period=2026-10`));
    expect(rep.from_period).toBe("2026-01");
    expect(rep.groups.map((g) => g.main_code)).toEqual(["120", "320"]);
    const [g120, g320] = rep.groups;
    expect(g120).toMatchObject({ debit: "1550.00", credit: "700.00", balance: "850.00", count: 2, reverse_count: 1 });
    expect(g120.accounts.map((a) => a.code)).toEqual(["120", "120.01.001", "120.01.002"]);
    expect(g120.accounts[0]).toMatchObject({ unassigned: true, balance: "50.00", share: "4.8", reverse: false });
    expect(g120.accounts[1]).toMatchObject({ name: "ALFA GIDA LTD. ŞTİ.", balance: "1000.00", share: "95.2", reverse: false });
    expect(g120.accounts[2]).toMatchObject({ name: "BETA MARKET A.Ş.", balance: "-200.00", share: null, reverse: true });
    expect(g320).toMatchObject({ balance: "-300.00", count: 1, reverse_count: 0 });
    expect(rep.totals).toEqual({
      debit: "1550.00",
      credit: "1000.00",
      debit_balance: "1050.00",
      credit_balance: "500.00",
      count: 3,
      reverse_count: 1,
      unassigned_count: 1,
    });

    // Genel mizanla uyum: her grubun bakiyesi canlı mizandaki ana hesap satırı.
    const live = await api.json<{ lines: { code: string; balance: string }[] }>(
      await api.get(`${base()}/account-plan/live-trial-balance?period=2026-10&level=1`),
    );
    for (const g of rep.groups) {
      expect(live.lines.find((l) => l.code === g.main_code)?.balance).toBe(g.balance);
    }

    const drafts = await api.json<CariReport>(
      await api.get(`${base()}/account-plan/cari-balances?period=2026-10&include_drafts=true`),
    );
    expect(drafts.groups[0].accounts[1].balance).toBe("1100.00");

    // Eylül: o dönemde (Ocak – Eylül) cari hareketi yok.
    const sep = await api.json<CariReport>(await api.get(`${base()}/account-plan/cari-balances?period=2026-09`));
    expect(sep.groups).toEqual([]);
    expect((await api.get(`${base()}/account-plan/cari-balances?period=2026-13`)).status()).toBe(400);
  });
});
