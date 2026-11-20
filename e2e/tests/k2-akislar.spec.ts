import { readFileSync } from "node:fs";

import { expect, test, type Page } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  pickTaxpayer,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * EPIC #560 akışları: problemden çözüm ekranına, işlem ekranda bitirilir ve
 * sonucu veritabanında doğrulanır.
 *
 * - #567 Mükellefler satırı → detay; Komuta Merkezi hücresi → hedef ekran.
 * - #572 Mizan hafızası kimin olduğunu yazar; mükellef değişince değişir.
 * - #569 İnceleme kuyruğunda banka hesabı önerisinde "Kodu düzelt".
 * - #570 Dönem uyarısı → İncele → hareketi eşleştir → uyarı kapanır.
 * - #565 Eşleşmeyi geri al → hesaba işle → yeniden eşleştir.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Atlas'ın ATL…102 faturası
 * (5.400,00) bu mükellefe türetilir ve Ziraat ekstresi (fixtures/atlas-ziraat-
 * 2026-10.sta: 5.400,00 tahsilat, 12,50 ücret) yüklenir. Eşleme çalışınca
 * tahsilat öneride kalır ve "Faturası olmayan tahsilat" uyarısı açılır.
 * Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Akış Ticaret ${run} A.Ş.` };
const PERIOD = "2026-10";
const UNINVOICED = "BNK_UNINVOICED_COLLECTION";

type Matching = {
  match_status: string;
  matches: { invoice_number: string }[];
  posting: { status: string; account_code: string } | null;
};

test.describe("EPIC #560 akışları (koordinatör 2)", () => {
  let engagement = "";
  let collection = "";
  let fee = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const matching = async (tx: string) =>
    api.json<Matching>(await api.get(`${base()}/bank-transactions/${tx}/matching`));
  const openUninvoiced = async () =>
    (
      await api.json<{ total: number }>(
        await api.get(`${base()}/warnings?status=open&code=${UNINVOICED}&period=${PERIOD}&limit=5`),
      )
    ).total;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "akis");

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
    // Hareketlerin karşı hesap önerileri: ücret 780 önerisiyle kuyruğa düşer.
    await api.json(await api.post(`${base()}/bank-postings/suggest`, { period: PERIOD }));

    const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base()}/bank-transactions?period=${PERIOD}&limit=10`),
    );
    collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;
    fee = txs.items.find((x) => x.amount === "12.50")!.uuid;
    expect(await openUninvoiced()).toBe(1);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  const tabs = (page: Page) => page.getByRole("navigation", { name: "Mükellef bölümleri" });

  test("#567 Mükellefler satırı detaya, Komuta Merkezi hücresi hedef ekrana götürür", async ({ page }) => {
    await uiLogin(page, DEMO.owner);

    // Mükellefler: bağlantı olmayan hücreye (VKN) tıklamak da detayı açar.
    await page.goto(officePath("/taxpayers"));
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.title);
    const row = page.getByRole("row").filter({ hasText: taxpayer.title });
    await expect(row).toHaveCount(1);
    await row.getByText(taxpayer.vkn).click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}`));
    await expect(page.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();

    // Komuta Merkezi: Ekim hücresi, hücrenin söylediği hedefe gider.
    await page.goto(officePath("/command"));
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.title);
    const cell = page.getByRole("link", { name: new RegExp(`^${taxpayer.title.replace(/[.]/g, "\\.")} · Eki 26:`) });
    await expect(cell).toHaveCount(1);
    const target = await cell.getAttribute("data-target");
    expect(["blockers", "warnings", "overview"]).toContain(target);
    await cell.click();
    if (target === "blockers") {
      await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/warnings`) && u.searchParams.get("severity") === "blocker");
    } else if (target === "warnings") {
      await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/warnings`));
    } else {
      await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}`));
    }
    // Açık uyarısı olan dönemde hücre özet yerine uyarılara gider.
    expect(target).not.toBe("overview");
    await expect(page.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
    await expect(tabs(page).getByRole("link", { name: "Uyarılar", exact: true })).toHaveAttribute("aria-current", /page|true/);
  });

  test("#572 Mizan hafızası kimin olduğunu yazar ve mükellef değişince değişir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/account-plan?taxpayer=${engagement}`));
    const scope = page.getByTestId("memory-scope");
    await expect(scope).toContainText(taxpayer.title);
    await expect(scope).toContainText("yalnızca bu mükellefe aittir");
    // Planı yüklenmemiş mükellefte "eksik hesap yok" denmez.
    await expect(page.getByTestId("missing-accounts-empty")).toContainText("henüz yüklenmedi");

    await pickTaxpayer(page, DEMO.atlas.title);
    await expect(scope).toContainText(DEMO.atlas.title);
    await expect(scope).not.toContainText(taxpayer.title);
  });

  test("#569 İnceleme kuyruğunda banka hesabı önerisinin kodu düzeltilir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/review`));
    await expect(page.getByRole("heading", { level: 2, name: "İnceleme kuyruğu" })).toBeVisible();

    // Ücret hareketinin karşı hesabı 780 önerilmiş; müşavir alt hesabı seçer.
    const item = page.getByRole("listitem").filter({ hasText: "EFT UCRETI" });
    await expect(item).toHaveCount(1);
    await item.click();
    const card = page.getByRole("article").filter({ hasText: "EFT UCRETI" });
    const fix = card.getByRole("button", { name: "Kodu düzelt" });
    await expect(fix).toBeEnabled();
    await fix.click();
    const input = page.getByPlaceholder("Hesap kodu ya da adı…");
    await input.fill("780.01");
    await expect(page.getByRole("option", { name: /^780\.01/ })).toHaveAttribute("aria-selected", "true");
    await input.press("Enter");
    await expect(page.getByRole("listitem").filter({ hasText: "EFT UCRETI" })).toHaveCount(0);

    // Veritabanı: hareket 780.01'e işlendi.
    await expect.poll(async () => (await matching(fee)).posting).toMatchObject({ status: "approved", account_code: "780.01" });
  });

  test("#570 Dönem uyarısı İncele panelinden çözülür: hareket eşleşir, uyarı kapanır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
    await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();

    const sheet = page.getByTestId("warning-sheet");
    await expect(sheet).toBeVisible();
    // Uyarının işaret ettiği tek hareket açık gelir; adayı faturası.
    const txs = sheet.getByTestId("warning-transactions");
    await expect(txs).toContainText("Bu uyarıdaki hareketler (1)");
    const panel = txs.getByTestId("bank-transaction-matching");
    await expect(panel).toContainText("EFT GELEN ATL2026000000102");
    await panel.getByRole("checkbox", { name: "ATL2026000000102 faturasını seç" }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();

    await expect(sheet.getByText("Bu uyarı kapandı.")).toBeVisible();
    // Veritabanı: hareket faturaya bağlı, uyarı açık değil.
    const v = await matching(collection);
    expect(v.match_status).toBe("matched");
    expect(v.matches.map((m) => m.invoice_number)).toEqual(["ATL2026000000102"]);
    expect(await openUninvoiced()).toBe(0);
  });

  test("#565 Eşleşmeyi geri al → hesaba işle → yeniden eşleştir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    await expect(panel).toContainText("Eşleştiği faturalar");

    // Geri al: hareket eşleşmeyenlere döner, uyarı yeniden açılır.
    await panel.getByRole("button", { name: "Eşleşmeyi geri al" }).click();
    await expect(panel.getByText("Eşleşmedi", { exact: true })).toBeVisible();
    await expect(panel.getByText("Eşleştiği faturalar")).toHaveCount(0);
    expect((await matching(collection)).match_status).toBe("unmatched");
    expect(await openUninvoiced()).toBe(1);

    // Fatura gelmeyecekse: hareket avans hesabına işlenir, uyarı kapanır.
    const account = panel.getByTestId("bank-transaction-account");
    await account.getByPlaceholder("Hesap ara: kod ya da ad (ör. 320 ya da cari adı)").fill("340.01");
    await account.getByRole("button", { name: "340.01 hesabına işle" }).click();
    await expect(account).toContainText("340.01");
    await expect(account).toContainText("Hesaba işlendi");
    const booked = await matching(collection);
    expect(booked.posting).toMatchObject({ status: "approved", account_code: "340.01" });
    expect(booked.match_status).toBe("unmatched");
    expect(await openUninvoiced()).toBe(0);

    // Fatura sonradan bulunur: aynı panelden yeniden eşleştirilir.
    await panel.getByRole("checkbox", { name: "ATL2026000000102 faturasını seç" }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    await expect(panel).toContainText("Eşleştiği faturalar");
    expect((await matching(collection)).match_status).toBe("matched");
  });
});
