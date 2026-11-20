import { randomUUID } from "node:crypto";

import { expect, test, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn, openNewEntry } from "./lib";

/**
 * Yevmiye fiş girişi alanları (#573, #597).
 *
 * Yeni fiş formunda:
 * - Hesap kodu mükellefin hesap planından aranır (kod ya da ad); öneriler
 *   `datalist` olarak gelir. Odak çıkınca kutunun altında hesabın adı,
 *   ana hesapsa "alt hesap seçin", planda yoksa "Hesap planında yok", biçim
 *   hatalıysa örnekli uyarı yazar.
 * - Tutarlar Türkçe yazılır ("1250,5" → "1.250,50"); bir satır ya borç ya
 *   alacaktır (biri yazılınca öbürü silinir); okunamayan tutar satır
 *   numarasıyla söylenir ve kaydetme kapanır.
 * - "Farkı kapat" farkı son boş satıra eksik tarafa yazar.
 * - Enter sonraki alana geçer (borç yazılmışsa alacak atlanır), son satırın
 *   sonunda yeni satır açar; Ctrl+Enter taslağı kaydeder.
 * Kaydedilen fişin satırları API'den doğrulanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve ona yüklenen küçük bir
 * Luca mizanı (hesap planı). Hesap adları ASCII ve koşuya özgü: ad araması
 * (ILIKE) harf tablosuna bağlı kalmaz. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Fiş Girişi ${run} Ltd. Şti.` };
const DESCRIPTION = `E2E fiş girişi ${run}`;
const CARGO = `KARGO GIDERLERI ${run}`;
const PARENT = `GENEL YONETIM GIDERLERI ${run}`;

/** Planı kuran Luca mizanı: 770 ana hesap, iki alt hesap; 320.01 satıcı. */
function lucaMizan() {
  const rows = [
    `FIS GIRISI ${run} GENEL GEÇİCİ MİZAN 01.01.2026 - 31.10.2026`,
    "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye",
    "320;SATICILAR;0,00;300,00;;300,00",
    "320.01;YURT ICI SATICILAR;0,00;300,00;;300,00",
    `770;${PARENT};300,00;0,00;300,00;`,
    "770.01;KIRTASIYE GIDERLERI;200,00;0,00;200,00;",
    `770.02;${CARGO};100,00;0,00;100,00;`,
  ];
  return {
    name: `luca-mizan-fis-${randomUUID().slice(0, 8)}.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(rows.join("\n") + "\n", "utf8"),
  };
}

type Draft = { uuid: string; description: string };
type DraftLine = { line_no: number; account: string; debit: string; credit: string };

let api: Api;
let office: Page;
let engagement = "";

const account = (n: number) => office.getByLabel(`Hesap kodu ${n}`, { exact: true });
const lineText = (n: number) => office.getByLabel(`Satır açıklaması ${n}`, { exact: true });
const debit = (n: number) => office.getByLabel(`Borç ${n}`, { exact: true });
const credit = (n: number) => office.getByLabel(`Alacak ${n}`, { exact: true });
const entryDescription = () => office.getByRole("textbox", { name: "Fiş açıklaması", exact: true }).first();
const accountStatus = (n: number) => account(n).locator("xpath=following-sibling::p[1]");

test.beforeAll(async ({ browser }) => {
  test.setTimeout(180_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "fisgirisi");
  const res = await api.uploadWith(
    `/v1/tenant/engagements/${engagement}/account-plan/imports`,
    lucaMizan(),
    { period: "2026-10" },
  );
  expect(res.status(), await res.text()).toBe(201);

  office = await (await browser.newContext()).newPage();
  await uiLogin(office, DEMO.owner);
});

test.afterAll(async () => {
  await office?.context().close();
  await api?.dispose();
});

test("hesap kodu plandan aranır; altında adı ya da uyarısı görünür (#573)", async () => {
  await office.goto(officePath(`/journals?taxpayer=${engagement}`));
  await openNewEntry(office);
  await expect(entryDescription()).toBeVisible();

  // Adla arama: "kargo" yalnız 770.02'yi önerir.
  await account(1).fill("kargo");
  const option = (code: string) => office.locator(`datalist#journal-new-accounts option[value="${code}"]`);
  await expect(option("770.02")).toHaveCount(1);
  await expect(option("770.01")).toHaveCount(0);

  // Kod seçilip alandan çıkılınca hesabın adı yazar.
  await account(1).fill("770.02");
  await account(1).press("Tab");
  await expect(accountStatus(1)).toHaveText(CARGO);
  await expect(account(1)).not.toHaveAttribute("aria-invalid", "true");

  // Ana hesap: alt hesap seçilmesi istenir.
  await account(2).fill("770");
  await account(2).press("Tab");
  await expect(accountStatus(2)).toHaveText(`Ana hesap: ${PARENT} · alt hesap seçin`);

  // Planda olmayan kod ve biçimi bozuk kod işaretlenir.
  await account(2).fill("999.99");
  await account(2).press("Tab");
  await expect(accountStatus(2)).toHaveText("Hesap planında yok");
  await expect(account(2)).toHaveAttribute("aria-invalid", "true");

  await account(2).fill("77x");
  await account(2).press("Tab");
  await expect(accountStatus(2)).toHaveText("Hesap kodu biçimi hatalı (ör. 770.01)");

  await account(2).fill("320.01");
  await account(2).press("Tab");
  await expect(accountStatus(2)).toHaveText("YURT ICI SATICILAR");
});

