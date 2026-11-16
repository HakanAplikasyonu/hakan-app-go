import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { expect, test, type Page } from "@playwright/test";

import {
  Api,
  consentTokenFor,
  DEMO,
  engagementOf,
  officePath,
  pickTaxpayer,
  registerWithToken,
  uiLogin,
} from "./lib";
import { exportAndDownload, newApprovedEntry } from "./journal";

/**
 * Duman akışı (#170, docs/dogrulama-raporu.md §2.2): ofis girişi → mükellef
 * ekleme → UBL yükle → inceleme → uyarılar → banka eşleme → dönem kilidi →
 * yevmiye → Luca / Zirve indirme → mükellef web paneli.
 *
 * Adımlar sırayla koşar ve bir öncekinin verisini kullanır. Ortam her koşuda
 * sıfırdan kurulur (scripts/e2e-up.sh: migration + demo-seed -reset).
 * Arayüzü olmayan iki adım API'den yapılır ve sonucu arayüzde denetlenir:
 * banka hesabı / ekstre (#144) ve dönem kilidi.
 */
test.describe.configure({ mode: "serial" });

const ublFiles = readdirSync(DEMO.ublDir)
  .filter((f) => f.startsWith("atlas-") && f.endsWith(".xml"))
  .sort()
  .map((f) => path.join(DEMO.ublDir, f));
const INVOICE_PERIOD = "2026-10"; // demo UBL setinin dönemi

let office: Page;
let api: Api;
let atlas: string; // Atlas Gıda engagement uuid

test.beforeAll(async ({ browser }) => {
  office = await (await browser.newContext()).newPage();
  api = await Api.login(DEMO.owner);
  atlas = await engagementOf(api, DEMO.atlas.title);
});

test.afterAll(async () => {
  await api?.dispose();
  await office?.context().close();
});

test("ofis girişi: genel bakış API'den yükleniyor", async () => {
  await uiLogin(office, DEMO.owner);
  await office.goto(officePath());
  await expect(office.getByText("Sahip olarak giriş yaptınız")).toBeVisible();
  await expect(office.getByText(DEMO.atlas.title).first()).toBeVisible();
  await expect(office.getByText(DEMO.selin.title).first()).toBeVisible();
  await expect(office.getByText("Demo verisi")).toHaveCount(0);
});

test("oturum uçları: realtime ve KVKK onayı 403 vermiyor", async () => {
  for (const email of [DEMO.owner, DEMO.intern, DEMO.taxpayerUser]) {
    const a = email === DEMO.owner ? api : await Api.login(email);
    expect((await a.post("/v1/realtime/connection-token")).status(), email).toBe(200);
    expect((await a.get("/v1/me/legal-acceptances")).status(), email).toBe(200);
    if (a !== api) await a.dispose();
  }
});

test("mükellef ekleme: onay e-postası, kayıt ve onay", async ({ browser }) => {
  const email = `e2e-${Date.now()}@deneme.local`;
  await office.goto(officePath("/taxpayer-requests"));
  await office.locator('input[name="tax_id"]').fill("1234567890");
  await office.locator('input[name="title"]').fill("Deneme Yapı Ltd. Şti.");
  await office.locator('input[name="contact_email"]').fill(email);
  await office.getByRole("button", { name: "Onay iste" }).click();
  await expect(office.getByText(email).first()).toBeVisible();

  const token = await consentTokenFor(email);
  const owner = await (await browser.newContext()).newPage();
  try {
    await registerWithToken(owner, token, `/consent/${token}`, {
      name: "Deneme",
      surname: "Sahibi",
      password: "E2e-Parola-123!",
    });
    // Aydınlatma metni onayı işaretlenmeden "Onayla" kapalı kalır.
    await expect(owner.getByRole("checkbox").first()).toBeVisible();
    for (const box of await owner.getByRole("checkbox").all()) await box.check();
    await expect(owner.getByRole("button", { name: "Onayla" })).toBeEnabled();
    await owner.getByRole("button", { name: "Onayla" }).click();
    await owner.waitForURL(/\/m\/[0-9a-f-]{36}/);
    await expect(owner.getByText("Deneme Yapı Ltd. Şti.").first()).toBeVisible();
  } finally {
    await owner.context().close();
  }

  const reqs = await api.json<{ items: { contact_email: string; status: string }[] }>(
    await api.get("/v1/tenant/taxpayer-requests"),
  );
  expect(reqs.items.find((r) => r.contact_email === email)?.status).toBe("approved");
});

test("UBL yükle: Atlas'ın 7 faturası sunucuda işleniyor", async () => {
  await office.goto(officePath("/upload"));
  await pickTaxpayer(office, DEMO.atlas.title);
  await office.locator('input[type="file"]').setInputFiles(ublFiles);
  await office.getByRole("button", { name: `${ublFiles.length} dosyayı yükle` }).click();
  await expect(office.getByText("Yükleme raporu").first()).toBeVisible({ timeout: 60_000 });
  // Her dosya ayrı bir iş: hepsi bitmeli ve toplam 7 fatura eklenmeli.
  await expect
    .poll(
      async () => {
        const jobs = await api.json<{ items: { status: string; report?: { added: number } }[] }>(
          await api.get(`/v1/tenant/engagements/${atlas}/ubl-uploads?limit=50`),
        );
        if (jobs.items.some((j) => j.status !== "done")) return -1;
        return jobs.items.reduce((n, j) => n + (j.report?.added ?? 0), 0);
      },
      { timeout: 90_000 },
    )
    .toBe(ublFiles.length);

  await office.goto(officePath(`/invoices?taxpayer=${atlas}`));
  for (const no of ["ATL2026000000101", "ATL2026000000102", "ATL2026000000106"]) {
    await expect(office.getByText(no).first()).toBeVisible();
  }
});

