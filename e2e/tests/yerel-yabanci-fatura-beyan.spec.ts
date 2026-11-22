import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * KDV2 ve muhtasar beyanına yabancı hizmet faturaları (#780 C): API ile
 * yurt dışı yazılım lisansı faturası eklenir (100 USD, kur 34,50, %15
 * stopaj, brütleştirme) ve onaylanır. KDV2 önizlemesinde "sorumlu
 * sıfatıyla KDV (yurt dışı hizmet)" satırı (matrah 4.058,93; KDV2 811,79)
 * ve toplamı, muhtasar önizlemesinde "yurt dışı hizmet stopajı (KVK 30)"
 * satırı (matrah 4.058,93; stopaj 608,84) çıkar. Onaysız fatura beyana
 * girmez, kontrol listesinde uyarı olur. Ekranda satır ve "Yabancı
 * faturaları aç" bağlantısı görünür.
 *
 * Veri: her koşuda yeni mükellef; faturalar finally içinde silinir. Bölüm
 * kapalıysa test açar ve sonunda eski hâline döndürür.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Yabanci Beyan ${run} Ltd. Sti.` };

type Item = { uuid: string; status: string; kdv2_try: string | null };
type Check = { code: string; count: number };
type KDV2Row = { code: string; source?: string; invoice_count: number; base: string; vat: string; withheld: string };
type Withholding = { kind: string; rate: string; count: number; base: string; tax: string };
type Preview = {
  checklist: Check[];
  kdv2?: { rows: KDV2Row[]; totals: { base: string; vat: string; payable: string } };
  muhsgk?: { withholdings: Withholding[]; totals: { income_tax: string } };
};
type Kinds = { enabled: boolean };

/** Bu ay (İstanbul) YYYY-MM. */
const period = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Istanbul" }).format(new Date()).slice(0, 7);

test("onaylı yabancı fatura KDV2 ve muhtasar önizlemesine girer (#780)", async ({ browser, page }) => {
  test.setTimeout(300_000);
  const office = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, office, taxpayer, "yabancibeyan");
  const base = `/v1/tenant/engagements/${engagement}/foreign-invoices`;
  const kinds = await office.json<Kinds>(await office.get("/v1/tenant/foreign-service-kinds"));
  const created: string[] = [];
  try {
    if (!kinds.enabled) {
      const on = await office.put("/v1/tenant/foreign-service-kinds", { enabled: true, items: [] });
      expect(on.status(), await on.text()).toBe(200);
    }
    // 1) Lisans faturası: kur girilir, onaylanır.
    const lic = await office.post(base, {
      period,
      vendor: `Lisans Yazilim GmbH ${run}`,
      country: "DE",
      amount: "100.00",
      currency: "USD",
      exchange_rate: "34.50",
      service_kind: "yazilim_lisans",
      treatment: "kdv2_stopaj",
      stopaj_rate: "15",
      gross_up: true,
      vat_rate: "20",
    });
    expect(lic.status(), await lic.text()).toBe(201);
    const licItem = (await lic.json()).data as Item;
    created.push(licItem.uuid);
    expect(licItem.kdv2_try).toBe("811.79");
    const ok = await office.post(`${base}/${licItem.uuid}/confirm`);
    expect(ok.status(), await ok.text()).toBe(200);

    // 2) Onaysız reklam faturası: beyana girmez, uyarı olur.
    const ads = await office.post(base, {
      period,
      vendor: `Google Ireland Ltd. ${run}`,
      country: "IE",
      amount: "50.00",
      currency: "USD",
      exchange_rate: "34.50",
      service_kind: "reklam",
    });
    expect(ads.status(), await ads.text()).toBe(201);
    created.push(((await ads.json()).data as Item).uuid);

    const preview = async (kind: string) =>
      office.json<Preview>(await office.get(`/v1/tenant/engagements/${engagement}/tax-returns/${kind}/${period}`));

    // 3) KDV2: yurt dışı hizmet satırı ve toplam.
    const k = await preview("kdv2");
    const rows = (k.kdv2?.rows ?? []).filter((r) => r.source === "foreign_service");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ code: "yazilim_lisans", invoice_count: 1, base: "4058.93", vat: "811.79", withheld: "811.79" });
    expect(k.kdv2?.totals).toMatchObject({ base: "4058.93", payable: "811.79" });
    expect(k.checklist.find((c) => c.code === "FOREIGN_UNCONFIRMED")?.count).toBe(1);
    expect(k.checklist.some((c) => c.code === "FOREIGN_SERVICE_KDV2")).toBe(true);

    // 4) Muhtasar: yurt dışı hizmet stopajı (KVK 30).
    const m = await preview("muhsgk");
    const w = (m.muhsgk?.withholdings ?? []).filter((x) => x.kind === "foreign_service");
    expect(w).toHaveLength(1);
    expect(w[0]).toMatchObject({ rate: "15", count: 1, base: "4058.93", tax: "608.84" });
    expect(m.muhsgk?.totals.income_tax).toBe("608.84");

    // 5) Ekran: Türkçe satır ve Yabancı faturalar bağlantısı.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/tax-returns?kind=kdv2&period=${period}`));
    await expect(page.getByTestId("kdv2-foreign-row")).toHaveText(
      "Sorumlu sıfatıyla KDV (yurt dışı hizmet) — Yazılım / lisans",
    );
    const link = page.getByTestId("tax-return-foreign-link");
    await expect(link).toHaveText("Yabancı faturaları aç");
    await expect(link).toHaveAttribute("href", new RegExp(`/engagements/${engagement}/foreign-invoices`));

    await page.goto(officePath(`/engagements/${engagement}/tax-returns?kind=muhsgk&period=${period}`));
    await expect(page.getByText("Yurt dışı hizmet stopajı (KVK 30)", { exact: true })).toBeVisible();
  } finally {
    for (const id of created) {
      await office.post(`${base}/${id}/unconfirm`);
      await office.delete(`${base}/${id}`);
    }
    if (!kinds.enabled) {
      await office.put("/v1/tenant/foreign-service-kinds", { enabled: false, items: [] });
    }
    await office.dispose();
  }
});
