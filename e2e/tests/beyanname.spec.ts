import { readFileSync } from "node:fs";

import { expect, test, type Download, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Beyannameler ekranı (#390, #452): KDV1, MUHSGK ve Ba/Bs önizlemesi, kontrol
 * listesi, düzeltme, hazırlama, Excel çıktısı ve verildi kaydı.
 *
 * Veri: demo ofisi, Atlas Gıda; içinde bulunulan beyan dönemi (demo verisi
 * bugüne göre üretilir). Dosya beyan dönemindeki KDV1 ve MUHSGK kayıtlarını
 * değiştirir; başka akış dosyası bu kayıtları okumaz.
 */
test.describe.configure({ mode: "serial" });

let engagement = "";

test.beforeAll(async () => {
  const api = await Api.login(DEMO.owner);
  engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();
});

async function openTaxReturns(page: Page, kind?: "KDV1" | "MUHSGK" | "Ba / Bs") {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns`));
  const kinds = page.getByRole("group", { name: "Beyan türü" });
  await expect(kinds).toBeVisible();
  if (kind) {
    await kinds.getByRole("button", { name: kind, exact: true }).click();
    await expect(kinds.getByRole("button", { name: kind, exact: true })).toHaveAttribute("aria-pressed", "true");
  }
  return page.getByRole("region", { name: "Beyan durumu" });
}

/** İndirilen dosya bir .xlsx (ZIP) olmalı. */
async function expectXlsx(download: Download) {
  expect(download.suggestedFilename()).toMatch(/\.xlsx$/);
  const file = await download.path();
  expect(readFileSync(file).subarray(0, 2).toString("latin1")).toBe("PK");
}

/** "₺ 16.369,18" → 16369.18 */
const money = (text: string) => Number(text.replace(/[^\d,-]/g, "").replace(",", "."));

test.describe("beyanname", () => {
  test("KDV1: kontrol listesi, düzeltme, hazırla, Excel ve verildi", async ({ page }) => {
    const status = await openTaxReturns(page, "KDV1");
    await expect(status).toContainText("1 No.lu KDV beyannamesi");
    // Yerelde yeniden koşu: önceki koşu hazırladıysa taslağa dön.
    const reopen = status.getByRole("button", { name: "Taslağa geri al" });
    if (await reopen.isVisible()) await reopen.click();
    await expect(status).toContainText("Taslak");
    await expect(page.getByRole("heading", { name: "Matrah — teslim ve hizmetler" })).toBeVisible();

    // Düzeltme: "Diğer indirimler" ödenecek KDV'yi o kadar azaltır.
    const payable = page.getByRole("term").filter({ hasText: /^Ödenecek KDV$/ }).locator("xpath=following-sibling::*[1]");
    const before = money(await payable.innerText());
    expect(before).toBeGreaterThan(100);
    const other = page.getByRole("textbox", { name: "Diğer indirimler" });
    const current = (await other.inputValue()).trim();
    await other.fill(String((current ? money(current) : 0) + 100).replace(".", ","));
    await page.getByRole("button", { name: "Düzeltmeleri kaydet" }).click();
    await expect.poll(async () => money(await payable.innerText())).toBeCloseTo(before - 100, 2);

    // Sayfa yenilenip form değiştirilmeden yeniden kaydedilince tutar aynı
    // kalır (kayıtlı "100.00" binlik ayırıcı sayılıp 100 katına çıkıyordu).
    await page.reload();
    await expect(other).not.toHaveValue("");
    await page.getByRole("button", { name: "Düzeltmeleri kaydet" }).click();
    await page.waitForLoadState("networkidle");
    await expect.poll(async () => money(await payable.innerText())).toBeCloseTo(before - 100, 2);

    // Kontrol listesi: uyarılar onaylanmadan hazırlanamaz.
    const prepare = status.getByRole("button", { name: "Hazırla" });
    const warnings = page.getByRole("listitem").filter({ hasText: /^Uyarı:/ });
    const count = await warnings.count();
    if (count > 0) {
      await expect(prepare).toBeDisabled();
      // #757: durum kartı kontrol listesini işaret eder, madde kutusu "Kontrol ettim".
      await expect(status).toContainText(
        `Hazırlamak için kontrol listesindeki ${count} uyarıyı inceleyip "Kontrol ettim" ile işaretleyin.`,
      );
      for (let i = 0; i < count; i++) {
        // Onay sunucuya yazılınca işaretlenir (iyimser güncelleme yok).
        const box = warnings.nth(i).getByRole("checkbox", { name: "Kontrol ettim" });
        await box.click();
        await expect(box).toBeChecked();
      }
    }
    await expect(prepare).toBeEnabled();
    await prepare.click();
    await expect(status).toContainText("Hazırlandı");

    // Excel çıktısı.
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "Excel indir" }).click(),
    ]);
    await expectXlsx(download);

    // Verildi kaydı: tarih ve tahakkuk fiş numarası.
    const accrual = `KDV1-E2E-${Date.now()}`;
    await status.getByRole("button", { name: "Verildi olarak işaretle" }).click();
    await page.getByLabel("Tahakkuk fiş no").fill(accrual);
    await page.getByRole("button", { name: "Kaydet", exact: true }).click();
    await expect(status).toContainText("Verildi");

    const saved = page.getByRole("table").last().getByRole("row").filter({ hasText: accrual });
    await expect(saved).toContainText("KDV1");
    await expect(saved).toContainText("Verildi");
  });

  test("MUHSGK: bordro toplamları, hazırla, Excel ve taslağa geri al", async ({ page }) => {
    const status = await openTaxReturns(page, "MUHSGK");
    await expect(status).toContainText("Muhtasar ve prim hizmet beyannamesi");
    await expect(page.getByRole("heading", { name: "Vergi bildirimi" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: "Ücret ödemeleri" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Bordro toplamları" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Personel (e-bildirge eki)" })).toBeVisible();

    // #757: uyarılar "Kontrol ettim" ile işaretlenmeden hazırlanamaz (KDV1 ile aynı).
    const prepare = status.getByRole("button", { name: "Hazırla" });
    const warnings = page.getByRole("listitem").filter({ hasText: /^Uyarı:/ });
    const count = await warnings.count();
    if (count > 0) {
      await expect(prepare).toBeDisabled();
      for (let i = 0; i < count; i++) {
        const box = warnings.nth(i).getByRole("checkbox", { name: "Kontrol ettim" });
        await box.click();
        await expect(box).toBeChecked();
      }
    }
    await expect(prepare).toBeEnabled();
    await prepare.click();
    await expect(status).toContainText("Hazırlandı");
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "Excel indir" }).click(),
    ]);
    await expectXlsx(download);

    await status.getByRole("button", { name: "Taslağa geri al" }).click();
    await expect(status).toContainText("Taslak");
  });

  test("Ba / Bs: formlar ve kayıtlı beyanlar", async ({ page }) => {
    const status = await openTaxReturns(page, "Ba / Bs");
    await expect(status).toContainText("Form Ba ve Form Bs");
    await expect(page.getByRole("heading", { name: /^Form Ba \(alışlar\)/ })).toBeVisible();
    await expect(page.getByRole("heading", { name: /^Form Bs \(satışlar\)/ })).toBeVisible();

    // Kayıtlı beyanlar: demo verisinde önceki dönemin beyanları verilmiş.
    await expect(page.getByRole("heading", { name: "Kayıtlı beyanlar" })).toBeVisible();
    const saved = page.getByRole("table").last();
    await expect(saved.getByRole("row").filter({ hasText: "MUHSGK" }).filter({ hasText: "Verildi" }).first()).toBeVisible();
  });
});
