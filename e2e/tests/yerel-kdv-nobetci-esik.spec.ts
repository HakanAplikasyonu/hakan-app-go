import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayerWithUser,
  Api,
  DEMO,
  deriveUbl,
  expectNoRawText,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * KDV nöbetçisi eşik tutarı, kanal ve son uyarılar (#780 B, demo geri
 * bildirimi: "uyarı mekanizmasını ve eşiği bulamadım").
 *
 * Ayarlar → KDV takibi → KDV nöbetçisi: aç/kapa, "Ödenecek KDV şu tutarı
 * geçerse uyar" ve "Uyarı nasıl gelsin". Eşik uyarı kurallarındaki
 * VAT_PAYABLE_THRESHOLD'a yazılır (ikinci bir depo yok). Altında "Son KDV
 * uyarıları": ofis genelindeki son nöbetçi olayları, mükellefe bağlantıyla.
 *
 * Veri: demo ofisine yeni bir mükellef; bugünün tarihli 72 TL KDV'li bir
 * satış faturası. Bu mükellefe özel eşik 50 TL (mükellef katmanı), böylece
 * ofis eşiği diğer testleri etkilemez. Ayarlar finally'de eski hâline döner.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Esik ${run} A.S.` };

type Tracking = {
  report_enabled: boolean;
  report_day: number;
  watch_enabled: boolean;
  watch_email: boolean;
  payable_limit: string | null;
};
type WatchEvent = {
  uuid: string;
  period: string;
  event: string;
  payload: Record<string, unknown>;
};
type OfficeEvent = WatchEvent & {
  engagement_uuid: string;
  taxpayer_uuid: string;
  taxpayer_title: string;
};
type Rule = {
  code: string;
  effective: { enabled: boolean; params: Record<string, unknown> };
};

/** Bugün (İstanbul) YYYY-MM-DD. */
function today() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
  }).format(new Date());
}

