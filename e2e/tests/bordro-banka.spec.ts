import { readFileSync } from "node:fs";

import { expect, test, type Download, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Personel ve bordro (#382) ile banka hareketleri (#383), Excel içe ve dışa
 * aktarma (#395) (#452).
 *
 * Veri:
 * - Bordro: demo ofisi, Atlas Gıda. Demo dönemlerine dokunulmaz; içe aktarma
 *   2025-06 dönemine, benzersiz adlı bir personelle yapılır.
 * - Banka: Selin Yılmaz. Atlas'ın banka verisini duman testi (smoke.spec.ts)
 *   kullanır; bu dosya Selin'e her koşuda yeni IBAN'lı bir hesap ekler ve
 *   ekstresini arayüzden yükler.
 */
test.describe.configure({ mode: "serial" });

let atlas = "";
let selin = "";

test.beforeAll(async () => {
  const api = await Api.login(DEMO.owner);
  atlas = await engagementOf(api, DEMO.atlas.title);
  selin = await engagementOf(api, DEMO.selin.title);
  await api.dispose();
});

async function expectXlsx(download: Download) {
  expect(download.suggestedFilename()).toMatch(/\.xlsx$/);
  expect(readFileSync(await download.path()).subarray(0, 2).toString("latin1")).toBe("PK");
}

async function downloadOf(page: Page, click: () => Promise<void>) {
  const [download] = await Promise.all([page.waitForEvent("download"), click()]);
  await expectXlsx(download);
  return download;
}

/** Ofis saatiyle önceki ay (ekranların varsayılan beyan dönemi), YYYY-MM. */
function declarationPeriod(): string {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul", year: "numeric", month: "2-digit" })
    .formatToParts(new Date());
  const y = Number(parts.find((p) => p.type === "year")!.value);
  const m = Number(parts.find((p) => p.type === "month")!.value);
  const d = new Date(Date.UTC(y, m - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** Her koşuda farklı, algoritması geçerli TC kimlik numarası. */
function uniqueTckn(): string {
  const d = `9${String(Date.now()).slice(-6)}${String(Math.floor(Math.random() * 100)).padStart(2, "0")}`.split("").map(Number);
  const odd = d[0] + d[2] + d[4] + d[6] + d[8];
  const even = d[1] + d[3] + d[5] + d[7];
  d.push((((odd * 7 - even) % 10) + 10) % 10);
  d.push(d.reduce((a, b) => a + b, 0) % 10);
  return d.join("");
}

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `00010${"0"}${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  // TR = 29 27; kontrol: 98 − (BBAN + "292700") mod 97.
  const rearranged = `${bban}292700`;
  let rem = 0;
  for (const ch of rearranged) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

/** İki hareketli MT940 ekstresi (atlas-ziraat-2026-10.sta biçimi). */
function mt940(iban: string, period: string): Buffer {
  const yymm = period.slice(2, 4) + period.slice(5, 7);
  return Buffer.from(
    [
      ":20:E2E452",
      `:25:${iban}`,
      `:60F:C${yymm}01TRY10000,00`,
      `:61:${yymm}03${yymm.slice(2)}03D2500,00NTRFNONREF//S1`,
      ":86:KIRA ODEMESI E2E",
      `:61:${yymm}15${yymm.slice(2)}15C4200,00NTRFNONREF//S2`,
      ":86:HAVALE GELEN E2E MUSTERI",
      `:62F:C${yymm}15TRY11700,00`,
      "",
    ].join("\r\n"),
    "utf8",
  );
}

test.describe("bordro", () => {
  test("dönem bordrosu, personel, SGK ve Excel'e aktar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/payroll`));
    const sections = page.getByRole("group", { name: "Bordro bölümleri" });
    await expect(sections.getByRole("button", { name: "Dönem bordrosu" })).toHaveAttribute("aria-pressed", "true");

    // Dönem bordrosu: demo personeli ve bordro Excel'i.
    const run = page.getByRole("region", { name: /bordrosu$/ }).first();
    await expect(run.getByRole("row").filter({ hasText: "Deniz Aksoy" })).toBeVisible();
    await downloadOf(page, () => run.getByRole("button", { name: "Excel indir" }).click());
    // Bordro listesi Excel'e.
    await downloadOf(page, () => page.getByRole("button", { name: "Excel'e aktar" }).click());

    // Personel: filtre ve Excel.
    await sections.getByRole("button", { name: "Personel", exact: true }).click();
    const status = page.getByRole("group", { name: "Durum" });
    await expect(status.getByRole("button", { name: /^Çalışanlar/ })).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByRole("row").filter({ hasText: "Elif Kara" })).toContainText("Çalışıyor");
    await status.getByRole("button", { name: /^Ayrılanlar/ }).click();
    await expect(page.getByRole("row").filter({ hasText: "Elif Kara" })).toHaveCount(0);
    await status.getByRole("button", { name: "Tümü" }).click();
    await expect(page.getByRole("row").filter({ hasText: "Elif Kara" })).toBeVisible();
    await downloadOf(page, () => page.getByRole("button", { name: "Excel'e aktar" }).click());

    // SGK: tahakkuk ve ödeme durumu, Excel.
    await sections.getByRole("button", { name: "SGK", exact: true }).click();
    await expect(page.getByRole("row").filter({ hasText: "Ödendi" }).first()).toBeVisible();
    await downloadOf(page, () => page.getByRole("button", { name: "Excel'e aktar" }).click());
  });

  test("Excel / CSV bordro içe aktarma: önizle ve içe aktar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/payroll`));
    const upload = page.getByRole("region", { name: "Bordro yükle" });
    await expect(upload).toBeVisible();

    const person = `E2E Personel ${Date.now()}`;
    const csv =
      "Adı Soyadı;TC Kimlik No;SGK Sicil No;Görevi;İşe Giriş Tarihi;Gün;Brüt Ücret;SGK İşçi Payı;" +
      "İşsizlik İşçi Payı;GV İstisnası;Ödenecek Gelir Vergisi;DV İstisnası;Damga Vergisi;Net Ücret;SGK İşveren Payı;" +
      "İşsizlik İşveren Payı;SGK Teşvik\n" +
      `${person};${uniqueTckn()};3400199;Stajyer;01.06.2025;30;30.000,00;4.200,00;300,00;3.315,70;0,00;197,38;30,32;25.469,68;6.150,00;600,00;1.500,00\n`;
    await upload.locator('input[type="file"]').setInputFiles({
      name: "e2e-bordro-2025-06.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(csv, "utf8"),
    });
    // Dönem: ay seçici (#520). Yıl 2025'e çekilir, Haziran seçilir.
    await upload.locator("#payroll-import-period").click();
    const picker = page.locator('[data-slot="popover-content"]').last();
    for (let i = 0; i < 6 && !(await picker.getByText("2025", { exact: true }).isVisible()); i++) {
      await picker.getByRole("button", { name: "Önceki yıl" }).click();
    }
    await picker.getByRole("button", { name: "Haz", exact: true }).click();
    await upload.getByRole("button", { name: "Önizle" }).click();
    // Önizleme: personel sayısı, dönem ve kolon eşlemesi.
    await expect(upload.getByText("1 personel", { exact: false })).toContainText("Haziran 2025");
    await expect(upload.getByRole("listitem").filter({ hasText: "Brüt Ücret → Brüt ücret" })).toBeVisible();
    await upload.getByRole("button", { name: "İçe aktar" }).click();

    // Yeni dönem listede; personel kartına eklendi.
    const runs = page.getByRole("table").last();
    await expect(runs.getByRole("row").filter({ hasText: "Haziran 2025" })).toBeVisible();
    await page.getByRole("group", { name: "Bordro bölümleri" }).getByRole("button", { name: "Personel", exact: true }).click();
    await page.getByRole("group", { name: "Durum" }).getByRole("button", { name: "Tümü" }).click();
    await expect(page.getByRole("row").filter({ hasText: person })).toBeVisible();
  });
});

