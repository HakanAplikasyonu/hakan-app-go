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
 * KDV nöbetçisi (#780 B, toplantı örneği): mükellefe "bu ay ödeme
 * çıkmayacak" denir; sonra SSK'ya bir satış faturası kesilir ve ödeme
 * çıkar. Nöbetçi (her sabah ya da "Şimdi kontrol et") "KDV durumu değişti"
 * olayını bir kez üretir; rapor kartında uyarı ve "Mükellefe yeni bilgi
 * gönder" çıkar. Ofis ayardan kapatınca kontrol yapılmaz.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve kullanıcısı. Satış
 * faturası Atlas'ın temel satış UBL'inden türetilir, tarihi bugüne çekilir
 * (bu ayın KDV'si). Ayar testi sonunda eski hâline döner.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Nobetci ${run} A.S.` };

type WatchEvent = {
  uuid: string;
  period: string;
  event: string;
  payload: Record<string, unknown>;
};
type Watch = { raised: number; items: WatchEvent[] };
type Notice = { uuid: string; period: string; outcome: string };
type Tracking = {
  report_enabled: boolean;
  report_day: number;
  watch_enabled: boolean;
};

/** Bugün (İstanbul) YYYY-MM-DD. */
function today() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
  }).format(new Date());
}

test("mükellefe ödeme çıkmayacak denmişken satış faturası gelince nöbetçi durum değişti der (#780)", async ({
  browser,
  page,
}) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(
    browser,
    api,
    taxpayer,
    "kdvnobetci",
  );
  const vat = `/v1/tenant/engagements/${who.engagement}/vat-forecast`;
  const settings = await api.json<Tracking>(
    await api.get("/v1/tenant/vat-tracking-settings"),
  );
  try {
    // Nöbetçi açık olsun (varsayılan).
    const body = (watch: boolean) => ({
      report_enabled: settings.report_enabled,
      report_day: settings.report_day,
      watch_enabled: watch,
    });
    const on = await api.put("/v1/tenant/vat-tracking-settings", body(true));
    expect(on.status(), await on.text()).toBe(200);

    // 1) Ayın başında mükellefe "ödeme çıkmayacak" denir (faturası yok).
    const sent = await api.post(`${vat}/notices`, {
      message: `Bu ay odeme yok ${run}`,
    });
    expect(sent.status(), await sent.text()).toBe(201);
    const notice = (await sent.json()).data as Notice;
    expect(notice.outcome).toBe("no_payment");

    // Henüz değişen bir şey yok.
    let res = await api.post(`${vat}/watch`, {});
    expect(res.status(), await res.text()).toBe(200);
    let watch = (await res.json()).data as Watch;
    expect(watch.raised).toBe(0);

    // 2) SSK'ya satış faturası: bugünün tarihiyle, 72 TL KDV.
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
      `/v1/tenant/engagements/${who.engagement}/ubl-uploads`,
      ubl,
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
    await waitForUploads(api, who.engagement, 1);

    // 3) Nöbetçi: durum değişti, bir kez.
    await expect
      .poll(
        async () => {
          const r = await api.post(`${vat}/watch`, {});
          watch = (await r.json()).data as Watch;
          return watch.items.some((e) => e.event === "vat.status_changed");
        },
        { timeout: 60_000 },
      )
      .toBe(true);
    const changed = watch.items.find((e) => e.event === "vat.status_changed")!;
    expect(changed.period).toBe(notice.period);
    expect(changed.payload.notice_uuid).toBe(notice.uuid);
    expect(changed.payload.previous_outcome).toBe("no_payment");
    expect(changed.payload.outcome).toBe("payment");
    expect(changed.payload.taxpayer_title).toBe(taxpayer.title);
    expect(Number(changed.payload.payable)).toBeGreaterThan(0);
    expect(String(changed.payload.dedupe_key)).toContain(notice.uuid);

    res = await api.post(`${vat}/watch`, {});
    watch = (await res.json()).data as Watch;
    expect(watch.raised).toBe(0);
    expect(
      watch.items.filter((e) => e.event === "vat.status_changed"),
    ).toHaveLength(1);

    // 4) Ekran: rapor kartında uyarı ve yeniden gönderme düğmesi.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${who.engagement}/report`));
    const banner = page.getByTestId("vat-status-changed");
    await expect(banner).toContainText("KDV durumu değişti");
    await expect(banner).toContainText('"ödeme çıkmayacak" denmişti');
    await expect(banner).toContainText("KDV ödemesi çıkıyor");
    await expectNoRawText(banner);
    await banner
      .getByRole("button", { name: "Mükellefe yeni bilgi gönder" })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Gönder", exact: true }).click();
    await expect(dialog).toBeHidden();
    // Yeni bildirim "ödeme çıkacak" der; uyarı kalkar.
    await expect(banner).toHaveCount(0);
    await expect(page.getByTestId("vat-notice-card")).toContainText(
      "Ödeme çıkacak",
    );

    // 5) Ayardan kapatılınca kontrol yapılmaz.
    const off = await api.put("/v1/tenant/vat-tracking-settings", body(false));
    expect(off.status(), await off.text()).toBe(200);
    expect((await off.json()).data.watch_enabled).toBe(false);
    res = await api.post(`${vat}/watch`, {});
    expect(res.status(), await res.text()).toBe(409);
    expect(await res.text()).toContain("VAT_WATCH_DISABLED");
  } finally {
    const back = await api.put("/v1/tenant/vat-tracking-settings", {
      report_enabled: settings.report_enabled,
      report_day: settings.report_day,
      watch_enabled: settings.watch_enabled,
    });
    expect(back.status(), await back.text()).toBe(200);
    await api.dispose();
  }
});
