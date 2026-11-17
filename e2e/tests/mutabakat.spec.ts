import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, letterToken, officePath, uiLogin, uniqueEmail, uniqueVkn } from "./lib";

/**
 * Mutabakat panosu (#389, #452): Atlas Gıda'nın demo mutabakatı (mizan –
 * kaynak karşılaştırması, cari, BA/BS beyanı, mektuplar) ve karşı firmanın
 * yanıt sayfası.
 *
 * Veri: demo ofisi, Atlas Gıda. Dosya yalnızca kendi eklediği kayıtlara
 * dokunur: açık bir farkın kararı (açıklayıp yeniden açar), benzersiz adresli yeni
 * bir mektup ve benzersiz VKN'li bir BA/BS satırı. "Yeniden karşılaştır"
 * çalıştırılmaz (demo kararlarını sıfırlamasın).
 */
test.describe.configure({ mode: "serial" });

let engagement = "";

async function openReconciliation(page: Page) {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/reconciliation`));
  await expect(page.getByRole("heading", { level: 1, name: DEMO.atlas.title })).toBeVisible();
  await expect(page.getByRole("tablist", { name: "Mutabakat görünümü" })).toBeVisible();
}

test.beforeAll(async () => {
  const api = await Api.login(DEMO.owner);
  engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();
});

test.describe("mutabakat", () => {
  test("hesaplar: liste, filtre, ayrıntı ve karar", async ({ page }) => {
    await openReconciliation(page);
    await expect(page.getByRole("tab", { name: "Hesaplar" })).toHaveAttribute("aria-selected", "true");
    const rows = page.getByRole("table").getByRole("row");

    // Demo kararları (cmd/demo-seed/reconciliation.go): bir fark
    // açıklanmış, bir fark inceleme kuyruğunda. Hesap kodları demo verisiyle
    // değişebildiği için duruma göre aranır.
    await expect(rows.filter({ hasText: "Açıklandı" }).first()).toBeVisible();
    await expect(rows.filter({ hasText: "İncelemede" }).first()).toBeVisible();

    // Filtre: "Yalnızca açık farklar" kapanınca eşleşen hesaplar da görünür.
    const onlyOpen = page.getByRole("checkbox", { name: "Yalnızca açık farklar" });
    await expect(onlyOpen).toBeChecked();
    await expect(rows.filter({ hasText: "Eşleşti" })).toHaveCount(0);
    await onlyOpen.click();
    await expect(onlyOpen).not.toBeChecked();
    await expect(rows.filter({ hasText: "Eşleşti" }).first()).toBeVisible();
    await onlyOpen.click();
    await expect(rows.filter({ hasText: "Eşleşti" })).toHaveCount(0);

    // Karar verilecek satır: açık (açıklanmamış, incelemede olmayan) ilk fark;
    // "Açıklama" başlığı ve "Açıklandı" durumu eşleşmez.
    const open = rows.filter({ hasText: /Açık(?!la)/ }).filter({ hasNotText: "İncelemede" }).first();
    const code = (await open.getByRole("cell").first().innerText()).match(/^\d+/)?.[0];
    expect(code, "açık fark satırı").toBeTruthy();
    const row = rows.filter({ hasText: new RegExp(`^${code}`) });

    // Ayrıntı: kaynak türüne göre kırılım ve fişlenmemiş kayıtlar.
    await row.getByRole("button", { name: "Ayrıntı" }).click();
    const detail = page.getByRole("dialog", { name: new RegExp(`^${code}`) });
    await expect(detail.getByRole("heading", { name: "Kaynaklara göre" })).toBeVisible();
    await expect(detail.getByRole("heading", { name: "Fişlenmemiş kayıtlar" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(detail).toBeHidden();

    // Karar: fark açıklanır, not satırda görünür.
    const note = `e2e açıklama ${Date.now()}`;
    await page.getByRole("button", { name: `${code} için karar` }).click();
    await page.getByRole("menuitem", { name: "Açıkla" }).click();
    const explain = page.getByRole("dialog", { name: `${code}: farkı açıkla` });
    await explain.getByLabel("Not").fill(note);
    await explain.getByRole("button", { name: "Kaydet" }).click();
    await expect(explain).toBeHidden();
    await expect(row).toContainText("Açıklandı");
    await expect(row).toContainText(note);

    // Yeniden aç: fark tekrar açık olur.
    await page.getByRole("button", { name: `${code} için karar` }).click();
    await page.getByRole("menuitem", { name: "Yeniden aç" }).click();
    await expect(row).not.toContainText("Açıklandı");
    await expect(row).toContainText("Açık");
  });

  test("BA/BS beyanı: satır ekle ve sil", async ({ page }) => {
    await openReconciliation(page);
    await page.getByRole("tab", { name: "BA/BS beyanı" }).click();
    const table = page.getByRole("table");
    await expect(table.getByRole("row").filter({ hasText: "Kalem Kırtasiye" })).toBeVisible();

    const name = `E2E Tedarik ${Date.now()}`;
    // Satır ekle: form tablonun üstünde açılır (seçili form: BA).
    await page.getByRole("button", { name: "Satır ekle" }).click();
    await page.getByRole("textbox", { name: "VKN / TCKN" }).fill(uniqueVkn());
    await page.getByRole("textbox", { name: "Karşı firma" }).fill(name);
    await page.getByRole("textbox", { name: "Belge sayısı" }).fill("2");
    await page.getByRole("textbox", { name: "Tutar" }).fill("7500");
    await page.getByRole("button", { name: "Kaydet" }).click();
    const row = table.getByRole("row").filter({ hasText: name });
    await expect(row).toContainText("Elle");
    await expect(row).toContainText("7.500,00");

    await row.getByRole("button", { name: `${name} satırını sil` }).click();
    const confirm = page.getByRole("alertdialog");
    if (await confirm.isVisible().catch(() => false)) {
      await confirm.getByRole("button", { name: /Sil/ }).click();
    }
    await expect(row).toHaveCount(0);
  });

  test("cari mektup: e-postayla gönder, karşı firma yanıtlar", async ({ page, browser }) => {
    await openReconciliation(page);
    await page.getByRole("tab", { name: "Cari" }).click();
    const onlyIssues = page.getByRole("checkbox", { name: "Yalnızca sorunlu cariler" });
    await expect(onlyIssues).toBeChecked();
    await onlyIssues.click();
    const cari = page.getByRole("table").getByRole("row").filter({ hasText: "Örnek Alıcı" });
    await expect(cari).toBeVisible();

    // Mektup: benzersiz adrese hemen gönderilir.
    const to = uniqueEmail("mutabakat");
    await cari.getByRole("button", { name: "Mektup" }).click();
    const dialog = page.getByRole("dialog", { name: "Cari mutabakat mektubu" });
    await dialog.getByLabel("Alıcı e-posta").fill(to);
    await dialog.getByRole("checkbox", { name: "Hemen e-postayla gönder" }).check();
    await dialog.getByRole("button", { name: "Oluştur ve gönder" }).click();
    await expect(dialog).toBeHidden();

    await page.getByRole("tab", { name: "Mektuplar" }).click();
    const letter = page.getByRole("table").getByRole("row").filter({ hasText: to });
    await expect(letter).toBeVisible();
    await expect(letter).toContainText(/Kuyrukta|Gönderildi/);

    // Karşı firma mektuptaki bağlantıyla, oturum açmadan yanıtlar.
    // E-posta beklenmez: mektup gönderim taraması yalnızca ayrı worker
    // sürecinde (cmd/worker) çalışır; e2e ortamı API içi worker kullanır ve
    // mektup "Kuyrukta" kalır (#461). Belirteç lib.ts'teki letterToken ile.
    const api = await Api.login(DEMO.owner);
    const letters = await api.json<{ items: { uuid: string; recipient_email: string | null }[] }>(
      await api.get(`/v1/tenant/engagements/${engagement}/reconciliation/letters`),
    );
    await api.dispose();
    const created = letters.items.find((l) => l.recipient_email === to);
    expect(created, `letter to ${to}`).toBeTruthy();
    const token = letterToken(created!.uuid);
    const guest = await browser.newContext({ locale: "tr-TR", timezoneId: "Europe/Istanbul" });
    const response = await guest.newPage();
    await response.goto(`/mutabakat/${token}`);
    await expect(response.getByRole("heading", { name: "Cari hesap mutabakatı" })).toBeVisible();
    await expect(response.getByText(DEMO.atlas.title).first()).toBeVisible();
    await response.getByLabel("Mutabık değiliz").check();
    await response.getByLabel("Kayıtlarımızdaki bakiye (TL)").fill("1250");
    await response.getByLabel("Adınız soyadınız").fill("Deniz Cari");
    await response.getByRole("textbox", { name: "Açıklama", exact: true }).fill("Ekim sonu ödemesi kayıtlarımızda yok.");
    await response.getByRole("button", { name: "Yanıtı gönder" }).click();
    await expect(response.getByText("Teşekkürler, itirazınız iletildi.")).toBeVisible();

    // Aynı bağlantı yeniden açılınca yanıt görünür, form yoktur.
    await response.reload();
    await expect(response.getByText("Teşekkürler, itirazınız iletildi.")).toBeVisible();
    await expect(response.getByRole("button", { name: "Yanıtı gönder" })).toHaveCount(0);
    await guest.close();

    // Ofis panelinde mektup yanıtlandı, itiraz ve yanıtlayan görünür.
    await page.reload();
    await page.getByRole("tab", { name: "Mektuplar" }).click();
    await expect(letter).toContainText("Yanıtlandı");
    await expect(letter).toContainText("Mutabık değil");
    await expect(letter).toContainText("Deniz Cari");
  });

  test("geçersiz yanıt bağlantısı", async ({ page }) => {
    await page.goto("/mutabakat/gecersiz-belirtec-0000000000000000");
    await expect(page.getByRole("heading", { name: "Bağlantı geçersiz" })).toBeVisible();
  });
});
