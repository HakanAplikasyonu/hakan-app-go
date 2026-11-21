import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Belge arşivi web ekranı (#669 I): mükellef ekranının "Arşiv" sekmesi.
 * Belge yüklenir (kategori, başlık), kategori ve süresi yaklaşan süzgeci,
 * görüntüle, indir ve sil. Veritabanında kayıt doğrulanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; belgeler bu mükellefe
 * yüklenir. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Arşiv ${run} A.Ş.` };

/** Küçük ama geçerli başlıklı bir PDF (sunucu içeriği denetler). */
const pdf = (name: string, text: string) => ({
  name,
  mimeType: "application/pdf",
  buffer: Buffer.from(
    `%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [] /Count 0 >> endobj\n% ${text}\ntrailer << /Root 1 0 R >>\n%%EOF\n`,
    "latin1",
  ),
});

/** Bugünden `days` gün sonrası, YYYY-MM-DD (Türkiye günü yeterince yakın). */
const dayPlus = (days: number) =>
  new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 10);

test.describe("belge arşivi (#669 I)", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}/archive-documents`;
  const list = async (query = "") =>
    api.json<{
      total: number;
      items: {
        uuid: string;
        title: string;
        category: string;
        file_name: string;
      }[];
    }>(await api.get(`${base()}${query}`));

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "arsiv");
    // Vergi levhası: 10 gün sonra biter (süresi yaklaşan).
    const res = await api.uploadWith(
      base(),
      pdf(`vergi-levhasi-${run}.pdf`, `levha ${run}`),
      {
        category: "tax",
        title: "Vergi levhası",
        valid_until: dayPlus(10),
      },
    );
    expect(res.status(), await res.text()).toBe(201);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("yükle, süz, gör, indir ve sil", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}`));
    await page.getByRole("link", { name: "Arşiv", exact: true }).click();
    await expect(page).toHaveURL(/\/archive/);

    const table = page.getByRole("table", { name: "Arşiv belgeleri" });
    const levha = table.getByRole("row", { name: /Vergi levhası/ });
    await expect(levha).toContainText("gün kaldı");
    await expect(levha).toContainText("Vergi");

    // Sekmeden yükleme: ticaret sicil, başlık, mükellef görsün.
    const file = pdf(`faaliyet-${run}.pdf`, `faaliyet ${run}`);
    await page.getByRole("button", { name: "Belge yükle" }).click();
    const dialog = page.getByRole("dialog", { name: "Arşive belge yükle" });
    await dialog.getByLabel("Dosya").setInputFiles(file);
    await dialog.getByLabel("Kategori").selectOption("trade_registry");
    await dialog.getByLabel("Başlık").fill("Faaliyet belgesi");
    await dialog.getByRole("checkbox", { name: "Mükellef de görsün" }).click();
    await dialog.getByRole("button", { name: "Yükle", exact: true }).click();
    await expect(dialog).toBeHidden();
    const faaliyet = table.getByRole("row", { name: /Faaliyet belgesi/ });
    await expect(faaliyet).toContainText("Ticaret sicil");
    await expect(faaliyet).toContainText("Evet");

    // Veritabanı: kayıt ve kategori.
    const saved = (await list()).items.find(
      (d) => d.title === "Faaliyet belgesi",
    );
    expect(saved).toMatchObject({
      category: "trade_registry",
      file_name: file.name,
    });

    // Kategori süzgeci ve süresi yaklaşanlar.
    const chips = page.getByRole("group", { name: "Belge kategorisi" });
    await chips.getByRole("button", { name: "Ticaret sicil" }).click();
    await expect(page).toHaveURL(/kategori=trade_registry/);
    await expect(faaliyet).toBeVisible();
    await expect(levha).toHaveCount(0);
    await chips.getByRole("button", { name: /^Tümü/ }).click();
    await page.getByRole("checkbox", { name: /Süresi geçen ya da/ }).click();
    await expect(levha).toBeVisible();
    await expect(faaliyet).toHaveCount(0);
    await page.getByRole("checkbox", { name: /Süresi geçen ya da/ }).click();
    await expect(faaliyet).toBeVisible();

    // İndir: dosya adı ve içerik aynı.
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page
        .getByRole("button", { name: "Faaliyet belgesi belgesini indir" })
        .click(),
    ]);
    expect(download.suggestedFilename()).toBe(file.name);

    // Görüntüle: yeni sekmede açılır.
    const [popup] = await Promise.all([
      page.waitForEvent("popup"),
      page
        .getByRole("button", { name: "Faaliyet belgesi belgesini görüntüle" })
        .click(),
    ]);
    await popup.close();

    // Sil: onay sorulur; kayıt veritabanından gider.
    await page
      .getByRole("button", { name: "Faaliyet belgesi belgesini sil" })
      .click();
    const confirm = page.getByRole("alertdialog");
    await expect(confirm).toContainText("Faaliyet belgesi");
    // Onay penceresi açıkken (ve kapanırken) tablo erişilebilirlik ağacında
    // gizlidir; "satır yok" denetimi bu yüzden silme bitmeden geçebiliyordu.
    // Denetim silme isteğinin yanıtına ve pencerenin kapanmasına bağlanır.
    const [deleted] = await Promise.all([
      page.waitForResponse(
        (r) =>
          r.request().method() === "DELETE" &&
          /\/archive-documents\/[^/?]+$/.test(new URL(r.url()).pathname),
      ),
      confirm.getByRole("button", { name: "Sil", exact: true }).click(),
    ]);
    expect(deleted.status()).toBe(204);
    await expect(confirm).toHaveCount(0);
    await expect(levha).toBeVisible();
    await expect(faaliyet).toHaveCount(0);
    const after = await list();
    expect(after.items.map((d) => d.title)).toEqual(["Vergi levhası"]);
  });

  test("uç: aynı dosya ikinci kez reddedilir, PDF olmayan dosya 415", async () => {
    const dup = await api.uploadWith(
      base(),
      pdf(`vergi-levhasi-${run}.pdf`, `levha ${run}`),
      { category: "tax" },
    );
    expect(dup.status()).toBe(409);
    const txt = await api.uploadWith(
      base(),
      {
        name: "not.pdf",
        mimeType: "application/pdf",
        buffer: Buffer.from("düz metin"),
      },
      { category: "general" },
    );
    expect(txt.status()).toBe(415);
  });
});
