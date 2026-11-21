import { expect, test, type Route } from "@playwright/test";

import {
  Api,
  DEMO,
  engagementOf,
  officePath,
  openNewEntry,
  uiLogin,
} from "./lib";

/**
 * Fiş kaydedilemeyince ne olduğu Türkçe yazar (#710): sunucunun İngilizce
 * mesajı (ör. `body must be {"reason": ...}`) ya da ağ hatasının ham metni
 * ekrana çıkmaz; bilinen kod Türkçesiyle, bilinmeyen durum "Fiş
 * kaydedilemedi" ile söylenir. Eskiden yedek metin "Veriler yüklenemedi"
 * idi; kaydetme hatası için yanlıştı.
 *
 * Veri: demo ofisi, Atlas Gıda; kayıt isteği sahte yanıtla kesilir, hiçbir
 * şey yazılmaz.
 */
test("yeni fiş: kayıt hatası Türkçe, kod metni yok (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  let reply: (r: Route) => Promise<void> = (r) =>
    r.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({
        error: {
          code: "INVALID_BODY",
          message: 'body must be {"reason": "..."}',
        },
      }),
    });
  await page.route(/\/journal-drafts$/, async (r) =>
    r.request().method() === "POST" ? reply(r) : r.fallback(),
  );

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/journals?taxpayer=${engagement}`));
  await openNewEntry(page);
  const dialog = page.getByRole("dialog", { name: "Yeni fiş" });
  await dialog.getByLabel("Hesap kodu 1", { exact: true }).fill("770.01");
  await dialog.getByLabel("Borç 1", { exact: true }).fill("100");
  await dialog.getByLabel("Hesap kodu 2", { exact: true }).fill("100.01");
  await dialog.getByLabel("Alacak 2", { exact: true }).fill("100");
  const save = dialog.getByRole("button", { name: "Taslağı kaydet" });

  // Bilinen kod: Türkçesi; sunucunun İngilizce mesajı görünmez.
  await save.click();
  await expect(
    dialog.getByText(
      "Gönderilen bilgiler eksik ya da hatalı; alanları denetleyip yeniden deneyin.",
    ),
  ).toBeVisible();
  await expect(dialog.getByText(/body must be/)).toHaveCount(0);

  // Ağ hatası: ham metin yerine "Fiş kaydedilemedi".
  reply = (r) => r.abort();
  await save.click();
  await expect(dialog.getByText(/^Fiş kaydedilemedi\./)).toBeVisible();
  await expect(dialog.getByText("Veriler yüklenemedi")).toHaveCount(0);
  await expect(dialog.getByText(/Failed to fetch|NetworkError/)).toHaveCount(0);
});
