import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";
import { downloadExtension, fakeWhatsApp, launchWithExtension, pairInPopup } from "./whatsapp-chrome";

/**
 * WhatsApp uzantısı gerçek Chrome'da uçtan uca (#780): panelden indir → zip'i
 * aç → yükle → panel "Kuruldu ✓" der → kod al → açılır pencerede eşleştir →
 * panel "Bağlandı ✓" → sahte WhatsApp Web'de düğme görünür → belge kaydı →
 * kayıt panelde listelenir.
 *
 * Veri: demo ofisi; Selin Yılmaz'a bir WhatsApp belge kaydı ekler (sohbet
 * eşlemesi kaydedilmez). Uzantının sunucu adresi pakete gömülüdür
 * (scripts/e2e-up.sh, WHATSAPP_API_URL=http://127.0.0.1:8080).
 */
test("uzantı: indir, yükle, algıla, eşleştir, WhatsApp'tan kaydet", async ({ request }) => {
  test.setTimeout(240_000);
  const ext = await downloadExtension(request);
  const ctx = await launchWithExtension(ext);
  try {
    const panel = await ctx.newPage();
    await uiLogin(panel, DEMO.owner);
    await panel.goto(officePath("/whatsapp"));

    // Algılama: 2. adım "Kuruldu ✓", sayfa "Eşleştir" adımına geçer.
    await expect(panel.getByTestId("whatsapp-extension-installed")).toContainText("Kuruldu ✓");
    await expect(panel.getByTestId("whatsapp-extension-status")).toContainText("Bekleniyor");
    await panel.getByRole("button", { name: "Eşleştirme kodu al" }).click();
    const code = (await panel.getByTestId("whatsapp-pairing-code").textContent())?.trim() ?? "";
    expect(code).toMatch(/^[A-Z0-9]{4}-[A-Z0-9]{4}$/);

    const pop = await pairInPopup(ctx, code);
    await expect(panel.getByTestId("whatsapp-extension-status")).toContainText("Bağlandı ✓", { timeout: 20_000 });

    await fakeWhatsApp(ctx);
    const wa = await ctx.newPage();
    await wa.goto("https://web.whatsapp.com/");

    // Açılır pencerede bağlantı testi: sunucu ve WhatsApp Web sekmesi.
    await pop.bringToFront();
    await pop.getByRole("button", { name: "Bağlantıyı test et" }).click();
    await expect(pop.getByText(/Her şey hazır/)).toBeVisible();

    await wa.bringToFront();
    const bar = wa.locator(".hakan-wa-bar").first();
    await expect(bar).toBeVisible();
    await bar.locator('button[data-target="document"]').click();

    const picker = wa.locator(".hakan-wa-picker");
    await picker.getByRole("checkbox", { name: "Bu sohbet için hatırla" }).uncheck();
    await picker.getByRole("checkbox", { name: /Bu numarayı mükellefin telefonu/ }).uncheck();
    await picker.getByRole("searchbox", { name: "Mükellef ara" }).fill("Selin");
    await picker.getByRole("button", { name: new RegExp(DEMO.selin.title) }).first().click();
    const noTask = wa.getByRole("button", { name: "Talepsiz kaydet" });
    if (await noTask.isVisible({ timeout: 3_000 }).catch(() => false)) await noTask.click();
    await expect(wa.locator(".hakan-wa-toast")).toContainText(`${DEMO.selin.title}: belge`);

    await panel.reload();
    const saved = panel.getByRole("region", { name: "WhatsApp'tan kaydedilenler" });
    await expect(
      saved.getByRole("row").filter({ hasText: DEMO.selin.title }).filter({ hasText: "Belge" }).first(),
    ).toBeVisible();
  } finally {
    await ctx.close();
  }
});
