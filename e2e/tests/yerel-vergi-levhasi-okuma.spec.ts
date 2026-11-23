import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin, uniqueEmail, uniqueVkn } from "./lib";

/**
 * Mükellef eklerken vergi levhası okuma (Oğuz'un geri bildirimi): ofis
 * levhanın fotoğrafını yükler, yapay zekâ VKN/TCKN, unvan, vergi dairesi,
 * adres ve faaliyet kodunu forma yazar. Okunamayan / emin olunamayan /
 * geçersiz alanın yanında "Kontrol et" çıkar; hiçbir şey kendiliğinden
 * kaydedilmez, ofis "Onay iste"ye basınca talep vergi dairesi, adres ve
 * faaliyet koduyla saklanır. Taranmış PDF ve HEIC açık hatayla döner.
 * Ofis ayarı kapalıyken düğme görünmez, uç 403 döner.
 *
 * Yapay zekâ e2e'de yok: okuma ucu diğer fiş testlerindeki gibi tarayıcıda
 * sahte yanıtla kesilir (page.route). Dosya türü ve ayar denetimleri yapay
 * zekâdan önce çalıştığı için gerçek uçla denenir. Ofis ayarı testte
 * değişir, `finally` içinde eski hâline döner.
 */
test.describe.configure({ mode: "serial" });

const READ = "/v1/tenant/tax-certificates/read";
// 1×1 PNG: tarayıcıdaki sahte yanıt dosyanın içine bakmaz.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64",
);

type Field = { value: string; confidence: number; check: boolean; problem?: string };
type Read = {
  fields: Record<"tax_id" | "title" | "tax_office" | "address" | "activity_code", Field>;
  tax_id_type: string;
  kind: string;
  model: string;
  check_fields: string[];
};
type Settings = { settings: Record<string, unknown> };
type Requests = {
  items: { tax_id: string; title: string; tax_office: string; address: string; activity_code: string }[];
};

const ok = (value: string, confidence = 0.95): Field => ({ value, confidence, check: false });

/** Okuma ucunu sahte yanıtla keser; gelen isteğin çok parçalı olduğunu kaydeder. */
async function mockRead(page: Page, read: Read) {
  const seen: string[] = [];
  await page.route(`**${READ}`, async (route) => {
    seen.push(route.request().headers()["content-type"] ?? "");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ success: true, data: read, meta: { request_id: "e2e" } }),
    });
  });
  return seen;
}

async function upload(page: Page) {
  await page.getByLabel("Vergi levhası dosyası").setInputFiles({
    name: "vergi-levhasi.png",
    mimeType: "image/png",
    buffer: PNG,
  });
}

test("levha okunur, form dolar; eksik alan 'Kontrol et'; kayıt ofisin onayıyla", async ({ page }) => {
  const run = `${Date.now()}`.slice(-6);
  const vkn = uniqueVkn();
  const title = `Levha Gıda ${run} A.Ş.`;
  const seen = await mockRead(page, {
    fields: {
      tax_id: ok(vkn),
      title: { value: title, confidence: 0.6, check: true, problem: "low_confidence" },
      tax_office: ok("Kadıköy"),
      address: ok("Caferağa Mah. Moda Cad. No:12 Kadıköy/İstanbul"),
      activity_code: { value: "", confidence: 0, check: true, problem: "missing" },
    },
    tax_id_type: "vkn",
    kind: "legal",
    model: "e2e",
    check_fields: ["title", "activity_code"],
  });

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/taxpayer-requests"));
  const fill = page.getByTestId("tax-certificate-fill");
  await expect(fill.getByText("Vergi levhasından doldur")).toBeVisible();
  await upload(page);

  await expect(fill.getByRole("status")).toContainText("Kontrol et");
  expect(seen[0]).toContain("multipart/form-data");
  await expect(page.locator('input[name="tax_id"]')).toHaveValue(vkn);
  await expect(page.locator('input[name="title"]')).toHaveValue(title);
  await expect(page.locator('input[name="tax_office"]')).toHaveValue("Kadıköy");
  await expect(page.locator('input[name="address"]')).toHaveValue(
    "Caferağa Mah. Moda Cad. No:12 Kadıköy/İstanbul",
  );
  await expect(page.locator('input[name="activity_code"]')).toHaveValue("");
  // Unvan (emin olunamadı) ve faaliyet kodu (okunamadı) işaretli.
  await expect(page.getByTestId("cert-check")).toHaveCount(2);
  await expect(page.getByText("Emin olunamadı; levhayla karşılaştırın.")).toBeVisible();
  await expect(page.getByText("Levhada okunamadı; elle girin.")).toBeVisible();

  // Kendiliğinden kayıt yok: talep listesinde bu VKN henüz yok.
  const api = await Api.login(DEMO.owner);
  try {
    const before = await api.json<Requests>(await api.get("/v1/tenant/taxpayer-requests?limit=100"));
    expect(before.items.some((r) => r.tax_id === vkn)).toBe(false);

    // Ofis kontrol eder, eksik kodu girer ve onay ister.
    await page.locator('input[name="activity_code"]').fill("471101");
    await page.locator('input[name="contact_email"]').fill(uniqueEmail("levha"));
    await page.getByRole("button", { name: "Onay iste" }).click();
    await expect(page.getByText("Mükellefe onay gönderildi").first()).toBeVisible();
    await expect(page.getByTestId("cert-check")).toHaveCount(0);

    const after = await api.json<Requests>(await api.get("/v1/tenant/taxpayer-requests?limit=100"));
    expect(after.items.find((r) => r.tax_id === vkn)).toMatchObject({
      title,
      tax_office: "Kadıköy",
      address: "Caferağa Mah. Moda Cad. No:12 Kadıköy/İstanbul",
      activity_code: "47.11.01",
    });
  } finally {
    await api.dispose();
  }
});

