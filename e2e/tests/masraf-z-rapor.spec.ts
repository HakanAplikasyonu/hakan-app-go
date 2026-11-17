import { readFileSync } from "node:fs";

import { expect, test, type Download, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin, uniqueEmail } from "./lib";

/**
 * Masraf ve Z raporu (#384), Easytrade (#385), mükellef raporu ve toplu
 * gönderim (#386), Excel içe / dışa aktarma (#395) (#452).
 *
 * Veri: demo ofisi.
 * - Masraf ve Z: Atlas Gıda. Dışa aktarılan dosya değiştirilmeden geri
 *   yüklenir; hiçbir satır eklenmez (#484). Onay / ret yapılmaz: duman testi
 *   Ekim dönemini kilitler, bu dosya dönem durumuna bağlı kalmaz.
 * - Easytrade: Atlas Gıda, yalnızca okur.
 * - Rapor: Atlas Gıda (demo raporu zaten gönderilmiş); gönderim benzersiz
 *   alıcıya kuyruğa alınır. Selin'in gönderilmemiş raporu co-pilot testinindir
 *   (copilot-izgara.spec.ts). E-posta beklenmez: gönderim taraması e2e'deki
 *   API içi worker'da yok (#461).
 */
test.describe.configure({ mode: "serial" });

let atlas = "";

test.beforeAll(async () => {
  const api = await Api.login(DEMO.owner);
  atlas = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();
});

async function download(page: Page, click: () => Promise<void>): Promise<Download> {
  const [d] = await Promise.all([page.waitForEvent("download"), click()]);
  return d;
}

async function expectMagic(d: Download, ext: RegExp, magic: string) {
  expect(d.suggestedFilename()).toMatch(ext);
  expect(readFileSync(await d.path()).subarray(0, magic.length).toString("latin1")).toBe(magic);
}

/** Excel'e aktar → aynı dosyayı Excel'den yükle: yeni satır eklenmez. */
async function roundTrip(page: Page) {
  const exported = await download(page, () => page.getByRole("button", { name: "Excel'e aktar" }).click());
  await expectMagic(exported, /\.xlsx$/, "PK");
  await page.locator('main input[type="file"]').first().setInputFiles(await exported.path());
  await expect(page.getByText(/^\d+ satır okundu: 0 eklendi, \d+ zaten vardı, 0 hatalı$/)).toBeVisible();
}

test.describe("masraf ve Z raporu", () => {
  test("masraflar: filtre, kategoriler, Excel gidiş-dönüş", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/expenses`));
    await expect(page.getByRole("heading", { name: "Masraflar" })).toBeVisible();
    const rows = page.getByRole("table").getByRole("row");

    // Onay durumu filtresi.
    const status = page.getByRole("group", { name: "Onay durumu" });
    await status.getByRole("button", { name: "Bekliyor" }).click();
    await expect(rows.filter({ hasText: "Bekliyor" }).first()).toBeVisible();
    await expect(rows.filter({ hasText: "Onaylandı" })).toHaveCount(0);
    await status.getByRole("button", { name: "Tümü" }).click();

    // Kategori filtresi: yalnızca seçilen kategori.
    await page.getByRole("combobox", { name: "Kategori" }).first().selectOption({ label: "Akaryakıt" });
    // Onaylı satırda kategori metindir, bekleyende seçim kutusu.
    await expect
      .poll(async () => {
        const texts = await rows.evaluateAll((trs) =>
          trs
            .filter((tr) => tr.querySelector("td"))
            .map((tr) => {
              const select = tr.querySelector("select");
              return select ? (select.selectedOptions[0]?.text ?? "") : (tr.textContent ?? "");
            }),
        );
        return texts.length > 0 && texts.every((t) => t.includes("Akaryakıt"));
      })
      .toBe(true);
    await page.getByRole("combobox", { name: "Kategori" }).first().selectOption({ label: "Tümü" });

    // Kategoriler penceresi.
    await page.getByRole("button", { name: "Kategoriler" }).click();
    const categories = page.getByRole("dialog", { name: "Kategoriler" });
    await expect(categories.getByRole("button", { name: "Akaryakıt kategorisini düzenle" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(categories).toBeHidden();

    await roundTrip(page);
  });

  test("Z raporları: süreklilik, filtre, Excel gidiş-dönüş", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/z-reports`));
    await expect(page.getByRole("heading", { name: "Z raporları" })).toBeVisible();
    await expect(page.getByRole("region", { name: "KDV oranı özeti" })).toBeVisible();

    await page.getByRole("button", { name: "Sürekliliği denetle" }).click();
    await expect(page.getByText(/^Denetim bitti: \d+ raporda uyarı var$/)).toBeVisible();

    const status = page.getByRole("group", { name: "Onay durumu" });
    await status.getByRole("button", { name: "Bekliyor" }).click();
    await expect(page.getByRole("table").getByRole("row").filter({ hasText: "Bekliyor" }).first()).toBeVisible();
    await status.getByRole("button", { name: "Tümü" }).click();

    await roundTrip(page);
  });
});

