import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayerWithUser,
  Api,
  DEMO,
  expectNoRawText,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Belge talebi kanalları (Oğuz'un geri bildirimi): ofis mükelleften belge
 * isterken mükellefe hangi kanallardan haber verileceğini (uygulama
 * bildirimi, SMS, e-posta), son tarihi ve hatırlatmayı seçer. Her gönderim
 * iletişim geçmişine yazılır; satırda kanal durumu ve "Hatırlat" görünür.
 * Mükellef uygulama bildirimini "Mesajlarım"da görür. Ofis ayarı
 * (doc_request_channels) kapatılınca "şimdi gönder" 409 döner.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve kullanıcısı. Ofis
 * ayarı testi sonunda eski hâline döner.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Belge Kanal ${run} A.S.` };

type Delivery = { channel: string; kind: string; status: string };
type Task = {
  uuid: string;
  status: string;
  channels?: string[];
  remind_days_before?: number;
  remind_at?: string | null;
  channel_reminded_at?: string | null;
  deliveries?: Delivery[];
};
type HistoryItem = { channel: string; source_rule: string; source_ref: string };

/** Bugünden n gün sonrası (İstanbul) YYYY-MM-DD. */
function dayAfter(n: number) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(
    new Date(Date.now() + n * 86_400_000),
  );
}

test("belge isterken kanal, son tarih ve hatırlatma seçilir; her gönderim iletişim geçmişinde", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(browser, api, taxpayer, "belgekanal");
  const base = `/v1/tenant/engagements/${who.engagement}/document-tasks`;
  const orig = await api.json<{ settings: Record<string, unknown> }>(await api.get("/v1/tenant/settings"));
  const settings = orig.settings ?? {};
  try {
    // Ayar açık (varsayılan).
    let res = await api.patch("/v1/tenant/settings", { settings: { ...settings, doc_request_channels: true } });
    expect(res.status(), await res.text()).toBe(200);

    // 1) API (ofis mobil uygulamasının kullandığı uç): push + e-posta,
    // 3 gün önce hatırlatma.
    res = await api.post(base, {
      title: `Kira kontratı ${run}`,
      due_date: dayAfter(10),
      channels: ["push", "email"],
      remind_days_before: 3,
    });
    expect(res.status(), await res.text()).toBe(201);
    const task = (await res.json()).data as Task;
    expect(task.channels).toEqual(["push", "email"]);
    expect(task.remind_days_before).toBe(3);
    expect(task.remind_at).toBeTruthy();
    const byChannel = Object.fromEntries((task.deliveries ?? []).map((d) => [d.channel, d]));
    // Mükellefin uygulama kullanıcısı var: bildirim gitti.
    expect(byChannel.push?.status).toBe("sent");
    // E-posta mükellef kartındaki adrese; ortamda deneme modu olabilir,
    // kartta adres yoksa kanal "skipped" (talep yine açılır).
    const emailStatus = byChannel.email?.status ?? "";
    expect(["sent", "dry_run", "skipped"]).toContain(emailStatus);

    // Bilinmeyen kanal reddedilir.
    res = await api.post(base, { title: `Yanlış ${run}`, due_date: dayAfter(5), channels: ["fax"] });
    expect(res.status(), await res.text()).toBe(400);

    // 2) İletişim geçmişi: her kanal için bir kayıt, kaynak görev.
    const history = await api.json<{ items: HistoryItem[] }>(
      await api.get(`/v1/tenant/engagements/${who.engagement}/communications`),
    );
    const mine = history.items.filter((m) => m.source_ref === task.uuid);
    // Gönderilmeyen (skipped) kanalın kaydı olmaz.
    expect(mine.map((m) => m.channel).sort()).toEqual(
      emailStatus === "skipped" ? ["push"] : ["email", "push"],
    );
    expect(mine.every((m) => m.source_rule === "doc_request")).toBe(true);

    // 3) Şimdi gönder (Hatırlat): yalnız push; hemen ikincisi reddedilir.
    res = await api.post(`${base}/${task.uuid}/send`, { channels: ["push"], reminder: true });
    expect(res.status(), await res.text()).toBe(200);
    const sent = (await res.json()).data as Task;
    expect(sent.channel_reminded_at).toBeTruthy();
    expect(sent.deliveries?.find((d) => d.channel === "push")?.kind).toBe("reminder");
    res = await api.post(`${base}/${task.uuid}/send`, { reminder: true });
    expect(res.status(), await res.text()).toBe(409);
    expect(await res.text()).toContain("RECENTLY_REMINDED");

    // 4) Mükellef "Mesajlarım"da uygulama bildirimini görür.
    const tp = await Api.login(who.email, who.password);
    try {
      const msgs = await tp.json<{ items: { channel: string; subject: string }[] }>(
        await tp.get("/v1/taxpayer/messages", { "X-Taxpayer": who.taxpayerUuid }),
      );
      expect(msgs.items.some((m) => m.channel === "push")).toBe(true);
    } finally {
      await tp.dispose();
    }

    // 5) Ekran: formda kanal, son tarih ve hatırlatma; satırda kanal durumu.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${who.engagement}`));
    await page.getByTestId("doc-request-open").click();
    const form = page.getByTestId("doc-request-form");
    await form.getByLabel("Ne istiyorsunuz?").fill(`Kasa defteri ${run}`);
    await expect(form.getByTestId("doc-request-channel-push")).toBeChecked();
    await form.getByTestId("doc-request-channel-email").click();
    await form.getByLabel("Hatırlatma").selectOption("2");
    await form.getByTestId("doc-request-submit").click();
    await expect(form).toBeHidden();
    const row = page.getByTestId("doc-task-row").filter({ hasText: `Kasa defteri ${run}` });
    await expect(row).toBeVisible();
    await expect(row.getByTestId("doc-task-delivery-push")).toHaveAttribute("data-status", "sent");
    await expect(row.getByTestId("doc-task-delivery-email")).toBeVisible();
    await expect(row.getByTestId("doc-task-reminder")).toContainText("Hatırlatma");
    await expectNoRawText(row);
    await row.getByTestId("doc-task-remind").click();
    await expect(page.getByText("Hatırlatma gönderildi.")).toBeVisible();

    // İletişim geçmişi ekranında uygulama bildirimi kanalı.
    await page.goto(officePath(`/engagements/${who.engagement}/communications`));
    await expect(page.getByTestId("communications-filter-push")).toBeVisible();
    await expect(page.locator('[data-testid="communication-item"][data-channel="push"]').first()).toBeVisible();

    // 6) Ayar kapalı: şimdi gönder 409, form kanal sormaz.
    res = await api.patch("/v1/tenant/settings", { settings: { ...settings, doc_request_channels: false } });
    expect(res.status(), await res.text()).toBe(200);
    res = await api.post(`${base}/${task.uuid}/send`, { channels: ["push"] });
    expect(res.status(), await res.text()).toBe(409);
    expect(await res.text()).toContain("DOC_REQUEST_CHANNELS_DISABLED");
  } finally {
    const back = await api.patch("/v1/tenant/settings", { settings });
    expect(back.status(), await back.text()).toBe(200);
    await api.dispose();
  }
});