test("geçersiz VKN okunursa rakamlar kalır, 'Kontrol et' ve düzeltme notu çıkar", async ({ page }) => {
  await mockRead(page, {
    fields: {
      tax_id: { value: "1234567891", confidence: 0.97, check: true, problem: "invalid" },
      title: ok("Deneme Ltd. Şti."),
      tax_office: ok("Beşiktaş"),
      address: ok("Sinanpaşa Mah. No:3 Beşiktaş/İstanbul"),
      activity_code: ok("62.01.01"),
    },
    tax_id_type: "",
    kind: "",
    model: "e2e",
    check_fields: ["tax_id"],
  });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/taxpayer-requests"));
  await upload(page);
  await expect(page.locator('input[name="tax_id"]')).toHaveValue("1234567891");
  await expect(page.getByTestId("cert-check")).toHaveCount(1);
  await expect(page.getByText(/Okunan değer geçersiz/)).toBeVisible();
  await expect(page.locator('input[name="activity_code"]')).toHaveValue("62.01.01");
});

test("taranmış PDF ve HEIC açık hata; ofis ayarı kapalıyken düğme yok, uç 403", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const original = (await api.json<Settings>(await api.get("/v1/tenant/settings"))).settings;
  try {
    // Yazısız PDF: yapay zekâya gitmeden SCANNED_PDF.
    const pdf = await api.upload(READ, {
      name: "levha.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4\n%%EOF\n"),
    });
    expect(pdf.status(), await pdf.text()).toBe(400);
    expect((await pdf.json()).error.code).toBe("SCANNED_PDF");

    const heic = await api.upload(READ, {
      name: "levha.heic",
      mimeType: "image/heic",
      buffer: Buffer.concat([Buffer.from([0, 0, 0, 24]), Buffer.from("ftypheic0000mif1heic")]),
    });
    expect(heic.status(), await heic.text()).toBe(400);
    expect((await heic.json()).error.code).toBe("HEIC_NOT_SUPPORTED");

    // Ekranda Türkçe hata (gerçek uç).
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/taxpayer-requests"));
    await page.getByLabel("Vergi levhası dosyası").setInputFiles({
      name: "levha.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4\n%%EOF\n"),
    });
    await expect(page.getByTestId("tax-certificate-fill").getByRole("alert")).toContainText(
      "Bu PDF taranmış bir görüntü",
    );

    // Ayardan kapat: düğme görünmez, uç 403.
    await page.goto(officePath("/settings"));
    const toggle = page.getByRole("switch", { name: "Mükellef eklerken vergi levhasını yapay zekâ okusun" });
    await expect(toggle).toBeChecked();
    await toggle.click();
    await expect(toggle).not.toBeChecked();
    await expect
      .poll(async () => (await api.json<Settings>(await api.get("/v1/tenant/settings"))).settings.tax_certificate_ai)
      .toBe(false);

    const off = await api.upload(READ, { name: "levha.png", mimeType: "image/png", buffer: PNG });
    expect(off.status()).toBe(403);
    expect((await off.json()).error.code).toBe("TAX_CERTIFICATE_READING_OFF");

    await page.goto(officePath("/taxpayer-requests"));
    await expect(page.locator('input[name="tax_id"]')).toBeVisible();
    await expect(page.getByTestId("tax-certificate-fill")).toHaveCount(0);
  } finally {
    await api.patch("/v1/tenant/settings", { settings: original });
    await api.dispose();
  }
});