test.describe("Easytrade", () => {
  test("bağlantı, ödeme türü özeti ve finansal işlemler", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/easytrade`));
    const connection = page.getByRole("region", { name: "Bağlantı" });
    await expect(connection).toContainText("Etkin");
    await expect(connection.getByRole("combobox", { name: "Senkron aralığı" })).toBeVisible();
    await expect(page.getByRole("region", { name: /^Ödeme türüne göre/ })).toContainText("Nakit");
    const txns = page.getByRole("region", { name: /^Finansal işlemler/ });
    // Bölgede iki tablo var: işlemler ve cari toplamları.
    const list = txns.getByRole("table").first();
    await expect(list.getByRole("row").filter({ hasText: "₺" }).first()).toBeVisible();

    // Tablo araması süzer.
    await txns.getByRole("textbox", { name: "Tabloda ara…" }).fill("bulunmayan-e2e-kaydi");
    await expect(list.getByRole("row").filter({ hasText: "₺" })).toHaveCount(0);
  });
});

test.describe("mükellef raporu", () => {
  test("aylık / yıllık, PDF ve e-postayla gönder", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/report`));
    await expect(page.getByRole("heading", { name: "Mükellef raporu" })).toBeVisible();

    const kind = page.getByRole("radiogroup", { name: "Rapor türü" });
    await kind.getByRole("radio", { name: "Yıllık" }).click();
    await expect(kind.getByRole("radio", { name: "Yıllık" })).toBeChecked();
    await kind.getByRole("radio", { name: "Aylık" }).click();

    const pdf = await download(page, () => page.getByRole("button", { name: "PDF indir" }).click());
    await expectMagic(pdf, /\.pdf$/, "%PDF");

    const to = uniqueEmail("rapor");
    await page.getByRole("button", { name: "E-postayla gönder" }).click();
    const dialog = page.getByRole("dialog", { name: "Raporu mükellefe gönder" });
    await dialog.getByRole("textbox", { name: "Alıcı" }).fill(to);
    await dialog.getByRole("button", { name: "Gönder" }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(to).first()).toBeVisible();
  });

  test("toplu gönderim: seçilen mükellefe kuyruğa alınır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/reports"));
    await expect(page.getByRole("heading", { level: 1, name: "Mükellef raporları" })).toBeVisible();
    const send = page.getByRole("button", { name: /^Seçilenlere gönder/ });
    await expect(send).toBeDisabled();

    await page.getByRole("row").filter({ hasText: DEMO.atlas.title }).getByRole("checkbox").check();
    await expect(send).toHaveText(/\(1\)/);
    await send.click();
    const confirm = page.getByRole("dialog");
    if (await confirm.isVisible().catch(() => false)) {
      await confirm.getByRole("button", { name: /Gönder/ }).last().click();
    }
    await expect(page.getByRole("table").last().getByRole("row").filter({ hasText: DEMO.atlas.title }).first()).toBeVisible();
  });
});
