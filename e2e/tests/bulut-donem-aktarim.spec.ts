import { randomUUID } from "node:crypto";

import { expect, test, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Dönem aktarımı → programa aktarım → mizan durumu (#566, #574).
 *
 * `fis-onay-akisi.spec.ts` yalnız seçili fişi aktarır; aktarım panelindeki
 * mizan durumu ise yalnız dönem çıktısında görünür (seçili fiş aktarımında
 * dönem yok, sütun "—"). Bu dosya bir dönemin TAMAMINI Luca'ya aktarır:
 *
 *   iki onaylı fiş (API) → seçim yapmadan "Çıktı al" (dönemin tümü) →
 *   "Masaüstü ajanına gönder" → ajan alır ve klasöre bırakır (API, masaüstü
 *   uygulamasının yaptığı gibi) → müşavir "Aktarıldı" der → satırda
 *   "Mizanla karşılaştırılmadı" ve "Mizanla karşılaştır" → Mutabakat ekranı →
 *   Luca mizanı yüklenir (API) ve "Karşılaştır" → satırda "Luca mizanı
 *   kayıtlarla tutuyor" → farklı tutarlı ikinci mizan → "1 hesapta fark var"
 *   ve "Farkları gör" → Mutabakat'ta farklı hesap.
 *
 * Her adımın sonucu API'den de doğrulanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef (fatura, banka, masraf
 * yok; karşılaştırmanın kaynağı yalnız bu dosyanın iki fişi) ve benzersiz
 * adlı bir masaüstü ajanı (sonunda kaldırılır). Başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Dönem Aktarım ${run} Ltd. Şti.` };
const PERIOD = "2026-10";
const PERIOD_YEAR = 2026;
const PERIOD_MONTH = "Eki"; // MonthPicker'ın kısa ay adı (date-fns tr, "LLL")
const SALE = `E2E dönem satış ${run}`;
const EXPENSE = `E2E dönem gider ${run}`;

/** Fişlerin hesap toplamları; mizan bunlardan yazılır. */
const DRAFTS = [
  {
    description: SALE,
    lines: [
      { account: "120.01.001", description: "Yıldız Market", debit: "1200.00", credit: "0" },
      { account: "600.01", description: "Satış", debit: "0", credit: "1000.00" },
      { account: "391.01", description: "Hesaplanan KDV", debit: "0", credit: "200.00" },
    ],
  },
  {
    description: EXPENSE,
    lines: [
      { account: "770.01", description: "Kırtasiye", debit: "350.00", credit: "0" },
      { account: "320.01", description: "Kırtasiyeci", debit: "0", credit: "350.00" },
    ],
  },
];

/**
 * Luca "Genel geçici mizan" dökümü (CSV, Türkçe sayı biçimi). Başlıktaki
 * tarih aralığı dönemi verir (accountplan.detectPeriod). `expense`: 770
 * hesabının borç toplamı; 350,00 fişlerle tutar, başkası tek hesapta fark.
 */
function lucaMizan(expense: string) {
  const rows = [
    `DÖNEM AKTARIM ${run} GENEL GEÇİCİ MİZAN 01.01.2026 - 31.10.2026`,
    "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye",
    "120;ALICILAR;1.200,00;0,00;1.200,00;",
    "120.01;YURT İÇİ ALICILAR;1.200,00;0,00;1.200,00;",
    "120.01.001;YILDIZ MARKET;1.200,00;0,00;1.200,00;",
    "320;SATICILAR;0,00;350,00;;350,00",
    "320.01;YURT İÇİ SATICILAR;0,00;350,00;;350,00",
    "391;HESAPLANAN KDV;0,00;200,00;;200,00",
    "391.01;HESAPLANAN KDV %20;0,00;200,00;;200,00",
    "600;YURT İÇİ SATIŞLAR;0,00;1.000,00;;1.000,00",
    "600.01;YURT İÇİ SATIŞLAR;0,00;1.000,00;;1.000,00",
    `770;GENEL YÖNETİM GİDERLERİ;${expense};0,00;${expense};`,
    `770.01;KIRTASİYE GİDERLERİ;${expense};0,00;${expense};`,
  ];
  return {
    name: `luca-mizan-${PERIOD}-${randomUUID().slice(0, 8)}.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(rows.join("\n") + "\n", "utf8"),
  };
}