test("inceleme: fatura detayından öneri, kuyruktan onay", async () => {
  await office.goto(officePath(`/invoices?taxpayer=${atlas}`));
  await office.getByText("ATL2026000000102").first().click();
  await office.getByRole("button", { name: "Hesap kodu öner" }).click();
  await expect(office.getByText(/öneri hazır/)).toBeVisible();

  await office.goto(officePath("/review"));
  const pending = office.getByText(/\d+ öneri bekliyor/);
  await expect(pending).toBeVisible();
  const before = Number((await pending.innerText()).match(/\d+/)![0]);
  expect(before).toBeGreaterThan(0);
  await office.getByRole("button", { name: "Onayla", exact: true }).first().click();
  await expect(office.getByText(`${before - 1} öneri bekliyor`)).toBeVisible();

  const accepted = await api.json<{ total: number }>(
    await api.get("/v1/tenant/review/suggestions?status=accepted"),
  );
  expect(accepted.total).toBeGreaterThanOrEqual(1);
});

test("banka eşleme: hesap, MT940 ve BNK uyarısı (API + uyarılar ekranı)", async () => {
  await api.json(
    await api.post(`/v1/tenant/engagements/${atlas}/cash-boxes`, {
      iban: "TR76 0001 0005 1978 6457 8413 26",
      name: "Ziraat TL",
    }),
  );
  const imp = await api.json<{ accounts: { inserted: number }[] }>(
    await api.upload(`/v1/tenant/engagements/${atlas}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  expect(imp.accounts[0].inserted).toBe(2);
  const match = await api.json<{ findings: string[] }>(
    await api.post(`/v1/tenant/engagements/${atlas}/bank-matching?period=${INVOICE_PERIOD}`),
  );
  expect(match.findings).toContain("BNK_UNINVOICED_COLLECTION");

  await office.goto(officePath("/warnings"));
  await expect(office.getByText("Faturası olmayan tahsilat").first()).toBeVisible();
});

test("dönem kilidi: kilit API ile, dönem matrisinde kilitli ve komuta merkezi açılıyor", async () => {
  const lock = await api.json<{ locked: boolean }>(
    await api.post(`/v1/tenant/engagements/${atlas}/periods/${INVOICE_PERIOD}/lock`),
  );
  expect(lock.locked).toBe(true);

  await office.goto(officePath("/command"));
  await expect(office.getByText("Komuta merkezi").first()).toBeVisible();
  const matrix = await api.json<{ items: unknown[] }>(
    await api.get(`/v1/tenant/periods/matrix?period=${INVOICE_PERIOD}`),
  );
  expect(JSON.stringify(matrix)).toContain('"locked":true');
});

test("yevmiye: taslak, onay, Luca ve Zirve dosyası indirme", async () => {
  await office.goto(officePath("/journals"));
  await pickTaxpayer(office, DEMO.atlas.title);
  await expect(office.locator('select:has(option[value="luca"]) option')).toHaveText([
    "Excel (formüllü)",
    "Luca",
    "Zirve",
  ]);

  await newApprovedEntry(office, "E2E Luca fişi");
  await exportAndDownload(office, "luca");
  await expect(office.locator("tr", { hasText: "Aktarıldı" }).first()).toBeVisible();

  await newApprovedEntry(office, "E2E Zirve fişi");
  await exportAndDownload(office, "zirve");
});

test("mükellef web paneli: ofisin yüklediği faturalar görünüyor", async ({ browser }) => {
  const page = await (await browser.newContext()).newPage();
  try {
    await uiLogin(page, DEMO.taxpayerUser);
    await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
    await expect(page.getByText(DEMO.atlas.title).first()).toBeVisible();
    await page.getByRole("link", { name: "Faturalar" }).first().click();
    await expect(page.getByText("ATL2026000000102").first()).toBeVisible();
    await expect(page.getByText("YLD2026000000005").first()).toBeVisible();
  } finally {
    await page.context().close();
  }
});

test("tenant izolasyonu: başka mükellefin verisi kapalı", async () => {
  const taxpayer = await Api.login(DEMO.taxpayerUser);
  try {
    const selin = await engagementOf(api, DEMO.selin.title);
    const tp = await api.json<{ uuid: string }>(
      await api.get(`/v1/tenant/engagements/${selin}/taxpayer`),
    );
    const res = await taxpayer.get("/v1/taxpayer/invoices", { "X-Taxpayer": tp.uuid });
    expect(res.status()).toBe(403);
    expect((await res.json()).error.code).toBe("NO_TAXPAYER_MEMBERSHIP");
  } finally {
    await taxpayer.dispose();
  }
  const intern = await Api.login(DEMO.intern);
  try {
    const res = await intern.get(`/v1/tenant/engagements/${atlas}/journal-drafts`);
    expect((await res.json()).error?.code).toBe("NO_ENGAGEMENT_ACCESS");
  } finally {
    await intern.dispose();
  }
});
