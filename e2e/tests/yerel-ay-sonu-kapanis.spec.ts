import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Ay sonu kapanış zinciri (#780): Sıradaki iş zinciri Toplandı → Eşleşti →
 * Fişler onaylı → Luca'ya aktarıldı → Luca'dan alındı → KDV bildirildi →
 * Rapor adımlarını sayar; ofis sayaçları sıradaki adıma göre mükellef
 * sayısını verir ve sayaca tıklayınca liste o adıma süzülür. Komuta
 * merkezinden bu ekrana bağlantı vardır.
 *
 * Veri: demo ofisi, Ekim 2026 (seedCloseChain): Selin'in Luca mizanı
 * yüklü (sıradaki adım KDV bildirimi), Kaya'nın onay bekleyen masrafı var,
 * Deniz'in çalışanı var ama Luca bordrosu yok, Av. Elif Arslan serbest
 * meslek (mizan beklenmez). Yalnız okur ve gezinir.
 */
type Step = { key: string; status: string; count: number; action: string | null };
type Row = { taxpayer_title: string; next_step: string | null; steps: Step[] };
type Page = {
  items: Row[];
  total: number;
  summary?: { steps: { key: string; count: number }[]; waiting: number; closed: number; scanned: number };
};

const KEYS = ["documents", "bank", "review", "journals", "transfer", "luca_import", "vat", "report"];

test("Ay sonu kapanış: yeni adımlar, ofis sayaçları ve adım süzgeci (#780)", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const res = await api.get("/v1/tenant/copilot/workflow?period=2026-10&summary=true&limit=200");
  expect(res.status()).toBe(200);
  const data = ((await res.json()) as { data: Page }).data;
  const step = (title: string, key: string) => {
    const row = data.items.find((r) => r.taxpayer_title.startsWith(title));
    expect(row, title).toBeTruthy();
    expect(row!.steps.map((s) => s.key)).toEqual(KEYS);
    return row!.steps.find((s) => s.key === key)!;
  };
  // Selin: Luca mizanı yüklü, KDV bildirilmedi.
  expect(step(DEMO.selin.title, "luca_import").status).not.toBe("todo");
  expect(step(DEMO.selin.title, "vat")).toMatchObject({ status: "todo", action: "open_vat" });
  // Kaya: onay bekleyen masraf Fişler adımını açık tutar.
  expect(step("Hüseyin Kaya", "journals")).toMatchObject({ status: "todo", action: "open_expenses" });
  // Deniz: çalışanı var, Luca bordrosu yok.
  expect(step("Deniz Aydın", "luca_import")).toMatchObject({ status: "todo", action: "open_payroll" });
  // Serbest meslek: mizan yok, bordro yok → adım yok; KDV bildirildi.
  expect(step("Av. Elif Arslan", "luca_import").status).toBe("na");
  expect(step("Av. Elif Arslan", "vat").status).toBe("done");

  // Sayaçlar: her adım bir kez, toplam taranan mükellef sayısı.
  const summary = data.summary!;
  expect(summary.steps.map((s) => s.key)).toEqual(KEYS);
  const counted = summary.steps.reduce((n, s) => n + s.count, 0) + summary.waiting + summary.closed;
  expect(counted).toBe(summary.scanned);

  // Adım süzgeci sunucuda: yalnız sıradaki adımı KDV olanlar.
  const vat = ((await (await api.get("/v1/tenant/copilot/workflow?period=2026-10&step=vat")).json()) as { data: Page })
    .data;
  expect(vat.items.length).toBeGreaterThan(0);
  expect(vat.items.every((r) => r.next_step === "vat")).toBe(true);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  // Arayüz: dönemi Ekim'e sabitle, Komuta merkezinden Sıradaki iş'e geç.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${atlas}/bank?period=2026-10`));
  await expect(page.getByRole("region", { name: "Sıradaki iş · Ekim 2026" })).toBeVisible();
  await page.goto(officePath("/command"));
  await page.getByTestId("command-close-chain-link").click();
  await expect(page).toHaveURL(/\/copilot/);
  await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();

  const counters = page.getByTestId("copilot-step-counters");
  await expect(counters).toBeVisible();
  await counters.locator('[data-step="vat"]').click();
  await expect(page).toHaveURL(/adim=vat/);
  await expect(page.getByRole("row").filter({ hasText: vat.items[0].taxpayer_title })).toBeVisible();
  // Süzgeç işin adını söyler: bitmemiş adım "KDV bildirildi" demez.
  await expect(page.getByRole("button", { name: /Yalnız: KDV bildirimi/ })).toBeVisible();
});
