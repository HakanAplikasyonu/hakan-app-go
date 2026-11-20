import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Bağlantı hataları Komuta Merkezi'nde (#669 K).
 *
 * - Hata veren bir entegrasyon bağlantısı (`source_connections`)
 *   `INT_CONNECTION_FAILED` uyarısını açar (önem: uyarı; dönem onayını
 *   engellemez): varlık
 *   `source_connection`, dönem içinde bulunulan ay (Europe/Istanbul).
 *   Kontroller yeniden çalışınca ikinci uyarı açılmaz.
 * - Uyarı mükellefin Uyarılar sekmesinde görünür; "Bağlantıya git"
 *   Bağlantılar ekranını açar.
 * - Komuta Merkezi'nde mükellefin bu ayki hücresi uyarı listesine
 *   götürür; oradan da "Bağlantıya git" Bağlantılar ekranını açar.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Bağlantıyı hata durumuna
 * sokmak için arayüzdeki akış API ile izlenir: Vakıfbank web servisi
 * bağlantısı (test ortamı) açılır, uydurma servis kullanıcısı girilir. Banka
 * kimliği reddetse de (auth_failed) bankaya ulaşılamasa da (bank_error)
 * bağlantı `last_error` ile kalır; ikisi de bağlantı hatasıdır. Uyarılar
 * zamanlanmış taramayı beklemeden kontrol çalıştırmayla (`controls/run`)
 * üretilir. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Baglanti Uyari ${run} A.Ş.` };
const CODE = "INT_CONNECTION_FAILED";
const TEXT = "Vakıfbank bağlantısı hata veriyor, veri gelmiyor";

/** Uyarının yazıldığı ay: İstanbul saatine göre içinde bulunulan ay. */
function currentPeriod(): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(new Date());
  const get = (t: string) => parts.find((p) => p.type === t)!.value;
  return `${get("year")}-${get("month")}`;
}

/** Komuta Merkezi kolon başlığı (command-center.tsx monthLabel, tr). */
function monthLabel(period: string): string {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat("tr-TR", { month: "short", year: "2-digit", timeZone: "UTC" }).format(
    new Date(Date.UTC(y, m - 1, 1)),
  );
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

type BankConnection = { uuid: string; status: string; setup_state: string; error?: { reason: string } };
type WarningList = {
  items: { code: string; severity: string; period: string | null; entity_type: string; params: Record<string, unknown> }[];
  total: number;
};

test.describe("Bağlantı hataları Komuta Merkezi'nde (#669 K)", () => {
  let engagement = "";
  let api: Api;
  const period = currentPeriod();
  const base = () => `/v1/tenant/engagements/${engagement}`;
  const openWarnings = async () =>
    api.json<WarningList>(await api.get(`${base()}/warnings?status=open&code=${CODE}&limit=5`));

  test.beforeAll(async ({ browser }) => {
    // Banka çağrısı (en çok 2 dk) dahil.
    test.setTimeout(360_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "baglanti");
    const conn = await api.json<BankConnection>(
      await api.post(`${base()}/bank-connections`, { bank_id: "vakifbank", environment: "test", ibans: [] }),
    );
    // Kimlik doğrulaması bankayı çağırır: yanıt sunucunun yazma süresini
    // (30 sn) aşarsa istek kopar ama sonuç yine bağlantıya yazılır; durum
    // GET ile beklenir.
    await api
      .put(`${base()}/bank-connections/${conn.uuid}/credentials`, {
        values: { customer_no: "123456", username: `e2e-${run}`, password: `yanlis-${run}` },
      })
      .catch(() => undefined);
    const current = async () =>
      api.json<BankConnection>(await api.get(`${base()}/bank-connections/${conn.uuid}`));
    await expect
      .poll(async () => (await current()).error?.reason ?? "", { timeout: 150_000, intervals: [2_000] })
      .not.toBe("");
    const failed = await current();
    expect(failed.status, `uydurma kimlikle bağlantı etkin olmamalı: ${JSON.stringify(failed)}`).not.toBe("active");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("hata veren bağlantı uyarı açar; yeniden çalışınca tek uyarı", async () => {
    await api.json(await api.post(`${base()}/controls/run`, { period }));
    const list = await openWarnings();
    expect(list.total).toBe(1);
    const w = list.items[0];
    expect(w.severity).toBe("warning");
    expect(w.entity_type).toBe("source_connection");
    expect(w.period).toBe(period);
    expect(w.params.source).toBe("Vakıfbank");
    expect(w.params.source_system).toBe("bank_vakifbank");

    await api.json(await api.post(`${base()}/controls/run`, { period }));
    expect((await openWarnings()).total).toBe(1);
  });

  test("mükellefin Uyarılar sekmesinde görünür; Bağlantıya git Bağlantılar'ı açar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/warnings`));
    await expect(page.getByText(TEXT).first()).toBeVisible({ timeout: 30_000 });
    const go = page.getByRole("link", { name: "Bağlantıya git" });
    await expect(go).toHaveCount(1);
    await go.click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/connections`));
    await expect(page.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
  });

  test("Komuta Merkezi'nde bu ayın hücresinden uyarıya, oradan Bağlantılar'a", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/command"));
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(taxpayer.title);
    const cell = page.getByRole("link", { name: new RegExp(`^${escape(taxpayer.title)} · ${escape(monthLabel(period))}:`) });
    await expect(cell).toHaveCount(1);
    await expect(cell).toHaveAttribute("data-target", "warnings");
    await cell.click();
    await page.waitForURL(
      (u) => u.pathname.endsWith(`/engagements/${engagement}/warnings`),
    );
    await expect(page.getByText(TEXT).first()).toBeVisible({ timeout: 30_000 });
    await page.getByRole("link", { name: "Bağlantıya git" }).first().click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/connections`));
  });
});