type Transfer = {
  uuid: string;
  batch_uuid: string;
  period?: string;
  driver: string;
  status: string;
  files: { name: string }[];
  completed_at?: string;
};
type RunSummary = {
  accounts: number;
  matched: number;
  differences: number;
  missing_in_ledger: number;
  missing_in_sources: number;
};
type RunDetail = {
  run: { created_at: string; summary: RunSummary };
  items: { main_code: string; result: string; difference: string }[];
};

let api: Api;
let office: Page;
let engagement = "";
let agent = "";
let transfer = "";

const base = () => `/v1/tenant/engagements/${engagement}`;

async function transfers() {
  return (await api.json<{ items: Transfer[] }>(await api.get(`${base()}/journal-transfers?limit=20`))).items;
}

async function latestRun() {
  return api.get(`${base()}/reconciliation/runs/latest?period=${PERIOD}`);
}

async function uploadMizan(expense: string) {
  const res = await api.uploadWith(`${base()}/account-plan/imports`, lucaMizan(expense), { period: PERIOD });
  expect(res.status(), await res.text()).toBe(201);
}

/** Yevmiye ekranı: bu mükellef ve dönem (global dönem seçici). */
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

/** "Programa aktarım" panelindeki bu dönemin aktarım satırı. */
function transferRow() {
  const panel = office
    .locator("section")
    .filter({ has: office.getByRole("heading", { name: "Programa aktarım", exact: true }) });
  return panel.getByRole("row").filter({ hasText: PERIOD });
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "donem");

  // İki onaylı fiş: dönem aktarımı ikisini birden götürmeli.
  for (const d of DRAFTS) {
    const draft = await api.json<{ uuid: string; period: string }>(
      await api.post(`${base()}/journal-drafts`, {
        entry_type: "mahsup",
        entry_date: `${PERIOD}-15T00:00:00Z`,
        description: d.description,
        lines: d.lines,
      }),
    );
    expect(draft.period).toBe(PERIOD);
    const approved = await api.json<{ status: string }>(
      await api.post(`${base()}/journal-drafts/${draft.uuid}/approve`),
    );
    expect(approved.status).toBe("approved");
  }

  // Masaüstü ajanı: kaydı masaüstü uygulamasının yaptığı gibi API'den.
  const registered = await api.json<{ uuid: string }>(
    await api.post("/v1/tenant/agents", {
      machine_id: `e2e-donem-${randomUUID()}`,
      name: `E2E-Donem-${run}`,
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

test("seçim yapmadan dönemin tüm onaylı fişleri Luca'ya aktarılır", async () => {
  await openJournals();
  for (const d of [SALE, EXPENSE]) {
    await expect(office.locator("tr", { hasText: d }).first()).toContainText("Onaylı");
  }
  // Seçim yok: kapsam dönemin tamamı.
  await expect(
    office.getByText("Dönemin tüm onaylı fişleri aktarılır. Yalnız bazılarını aktarmak için tablodan seçin."),
  ).toBeVisible();
  await office.locator('select:has(option[value="luca"])').selectOption("luca");
  await office.getByRole("button", { name: "Çıktı al", exact: true }).last().click();
  await expect(office.getByText("2 fiş aktarıldı.")).toBeVisible();
  for (const d of [SALE, EXPENSE]) {
    await expect(office.locator("tr", { hasText: d }).first()).toContainText("Aktarıldı");
  }

  // API: tek çıktı, dönemli (seçili fiş aktarımında dönem boş kalır), iki fiş.
  const batches = await api.json<{ items: { uuid: string; driver: string; period?: string; entries: number }[] }>(
    await api.get(`${base()}/journal-exports?limit=20`),
  );
  expect(batches.items).toHaveLength(1);
  expect(batches.items[0]).toMatchObject({ driver: "luca", period: PERIOD, entries: 2 });
  const drafts = await api.json<{ items: { description: string; status: string }[] }>(
    await api.get(`${base()}/journal-drafts?period=${PERIOD}&limit=50`),
  );
  expect(drafts.items.map((d) => d.status).sort()).toEqual(["exported", "exported"]);
});

test("ajan dosyayı klasöre bırakır, müşavir programa aktarıldı der", async () => {
  await office.getByRole("button", { name: "Masaüstü ajanına gönder" }).click();
  await expect(transferRow()).toContainText("Ajan bekleniyor");

  const queued = await transfers();
  expect(queued).toHaveLength(1);
  expect(queued[0]).toMatchObject({ driver: "luca", period: PERIOD, status: "queued" });
  transfer = queued[0].uuid;
  const file = queued[0].files[0]?.name;
  expect(file, "aktarımın dosyası").toMatch(/^luca_fis_2026-10\.xlsx$/);

  // Masaüstü ajanının adımları: al ve klasöre bırak.
  const claimed = await api.json<Transfer>(
    await api.post(`${base()}/journal-transfers/${transfer}/claim`, { agent_uuid: agent }),
  );
  expect(claimed.status).toBe("claimed");
  const delivered = await api.json<Transfer>(
    await api.post(`${base()}/journal-transfers/${transfer}/delivery`, {
      agent_uuid: agent,
      outcome: "delivered",
      folder_label: "Luca aktarım klasörü",
      files: [{ name: file, sha256: "a".repeat(64), size: 4096 }],
    }),
  );
  expect(delivered.status).toBe("delivered");

  await office.reload();
  const row = transferRow();
  await expect(row).toContainText("Klasörde, içe aktarılmayı bekliyor");
  await row.getByRole("button", { name: "Aktarıldı", exact: true }).click();
  const dialog = office.getByRole("alertdialog").or(office.getByRole("dialog"));
  await expect(dialog).toContainText("Programa aktarıldı mı?");
  await dialog.getByRole("button", { name: "Aktarıldı", exact: true }).click();
  await expect(row).toContainText("Programa aktarıldı");

  const done = (await transfers()).find((t) => t.uuid === transfer);
  expect(done).toMatchObject({ status: "imported", period: PERIOD });
  expect(done?.completed_at).toBeTruthy();
});

test("karşılaştırılmamış dönem: 'Mizanla karşılaştır' Mutabakat ekranına götürür", async () => {
  // Henüz mizan yok, karşılaştırma yapılmadı.
  expect((await latestRun()).status()).toBe(404);

  const row = transferRow();
  await expect(row).toContainText("Programa aktarıldı");
  await expect(row).toContainText("Mizanla karşılaştırılmadı");
  await row.getByRole("button", { name: "Mizanla karşılaştır" }).click();
  await office.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/reconciliation`));
  await expect(office.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
  await expect(office.getByText("Bu dönem henüz karşılaştırılmadı")).toBeVisible();
});

test("Luca mizanı yüklenip karşılaştırılınca aktarım satırı 'tutuyor' der", async () => {
  await uploadMizan("350,00");

  // Mutabakat ekranında (dönem aktarım satırından seçili geldi) karşılaştır.
  await office.getByRole("button", { name: "Karşılaştır", exact: true }).click();
  await expect(office.getByText("Karşılaştırma tamamlandı.")).toBeVisible();

  const res = await latestRun();
  const detail = await api.json<RunDetail>(res);
  expect(detail.run.summary).toMatchObject({
    accounts: 5,
    matched: 5,
    differences: 0,
    missing_in_ledger: 0,
    missing_in_sources: 0,
  });
  const done = (await transfers()).find((t) => t.uuid === transfer);
  // Karşılaştırma aktarımdan sonra: eski sayılmaz.
  expect(new Date(detail.run.created_at).getTime()).toBeGreaterThanOrEqual(new Date(done!.completed_at!).getTime());

  await openJournals();
  const row = transferRow();
  await expect(row).toContainText("Luca mizanı kayıtlarla tutuyor");
  await expect(row.getByRole("button", { name: /Mizanla karşılaştır|Farkları gör/ })).toHaveCount(0);
});

test("mizan kayıtlardan farklıysa 'hesapta fark var', 'Farkları gör' farklı hesabı açar", async () => {
  // Luca'da 770'e 25,00 fazla işlenmiş: tek hesapta fark.
  await uploadMizan("375,00");
  const detail = await api.json<RunDetail>(await api.post(`${base()}/reconciliation/runs`, { period: PERIOD }));
  expect(detail.run.summary).toMatchObject({ differences: 1, missing_in_ledger: 0, missing_in_sources: 0 });
  const expense = detail.items.find((i) => i.main_code === "770");
  expect(expense).toMatchObject({ result: "difference" });
  expect(Number(expense!.difference)).toBe(25);

  await openJournals();
  const row = transferRow();
  await expect(row).toContainText("Mizan karşılaştırmasında 1 hesapta fark var");
  await row.getByRole("button", { name: "Farkları gör" }).click();
  await office.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/reconciliation`));
  await expect(office.getByRole("heading", { level: 1, name: taxpayer.title })).toBeVisible();
  // "Yalnızca açık farklar" varsayılan: yalnız 770 listelenir.
  const rows = office.getByRole("table").getByRole("row");
  await expect(rows.filter({ hasText: /^770/ })).toHaveCount(1);
  await expect(rows.filter({ hasText: /^600/ })).toHaveCount(0);
});
