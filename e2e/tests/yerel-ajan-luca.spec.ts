import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Luca'nın API'si yok (#780 e): müşavir Luca'dan bordro dökümünü ve mizanı
 * izlenen klasöre bırakır, masaüstü ajanı `POST /v1/tenant/agents/{a}/documents`
 * ile yükler. Sunucu dosyayı içeriğinden tanır; mizan banner'ındaki VKN ile
 * mükellef bulunur ve elle yüklemeyle aynı içe aktarma çalışır (Luca'dan
 * alındı adımının okuduğu account_plan_imports satırı). Mükellefi
 * anlaşılamayan bordro dökümü "eşleştir" kaydı olur; ofis mükellefi ve dönemi
 * seçince bordro dönemi oluşur. Yerel ajanlar sayfası bekleyen dosyayı ve
 * "Eşleştir" iletişimini gösterir.
 *
 * Veri: kendi açtığı yeni mükellef ve yeni bir ajan kaydı; demo
 * mükelleflerine dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const vkn = uniqueVkn();
const title = `Ajan Luca ${stamp % 100000} Ltd. Şti.`;
let api: Api;
let engagement = "";
let agent = "";

type Doc = {
  uuid: string;
  kind: string;
  status: string;
  period: string | null;
  match_source: string;
  engagement_uuid: string | null;
  replayed: boolean;
  file_name: string;
};

const csv = (name: string, body: string) => ({ name, mimeType: "text/csv", buffer: Buffer.from(body, "utf-8") });

const mizan = (banner: string) =>
  `${banner}\nMizan 01.01.2026 - 30.09.2026\n` +
  "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n" +
  "100;KASA;10.000,00;4.000,00;6.000,00;0,00\n" +
  "320;SATICILAR;0,00;5.000,00;0,00;5.000,00\n" +
  "600;YURT İÇİ SATIŞLAR;0,00;1.000,00;0,00;1.000,00\n";

const payroll =
  `LUCA BORDRO DÖKÜMÜ ${stamp}\n\n` +
  "Adı Soyadı;Gün;Brüt Ücret;SGK İşçi Payı;İşsizlik İşçi Payı;Ödenecek Gelir Vergisi;Damga Vergisi;Net Ücret\n" +
  "Ali Veli;30;30.000,00;4.200,00;300,00;1.500,00;100,00;23.900,00\n";

test.beforeAll(async ({ browser }) => {
  test.setTimeout(180_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, { vkn, title }, "ajan-luca");
  const reg = await api.json<{ uuid: string }>(
    await api.post("/v1/tenant/agents", {
      machine_id: `e2e-ajan-luca-${stamp}`,
      name: "E2E ajan",
      platform: "windows",
      version: "1.0.0",
    }),
  );
  agent = reg.uuid;
});

test.afterAll(async () => {
  await api?.dispose();
});

test("Mizan VKN ile mükellefe gider ve içe aktarılır; aynı dosya tekrar yüklenmez", async () => {
  const file = csv("mizan.csv", mizan(`${title} VKN: ${vkn}`));
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, { file, folder: "C:\\Luca\\Mizan" });
  expect(res.status(), await res.text()).toBe(201);
  const doc = await api.json<Doc>(res);
  expect(doc).toMatchObject({
    kind: "trial_balance",
    status: "imported",
    period: "2026-09",
    match_source: "content",
    engagement_uuid: engagement,
  });

  // Elle yüklemeyle aynı satır: Luca'dan alındı adımı bunu okur.
  const imports = await api.json<{ items: { source_kind: string; period: string | null }[] }>(
    await api.get(`/v1/tenant/engagements/${engagement}/account-plan/imports`),
  );
  expect(imports.items.some((i) => i.source_kind === "trial_balance" && i.period === "2026-09")).toBeTruthy();

  const again = await api.postForm(`/v1/tenant/agents/${agent}/documents`, { file, folder: "C:\\Luca\\Mizan" });
  expect(again.status()).toBe(200);
  expect((await api.json<Doc>(again)).replayed).toBe(true);
});

test("Mükellefi anlaşılamayan bordro dökümü eşleştirilir ve bordro dönemi oluşur", async () => {
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, {
    file: csv("bordro_dokumu.csv", payroll),
    folder: "C:\\Luca\\Genel",
  });
  expect(res.status(), await res.text()).toBe(201);
  const doc = await api.json<Doc>(res);
  expect(doc).toMatchObject({ kind: "payroll", status: "needs_match", engagement_uuid: null });

  const open = await api.json<{ items: Doc[] }>(await api.get("/v1/tenant/agent-documents?status=open&limit=200"));
  expect(open.items.some((d) => d.uuid === doc.uuid)).toBeTruthy();

  const resolved = await api.json<Doc>(
    await api.post(`/v1/tenant/agent-documents/${doc.uuid}/resolve`, {
      engagement_uuid: engagement,
      kind: "payroll",
      period: "2026-09",
    }),
  );
  expect(resolved).toMatchObject({ status: "imported", match_source: "office", period: "2026-09" });

  const runs = await api.json<{ items: { period: string }[] }>(
    await api.get(`/v1/tenant/engagements/${engagement}/payroll/runs`),
  );
  expect(runs.items.some((r) => r.period === "2026-09")).toBeTruthy();

  // İçe aktarılan dosya yeniden eşleştirilmez.
  const twice = await api.post(`/v1/tenant/agent-documents/${doc.uuid}/resolve`, {
    engagement_uuid: engagement,
    kind: "payroll",
  });
  expect(twice.status()).toBe(409);
});

test("Yerel ajanlar sayfası bekleyen dosyayı ve Eşleştir iletişimini gösterir", async ({ page }) => {
  const name = `tanimsiz-${stamp}.csv`;
  const res = await api.postForm(`/v1/tenant/agents/${agent}/documents`, {
    file: csv(name, `Liste ${stamp}\nAd;Not\nBir;İki\n`),
    folder: "C:\\Luca\\Genel",
  });
  expect(res.status(), await res.text()).toBe(201);
  expect((await api.json<Doc>(res)).kind).toBe("unknown");

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/agents"));
  await expect(page.getByText(/Ajandan gelen dosyalar/).first()).toBeVisible();
  const row = page.getByText(name).first();
  await expect(row).toBeVisible();
  await page.getByRole("button", { name: "Eşleştir" }).first().click();
  await expect(page.getByRole("dialog").getByText("Dosyayı eşleştir")).toBeVisible();
  await page.getByRole("button", { name: "Vazgeç" }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
});
