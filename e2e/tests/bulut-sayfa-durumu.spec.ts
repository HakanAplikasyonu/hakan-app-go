import { expect, test, type Browser, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, pickTaxpayer, uiLogin, uniqueVkn } from "./lib";

/**
 * Sayfa durumu adreste (#669 D, #564 madde 15): İnceleme kuyruğu ve Fişler
 * ekranındaki seçimler (mükellef, öneri türü, güven kademesi, dönem) sorgu
 * dizesinde tutulur. Başka sayfaya gidip geri tuşuyla dönünce aynı süzgeçler
 * ve aynı adres açılır; aynı bağlantı yeni oturumda da aynı yeri açar.
 * Varsayılan değerler ("Tümü") adrese yazılmaz.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve API ile bir onaylı fiş
 * (Ekim 2026). Başka dosyanın kaydına bağlı değil; çıktı alınmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Sayfa Durumu ${run} Ltd. Şti.` };
const description = `E2E sayfa durumu ${run}`;
const PERIOD = "2026-10";
const PERIOD_YEAR = 2026;
const PERIOD_MONTH = "Eki"; // MonthPicker'ın kısa ay adı (date-fns tr, "LLL")

/** Başka bir sayfaya git (geçmişe yeni kayıt), sonra geri dön. */
async function awayAndBack(page: Page, expected: string) {
  await page.goto(officePath("/warnings"));
  await expect(page).not.toHaveURL(expected);
  await page.goBack();
  await expect(page).toHaveURL(expected);
}

/** Aynı adresi yeni bir oturumda (boş tarayıcı belleği) açar. */
async function freshPage(browser: Browser, url: string) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await uiLogin(page, DEMO.owner);
  await page.goto(url);
  return page;
}

test.describe("Sayfa durumu adreste (#669 D)", () => {
  let engagement = "";
  let api: Api;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "durum");
    const base = `/v1/tenant/engagements/${engagement}`;
    const draft = await api.json<{ uuid: string }>(
      await api.post(`${base}/journal-drafts`, {
        entry_date: `${PERIOD}-20T00:00:00Z`,
        description,
        lines: [
          { account: "770.01", debit: "88.20" },
          { account: "100.01", credit: "88.20" },
        ],
      }),
    );
    await api.json(await api.post(`${base}/journal-drafts/${draft.uuid}/approve`));
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("inceleme kuyruğu: mükellef, tür ve güven süzgeci geri tuşunda korunur", async ({ page, browser }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/review"));
    const taxpayerSelect = page.locator(`select:has(option[value="${engagement}"])`).first();
    const kindSelect = page.getByLabel("Öneri türü");
    const lowTier = page.getByRole("button", { name: /^Düşük güven/ });
    await expect(taxpayerSelect).toBeVisible();

    // Varsayılanlar adrese yazılmaz.
    const initial = new URL(page.url()).searchParams;
    for (const key of ["taxpayer", "kind", "tier"]) expect(initial.has(key), key).toBe(false);

    await taxpayerSelect.selectOption(engagement);
    await page.waitForURL((u) => u.searchParams.get("taxpayer") === engagement);
    await kindSelect.selectOption("account_code");
    await page.waitForURL((u) => u.searchParams.get("kind") === "account_code");
    await lowTier.click();
    await page.waitForURL((u) => u.searchParams.get("tier") === "low");
    const url = page.url();

    await awayAndBack(page, url);
    await expect(taxpayerSelect).toHaveValue(engagement);
    await expect(kindSelect).toHaveValue("account_code");
    await expect(lowTier).toHaveAttribute("aria-pressed", "true");

    // "Tümü"ne dönünce anahtar adresten düşer.
    await kindSelect.selectOption("all");
    await page.waitForURL((u) => !u.searchParams.has("kind"));
    expect(new URL(page.url()).searchParams.get("taxpayer")).toBe(engagement);

    // Bağlantı yeni oturumda aynı süzgeçleri açar.
    const other = await freshPage(browser, url);
    try {
      await expect(other.locator(`select:has(option[value="${engagement}"])`).first()).toHaveValue(engagement);
      await expect(other.getByLabel("Öneri türü")).toHaveValue("account_code");
      await expect(other.getByRole("button", { name: /^Düşük güven/ })).toHaveAttribute("aria-pressed", "true");
    } finally {
      await other.context().close();
    }
  });

  test("fişler: mükellef ve dönem geri tuşunda korunur", async ({ page, browser }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/journals"));
    await pickTaxpayer(page, taxpayer.title);

    const picker = page.locator("#journals-period");
    await expect(picker).toBeVisible();
    if (!/Ekim 2026/i.test(await picker.innerText())) {
      await picker.click();
      const prev = page.getByRole("button", { name: "Önceki yıl" });
      const next = page.getByRole("button", { name: "Sonraki yıl" });
      const year = prev.locator("xpath=following-sibling::span[1]");
      for (let i = 0; i < 20; i++) {
        const shown = Number((await year.innerText()).trim());
        if (shown === PERIOD_YEAR) break;
        await (shown > PERIOD_YEAR ? prev : next).click();
      }
      await expect(year).toHaveText(String(PERIOD_YEAR));
      await page.getByRole("button", { name: PERIOD_MONTH, exact: true }).click();
    }
    await expect(picker).toContainText(/Ekim 2026/i);
    await page.waitForURL(
      (u) => u.searchParams.get("taxpayer") === engagement && u.searchParams.get("period") === PERIOD,
    );
    const entry = page.locator("tr", { hasText: description }).first();
    await expect(entry).toBeVisible();
    const url = page.url();

    const taxpayerPicker = page.getByRole("combobox", { name: /^Mükellef:/ }).first();
    await awayAndBack(page, url);
    await expect(taxpayerPicker).toHaveAccessibleName(new RegExp(`^Mükellef: ${taxpayer.title.replace(/\./g, "\\.")}`));
    await expect(picker).toContainText(/Ekim 2026/i);
    await expect(entry).toBeVisible();

    // Bağlantı yeni oturumda (dönem belleği boş) aynı mükellef ve dönemi açar.
    const other = await freshPage(browser, url);
    try {
      await expect(other.locator("#journals-period")).toContainText(/Ekim 2026/i);
      await expect(other.locator("tr", { hasText: description }).first()).toBeVisible();
      expect(new URL(other.url()).searchParams.get("taxpayer")).toBe(engagement);
    } finally {
      await other.context().close();
    }
  });
});
