import { randomUUID } from "node:crypto";

import { expect, test, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Hata veren Luca aktarımı → "Yeniden dene" → programa aktarıldı (#574).
 *
 * `bulut-donem-aktarim.spec.ts` aktarımın sorunsuz yolunu, `whatsapp-aktarim`
 * hatalı satırın göründüğünü dener; düğmeye basılıp işin bitirildiği yol
 * yoktu. Bu dosya:
 *
 *   onaylı fiş (API) → "Çıktı al" → "Masaüstü ajanına gönder" → ajan alır,
 *   klasör bulunamaz (API, masaüstü uygulamasının yaptığı gibi) → satırda
 *   "Hata verdi" + neden, üstte "1 aktarım hata verdi" → "Yeniden dene" →
 *   aynı çıktı için yeni görev "Ajan bekleniyor", uyarı kalkar, eski satırda
 *   düğme kalmaz (API de ikinci kez açmayı reddeder) → ajan bu kez bırakır →
 *   müşavir "Aktarıldı" der → "Programa aktarıldı".
 *
 * Her adımın sonucu API'den de doğrulanır. Veri: her koşuda yeni mükellef ve
 * benzersiz adlı masaüstü ajanı (sonunda kaldırılır).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Aktarım Yeniden ${run} Ltd. Şti.` };
const PERIOD = "2026-10";
const PERIOD_YEAR = 2026;
const PERIOD_MONTH = "Eki"; // MonthPicker'ın kısa ay adı
const SALE = `E2E yeniden dene ${run}`;

type Transfer = {
  uuid: string;
  batch_uuid: string;
  period?: string;
  status: string;
  error_code?: string;
  files: { name: string }[];
};

let api: Api;
let office: Page;
let engagement = "";
let agent = "";
let failedUuid = "";

const base = () => `/v1/tenant/engagements/${engagement}`;

async function transfers() {
  return (await api.json<{ items: Transfer[] }>(await api.get(`${base()}/journal-transfers?limit=20`))).items;
}

/** "Programa aktarım" paneli ve satırları. */
function panel() {
  return office
    .locator("section")
    .filter({ has: office.getByRole("heading", { name: "Programa aktarım", exact: true }) });
}
const rowWith = (text: string | RegExp) => panel().getByRole("row").filter({ hasText: text });

async function openJournals() {
  await office.goto(officePath(`/journals?taxpayer=${engagement}`));
  const picker = office.locator("#journals-period");
  await expect(picker).toBeVisible();
  if (!/Ekim 2026/i.test(await picker.innerText())) {
    await picker.click();
    const prev = office.getByRole("button", { name: "Önceki yıl" });
    const next = office.getByRole("button", { name: "Sonraki yıl" });
    const year = prev.locator("xpath=following-sibling::span[1]");
    for (let i = 0; i < 20; i++) {
      const shown = Number((await year.innerText()).trim());
      if (shown === PERIOD_YEAR) break;
      await (shown > PERIOD_YEAR ? prev : next).click();
    }
    await expect(year).toHaveText(String(PERIOD_YEAR));
    await office.getByRole("button", { name: PERIOD_MONTH, exact: true }).click();
  }
  await expect(picker).toContainText(/Ekim 2026/i);
  await expect(office.getByRole("heading", { name: "Programa aktarım", exact: true })).toBeVisible();
}

async function claim(transfer: string) {
  const claimed = await api.json<Transfer>(
    await api.post(`${base()}/journal-transfers/${transfer}/claim`, { agent_uuid: agent }),
  );
  expect(claimed.status).toBe("claimed");
  return claimed;
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "yeniden");

  const draft = await api.json<{ uuid: string; period: string }>(
    await api.post(`${base()}/journal-drafts`, {
      entry_type: "mahsup",
      entry_date: `${PERIOD}-15T00:00:00Z`,
      description: SALE,
      lines: [
        { account: "120.01.001", description: "Yıldız Market", debit: "600.00", credit: "0" },
        { account: "600.01", description: "Satış", debit: "0", credit: "500.00" },
        { account: "391.01", description: "Hesaplanan KDV", debit: "0", credit: "100.00" },
      ],
    }),
  );
  expect(draft.period).toBe(PERIOD);
  const approved = await api.json<{ status: string }>(await api.post(`${base()}/journal-drafts/${draft.uuid}/approve`));
  expect(approved.status).toBe("approved");

  const registered = await api.json<{ uuid: string }>(
    await api.post("/v1/tenant/agents", {
      machine_id: `e2e-yeniden-${randomUUID()}`,
      name: `E2E-Yeniden-${run}`,
      platform: "windows",
      version: "0.1.0",
    }),
  );
  agent = registered.uuid;

  office = await (await browser.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  await uiLogin(office, DEMO.owner);
});

test.afterAll(async () => {
  if (agent) await api?.delete(`/v1/tenant/agents/${agent}`);
  await office?.context().close();
  await api?.dispose();
});

test("ajan klasörü bulamaz: satırda neden ve üstte uyarı görünür", async () => {
  await openJournals();
  await office.locator('select:has(option[value="luca"])').selectOption("luca");
  await office.getByRole("button", { name: "Çıktı al", exact: true }).last().click();
  await expect(office.getByText("1 fiş aktarıldı.")).toBeVisible();

  await office.getByRole("button", { name: "Masaüstü ajanına gönder" }).click();
  await expect(rowWith("Ajan bekleniyor")).toHaveCount(1);
  const [queued] = await transfers();
  expect(queued).toMatchObject({ period: PERIOD, status: "queued" });
  failedUuid = queued.uuid;

  await claim(failedUuid);
  const failed = await api.json<Transfer>(
    await api.post(`${base()}/journal-transfers/${failedUuid}/delivery`, {
      agent_uuid: agent,
      outcome: "failed",
      error_code: "FOLDER_MISSING",
      error_message: "C:\\Luca\\Aktarim yok",
    }),
  );
  expect(failed).toMatchObject({ status: "failed", error_code: "FOLDER_MISSING" });

  await office.reload();
  const row = rowWith("Hata verdi");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Aktarım klasörü bulunamadı");
  await expect(panel().getByText("1 aktarım hata verdi")).toBeVisible();
});

test("'Yeniden dene' aynı çıktıyı yeniden kuyruğa alır; eski satır bir daha denenemez", async () => {
  await rowWith("Hata verdi").getByRole("button", { name: "Yeniden dene" }).click();
  await expect(rowWith("Ajan bekleniyor")).toHaveCount(1);
  // Yerine yenisi açıldı: uyarı kalkar, eski satırda düğme kalmaz.
  await expect(panel().getByText(/aktarım hata verdi/)).toHaveCount(0);
  await expect(rowWith("Hata verdi")).toHaveCount(1);
  await expect(rowWith("Hata verdi").getByRole("button", { name: "Yeniden dene" })).toHaveCount(0);

  const list = await transfers();
  expect(list).toHaveLength(2);
  const old = list.find((t) => t.uuid === failedUuid);
  const fresh = list.find((t) => t.uuid !== failedUuid);
  expect(old?.status).toBe("failed");
  expect(fresh).toMatchObject({ status: "queued", batch_uuid: old?.batch_uuid, period: PERIOD });

  // Açık görev varken eskisi ikinci kez denenemez (çift aktarım olmaz).
  const again = await api.post(`${base()}/journal-transfers/${failedUuid}/retry`);
  expect(again.status()).toBe(409);
  expect(await transfers()).toHaveLength(2);
});

test("ikinci denemede dosya klasöre bırakılır, müşavir aktarıldı der", async () => {
  const fresh = (await transfers()).find((t) => t.uuid !== failedUuid)!;
  await claim(fresh.uuid);
  const file = fresh.files[0]?.name;
  expect(file, "aktarımın dosyası").toMatch(/^luca_fis_2026-10\.xlsx$/);
  const delivered = await api.json<Transfer>(
    await api.post(`${base()}/journal-transfers/${fresh.uuid}/delivery`, {
      agent_uuid: agent,
      outcome: "delivered",
      folder_label: "Luca aktarım klasörü",
      files: [{ name: file, sha256: "b".repeat(64), size: 4096 }],
    }),
  );
  expect(delivered.status).toBe("delivered");

  await office.reload();
  const row = rowWith("Klasörde, içe aktarılmayı bekliyor");
  await row.getByRole("button", { name: "Aktarıldı", exact: true }).click();
  const dialog = office.getByRole("alertdialog").or(office.getByRole("dialog"));
  await expect(dialog).toContainText("Programa aktarıldı mı?");
  await dialog.getByRole("button", { name: "Aktarıldı", exact: true }).click();
  await expect(rowWith("Programa aktarıldı")).toHaveCount(1);
  await expect(panel().getByText(/aktarım hata verdi/)).toHaveCount(0);

  const statuses = (await transfers()).map((t) => t.status).sort();
  expect(statuses).toEqual(["failed", "imported"]);
  const drafts = await api.json<{ items: { description: string; status: string }[] }>(
    await api.get(`${base()}/journal-drafts?period=${PERIOD}&limit=50`),
  );
  expect(drafts.items.find((d) => d.description === SALE)?.status).toBe("exported");
});
