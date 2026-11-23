import { expect, test, type Page } from "@playwright/test";

import {
  Api,
  DEMO,
  deriveUbl,
  engagementOf,
  mailToken,
  pickTaxpayer,
  registerWithToken,
  uiLogin,
  uniqueEmail,
  uniqueVkn,
  waitForUploads,
} from "./lib";
import { exportAndDownload, newApprovedEntry } from "./journal";

/**
 * Ofis ana akışı, yalıtılmış (#181): demo verisine dokunmaz, her koşuda
 * yeni bir ofis, yeni bir sahip ve yeni bir VKN ile çalışır. Bu yüzden diğer
 * dosyalarla paralel koşabilir.
 *
 * ofis kaydı (platform yöneticisi açar, sahibi e-posta davetiyle katılır) →
 * mükellef ekleme (onay e-postası, kayıt, onay) → UBL yükle → hesap önerisi
 * onayı → yevmiye taslağı → Luca / Zirve indirme → mükellefin kendi paneli.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-7);
const slug = `e2e-${run}`;
const officeName = `E2E Mali Müşavirlik ${run}`;
const ownerEmail = uniqueEmail("sahip");
const taxpayer = { vkn: uniqueVkn(), title: `E2E Ticaret ${run} A.Ş.` };
const taxpayerEmail = uniqueEmail("mukellef");
const password = "E2e-Parola-123!";

let owner: Page;

test.beforeAll(async ({ browser }) => {
  owner = await (await browser.newContext()).newPage();
});

test.afterAll(async () => {
  await owner?.context().close();
});

test("ofis kaydı: platform yöneticisi ofisi açar ve sahibi davet eder", async ({ browser }) => {
  const admin = await (await browser.newContext()).newPage();
  try {
    await uiLogin(admin, DEMO.admin, DEMO.adminPassword);
    await admin.goto("/platform/organizations/create");
    await admin.locator('input[name="name"]').fill(officeName);
    await admin.locator('input[name="slug"]').fill(slug);
    await admin.getByRole("button", { name: "Organizasyon oluştur" }).click();
    await admin.waitForURL((u) => /\/platform\/organizations\/[0-9a-f-]{36}/.test(u.pathname));

    // Yönetici yeni ofisin ilk sahibi; gerçek sahibi davet eder.
    await admin.goto(`/t/${slug}/members`);
    await admin.locator('input[name="email"]').fill(ownerEmail);
    await admin.getByRole("combobox", { name: "Rol" }).click();
    await admin.getByRole("option", { name: "Sahip", exact: true }).click();
    await admin.getByRole("button", { name: "Davet gönder" }).click();
    await expect(admin.getByText(ownerEmail).first()).toBeVisible();
  } finally {
    await admin.context().close();
  }

  const token = await mailToken(ownerEmail, "/invite/");
  await registerWithToken(owner, token, `/invite/${token}`, { name: "Ece", surname: "Sahip", password });
  await owner.getByRole("button", { name: "Daveti kabul et" }).click();
  await owner.waitForURL((u) => u.pathname.startsWith(`/t/${slug}`));
  await owner.goto(`/t/${slug}`);
  await expect(owner.getByText("İnceleme bekleyen öneri")).toBeVisible();
});

test("mükellef ekleme: onay e-postası, mükellefin kaydı ve onayı", async ({ browser }) => {
  await owner.goto(`/t/${slug}/taxpayer-requests`);
  await owner.locator('input[name="tax_id"]').fill(taxpayer.vkn);
  await owner.locator('input[name="title"]').fill(taxpayer.title);
  await owner.locator('input[name="contact_email"]').fill(taxpayerEmail);
  await owner.getByRole("button", { name: "Onay iste" }).click();
  await expect(owner.getByText(taxpayerEmail).first()).toBeVisible();

  const token = await mailToken(taxpayerEmail, "/consent/");
  const tp = await (await browser.newContext()).newPage();
  try {
    await registerWithToken(tp, token, `/consent/${token}`, { name: "Murat", surname: "Mükellef", password });
    await expect(tp.getByText(officeName).first()).toBeVisible();
    await expect(tp.getByRole("checkbox").first()).toBeVisible();
    for (const box of await tp.getByRole("checkbox").all()) await box.check();
    await tp.getByRole("button", { name: "Onayla" }).click();
    await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
    await expect(tp.getByText(taxpayer.title).first()).toBeVisible();
  } finally {
    await tp.context().close();
  }

  await owner.goto(`/t/${slug}/taxpayers`);
  await expect(owner.getByText(taxpayer.title)).toBeVisible();
});

test("UBL yükle: yeni mükellefin faturaları işleniyor ve listede", async () => {
  await owner.goto(`/t/${slug}/upload`);
  await pickTaxpayer(owner, taxpayer.title);
  const files = ["atlas-01-temel-satis.xml", "atlas-02-ticari-satis.xml"].map((f) =>
    deriveUbl(f, taxpayer.vkn, taxpayer.title),
  );
  await owner.getByTestId("ubl-input").setInputFiles(files);
  await owner.getByRole("button", { name: "2 dosyayı yükle" }).click();
  await expect(owner.getByText("Eklendi").first()).toBeVisible({ timeout: 60_000 });
  const api = await Api.login(ownerEmail, password, slug);
  try {
    await waitForUploads(api, await engagementOf(api, taxpayer.title), files.length);
  } finally {
    await api.dispose();
  }

  await owner.goto(`/t/${slug}/invoices`);
  await pickTaxpayer(owner, taxpayer.title);
  await expect(owner.getByText("ATL2026000000101").first()).toBeVisible();
  await expect(owner.getByText("ATL2026000000102").first()).toBeVisible();
});

test("hesap önerisi: fatura detayından üret, kuyruktan onayla", async () => {
  await owner.getByText("ATL2026000000101").first().click();
  await owner.getByRole("button", { name: "Hesap kodu öner" }).click();
  await expect(owner.getByText(/öneri hazır/)).toBeVisible();

  await owner.goto(`/t/${slug}/review`);
  // Kuyruk yüklenirken kısa süre "0 öneri bekliyor" görünür: sıfırdan büyük
  // sayıyı bekle (aksi halde ara durumu okuyup kararsız düşer).
  const pending = owner.getByText(/^[1-9]\d* öneri bekliyor/);
  await expect(pending).toBeVisible();
  const before = Number((await pending.innerText()).match(/\d+/)![0]);
  await owner.getByRole("button", { name: "Onayla", exact: true }).first().click();
  await expect(owner.getByText(`${before - 1} öneri bekliyor`)).toBeVisible();
});

test("yevmiye: taslak, onay, Luca ve Zirve dosyası", async () => {
  await owner.goto(`/t/${slug}/journals`);
  await pickTaxpayer(owner, taxpayer.title);
  await newApprovedEntry(owner, "E2E Luca fişi");
  await exportAndDownload(owner, "luca");
  await newApprovedEntry(owner, "E2E Zirve fişi");
  await exportAndDownload(owner, "zirve");
});

test("mükellef paneli: yeni mükellef ofisin yüklediği faturaları görüyor", async ({ browser }) => {
  const tp = await (await browser.newContext()).newPage();
  try {
    await uiLogin(tp, taxpayerEmail, password);
    await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
    await tp.getByRole("link", { name: "Faturalar" }).first().click();
    await expect(tp.getByText("ATL2026000000101").first()).toBeVisible();
    await expect(tp.getByText("ATL2026000000102").first()).toBeVisible();
  } finally {
    await tp.context().close();
  }

  // Yeni ofis demo ofisinin hiçbir kaydını görmüyor.
  const api = await Api.login(ownerEmail, password);
  try {
    const res = await api.get("/v1/tenant/engagements", { "X-Organization": DEMO.org });
    expect(res.status()).toBe(403);
    expect((await res.json()).error.code).toBe("NO_TENANT_MEMBERSHIP");
  } finally {
    await api.dispose();
  }
});