test("KDV nöbetçisi eşiği ayarlanır, kanal seçilir, son KDV uyarıları mükellef bağlantısıyla görünür (#780)", async ({
  browser,
  page,
}) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const settings = await api.json<Tracking>(
    await api.get("/v1/tenant/vat-tracking-settings"),
  );
  const put = (extra: Partial<Tracking>) =>
    api.put("/v1/tenant/vat-tracking-settings", {
      report_enabled: settings.report_enabled,
      report_day: settings.report_day,
      ...extra,
    });
  let engagement = "";
  try {
    // 1) Eşik ve kanal API'den kaydedilir, geri okunur.
    let res = await put({
      watch_enabled: true,
      watch_email: true,
      payable_limit: "12345.67",
    });
    expect(res.status(), await res.text()).toBe(200);
    let saved = (await res.json()).data as Tracking;
    expect(saved.payable_limit).toBe("12345.67");
    expect(saved.watch_email).toBe(true);
    saved = await api.json<Tracking>(
      await api.get("/v1/tenant/vat-tracking-settings"),
    );
    expect(saved.payable_limit).toBe("12345.67");
    expect(saved.watch_email).toBe(true);
    expect(saved.watch_enabled).toBe(true);

    // Eşik uyarı kurallarındaki VAT_PAYABLE_THRESHOLD'dur (tek depo).
    const rules = await api.json<{ items: Rule[] }>(
      await api.get("/v1/tenant/warning-rules"),
    );
    const rule = rules.items.find((r) => r.code === "VAT_PAYABLE_THRESHOLD")!;
    expect(rule.effective.params.limit).toBe("12345.67");
    expect(rule.effective.enabled).toBe(true);

    // Geçersiz tutar reddedilir.
    res = await put({ payable_limit: "-5" });
    expect(res.status(), await res.text()).toBe(400);
    res = await put({ payable_limit: "abc" });
    expect(res.status(), await res.text()).toBe(400);

    // 2) Yeni mükellef ve bu ayın satış faturası (72 TL KDV).
    const who = await addConsentedTaxpayerWithUser(
      browser,
      api,
      taxpayer,
      "kdvesik",
    );
    engagement = who.engagement;
    const vat = `/v1/tenant/engagements/${engagement}/vat-forecast`;
    const ubl = deriveUbl(
      "atlas-01-temel-satis.xml",
      taxpayer.vkn,
      taxpayer.title,
    );
    ubl.buffer = Buffer.from(
      ubl.buffer.toString("utf8").split("2026-10-01").join(today()),
      "utf8",
    );
    const up = await api.upload(
      `/v1/tenant/engagements/${engagement}/ubl-uploads`,
      ubl,
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, engagement, 1);
    // Bu mükellef için eşik 50 TL (mükellef katmanı).
    res = await api.put(
      `/v1/tenant/engagements/${engagement}/warning-rules/VAT_PAYABLE_THRESHOLD`,
      { params: { limit: "50" } },
    );
    expect(res.status(), await res.text()).toBe(200);

    // 3) Nöbetçi kapalıyken uyarı üretmez.
    res = await put({ watch_enabled: false });
    expect(res.status(), await res.text()).toBe(200);
    expect((await res.json()).data.watch_enabled).toBe(false);
    res = await api.post(`${vat}/watch`, {});
    expect(res.status(), await res.text()).toBe(409);
    expect(await res.text()).toContain("VAT_WATCH_DISABLED");
    const quiet = await api.json<{ items: WatchEvent[] }>(
      await api.get(`${vat}/watch-events`),
    );
    expect(
      quiet.items.filter((e) => e.event === "vat.threshold_exceeded"),
    ).toHaveLength(0);

    // 4) Açınca "KDV eşiği aşıldı" bir kez oluşur.
    res = await put({ watch_enabled: true });
    expect(res.status(), await res.text()).toBe(200);
    let items: WatchEvent[] = [];
    await expect
      .poll(
        async () => {
          const r = await api.post(`${vat}/watch`, {});
          items = ((await r.json()).data?.items ?? []) as WatchEvent[];
          return items.some((e) => e.event === "vat.threshold_exceeded");
        },
        { timeout: 60_000 },
      )
      .toBe(true);
    const hit = items.find((e) => e.event === "vat.threshold_exceeded")!;
    expect(hit.payload.limit).toBe("50.00");
    expect(Number(hit.payload.payable)).toBeGreaterThanOrEqual(50);

    // 5) Ofis geneli son uyarılar: mükellef adı, olay, tutar.
    const office = await api.json<{ items: OfficeEvent[] }>(
      await api.get("/v1/tenant/vat-forecast/watch-events?limit=50"),
    );
    const row = office.items.find(
      (e) =>
        e.engagement_uuid === engagement &&
        e.event === "vat.threshold_exceeded",
    );
    expect(row, JSON.stringify(office.items.slice(0, 3))).toBeTruthy();
    expect(row!.taxpayer_title).toBe(taxpayer.title);
    expect(row!.taxpayer_uuid).toBe(who.taxpayerUuid);
    const one = await api.json<{ items: OfficeEvent[] }>(
      await api.get("/v1/tenant/vat-forecast/watch-events?limit=1"),
    );
    expect(one.items).toHaveLength(1);
    res = await api.get("/v1/tenant/vat-forecast/watch-events?limit=abc");
    expect(res.status()).toBe(400);

    // 6) Ekran: ayarlarda eşik, kanal ve son uyarılar Türkçe görünür.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/settings"));
    const box = page.getByTestId("vat-watch-settings");
    await expect(box).toContainText("KDV nöbetçisi");
    await expect(box.getByRole("switch", { name: "KDV nöbetçisi" })).toBeChecked();
    const limit = box.getByLabel("Ödenecek KDV şu tutarı geçerse uyar");
    await expect(limit).toHaveValue("12345,67");
    await expect(box.locator("#vat-watch-channel")).toContainText(
      "Sorumluya e-posta da gönder",
    );
    const alerts = page.getByTestId("vat-recent-alerts");
    await expect(alerts).toContainText("Son KDV uyarıları");
    const alertRow = alerts
      .getByTestId("vat-alert-row")
      .filter({ hasText: taxpayer.title })
      .first();
    await expect(alertRow).toContainText("KDV eşiği aşıldı");
    await expect(
      alertRow.getByRole("link", { name: taxpayer.title }),
    ).toHaveAttribute("href", new RegExp(`/engagements/${engagement}/report$`));
    await expectNoRawText(box);

    // Eşik ekrandan değiştirilir ve kaydedilir.
    await limit.fill("15.000");
    await box.getByRole("button", { name: "Eşiği kaydet" }).click();
    await expect(page.getByText("KDV eşiği kaydedildi.")).toBeVisible();
    await expect
      .poll(
        async () =>
          (
            await api.json<Tracking>(
              await api.get("/v1/tenant/vat-tracking-settings"),
            )
          ).payable_limit,
      )
      .toBe("15000.00");

    // Bağlantı mükellefin rapor sayfasını açar.
    await alertRow.getByRole("link", { name: taxpayer.title }).click();
    await expect(page).toHaveURL(
      new RegExp(`/engagements/${engagement}/report`),
    );
  } finally {
    if (engagement) {
      await api.delete(
        `/v1/tenant/engagements/${engagement}/warning-rules/VAT_PAYABLE_THRESHOLD`,
      );
    }
    const back = await put({
      watch_enabled: settings.watch_enabled,
      watch_email: settings.watch_email,
      payable_limit: settings.payable_limit,
    });
    expect(back.status(), await back.text()).toBe(200);
    await api.dispose();
  }
});
