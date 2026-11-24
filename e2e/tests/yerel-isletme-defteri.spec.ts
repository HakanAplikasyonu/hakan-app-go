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
 * İşletme defteri (#780): mükellefin defter türü (bilanço / işletme defteri /
 * serbest meslek) genel bakıştan değişir ve başlıkta görünür. İşletme
 * defterinde hesap planı yok: satış faturası "Satış hasılatı" olarak işletme
 * hesabına gelir yazılır, KDV ayrı durur; yevmiye taslağı açılmaz (409),
 * Mizan ve Fişler sekmeleri yerine İşletme hesabı sekmesi gelir. KDV1
 * hesabı defter türünden etkilenmez. Kategori değiştirilebilir; yıllık
 * görünümde mal mevcudu kâra katılır.
 *
 * Veri: her koşuda yeni bir mükellef; demo UBL'den türetilen bir satış
 * faturası API ile yüklenir. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `İşletme Defteri ${run} Ltd.` };

type Row = {
  source: string;
  record_uuid: string;
  side: "income" | "expense";
  category: string;
  overridden: boolean;
  net: string;
  vat: string;
};
type Book = {
  ledger_type: string;
  summary: {
    income: string;
    expense: string;
    income_vat: string;
    profit: string;
    inventory_opening: string | null;
  };
  rows: Row[];
  inventory: { year: number; opening: string; closing: string | null } | null;
};

