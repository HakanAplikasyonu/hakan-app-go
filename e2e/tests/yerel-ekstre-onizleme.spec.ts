import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Ekstre önizleme (#669): bilinmeyen düzendeki bir CSV ekstre Excel benzeri
 * tabloda açılır. Müşavir kolonların anlamını seçer, devreden satırını
 * dışarıda bırakır, okunamayan tutarı yerinde düzeltir ve içe aktarır.
 * Aynı dosya yeniden yüklenince seçilen eşleme kendiliğinden gelir ve
 * hareketlerin daha önce yüklendiği söylenir.
 *
 * Veri: Selin Yılmaz'a her koşuda yeni (sahte) IBAN'lı bir hesap eklenir.
 * Kolon başlığı koşuya özgüdür; ofis şablonu her koşuda yeniden öğrenilir.
 */

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli, her koşuda farklı sahte TR IBAN. */
function uniqueIban(): string {
  const bban = `00010${"0"}${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  const rearranged = `${bban}292700`;
  let rem = 0;
  for (const ch of rearranged) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

const tag = Date.now().toString(36).toUpperCase();
const textHeader = `Metin ${tag}`;

/**
 * Satırlar (dosyada): 1 hesap, 2 başlık, 3 devreden, 4–6 hareketler (5'te
 * okunamayan tutar), 7 toplam, 8 not.
 */
function statement(iban: string): Buffer {
  return Buffer.from(
    [
      `Hesap;${iban};;`,
      `Gün;${textHeader};Para;Son Durum`,
      "DEVREDEN;;;1.000,00",
      `01.10.2026;FAST GELEN E2E ${tag};500,00;1.500,00`,
      `02.10.2026;HESAP ISLETIM UCRETI E2E ${tag};on iki elli;1.487,50`,
      `03.10.2026;EFT GIDEN KIRA E2E ${tag};-400,00;1.087,50`,
      "Toplam;;87,50;",
      "Not: bu satır okunmaz;;;",
      "",
    ].join("\n"),
    "utf8",
  );
}

async function upload(page: Page, iban: string) {
  await page.locator('input[type="file"][aria-label="Ekstre yükle"]').setInputFiles({
    name: `e2e-onizleme-${tag}.csv`,
    mimeType: "text/csv",
    buffer: statement(iban),
  });
}

async function mapColumn(page: Page, header: string, field: string) {
  await page.getByRole("combobox", { name: `${header} kolonunun anlamı` }).click();
  await page.getByRole("option", { name: field, exact: true }).click();
}

test("ekstre önizleme: kolonları eşle, satırı dışarıda bırak, hücreyi düzelt; eşleme hatırlanır", async ({ page }) => {
  const iban = uniqueIban();
  const api = await Api.login(DEMO.owner);
  const selin = await engagementOf(api, DEMO.selin.title);
  await api.json(await api.post(`/v1/tenant/engagements/${selin}/cash-boxes`, { iban, name: `E2E Önizleme ${tag}` }));
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${selin}/bank`));
  await upload(page, iban);

  // Bilinmeyen düzen: tablo açılır, kolonlar seçilir.
  const preview = page.getByTestId("bank-statement-preview");
  await expect(preview.getByRole("heading", { name: "Ekstre biçimi tanınmadı" })).toBeVisible();
  await expect(preview.getByTestId("bank-statement-row")).toHaveCount(6);
  await mapColumn(page, "Gün", "İşlem tarihi");
  await mapColumn(page, textHeader, "Açıklama");
  await mapColumn(page, "Para", "Tutar (işaretli)");
  await mapColumn(page, "Son Durum", "Bakiye");

  // Deneme okuması: devreden ve bozuk tutar okunamaz.
  const counts = preview.getByTestId("bank-statement-counts");
  await expect(counts).toContainText("2 yeni hareket eklenecek; 2 satır okunamadı.");

  // Devreden satırı dışarıda; bozuk tutar düzeltilir.
  await preview.getByRole("checkbox", { name: "3. satırı içe aktar" }).click();
  await expect(counts).toContainText("2 yeni hareket eklenecek; 1 satır okunamadı.");
  const cell = preview.getByRole("textbox", { name: "5. satır, Para" });
  await cell.fill("-12,50");
  await expect(counts).toContainText("3 yeni hareket eklenecek; 0 satır okunamadı.");

  await preview.getByRole("button", { name: "İçe aktar (3 hareket)" }).click();
  await expect(page.getByText(/^3 yeni hareket eklendi/)).toBeVisible();
  await expect(preview).toHaveCount(0);
  const table = page.getByRole("table");
  await expect(table.getByRole("row").filter({ hasText: `HESAP ISLETIM UCRETI E2E ${tag}` })).toContainText("12,50");
  await expect(table.getByRole("row").filter({ hasText: `E2E ${tag}` })).toHaveCount(3);

  // Aynı dosya yeniden: eşleme kendiliğinden gelir, hareketler zaten yüklü.
  await upload(page, iban);
  await expect(preview.getByRole("heading", { name: "Ekstre önizleme" })).toBeVisible();
  await expect(preview.getByRole("combobox", { name: "Gün kolonunun anlamı" })).toContainText("İşlem tarihi");
  await expect(preview.getByRole("combobox", { name: "Para kolonunun anlamı" })).toContainText("Tutar (işaretli)");
  await expect(counts).toContainText("daha önce yüklenmiş");
  await expect(preview.getByRole("button", { name: /^İçe aktar/ })).toBeDisabled();
  await preview.getByRole("button", { name: "İptal" }).click();
  await expect(table.getByRole("row").filter({ hasText: `E2E ${tag}` })).toHaveCount(3);
});