test("tutar Türkçe yazılır; satır tek taraflıdır; okunamayan tutar söylenir (#573)", async () => {
  await debit(1).fill("1250,5");
  await debit(1).press("Tab");
  await expect(debit(1)).toHaveValue("1.250,50");

  // Aynı satıra alacak yazılınca borç silinir.
  await credit(1).fill("10");
  await expect(debit(1)).toHaveValue("");
  await debit(1).fill("1.250,50");
  await expect(credit(1)).toHaveValue("");

  // Okunamayan tutar: satır numarasıyla uyarı, kaydetme kapalı.
  await credit(2).fill("12,3,4");
  await expect(office.getByText("2. satırdaki tutar okunamadı (ör. 1.250,50).")).toBeVisible();
  await expect(office.getByRole("button", { name: "Taslağı kaydet" })).toBeDisabled();
  await credit(2).fill("");

  // Fark canlı; "Farkı kapat" son boş satırın alacağına yazar.
  await expect(office.getByText(/^Fark: ₺\s?1\.250,50$/)).toBeVisible();
  await office.getByRole("button", { name: "Farkı kapat" }).click();
  await expect(credit(2)).toHaveValue("1.250,50");
  await expect(office.getByText("Dengeli", { exact: true })).toBeVisible();
  await expect(office.getByRole("button", { name: "Farkı kapat" })).toHaveCount(0);
});

test("Enter alanlar arasında ilerler, satır açar; Ctrl+Enter kaydeder (#573)", async () => {
  await entryDescription().fill(DESCRIPTION);

  // Son satırın son alanında Enter yeni satır açar ve hesabına odaklanır.
  await credit(2).press("Enter");
  await expect(account(3)).toBeFocused();
  await office.keyboard.type("770.01");
  await office.keyboard.press("Enter");
  await expect(lineText(3)).toBeFocused();
  await office.keyboard.type("Koli bandı");
  await office.keyboard.press("Enter");
  await expect(debit(3)).toBeFocused();
  await office.keyboard.type("100");
  // Borç yazıldı: alacak atlanır, yeni satır açılır.
  await office.keyboard.press("Enter");
  await expect(account(4)).toBeFocused();
  await office.keyboard.type("320.01");
  await office.keyboard.press("Enter");
  await office.keyboard.press("Enter");
  await expect(debit(4)).toBeFocused();
  // Borç boş: Enter alacağa geçer.
  await office.keyboard.press("Enter");
  await expect(credit(4)).toBeFocused();
  await office.keyboard.type("100");
  await expect(office.getByText("Dengeli", { exact: true })).toBeVisible();

  await office.keyboard.press("Control+Enter");
  // Kaydedilince pencere kapanır (#710); yeniden açılan form boştur.
  await expect(office.getByRole("dialog", { name: "Yeni fiş" })).toBeHidden();
  await openNewEntry(office);
  await expect(entryDescription()).toHaveValue("");
  await expect(account(3)).toHaveCount(0);
  await office.keyboard.press("Escape");

  const base = `/v1/tenant/engagements/${engagement}/journal-drafts`;
  const list = await api.json<{ items: Draft[] }>(await api.get(`${base}?status=draft&limit=50`));
  const draft = list.items.find((d) => d.description === DESCRIPTION);
  expect(draft, "kaydedilen taslak").toBeTruthy();
  const detail = await api.json<{ items: DraftLine[] }>(await api.get(`${base}/${draft!.uuid}`));
  const lines = [...detail.items]
    .sort((a, b) => a.line_no - b.line_no)
    .map((l) => [l.account, Number(l.debit), Number(l.credit)]);
  expect(lines).toEqual([
    ["770.02", 1250.5, 0],
    ["320.01", 0, 1250.5],
    ["770.01", 100, 0],
    ["320.01", 0, 100],
  ]);
});
