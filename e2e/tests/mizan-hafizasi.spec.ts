import { randomUUID } from "node:crypto";
import { mkdirSync, readFileSync } from "node:fs";
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
  uniqueEmail,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Mizan hafızası (#310, #319), ofis panelinden uçtan uca:
 *   yeni mükellef → Luca mizanı içe aktar → ilk ay faturaları (öneriler
 *   yükleme sonrası kendiliğinden) → kuyrukta 153 → 150.01 düzelt (3 kalem),
 *   yeni cariyi onayla → "Luca'da açılacak hesaplar" ve şablon → kural
 *   adayı → "Kural yap" → ikinci ay faturaları → "Hesap kodu öner"e
 *   basılmadan kalemler 150.01.
 *
 * Fikstürler: backend/testdata/mizan (go run testdata/mizan/generate.go).
 * Faturalardaki mükellef VKN'si her koşuda yeni bir VKN ile değiştirilir; bu
 * dosya yalnızca kendi eklediği mükellefe dokunur.
 *
 * E2E_DOCS_SHOTS=1 ile belgedeki (docs/kullanim/mizan-hafizasi.md) ekran
 * görüntülerini de üretir.
 */
test.describe.configure({ mode: "serial" });

const FIXTURES = path.resolve(__dirname, "../../backend/testdata/mizan");
const IMG_DIR = path.resolve(__dirname, "../../frontend/public/yardim/img");
const FIXTURE_VKN = "4730074556";
const FIXTURE_TITLE = "Oğuz Gıda Pazarlama Ltd. Şti.";
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Oğuz Gıda Pazarlama ${run} Ltd. Şti.` };
const contact = uniqueEmail("mizan");
const CORRECTED = "150.01";
const DENIZ_LINES = ["Oluklu Karton Koli 40x30x20", "Streç Film 50 cm", "Etiket Rulosu 100x150"];

let office: Page;
let engagement = "";

function ubl(file: string) {
  let xml = readFileSync(path.join(FIXTURES, "ubl", file), "utf8");
  const ettn = xml.match(/<cbc:UUID>([^<]+)<\/cbc:UUID>/)?.[1];
  if (!ettn) throw new Error(`no ETTN in ${file}`);
  xml = xml
    .split(FIXTURE_VKN).join(taxpayer.vkn)
    .split(FIXTURE_TITLE).join(taxpayer.title)
    .split(ettn).join(randomUUID().toUpperCase());
  return { name: file, mimeType: "application/xml", buffer: Buffer.from(xml, "utf8") };
}

async function shot(page: Page, name: string) {
  if (process.env.E2E_DOCS_SHOTS !== "1") return;
  mkdirSync(IMG_DIR, { recursive: true });
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(300);
  await page.screenshot({ path: path.join(IMG_DIR, `${name}.jpg`), type: "jpeg", quality: 80, animations: "disabled" });
}

/** Yükleme sonrası öneri işi (#319) bitene kadar bekler. */
async function waitForSuggestions(atLeast: number) {
  const api = await Api.login(DEMO.owner);
  try {
    await expect
      .poll(
        async () =>
          (
            await api.json<{ items: unknown[] }>(
              await api.get(`/v1/tenant/engagements/${engagement}/suggestions?status=all&kind=account_code&limit=100`),
            )
          ).items.length,
        { timeout: 60_000 },
      )
      .toBeGreaterThanOrEqual(atLeast);
  } finally {
    await api.dispose();
  }
}

async function uploadMonth(month: "2026-10" | "2026-11") {
  const files = ["korkmaz-satis", "deniz-alis", "yeni-lojistik-alis"].map((f) => ubl(`${month}-${f}.xml`));
  await office.goto(officePath("/upload"));
  await pickTaxpayer(office, taxpayer.title);
  await office.locator('input[type="file"]').setInputFiles(files);
  await office.getByRole("button", { name: `${files.length} dosyayı yükle` }).click();
  const api = await Api.login(DEMO.owner);
  try {
    const before = month === "2026-10" ? 0 : 3;
    await waitForUploads(api, engagement, before + files.length);
  } finally {
    await api.dispose();
  }
}

/** İnceleme kuyruğu, yalnızca bu mükellef (kuyruk ofis geneli ve sayfalı). */
/** Kuyruk satırları, yalnızca bu koşunun mükellefi (filtre yüklenirken önceki
 * sayfa ekranda kalır). */
const queueRows = () => office.getByRole("listitem").filter({ hasText: taxpayer.title });

// textContent'te boşluk yok ("…1. satır153%50…"): kodu rakam/nokta sınırıyla ara.
const codeRe = (code: string) => new RegExp(`(?<![\\d.])${code.replace(".", "\\.")}(?![\\d.])`);
const lineRow = (item: string, code: string) =>
  queueRows().filter({ hasText: item }).filter({ hasText: codeRe(code) });

async function openQueue() {
  await office.goto(officePath("/review"));
  await expect(office.getByRole("heading", { name: "İnceleme kuyruğu" })).toBeVisible();
  await office.getByRole("combobox", { name: "Mükellef" }).selectOption({ label: taxpayer.title });
}

test.beforeAll(async ({ browser }) => {
  office = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  await uiLogin(office, DEMO.owner);
});

test.afterAll(async () => {
  await office?.context().close();
});

test("yeni mükellef: Oğuz Gıda onay verir", async ({ browser }) => {
  await office.goto(officePath("/taxpayer-requests"));
  await office.locator('input[name="tax_id"]').fill(taxpayer.vkn);
  await office.locator('input[name="title"]').fill(taxpayer.title);
  await office.locator('input[name="contact_email"]').fill(contact);
  await office.getByRole("button", { name: "Onay iste" }).click();
  await expect(office.getByText(contact).first()).toBeVisible();

  const token = await consentTokenFor(contact);
  const tp = await (await browser.newContext()).newPage();
  try {
    await registerWithToken(tp, token, `/consent/${token}`, { name: "Oğuz", surname: "Mükellef", password: "E2e-Parola-123!" });
    await expect(tp.getByRole("checkbox").first()).toBeVisible();
    for (const box of await tp.getByRole("checkbox").all()) await box.check();
    await tp.getByRole("button", { name: "Onayla" }).click();
    await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
  } finally {
    await tp.context().close();
  }
  const api = await Api.login(DEMO.owner);
  try {
    engagement = await engagementOf(api, taxpayer.title);
  } finally {
    await api.dispose();
  }
});

test("Luca mizanı içe aktarılır", async () => {
  await office.goto(officePath("/account-plan"));
  await pickTaxpayer(office, taxpayer.title);
  await expect(office.getByText("Henüz hesap planı yüklenmedi")).toBeVisible();
  await office.getByLabel("Mizan dosyası (.xlsx, .csv)").setInputFiles(path.join(FIXTURES, "oguz-luca-mizan.xlsx"));
  await office.getByRole("button", { name: "İçe aktar" }).click();
  await expect(office.getByRole("status").filter({ hasText: "34 hesap içe aktarıldı" })).toBeVisible();
  await expect(office.getByTestId("plan-summary")).toContainText("34 hesap");
  await shot(office, "mizan-hesap-plani");
});

test("ilk ay: öneriler yüklemeyle gelir, müşavir 153'ü 150.01'e düzeltir", async () => {
  await uploadMonth("2026-10");
  // Korkmaz 2 kalem × (hesap + KDV) + cari, Deniz 3 × 2 + cari, Yeni 1 × 2 + cari.
  await waitForSuggestions(15);

  await openQueue();
  // Planda olan cari mevcut hesabıyla, olmayan sıradaki boş kodla önerilir.
  const korkmaz = queueRows().filter({ hasText: "Cari hesap: Korkmaz Gıda" });
  await expect(korkmaz).toContainText("120.01.006");
  const yeni = queueRows().filter({ hasText: "Cari hesap: Yeni Lojistik" });
  await expect(yeni).toContainText("320.01.007");
  await shot(office, "mizan-inceleme-kuyrugu");

  // Her kalemin iki önerisi var: hesap kodu (153) ve KDV hesabı; KDV,
  // plandaki %20 alt hesabına (191.01) gider.
  await expect(lineRow(DENIZ_LINES[0], "191.01")).toHaveCount(1);
  for (const item of DENIZ_LINES) {
    const row = lineRow(item, "153");
    await expect(row).toHaveCount(1);
    // Kuyruk ve karar ekranı tek sayfa (#326): satırı seç, "Kodu düzelt".
    await row.click();
    await office.getByRole("button", { name: "Kodu düzelt" }).click();
    const input = office.getByPlaceholder("Hesap kodu ya da adı…");
    await input.fill(CORRECTED);
    await input.press("Enter");
    await expect(lineRow(item, "153")).toHaveCount(0);
  }
});

test("onaylanan yeni cari Luca'da açılacak hesaplarda, şablon indirilir", async () => {
  await openQueue();
  const yeni = queueRows().filter({ hasText: "Cari hesap: Yeni Lojistik" });
  await yeni.click();
  await office.getByRole("button", { name: "Onayla", exact: true }).click();
  await expect(queueRows().filter({ hasText: "Cari hesap: Yeni Lojistik" })).toHaveCount(0);

  await office.goto(officePath(`/account-plan?taxpayer=${engagement}`));
  const missing = office.getByRole("row").filter({ hasText: "320.01.007" });
  await expect(missing).toContainText("Yeni Lojistik");
  await shot(office, "mizan-acilacak-hesaplar");
  const download = office.waitForEvent("download");
  await office.getByRole("button", { name: "Eksik hesap şablonunu indir" }).click();
  expect((await download).suggestedFilename()).toBe("eksik-hesaplar.xlsx");
});

test("kural adayı panelde görünür ve kurala terfi eder", async () => {
  await office.goto(officePath(`/account-plan?taxpayer=${engagement}`));
  const candidate = office.getByRole("row").filter({ hasText: "Deniz Ambalaj" });
  await expect(candidate).toContainText(CORRECTED);
  await expect(candidate).toContainText("Alış");
  await expect(candidate).toContainText("3");
  await shot(office, "mizan-kural-adayi");
  await candidate.getByRole("button", { name: "Kural yap" }).click();
  await expect(office.getByText("Şimdilik kural adayı yok")).toBeVisible();
  const rule = office.getByRole("row").filter({ hasText: "Deniz Ambalaj" });
  await expect(rule).toContainText(CORRECTED);
  await expect(rule).toContainText("Kararlarınızdan");
});

test("ikinci ay: Deniz Ambalaj kalemleri soru sorulmadan 150.01", async () => {
  // Yalnızca yükleme: "Hesap kodu öner"e basılmaz (#319).
  await uploadMonth("2026-11");
  // Korkmaz 2 × 2 + cari, Deniz 3 × 2 + cari, Yeni 1 × 2 (cari kodlu).
  await waitForSuggestions(15 + 13);

  await office.goto(officePath(`/account-plan?taxpayer=${engagement}`));
  for (const item of DENIZ_LINES) {
    const row = office.getByRole("row").filter({ hasText: item }).filter({ hasText: CORRECTED });
    await expect(row.first()).toContainText("Kural");
  }
  await shot(office, "mizan-otomatik-kodlama");

  // İnceleme kuyruğuna düşmezler.
  await openQueue();
  for (const item of DENIZ_LINES) {
    for (const code of ["153", CORRECTED]) {
      await expect(lineRow(item, code)).toHaveCount(0);
    }
  }
});

test("geri al ve kuralı kaldır", async () => {
  await office.goto(officePath(`/account-plan?taxpayer=${engagement}`));
  const autoRows = office.getByRole("row").filter({ hasText: "Deniz Ambalaj" }).filter({ hasText: "Kural" }).filter({ hasText: DENIZ_LINES[0] });
  await expect(autoRows).toHaveCount(1);
  await autoRows.getByRole("button", { name: "Geri al" }).click();
  await expect(autoRows).toHaveCount(0);

  const rule = office.getByRole("row").filter({ hasText: "Deniz Ambalaj Ltd. Şti. → " + CORRECTED });
  await rule.getByRole("button", { name: "Kuralı kaldır" }).click();
  await office.getByRole("alertdialog").or(office.getByRole("dialog")).getByRole("button", { name: "Kuralı kaldır" }).click();
  await expect(office.getByText("Bu mükellef için kural yok.")).toBeVisible();
});
