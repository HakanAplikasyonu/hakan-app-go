import { execFileSync } from "node:child_process";
import path from "node:path";

import { expect, test, type Page } from "@playwright/test";

import { API_URL, Api, DEMO, mailToken, registerWithToken, uniqueEmail } from "./lib";

/**
 * Pilot ofis kurulumu (#273), yalıtılmış: demo verisine dokunmaz, her koşuda
 * yeni bir ofis ve yeni VKN'lerle çalışır.
 *
 * Kabul: boş bir ofiste 200 mükellefin kurulumu 10 dakikada bitiyor.
 * scripts/new-office.sh ofisi açar ve sahibi davet eder → sahip kaydolur →
 * panelde kurulum kartı → sihirbaz: ofis bilgileri, ekip daveti, 200 satırlık
 * liste (3 hatalı satır satır satır raporlanır), Luca, entegratör adımı
 * atlanır, 197 mükellefe toplu onay e-postası.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const slug = `e2e-kurulum-${run}`;
const officeName = `E2E Kurulum ${run}`;
const ownerEmail = uniqueEmail("kurulum-sahip");
const teamEmail = uniqueEmail("kurulum-ekip");
const password = "E2e-Parola-123!";
const ROWS = 200;

/** 10 haneli VKN: 9 haneli tabana sağlama hanesi (lib.ts uniqueVkn ile aynı). */
function vkn(base: string): string {
  const d = base.split("").map(Number);
  let sum = 0;
  for (let i = 0; i < 9; i++) {
    const t = (d[i] + 9 - i) % 10;
    if (t === 0) continue;
    let v = (t * 2 ** (9 - i)) % 9;
    if (v === 0) v = 9;
    sum += v;
  }
  return base + String((10 - (sum % 10)) % 10);
}

/** 200 satırlık liste: 197 geçerli, 1 hatalı VKN, 1 e-postasız, 1 tekrar. */
function taxpayerList() {
  const rows: string[][] = [];
  for (let i = 0; i < ROWS - 3; i++) {
    const id = vkn(`6${run.slice(-5)}${String(i).padStart(3, "0")}`);
    rows.push([id, `Kurulum ${run} Ticaret ${i + 1} A.Ş.`, "Kadıköy", `e2e-kurulum-${run}-${i + 1}@deneme.local`]);
  }
  const bad = rows[0][0].slice(0, 9) + String((Number(rows[0][0][9]) + 1) % 10);
  rows.push([bad, "Hatalı VKN Ltd.", "Kadıköy", `e2e-kurulum-${run}-hatali@deneme.local`]);
  rows.push([vkn(`6${run.slice(-5)}998`), "E-postasız A.Ş.", "Kadıköy", ""]);
  rows.push([rows[1][0], "Tekrar A.Ş.", "Kadıköy", `e2e-kurulum-${run}-tekrar@deneme.local`]);
  const csv = ["VKN;Unvan;Vergi Dairesi;E-posta", ...rows.map((r) => r.join(";"))].join("\n");
  return { csv, first: rows[0] };
}

let owner: Page;
let started = 0;
const list = taxpayerList();

test.beforeAll(async ({ browser }) => {
  owner = await (await browser.newContext()).newPage();
});

test.afterAll(async () => {
  await owner?.context().close();
});

test("yeni ofis betiği: platform yöneticisi ofisi açar, sahibi davet eder", async () => {
  const out = execFileSync(
    "bash",
    [path.resolve("../scripts/new-office.sh"), "--name", officeName, "--slug", slug, "--owner-email", ownerEmail],
    { env: { ...process.env, API_URL, ADMIN_EMAIL: DEMO.admin, ADMIN_PASSWORD: DEMO.adminPassword }, encoding: "utf8" },
  );
  expect(out).toContain(`slug:  ${slug}`);

  const token = await mailToken(ownerEmail, "/invite/");
  await registerWithToken(owner, token, `/invite/${token}`, { name: "Kader", surname: "Kurulum", password });
  await owner.getByRole("button", { name: "Daveti kabul et" }).click();
  await owner.waitForURL((u) => u.pathname.startsWith(`/t/${slug}`));
});

test("ilk giriş: kurulum kartı sihirbazı açar", async () => {
  started = Date.now();
  await owner.goto(`/t/${slug}`);
  await expect(owner.getByText("Ofis kurulumunu tamamlayın")).toBeVisible();
  await owner.getByRole("link", { name: "Kuruluma devam et" }).click();
  await owner.waitForURL(`**/t/${slug}/setup`);
  await expect(owner.getByRole("heading", { name: "Ofis kurulumu" })).toBeVisible();
});

test("ofis bilgileri ve ekip daveti", async () => {
  const name = owner.locator('input[name="name"]');
  await expect(name).toHaveValue(officeName);
  await name.fill(`${officeName} SMMM`);
  await owner.getByRole("button", { name: "Kaydet ve devam et" }).click();

  await owner.locator('input[name="email"]').fill(teamEmail);
  await owner.getByRole("button", { name: "Davet ekle" }).click();
  await expect(owner.getByText(teamEmail)).toBeVisible();
  await owner.getByRole("button", { name: "Kaydet ve devam et" }).click();
  await mailToken(teamEmail, "/invite/");
});

