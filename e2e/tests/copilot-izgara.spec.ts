import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, deriveUbl, engagementOf, officePath, uiLogin, waitForUploads } from "./lib";

/**
 * Co-pilot "Sıradaki iş" ve ⌘K komut çubuğu (#392), düzenlenebilir ızgara
 * (#394) (#452). copilot.spec.ts Ctrl+K ile vazgeçilen planı, smoke.spec.ts
 * ızgarada Excel'den yapıştırıp kaydetmeyi sınar; bu dosya onaylanıp
 * çalıştırılan planı ve ızgarada düzenleme, geri alma ve klavye gezintisini.
 *
 * Veri: demo ofisi, Selin Yılmaz.
 * - Co-pilot: Selin'in aylık raporu kuyruğa alınır (Atlas'ın fişleri
 *   aktarılmaz: duman testi aktarılan fiş sayısını sayar).
 * - Izgara: Selin'e her koşuda yeni ETTN'li bir alış faturası yüklenir ve
 *   hesap kodu önerisi üretilir; ızgara Selin'e süzülür.
 */
test.describe.configure({ mode: "serial" });

let selin = "";

test.beforeAll(async () => {
  const api = await Api.login(DEMO.owner);
  selin = await engagementOf(api, DEMO.selin.title);
  await api.dispose();
});

const palette = (page: Page) => page.getByRole("dialog", { name: "Komut paleti" });

test.describe("co-pilot", () => {
  test("⌘K ile ekran aç; komuttan plan, onayla ve çalıştır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/copilot"));
    await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
    const selinRow = page.getByRole("row").filter({ hasText: DEMO.selin.title });
    await expect(selinRow.getByRole("list", { name: "Dönem kapanış adımları" })).toBeVisible();

    // ⌘K (macOS kısayolu) açar, Esc kapatır.
    await page.keyboard.press("Meta+k");
    await expect(palette(page)).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(palette(page)).toBeHidden();

    // Komut paletinden ekrana git: İnceleme kuyruğu.
    await page.keyboard.press("Meta+k");
    await page.keyboard.type("İnceleme");
    await page.getByRole("option", { name: /^İnceleme kuyruğu/ }).click();
    await expect(page).toHaveURL(/\/review/);
    await expect(page.getByRole("heading", { level: 1, name: "İnceleme kuyruğu" })).toBeVisible();

    // Komut → onay bekleyen plan → onayla ve çalıştır (Sıradaki iş ekranından).
    await page.goto(officePath("/copilot"));
    await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
    const command = `Selin Yılmaz raporunu gönder e2e-${Date.now()}`;
    await page.keyboard.press("Meta+k");
    await page.keyboard.type(command);
    await page.getByRole("option", { name: /Co-pilot'a sor/ }).click();
    const dialog = page.getByRole("dialog", { name: "Onay bekleyen işlemler" });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(DEMO.selin.title);
    await dialog.getByRole("button", { name: /Onayla ve çalıştır/ }).click();
    await expect(page.getByText("Rapor e-posta kuyruğuna alındı.").first()).toBeVisible();

    const api = await Api.login(DEMO.owner);
    await expect
      .poll(async () => {
        const plans = await api.json<{ items: { command: string; status: string; steps: { action: string; status: string }[] }[] }>(
          await api.get("/v1/tenant/copilot/plans"),
        );
        const plan = plans.items.find((p) => p.command === command);
        return plan ? `${plan.status}:${plan.steps.map((s) => `${s.action}=${s.status}`).join(",")}` : "";
      })
      .toMatch(/^(done|completed):report_send=done$/);
    await api.dispose();
  });
});

test.describe("düzenlenebilir ızgara", () => {
  test("hücre düzenle, geri al / yinele, klavye gezintisi, kaydet", async ({ page }) => {
    // Hazırlık: Selin'e yeni fatura ve hesap kodu önerisi.
    const api = await Api.login(DEMO.owner);
    const before = (
      await api.json<{ total: number }>(await api.get(`/v1/tenant/review/suggestions?status=pending&engagement=${selin}&kind=account_code`))
    ).total;
    const ubl = deriveUbl("selin-01-temel-alis.xml", DEMO.atlas.taxId, DEMO.atlas.title);
    await api.json(await api.upload(`/v1/tenant/engagements/${selin}/ubl-uploads`, ubl));
    await waitForUploads(api, selin, 1);
    const invoices = await api.json<{ items: { uuid: string; created_at?: string }[] }>(
      await api.get(`/v1/tenant/engagements/${selin}/invoices?limit=100`),
    );
    for (const inv of invoices.items) {
      await api.post(`/v1/tenant/engagements/${selin}/invoices/${inv.uuid}/account-suggestions`, {});
    }
    const pending = (
      await api.json<{ total: number }>(await api.get(`/v1/tenant/review/suggestions?status=pending&engagement=${selin}&kind=account_code`))
    ).total;
    expect(pending, "Selin'e hesap kodu önerisi").toBeGreaterThan(before);

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/review?view=grid"));
    await page.getByRole("combobox", { name: "Mükellef" }).first().selectOption({ label: DEMO.selin.title });
    const grid = page.getByRole("grid", { name: "Hesap kodu önerileri" });
    const cell = (row: number, col: number) =>
      grid.locator(`[role=row][aria-rowindex="${row}"] [role=gridcell][aria-colindex="${col}"]`);
    // Kolonlar: 1 satır başlığı, 9 "Hesap kodu", 10 "Karar" (Belge ve
    // Tutar sütunları, #569).
    const code = cell(2, 9);
    await expect(code).toBeVisible();
    const original = (await code.innerText()).trim();

    // Klavye gezintisi: oklar odağı taşır.
    await code.click();
    await expect(code).toBeFocused();
    await page.keyboard.press("ArrowRight");
    await expect(cell(2, 10)).toBeFocused();
    await page.keyboard.press("ArrowLeft");
    await expect(code).toBeFocused();

    // Düzenle: yazmaya başlamak düzenleyiciyi açar, Enter işler.
    await page.keyboard.type("770.01");
    await page.keyboard.press("Enter");
    await expect(code).toContainText("770.01");
    const saveAll = page.getByRole("button", { name: /^Tümünü kaydet/ });
    await expect(saveAll).toHaveText(/\(1\)/);

    // Geri al / yinele (düğme ve kısayol).
    await page.getByRole("button", { name: "Geri al" }).click();
    await expect(code).toHaveText(original);
    await expect(saveAll).toHaveText(/\(0\)/);
    await code.click();
    await page.keyboard.press("Control+y");
    await expect(code).toContainText("770.01");
    await page.keyboard.press("Control+z");
    await expect(code).toHaveText(original);
    await page.keyboard.press("Control+y");
    await expect(code).toContainText("770.01");

    // Esc düzenlemeden vazgeçer: yazılan değer işlenmez.
    await code.click();
    await page.keyboard.type("999.99");
    await page.keyboard.press("Escape");
    await expect(code).toContainText("770.01");
    await expect(saveAll).toHaveText(/\(1\)/);

    // Kaydet: düzeltilen kodla karar verilir, satır kuyruktan çıkar.
    await code.click();
    await page.keyboard.press("Control+s");
    await expect
      .poll(async () =>
        (await api.json<{ total: number }>(await api.get(`/v1/tenant/review/suggestions?status=pending&engagement=${selin}&kind=account_code`))).total,
      )
      .toBe(pending - 1);
    await api.dispose();
  });
});
