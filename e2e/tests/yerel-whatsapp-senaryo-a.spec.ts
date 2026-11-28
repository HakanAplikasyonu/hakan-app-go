import { expect, type Page, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";
import { badgeOf, downloadExtension, fakeWhatsApp, launchWithExtension, pairInPopup } from "./whatsapp-chrome";

/**
 * Senaryo A (#780), gerçek Chrome + uzantı, sahte WhatsApp Web:
 *  1. Müşavir uzantıyı kurar, panel "Kuruldu ✓" der, eşleştirir.
 *  2-3. Numarası mükellefin telefonuyla eşleşen sohbetten fiş → "Gider olarak
 *     kaydet" → mükellef sorulmadan bulunur.
 *  4. Rozet: "Okunuyor…" → "Kaydedildi · onay bekliyor" → ofis onaylar →
 *     "Onaylandı". Sayfa yenilense de rozet görünür; aynı fiş yeniden
 *     kaydedilmek istenirse "Bu fiş zaten kaydedildi" uyarısı çıkar.
 *  3b. Eşleşmeyen numara: seçici açılır, "Bu numarayı mükellefin telefonu
 *     olarak kaydet" (varsayılan işaretli) → numara mükellefe yazılır, bir
 *     dahaki kayıt sorulmadan olur.
 *
 * Yapay zekâ e2e ortamında yoksa fiş "okunamadı" olur: o zaman ofis fişi
 * reddeder ve rozetin "Reddedildi"ye döndüğü denetlenir (AI sahtesiyle
 * taslak → onay yolu koşar).
 *
 * Veri: demo ofisi; Atlas Gıda ve Selin Yılmaz'ın telefonu test boyunca bu
 * koşuya özgü numaralara çekilir, sonunda eski değerine döner.
 */

type Contact = { phone: string | null };
type WaCapture = {
  uuid: string;
  target: string;
  taxpayer: { title: string };
  receipt_uuid: string | null;
  receipt_status: string | null;
};

const run = String(Date.now()).slice(-7);
const PHONE_A = `90532${run}`; // Atlas'a yazılır: eşleşen numara
const PHONE_B = `90533${run}`; // kimsede yok: seçici + telefonu kaydet

async function saveReceipt(wa: Page, target = "expense") {
  const bar = wa.locator(".hakan-wa-bar").first();
  await expect(bar).toBeVisible();
  await bar.locator(`button[data-target="${target}"]`).click();
}

async function newestCapture(office: Api, title: string, target: string): Promise<WaCapture> {
  const page = await office.json<{ items: WaCapture[] }>(
    await office.get(`/v1/tenant/whatsapp/captures?target=${target}&limit=20`),
  );
  const hit = page.items.find((c) => c.taxpayer.title === title);
  expect(hit, `${title} için ${target} kaydı`).toBeTruthy();
  return hit!;
}

test("Senaryo A: kur, eşleştir, telefondan bul, rozet, onay, yeniden kayıt uyarısı (#780)", async ({ request }) => {
  test.setTimeout(300_000);
  const office = await Api.login(DEMO.owner);
  const atlas = await engagementOf(office, DEMO.atlas.title);
  const selin = await engagementOf(office, DEMO.selin.title);
  const contacts = (e: string) => `/v1/tenant/engagements/${e}/contacts`;
  const before = {
    atlas: (await office.json<Contact>(await office.get(contacts(atlas)))).phone ?? "",
    selin: (await office.json<Contact>(await office.get(contacts(selin)))).phone ?? "",
  };

  const ext = await downloadExtension(request);
  const ctx = await launchWithExtension(ext);
  try {
    // 1. Kurulum: algılama ve eşleştirme.
    const panel = await ctx.newPage();
    await uiLogin(panel, DEMO.owner);
    await panel.goto(officePath("/whatsapp"));
    await expect(panel.getByTestId("whatsapp-extension-installed")).toContainText("Kuruldu ✓");
    await panel.getByRole("button", { name: "Eşleştirme kodu al" }).click();
    const code = (await panel.getByTestId("whatsapp-pairing-code").textContent())?.trim() ?? "";
    await pairInPopup(ctx, code);
    await expect(panel.getByTestId("whatsapp-extension-status")).toContainText("Bağlandı ✓", { timeout: 20_000 });

    // 2-3. Atlas'ın telefonu sohbetin numarası: mükellef sorulmadan bulunur.
    expect((await office.put(contacts(atlas), { phone: `+${PHONE_A}` })).ok()).toBeTruthy();
    await fakeWhatsApp(ctx, PHONE_A, `A-${run}`);
    const wa = await ctx.newPage();
    await wa.goto("https://web.whatsapp.com/");
    await saveReceipt(wa);
    const toast = wa.locator(".hakan-wa-toast");
    await expect(toast).toContainText(`Mükellef telefondan bulundu: ${DEMO.atlas.title}`, { timeout: 30_000 });
    await expect(wa.locator(".hakan-wa-picker")).toHaveCount(0);

    // 4. Rozet ve panel listesi.
    const badge = badgeOf(wa);
    await expect(badge).toHaveText(/onay bekliyor|Okunamadı/);
    const cap = await newestCapture(office, DEMO.atlas.title, "expense");
    expect(cap.receipt_uuid).toBeTruthy();
    const base = `/v1/tenant/engagements/${atlas}`;
    await panel.reload();
    await expect(
      panel
        .getByRole("region", { name: "WhatsApp'tan kaydedilenler" })
        .getByRole("row")
        .filter({ hasText: DEMO.atlas.title })
        .first(),
    ).toBeVisible();

    let final: RegExp;
    if (cap.receipt_status === "draft") {
      await expect(badge).toContainText("Kaydedildi · onay bekliyor");
      // Onay listesinde (bekleyen masraflar) görünür; ofis onaylar.
      const pending = await office.json<{ items: { uuid: string }[] }>(
        await office.get(`${base}/expenses?status=pending&limit=200`),
      );
      expect(pending.items.length).toBeGreaterThan(0);
      expect((await office.post(`${base}/receipts/${cap.receipt_uuid}/approve`)).ok()).toBeTruthy();
      final = /Onaylandı/;
    } else {
      test.info().annotations.push({
        type: "not",
        description: `Yapay zekâ bu ortamda yok (fiş: ${cap.receipt_status}); onay yerine ret yolu denendi.`,
      });
      expect(cap.receipt_status).toBe("failed");
      await expect(badge).toContainText("Okunamadı");
      const list = await office.json<{ items: { uuid: string; status: string }[] }>(
        await office.get(`${base}/receipts?limit=200`),
      );
      expect(list.items.find((r) => r.uuid === cap.receipt_uuid)?.status).toBe("failed");
      expect((await office.post(`${base}/receipts/${cap.receipt_uuid}/reject`)).ok()).toBeTruthy();
      final = /Reddedildi/;
    }
    // Bekleyen rozet 15 sn'de bir tazelenir; ret "okunamadı"dan sonra
    // sayfa yenilenince görünür.
    if (cap.receipt_status !== "draft") await wa.reload();
    await expect(badge).toHaveText(final, { timeout: 40_000 });

    // Kaydedilmiş medya: sayfa yenilense de rozet; yeniden kayıtta uyarı.
    await wa.reload();
    await expect(badge).toHaveText(final, { timeout: 15_000 });
    await saveReceipt(wa);
    await expect(toast).toContainText(`Bu fiş zaten kaydedildi (${DEMO.atlas.title})`);
    expect((await newestCapture(office, DEMO.atlas.title, "expense")).uuid).toBe(cap.uuid);

    // 3b. Eşleşmeyen numara: seçici + "telefonu olarak kaydet".
    await fakeWhatsApp(ctx, PHONE_B, `B-${run}`);
    await wa.goto("https://web.whatsapp.com/");
    await saveReceipt(wa);
    const picker = wa.locator(".hakan-wa-picker");
    const savePhone = picker.getByRole("checkbox", { name: /Bu numarayı mükellefin telefonu olarak kaydet/ });
    await expect(savePhone).toBeChecked();
    // Sohbet eşlemesi kapalı: ikinci kaydı yalnızca telefon bulmalı.
    await picker.getByRole("checkbox", { name: "Bu sohbet için hatırla" }).uncheck();
    await picker.getByRole("searchbox", { name: "Mükellef ara" }).fill("Selin");
    await picker.getByRole("button", { name: new RegExp(DEMO.selin.title) }).first().click();
    await expect(toast).toContainText("Numara mükellefin telefonu olarak kaydedildi.", { timeout: 30_000 });
    const phone = (await office.json<Contact>(await office.get(contacts(selin)))).phone ?? "";
    expect(phone.replace(/\D/g, "").slice(-10)).toBe(PHONE_B.slice(-10));

    // Aynı sohbetten ikinci dosya (belge olarak): sorulmadan Selin'e gider.
    await saveReceipt(wa, "document");
    const noTask = wa.getByRole("button", { name: "Talepsiz kaydet" });
    if (await noTask.isVisible({ timeout: 3_000 }).catch(() => false)) await noTask.click();
    await expect(toast).toContainText(`Mükellef telefondan bulundu: ${DEMO.selin.title}`, { timeout: 30_000 });
    await expect(picker).toHaveCount(0);
  } finally {
    await ctx.close();
    await office.put(contacts(atlas), { phone: before.atlas });
    await office.put(contacts(selin), { phone: before.selin });
    await office.dispose();
  }
});
