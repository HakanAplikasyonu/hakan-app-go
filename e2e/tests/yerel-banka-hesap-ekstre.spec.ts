import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Müşavir mükellefe banka hesabı ekler, ekstre bankası tanınır (#780).
 *
 * - Mükellef sayfasındaki "Banka hesapları" bölümünden IBAN'la hesap
 *   eklenir; banka IBAN'ın banka kodundan (00062) kendiliğinden seçilir,
 *   kontrol hanesi yanlış IBAN ekranda reddedilir.
 * - Garanti BBVA'nın internet şubesi CSV'si (başlıkta IBAN, ";" ayraçlı,
 *   1.234,56 tutarlar) yüklenir: biçim ve banka tanınır, dosyadaki IBAN
 *   eklenen hesabı kendiliğinden seçer, hareketler listede görünür.
 * - Hesap pasife alınır ve yeniden kullanıma açılır.
 *
 * Veri: demo ofisine her koşuda yeni mükellef ve her koşuda farklı sahte
 * Garanti IBAN'ı.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka Hesap ${run} A.Ş.` };
const tag = `E2E${run}`;

/** Garanti (00062) banka kodlu, kontrol haneleri geçerli sahte TR IBAN. */
function garantiIban(): string {
  const bban = `000620${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

const spaced = (iban: string) => iban.replace(/(.{4})/g, "$1 ").trim();

/** Garanti BBVA "Hesap Hareketleri" CSV dışa aktarımı (sentetik). */
function garantiCsv(iban: string): Buffer {
  return Buffer.from(
    [
      "Garanti BBVA",
      "Hesap Hareketleri;",
      `IBAN;${iban}`,
      "Dönem;01.10.2026 - 31.10.2026",
      "",
      "Tarih;Açıklama;Etiket;Tutar;Bakiye;Dekont No",
      `01.10.2026;FAST GELEN ${tag} YILDIZ YAZILIM;;12.500,00;62.500,00;880000`,
      `02.10.2026;EFT GIDEN ${tag} KALEM KIRTASIYE;;-3.250,75;59.249,25;880001`,
      `05.10.2026;SGK PRIM ${tag};;-1.249,25;58.000,00;880002`,
      "",
      "Toplam;;",
      "",
    ].join("\n"),
    "utf8",
  );
}

test("mükellef sayfasından banka hesabı eklenir; Garanti CSV ekstresi bankası tanınarak yüklenir (#780)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bankahesap");
  const iban = garantiIban();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const section = page.getByTestId("bank-accounts");
  await expect(section).toContainText("Banka hesapları");
  await section.getByRole("button", { name: "Banka hesabı ekle" }).click();
  const form = section.getByRole("form", { name: "Banka hesabı ekle" });

  // Kontrol hanesi yanlış IBAN ekranda reddedilir.
  const wrong = iban.slice(0, 25) + String((Number(iban[25]) + 1) % 10);
  await form.getByLabel("IBAN").fill(wrong);
  await form.getByRole("button", { name: "Kaydet" }).click();
  await expect(form.getByRole("alert")).toContainText("IBAN geçersiz");

  // Doğru IBAN: banka kendiliğinden Garanti BBVA seçilir.
  await form.getByLabel("IBAN").fill(spaced(iban));
  await expect(form.getByRole("combobox", { name: "Banka" })).toContainText("Garanti BBVA");
  await form.getByLabel("Şube").fill("0450");
  await form.getByLabel("Hesap adı").fill(`Garanti TL ${tag}`);
  await form.getByRole("button", { name: "Kaydet" }).click();
  await expect(page.getByText("Banka hesabı eklendi.")).toBeVisible();
  const row = section.getByTestId("bank-account-row").filter({ hasText: `Garanti TL ${tag}` });
  await expect(row).toContainText("Garanti BBVA");
  await expect(row).toContainText("Şube 0450");
  await expect(row).toContainText(spaced(iban));
  await expect(row).toContainText("Kullanımda");

  // Ekstre: biçim ve banka tanınır, IBAN hesabı seçer, hareketler görünür.
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  await page.locator('input[type="file"][aria-label="Ekstre yükle"]').setInputFiles({
    name: `garanti-hesap-hareketleri-${run}.csv`,
    mimeType: "text/csv",
    buffer: garantiCsv(iban),
  });
  const preview = page.getByTestId("bank-statement-preview");
  await expect(preview.getByRole("heading", { name: "Ekstre önizleme" })).toBeVisible();
  await expect(preview).toContainText("banka: Garanti BBVA");
  await preview.getByRole("button", { name: "İçe aktar (3 hareket)" }).click();
  await expect(page.getByText("3 yeni hareket eklendi (banka: Garanti BBVA).")).toBeVisible();
  const table = page.getByRole("table");
  await expect(table.getByRole("row").filter({ hasText: tag })).toHaveCount(3);
  await expect(table.getByRole("row").filter({ hasText: `EFT GIDEN ${tag}` })).toContainText("3.250,75");

  // Hareketler eklenen hesaba bağlandı.
  const boxes = await api.json<{ items: { uuid: string; iban: string | null; bank_id: string | null; active: boolean }[] }>(
    await api.get(`/v1/tenant/engagements/${engagement}/cash-boxes`),
  );
  const box = boxes.items.find((b) => b.iban === iban)!;
  expect(box).toMatchObject({ bank_id: "garanti", active: true });

  // Pasife al ve yeniden kullan.
  await page.goto(officePath(`/engagements/${engagement}`));
  await row.getByRole("button", { name: "Pasife al" }).click();
  await expect(row).toContainText("Pasif");
  await row.getByRole("button", { name: "Yeniden kullan" }).click();
  await expect(row).toContainText("Kullanımda");
  await api.dispose();
});