test.describe("işletme defteri (#780)", () => {
  let engagement = "";
  let api: Api;
  let invoice = { uuid: "", period: "", net: "" };
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "isletme");
    const up = await api.upload(
      `${base()}/ubl-uploads`,
      deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title),
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    const list = await api.json<{
      items: {
        uuid: string;
        period: string;
        direction: string;
        tax_exclusive_amount: string;
      }[];
    }>(await api.get(`${base()}/invoices?limit=10`));
    const inv = list.items[0]!;
    expect(inv.direction).toBe("outgoing");
    invoice = {
      uuid: inv.uuid,
      period: inv.period,
      net: inv.tax_exclusive_amount,
    };
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("API: defter türü, işletme hesabı, yevmiye yok, KDV1 aynı", async () => {
    // Varsayılan bilanço: işletme hesabı tutulmaz.
    const ctx = await api.json<{ engagement: { ledger_type: string } }>(
      await api.get(`${base()}/context`),
    );
    expect(ctx.engagement.ledger_type).toBe("bilanco");
    const before = await api.json<{
      ledger_type: string;
      rows: unknown[];
    }>(await api.get(`${base()}/operating-account?period=${invoice.period}`));
    expect(before.ledger_type).toBe("bilanco");
    expect(before.rows).toHaveLength(0);
    const beforeCat = await api.put(`${base()}/operating-account/inventory`, {
      year: Number(invoice.period.slice(0, 4)),
      opening: "1.00",
      closing: null,
    });
    expect(beforeCat.status()).toBe(409);
    const kdvBefore = await api.json<{ kdv1: unknown }>(
      await api.get(`${base()}/tax-returns/kdv1/${invoice.period}`),
    );

    // Geçersiz tür reddedilir; işletme defterine geçilir.
    const bad = await api.put(`${base()}/ledger-type`, { ledger_type: "x" });
    expect(bad.status()).toBe(400);
    const set = await api.put(`${base()}/ledger-type`, {
      ledger_type: "isletme",
    });
    expect(set.status(), await set.text()).toBe(200);

    // Satış faturası gelir: Satış hasılatı, KDV ayrı.
    const book = await api.json<Book>(
      await api.get(`${base()}/operating-account?period=${invoice.period}`),
    );
    expect(book.ledger_type).toBe("isletme");
    const row = book.rows.find((r) => r.record_uuid === invoice.uuid);
    expect(row).toMatchObject({
      source: "invoice",
      side: "income",
      category: "sales_revenue",
      overridden: false,
    });
    expect(Number(row!.net)).toBeCloseTo(Number(invoice.net), 2);
    expect(Number(book.summary.income)).toBeCloseTo(Number(invoice.net), 2);
    expect(Number(book.summary.expense)).toBe(0);
    expect(Number(book.summary.profit)).toBeCloseTo(Number(invoice.net), 2);

    // Hesap planlı kayıt üretilmez.
    const draft = await api.post(`${base()}/journal-drafts`, {
      entry_date: `${invoice.period}-15T09:00:00Z`,
      description: `İşletme ${run}`,
      lines: [
        { account: "100.01", debit: "10", credit: "0" },
        { account: "600.01", debit: "0", credit: "10" },
      ],
    });
    expect(draft.status()).toBe(409);
    expect(await draft.text()).toContain("LEDGER_NOT_BALANCE_SHEET");

    // KDV1 defter türünden etkilenmez.
    const kdvAfter = await api.json<{ kdv1: unknown }>(
      await api.get(`${base()}/tax-returns/kdv1/${invoice.period}`),
    );
    expect(kdvAfter.kdv1).toEqual(kdvBefore.kdv1);

    // Ofis kategoriyi değiştirir ve geri alır.
    const cat = await api.put(`${base()}/operating-account/categories`, {
      source: "invoice",
      record_uuid: invoice.uuid,
      category: "other_income",
    });
    expect(cat.status(), await cat.text()).toBe(204);
    const changed = await api.json<Book>(
      await api.get(`${base()}/operating-account?period=${invoice.period}`),
    );
    expect(
      changed.rows.find((r) => r.record_uuid === invoice.uuid),
    ).toMatchObject({ category: "other_income", overridden: true });
    const reset = await api.put(`${base()}/operating-account/categories`, {
      source: "invoice",
      record_uuid: invoice.uuid,
      category: "",
    });
    expect(reset.status()).toBe(204);
    const foreign = await api.put(`${base()}/operating-account/categories`, {
      source: "invoice",
      record_uuid: "00000000-0000-4000-8000-000000000780",
      category: "general_expense",
    });
    expect(foreign.status()).toBe(404);

    // Yıllık görünüm: mal mevcudu kâra katılır (kâr = gelir - başı + sonu).
    const year = Number(invoice.period.slice(0, 4));
    const inv = await api.put(`${base()}/operating-account/inventory`, {
      year,
      opening: "1000.00",
      closing: "400.00",
    });
    expect(inv.status(), await inv.text()).toBe(200);
    const yearly = await api.json<Book>(
      await api.get(`${base()}/operating-account?year=${year}`),
    );
    expect(yearly.inventory).toMatchObject({ year });
    expect(Number(yearly.summary.profit)).toBeCloseTo(
      Number(yearly.summary.income) - Number(yearly.summary.expense) - 600,
      2,
    );
  });

  test("ekran: başlıkta defter türü, İşletme hesabı sekmesi, Mizan yok", async ({
    page,
  }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}`));
    await expect(page.getByTestId("ledger-badge")).toHaveText(
      "İşletme defteri",
    );
    // Kartın kendisi de "Defter türü" başlığıyla etiketli; seçim kutusu alınır.
    await expect(
      page.getByTestId("ledger-type-card").getByRole("combobox", { name: "Defter türü" }),
    ).toHaveValue("isletme");
    const tabs = page.getByTestId("engagement-tabs");
    await expect(
      tabs.getByRole("link", { name: "İşletme hesabı" }),
    ).toBeVisible();
    await expect(tabs.getByRole("link", { name: "Mizan" })).toHaveCount(0);
    await expect(tabs.getByRole("link", { name: "Fişler" })).toHaveCount(0);

    await expect(
      tabs.getByRole("link", { name: "İşletme hesabı" }),
    ).toHaveAttribute("href", /operating-account/);
    await page.goto(
      officePath(
        `/engagements/${engagement}/operating-account?period=${invoice.period}`,
      ),
    );
    await page.getByRole("button", { name: "Yıl", exact: true }).click();
    await expect(page).toHaveURL(/gorunum=yil/);
    await expect(page.getByText("İşletme hesabı özeti")).toBeVisible();
    await expect(page.getByTestId("operating-income")).toBeVisible();
    await expect(page.getByText("Dönem başı mal mevcudu (TL)")).toBeVisible();
    // Yazma yetkisi olan kullanıcı türü bir seçim kutusunda görür; önerilen
    // tür seçili seçenektir ("Satış hasılatı (önerilen)", #788).
    await expect(
      page
        .getByTestId("operating-rows")
        .locator("option:checked", { hasText: "Satış hasılatı" })
        .first(),
    ).toBeAttached();

    // Fatura ayrıntısında hesap kodu yerine gelir / gider türü.
    await page.goto(
      officePath(`/engagements/${engagement}/invoices/${invoice.uuid}`),
    );
    await expect(page.getByTestId("operating-category")).toContainText(
      "Gelir · Satış hasılatı",
    );
  });
});
