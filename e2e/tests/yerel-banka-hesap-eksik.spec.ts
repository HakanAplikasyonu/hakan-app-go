import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Eksik ekstre takibi (Oğuz'un geri bildirimi): müşavir mükellefin banka
 * hesabını listeye ekler; kapanmış geçen ay için ekstre gelmediğinden hesap
 * satırında o ay "eksik" görünür. "İste" ile hazır başlıklı belge isteği
 * ("Akbank TRY hesabı – <Ay Yıl> ekstresi") açılır, satır "İstendi" olur ve
 * istek mükellefin belge talepleri arasında görünür.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; IBAN'sız Akbank TRY hesabı.
 * Ofis ayarı (bank_statement_tracking) açılır, sonunda eski haline döner.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Eksik Ekstre ${run} A.Ş.` };

/** Bugünden bir önceki ay (YYYY-MM), İstanbul saatiyle. */
function lastMonth(): string {
  const [y, m] = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul", year: "numeric", month: "2-digit" })
    .format(new Date())
    .split("-")
    .map(Number);
  const d = new Date(Date.UTC(y, m - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

type Task = { uuid: string; title: string; kind: string };
type Status = { items: { cash_box_uuid: string; name: string; months: { period: string; status: string; title?: string }[] }[] };

test("banka hesabı eklenir, geçen ayın ekstresi eksik görünür, İste ile belge isteği açılır", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "eksikekstre");
  const orig = await api.json<{ settings: Record<string, unknown> }>(await api.get("/v1/tenant/settings"));
  const settings = orig.settings ?? {};
  const period = lastMonth();
  const name = `Akbank TL ${run}`;
  try {
    const res = await api.patch("/v1/tenant/settings", { settings: { ...settings, bank_statement_tracking: true } });
    expect(res.status(), await res.text()).toBe(200);

    // 1) Hesap eklenir: IBAN isteğe bağlı, banka ve para birimi yeter.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}`));
    const section = page.getByTestId("bank-accounts");
    await section.getByRole("button", { name: "Banka hesabı ekle" }).click();
    const form = section.getByRole("form", { name: "Banka hesabı ekle" });
    await form.getByRole("combobox", { name: "Banka" }).click();
    await page.getByRole("option", { name: "Akbank", exact: true }).click();
    await form.getByLabel("Hesap adı").fill(name);
    await form.getByRole("button", { name: "Kaydet" }).click();
    await expect(page.getByText("Banka hesabı eklendi.")).toBeVisible();

    // 2) Geçen ay eksik: ay şeridinde "eksik", altında İste satırı.
    const row = section.getByTestId("bank-account-row").filter({ hasText: name });
    await expect(row).toBeVisible();
    const month = row.locator(`[data-testid="bank-statement-month"][data-period="${period}"]`);
    await expect(month).toHaveAttribute("data-status", "missing");
    const missing = row.locator(`[data-testid="bank-statement-missing"][data-period="${period}"]`);
    await expect(missing).toBeVisible();

    // Başlık sunucudan: "Akbank TRY hesabı – <Ay Yıl> ekstresi".
    const status = await api.json<Status>(await api.get(`/v1/tenant/engagements/${engagement}/bank-statement-status`));
    const account = status.items.find((a) => a.name === name);
    expect(account, JSON.stringify(status)).toBeTruthy();
    const title = account!.months.find((m) => m.period === period)?.title ?? "";
    expect(title).toMatch(/^Akbank TRY hesabı – .+ ekstresi$/);

    // 3) İste: hazır başlıklı istek penceresi, gönderilince satır "İstendi".
    await missing.getByRole("button", { name: "İste", exact: true }).click();
    const dialog = page.getByTestId("bank-statement-request");
    await expect(dialog).toContainText(title);
    await dialog.getByRole("button", { name: "İsteği gönder" }).click();
    await expect(dialog).toBeHidden();
    await expect(missing).toContainText("İstendi");

    // 4) İstek mükellefin belge talepleri arasında.
    const tasks = await api.json<{ items: Task[] }>(
      await api.get(`/v1/tenant/engagements/${engagement}/document-tasks?status=all&limit=100`),
    );
    expect(tasks.items.filter((t) => t.title === title)).toHaveLength(1);
  } finally {
    const back = await api.patch("/v1/tenant/settings", { settings });
    expect(back.status(), await back.text()).toBe(200);
    await api.dispose();
  }
});
