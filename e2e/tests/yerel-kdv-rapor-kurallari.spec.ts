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
 * KDV raporu günleri (#780 B, Oğuz'un isteği): rapor ayın 29'una sabit
 * değil. Ofis birden çok gün tanımlar (15'i ara rapor, ay sonundan 3 gün
 * önce…); her satırda kime (yalnız ofis / ofis onayıyla / doğrudan
 * mükellefe) ve hangi mükellefler (tümü / seçili / yalnız ödemesi çıkanlar)
 * seçilir. Her çalışma mükellef başına bir "KDV raporu hazır"
 * (vat.report_ready) olayı üretir; K2'nin otomasyonları bu olaya bağlanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve panel kullanıcısı.
 * Kurallar ofis geneli olduğundan "seçili mükellefler" yalnız bu mükellefi
 * kapsar; test sonunda ofisin eski kuralları (ya da varsayılanı) geri gelir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Rapor Kural ${run} A.S.` };

type Rule = {
  uuid?: string;
  trigger_kind: "day" | "before_month_end";
  day: number | null;
  days_before: number | null;
  audience: string;
  scope: string;
  group_ref: string | null;
  engagement_uuids: string[];
  label: string | null;
  enabled: boolean;
  next_run_on?: string;
};
type Rules = { items: Rule[]; default: boolean };
type RunResult = {
  taxpayers: number;
  raised: number;
  payable: number;
  notified: number;
  skipped: number;
};
type WatchEvent = {
  event: string;
  period: string;
  payload: Record<string, unknown>;
};
type Tracking = {
  report_enabled: boolean;
  report_day: number;
  watch_enabled: boolean;
};

/** Sunucunun döndürdüğü kuralı PUT gövdesine çevirir (yalnız yazılabilir alanlar). */
function writable(r: Rule): Rule {
  return {
    trigger_kind: r.trigger_kind,
    day: r.day,
    days_before: r.days_before,
    audience: r.audience,
    scope: r.scope,
    group_ref: r.group_ref,
    engagement_uuids: r.engagement_uuids ?? [],
    label: r.label,
    enabled: r.enabled,
  };
}

test("KDV raporu birden çok günde, seçilen kitleye çalışır ve rapor hazır olayı üretir (#780)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(
    browser,
    api,
    taxpayer,
    "kdvkural",
  );
  const rulesPath = "/v1/tenant/vat-report-rules";
  const settings = await api.json<Tracking>(
    await api.get("/v1/tenant/vat-tracking-settings"),
  );
  const before = await api.json<Rules>(await api.get(rulesPath));
  try {
    // Rapor açık olsun.
    const on = await api.put("/v1/tenant/vat-tracking-settings", {
      report_enabled: true,
      report_day: settings.report_day,
      watch_enabled: settings.watch_enabled,
    });
    expect(on.status(), await on.text()).toBe(200);

    // 1) Kural yokken tek bir varsayılan kural görünür: rapor günü, yalnız ofis, tümü.
    const reset = await api.put(rulesPath, { items: [] });
    expect(reset.status(), await reset.text()).toBe(200);
    const def = (await reset.json()).data as Rules;
    expect(def.default).toBe(true);
    expect(def.items).toHaveLength(1);
    expect(def.items[0]).toMatchObject({
      trigger_kind: "day",
      day: settings.report_day,
      audience: "office",
      scope: "all",
    });

    // 2) Geçersiz kurallar reddedilir.
    for (const bad of [
      { trigger_kind: "day", day: 32, audience: "office", scope: "all" },
      {
        trigger_kind: "before_month_end",
        days_before: 28,
        audience: "office",
        scope: "all",
      },
      { trigger_kind: "day", day: 15, audience: "herkes", scope: "all" },
      {
        trigger_kind: "day",
        day: 15,
        audience: "office",
        scope: "selected",
        engagement_uuids: [],
      },
    ]) {
      const res = await api.put(rulesPath, {
        items: [{ enabled: true, ...bad }],
      });
      expect(res.status(), JSON.stringify(bad)).toBe(400);
    }

    // 3) Üç kural: 15'i ara rapor doğrudan mükellefe (yalnız bu mükellef),
    //    ay sonundan 3 gün önce yalnız ödemesi çıkanlar, 29'u ofise.
    const items: Rule[] = [
      {
        trigger_kind: "day",
        day: 15,
        days_before: null,
        audience: "taxpayer_direct",
        scope: "selected",
        group_ref: null,
        engagement_uuids: [who.engagement],
        label: `Ara rapor ${run}`,
        enabled: true,
      },
      {
        trigger_kind: "before_month_end",
        day: null,
        days_before: 3,
        audience: "taxpayer_approval",
        scope: "payable_only",
        group_ref: null,
        engagement_uuids: [],
        label: null,
        enabled: true,
      },
      {
        trigger_kind: "day",
        day: 29,
        days_before: null,
        audience: "office",
        scope: "all",
        group_ref: null,
        engagement_uuids: [],
        label: null,
        enabled: true,
      },
    ];
    const saved = await api.put(rulesPath, { items });
    expect(saved.status(), await saved.text()).toBe(200);
    const list = (await saved.json()).data as Rules;
    expect(list.default).toBe(false);
    expect(list.items).toHaveLength(3);
    const mid = list.items.find((r) => r.day === 15)!;
    expect(mid.uuid).toBeTruthy();
    expect(mid.engagement_uuids).toEqual([who.engagement]);
    expect(mid.next_run_on).toMatch(/^\d{4}-\d{2}-15$/);
    const end = list.items.find((r) => r.trigger_kind === "before_month_end")!;
    expect(end.days_before).toBe(3);

    // 4) "Şimdi çalıştır": ara rapor bu mükellefe gider, olay bir kez üretilir.
    let res = await api.post(`${rulesPath}/${mid.uuid}/run`, {});
    expect(res.status(), await res.text()).toBe(200);
    let ran = (await res.json()).data as RunResult;
    expect(ran.taxpayers).toBe(1);
    expect(ran.raised).toBe(1);
    expect(ran.notified).toBe(1);
    res = await api.post(`${rulesPath}/${mid.uuid}/run`, {});
    ran = (await res.json()).data as RunResult;
    expect(ran.raised).toBe(0);

    // Faturası olmayan mükellefte ödeme çıkmaz; "yalnız ödemesi çıkanlar" onu atlar.
    res = await api.post(`${rulesPath}/${end.uuid}/run`, {});
    expect(res.status(), await res.text()).toBe(200);
    const events = await api.json<{ items: WatchEvent[] }>(
      await api.get(
        `/v1/tenant/engagements/${who.engagement}/vat-forecast/watch-events`,
      ),
    );
    const ready = events.items.filter((e) => e.event === "vat.report_ready");
    expect(ready).toHaveLength(1);
    expect(ready[0].payload).toMatchObject({
      rule_uuid: mid.uuid,
      trigger: "day:15",
      audience: "taxpayer_direct",
      scope: "selected",
      taxpayer_title: taxpayer.title,
      engagement_uuid: who.engagement,
      period: ready[0].period,
    });
    expect(ready[0].payload.due_date).toBeTruthy();
    expect(ready[0].payload.payable).toBeDefined();

    // Mükellefe bildirim gitti (doğrudan mükellefe).
    const notices = await api.json<{ items: { period: string }[] }>(
      await api.get(
        `/v1/tenant/engagements/${who.engagement}/vat-forecast/notices`,
      ),
    );
    expect(notices.items.some((n) => n.period === ready[0].period)).toBe(true);

    // Bilinmeyen kural.
    res = await api.post(
      `${rulesPath}/00000000-0000-0000-0000-000000000000/run`,
      {},
    );
    expect(res.status()).toBe(404);

    // 5) Ekran: ayarlarda üç kural satırı Türkçe görünür.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/settings"));
    const box = page.getByTestId("vat-report-rules");
    await expect(box).toContainText("KDV raporu günleri");
    await expect(box.getByTestId("vat-report-rule")).toHaveCount(3);
    await expect(
      box.getByRole("textbox", { name: "Ad (örn. Ara rapor)" }).first(),
    ).toHaveValue(`Ara rapor ${run}`);
    await expect(box).toContainText("Doğrudan mükellefe");
    await expect(box).toContainText("Yalnız ödemesi çıkanlar");
    await expect(box).toContainText("Ay sonundan 3 gün önce");
    await expectNoRawText(box);
  } finally {
    const back = await api.put(rulesPath, {
      items: before.default ? [] : before.items.map(writable),
    });
    expect(back.status(), await back.text()).toBe(200);
    const restored = await api.put("/v1/tenant/vat-tracking-settings", {
      report_enabled: settings.report_enabled,
      report_day: settings.report_day,
      watch_enabled: settings.watch_enabled,
    });
    expect(restored.status(), await restored.text()).toBe(200);
    await api.dispose();
  }
});