test.describe("banka", () => {
  test("ekstre yükle, hareketler, filtre ve Excel'e aktar", async ({ page }) => {
    const period = declarationPeriod();
    const iban = uniqueIban();
    const api = await Api.login(DEMO.owner);
    await api.json(await api.post(`/v1/tenant/engagements/${selin}/cash-boxes`, { iban, name: "E2E Ziraat" }));
    await api.dispose();

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${selin}/bank`));
    await expect(page.getByRole("tab", { name: "Hareketler" })).toHaveAttribute("aria-selected", "true");

    // IBAN'sız dosya: hesap seçimi Türkçe sorulur (#823), ham backend iletisi
    // görünmez. Vazgeçilince hiçbir şey eklenmez.
    await page.locator('input[type="file"][aria-label="Ekstre yükle"]').setInputFiles({
      name: "iban-yok.csv",
      mimeType: "text/csv",
      buffer: Buffer.from("Tarih;Açıklama;Tutar;Bakiye\n03.10.2026;KIRA;-100,00;900,00\n", "utf8"),
    });
    const pickBox = page.getByTestId("bank-statement-account");
    await expect(pickBox.getByRole("heading", { name: "Hangi hesabın ekstresi?" })).toBeVisible();
    await expect(pickBox).toContainText("Dosyada IBAN ya da hesap numarası yok.");
    await expect(pickBox.getByRole("combobox", { name: "Banka hesabı" })).toBeVisible();
    await expect(page.getByText("send cash_box_uuid")).toHaveCount(0);
    await pickBox.getByRole("button", { name: "İptal" }).click();
    await expect(pickBox).toBeHidden();

    // MT940: iki hareket eklenir; bildirimde sayı yerine konur.
    await page.locator('input[type="file"][aria-label="Ekstre yükle"]').setInputFiles({
      name: "e2e-selin.sta",
      mimeType: "text/plain",
      buffer: mt940(iban, period),
    });
    await expect(page.getByText(/^2 yeni hareket eklendi/)).toBeVisible();

    const table = page.getByRole("table");
    await expect(table.getByRole("row").filter({ hasText: "KIRA ODEMESI E2E" })).toBeVisible();
    await expect(table.getByRole("row").filter({ hasText: "HAVALE GELEN E2E MUSTERI" })).toContainText("4.200,00");

    // Filtre: "Eşleşti" boş, "Eşleşmedi" iki hareket.
    const match = page.getByRole("group", { name: "Eşleşme durumu" });
    await match.getByRole("button", { name: /^Eşleşti/ }).click();
    await expect(table.getByRole("row").filter({ hasText: "E2E" })).toHaveCount(0);
    await match.getByRole("button", { name: /^Eşleşmedi/ }).click();
    await expect(table.getByRole("row").filter({ hasText: "E2E" })).toHaveCount(2);

    await downloadOf(page, () => page.getByRole("button", { name: "Excel'e aktar" }).click());

    // Diğer görünümler açılır.
    await page.getByRole("tab", { name: "Mizan eşleştirme" }).click();
    await expect(page.getByRole("tab", { name: "Mizan eşleştirme" })).toHaveAttribute("aria-selected", "true");
    await page.getByRole("tab", { name: "Takma adlar" }).click();
    await expect(page.getByRole("button", { name: "Takma ad ekle" })).toBeVisible();
  });
});