test("mükellef listesi: 200 satır, hatalı satırlar satır satır", async () => {
  await owner.locator('input[type="file"]').setInputFiles({
    name: "mukellefler.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(list.csv, "utf8"),
  });
  await expect(owner.getByText(`${ROWS - 3} geçerli, 3 hatalı satır`)).toBeVisible();
  // Satır numaraları başlık 1. satır olacak şekilde: 199, 200, 201. Hatalı
  // satırlar yerinde düzeltilir (#330): unvan metin değil, satırın kutusunda.
  const rows = owner.getByRole("table", { name: "Yüklenen mükellef listesi" });
  const bad = [
    [199, "Hatalı VKN Ltd.", "sağlama hanesi tutmuyor"],
    [200, "E-postasız A.Ş.", "onay e-postası için iletişim e-postası gerekli"],
    [201, "Tekrar A.Ş.", "aynı VKN/TCKN 3. satırda da var"],
  ] as const;
  for (const [n, title, reason] of bad) {
    const field = owner.getByRole("textbox", { name: `${n}. satır unvan` });
    const row = rows.getByRole("row").filter({ has: field });
    await expect(row.getByRole("textbox", { name: `${n}. satır unvan` })).toHaveValue(title);
    await expect(row).toContainText(reason);
  }
  // Gerekçe Türkçe; Go hata zinciri sızmaz.
  await expect(rows).not.toContainText("invalid request");
  await owner.getByRole("button", { name: "Kaydet ve devam et" }).click();
});

test("yarıda bırakılır, kaldığı yerden devam eder", async () => {
  await owner.goto(`/t/${slug}`);
  await expect(owner.getByText("Ofis kurulumunu tamamlayın")).toBeVisible();
  await owner.getByRole("link", { name: "Kuruluma devam et" }).click();
  // İlk bekleyen adım: muhasebe programı.
  await expect(owner.getByRole("radio", { name: "Luca" })).toBeVisible();
});

test("Luca seçimi, entegratör adımı atlanır, toplu davet", async () => {
  await owner.getByRole("radio", { name: "Luca" }).check();
  await owner.getByRole("button", { name: "Kaydet ve devam et" }).click();

  await expect(owner.getByRole("button", { name: "Bu adımı atla" })).toBeVisible();
  await owner.getByRole("button", { name: "Bu adımı atla" }).click();

  await expect(owner.getByText(`${ROWS - 3} mükellefe onay e-postası gönderilecek`)).toBeVisible();
  await owner.getByRole("button", { name: "Davetleri gönder" }).click();
  await expect(owner.getByText(`${ROWS - 3} davet gönderildi, 3 atlandı, 0 başarısız`)).toBeVisible({ timeout: 120_000 });
  await owner.getByRole("button", { name: "Kurulumu bitir" }).click();
  await owner.waitForURL((u) => u.pathname === `/t/${slug}`);
  await expect(owner.getByText("Ofis kurulumunu tamamlayın")).toHaveCount(0);

  const elapsed = Date.now() - started;
  expect(elapsed, `kurulum ${Math.round(elapsed / 1000)} sn sürdü`).toBeLessThan(10 * 60_000);
});

test("sonuç: talepler ofiste, onay e-postası mükellefte, ayarlar kayıtlı", async () => {
  const api = await Api.login(ownerEmail, password, slug);
  try {
    const setup = await api.json<{ accounting_program?: string; completed_at?: string; steps: Record<string, { status: string }> }>(
      await api.get("/v1/tenant/setup"),
    );
    expect(setup.accounting_program).toBe("luca");
    expect(setup.completed_at).toBeTruthy();
    expect(setup.steps.integrations.status).toBe("skipped");

    const settings = await api.json<{ name: string }>(await api.get("/v1/tenant/settings"));
    expect(settings.name).toBe(`${officeName} SMMM`);

    const imports = await api.json<{ items: { uuid: string; valid: number; invalid: number; sent_at: string | null }[] }>(
      await api.get("/v1/tenant/taxpayer-imports"),
    );
    expect(imports.items).toHaveLength(1);
    expect(imports.items[0]).toMatchObject({ valid: ROWS - 3, invalid: 3 });
    const batch = await api.json<{ outcome: { status: string }[] }>(
      await api.get(`/v1/tenant/taxpayer-imports/${imports.items[0].uuid}`),
    );
    expect(batch.outcome.filter((o) => o.status === "requested")).toHaveLength(ROWS - 3);

    // İkinci gönderim yok.
    const again = await api.post(`/v1/tenant/taxpayer-imports/${imports.items[0].uuid}/send`);
    expect(again.status()).toBe(409);
  } finally {
    await api.dispose();
  }
  // Listenin ilk mükellefine onay e-postası gitti.
  await mailToken(list.first[3], "/consent/");
});
