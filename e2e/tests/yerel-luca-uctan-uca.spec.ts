import { readFileSync } from "node:fs";
import { inflateRawSync } from "node:zlib";

import { expect, test, type Page } from "@playwright/test";

import {
  addConsentedTaxpayerWithUser,
  Api,
  DEMO,
  officePath,
  settledPopover,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Luca uçtan uca (Oğuz'un önceliği): mükellefin telefondan (mobil
 * uygulamanın ucu `POST /v1/taxpayer/receipts`) ve ofisin WhatsApp
 * uzantısıyla (`POST .../whatsapp/captures`, "Gider" / "Gelir olarak
 * kaydet") gönderdiği masraf fişi ve Z raporu ofise ulaşır; ofis Luca'ya
 * iki yoldan aktarır:
 *
 *   - Masraflar / Z raporları listesinin "Luca'ya aktar (Excel)" dosyası
 *     (her KDV oranı kendi satırında, borç = alacak); ikinci çıktı aynı
 *     belgeleri almaz (çift aktarım koruması);
 *   - onaylanan yevmiye fişlerinin Luca fiş dosyası (journal-exports): liste
 *     dosyasıyla gitmiş belge DOCUMENT_ALREADY_EXPORTED ile durur, yeniden
 *     dahil edilince aynı satırlarla gider.
 *
 * Yapay zekâ: e2e ortamında AI anahtarı yok; fotoğraflar "okunamayan" olur
 * ve taslak açılmaz (bkz. yerel-z-parcali.spec.ts). O zaman yükleme adımı
 * fotoğrafların ofise ulaştığını denetler; Luca adımları aynı fişleri ofisin
 * elle girişiyle (masraf: üç oranlı ve tek oranlı) ve Z raporu Excel'iyle
 * (iki yazarkasa, %1 / %10 / %20) kurar. AI açıkken (ya da sahtesiyle)
 * fotoğraflardan okunan kayıtların kendisi kullanılır. Fotoğraftan Luca'ya
 * okuma dahil tüm yol Go'da yapay zekâ sahtesiyle de denenir
 * (backend/internal/httpserver/luca_phone_whatsapp_db_test.go).
 *
 * Veri: her koşuda yeni mükellef ve kullanıcısı (uydurma), Ekim 2026.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Luca Uçtan Uca ${run} A.Ş.` };
const PERIOD = "2026-10";

type Capture = {
  uuid: string;
  status: string;
  draft: { type: string; uuid: string } | null;
};
type JournalRef = { uuid: string } | null;
type Expense = {
  uuid: string;
  period: string | null;
  receipt_number: string | null;
  amount: string | null;
  journal: JournalRef;
};
type RetailSale = {
  uuid: string;
  period: string | null;
  z_report_number: string | null;
  total_amount: string | null;
  journal: JournalRef;
};
/** Luca'ya giden bir belge ve dosyada beklenen KDV satırları. */
type Doc = {
  uuid: string;
  no: string;
  period: string;
  total: number;
  vat: number[];
};

let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;
let aiRead = false;
const expenses: Doc[] = [];
const zReports: Doc[] = [];
const base = () => `/v1/tenant/engagements/${engagement}`;

/** Ayırt edilebilir, PNG olarak tanınan küçük bir "fotoğraf". */
const png = (name: string) => ({
  name,
  mimeType: "image/png",
  buffer: Buffer.concat([
    Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
    Buffer.from(`${name}-${run}-${Math.random()}`),
  ]),
});

// ---- xlsx okuma (yalnızca Node) ----

function unzip(buf: Buffer): Map<string, Buffer> {
  const eocd = buf.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  expect(eocd, "zip sonu").toBeGreaterThan(0);
  const count = buf.readUInt16LE(eocd + 10);
  let off = buf.readUInt32LE(eocd + 16);
  const out = new Map<string, Buffer>();
  for (let i = 0; i < count; i++) {
    const method = buf.readUInt16LE(off + 10);
    const size = buf.readUInt32LE(off + 20);
    const nlen = buf.readUInt16LE(off + 28);
    const elen = buf.readUInt16LE(off + 30);
    const clen = buf.readUInt16LE(off + 32);
    const local = buf.readUInt32LE(off + 42);
    const name = buf.subarray(off + 46, off + 46 + nlen).toString("utf8");
    const start =
      local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const data = buf.subarray(start, start + size);
    out.set(name, method === 0 ? Buffer.from(data) : inflateRawSync(data));
    off += 46 + nlen + elen + clen;
  }
  return out;
}

const xmlText = (s: string) =>
  s
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, "&");

const colIndex = (ref: string) =>
  [...ref.replace(/\d+/g, "")].reduce((n, c) => n * 26 + c.charCodeAt(0) - 64, 0) - 1;

/** İlk sayfanın bütün satırları (paylaşılan / satır içi metin, sayı). */
function sheetRows(xlsx: Buffer): string[][] {
  const files = unzip(xlsx);
  const shared: string[] = [];
  const sst = files.get("xl/sharedStrings.xml")?.toString("utf8") ?? "";
  for (const si of sst.match(/<si>[\s\S]*?<\/si>/g) ?? []) {
    shared.push(
      [...si.matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)].map((m) => xmlText(m[1])).join(""),
    );
  }
  const sheetName = [...files.keys()].sort().find((k) => /^xl\/worksheets\/sheet\d+\.xml$/.test(k));
  expect(sheetName, "sayfa").toBeTruthy();
  const sheet = files.get(sheetName as string)!.toString("utf8");
  const rows: string[][] = [];
  for (const row of sheet.match(/<row\b[^>]*>[\s\S]*?<\/row>/g) ?? []) {
    const out: string[] = [];
    for (const c of row.matchAll(/<c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/c>)/g)) {
      const attrs = c[1];
      const body = c[2] ?? "";
      const ref = /\br="([A-Z]+\d+)"/.exec(attrs)?.[1];
      const t = /\bt="([^"]+)"/.exec(attrs)?.[1];
      const v = /<v>([\s\S]*?)<\/v>/.exec(body)?.[1] ?? "";
      let text = xmlText(v);
      if (t === "s") text = shared[Number(v)] ?? "";
      if (t === "inlineStr") text = xmlText(/<t[^>]*>([\s\S]*?)<\/t>/.exec(body)?.[1] ?? "");
      out[ref ? colIndex(ref) : out.length] = text;
    }
    rows.push(Array.from(out, (x) => x ?? ""));
  }
  return rows;
}

type LucaLine = { fis: string; account: string; docNo: string; detail: string; debit: number; credit: number };

/** Luca fiş dosyasının satırları; sütunlar başlıktan bulunur. */
function lucaLines(xlsx: Buffer): LucaLine[] {
  expect(xlsx.subarray(0, 2).toString()).toBe("PK");
  const [header, ...rows] = sheetRows(xlsx);
  const col = (h: string) => {
    const i = header.indexOf(h);
    expect(i, `Luca başlığında "${h}" (${header.join(" | ")})`).toBeGreaterThanOrEqual(0);
    return i;
  };
  const [fis, account, docNo, detail, debit, credit] = [
    "Fiş No",
    "Hesap Kodu",
    "Evrak No",
    "Detay Açıklama",
    "Borç",
    "Alacak",
  ].map(col);
  const num = (s: string | undefined) => Number((s ?? "").replace(",", ".")) || 0;
  return rows
    .filter((r) => r.some((x) => x !== ""))
    .map((r) => ({
      fis: r[fis] ?? "",
      account: r[account] ?? "",
      docNo: r[docNo] ?? "",
      detail: r[detail] ?? "",
      debit: num(r[debit]),
      credit: num(r[credit]),
    }));
}

const round2 = (n: number) => Math.round(n * 100) / 100;

/** Belgenin fişi (Evrak No ile), borç = alacak = toplam, KDV satırları. */
function expectVoucher(lines: LucaLine[], doc: Doc, vatMain: string) {
  const fisNo = lines.find((l) => l.docNo === doc.no)?.fis;
  expect(fisNo, `dosyada ${doc.no} fişi`).toBeTruthy();
  const own = lines.filter((l) => l.fis === fisNo);
  const debit = round2(own.reduce((s, l) => s + l.debit, 0));
  const credit = round2(own.reduce((s, l) => s + l.credit, 0));
  expect(debit, `${doc.no} borç = alacak`).toBe(credit);
  expect(debit, `${doc.no} toplam`).toBe(doc.total);
  const vat = own
    .filter((l) => l.account.startsWith(vatMain))
    .map((l) => round2(l.debit || l.credit))
    .sort((a, b) => a - b);
  expect(vat, `${doc.no} KDV satırları`).toEqual([...doc.vat].sort((a, b) => a - b));
  return own;
}

const MONTHS = ["Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"];

/** Ay seçicide (MonthPicker) dönemi seçer: yıl oklarla, ay kısa adıyla. */
async function pickMonth(page: Page, triggerId: string, period: string) {
  const [y, m] = period.split("-").map(Number);
  await page.locator(`#${triggerId}`).click();
  const pop = await settledPopover(page);
  const year = pop.locator('span[aria-live="polite"]');
  for (let i = 0; i < 5; i++) {
    const cur = Number(await year.textContent());
    if (cur === y) break;
    await pop.getByRole("button", { name: cur < y ? "Sonraki yıl" : "Önceki yıl" }).click();
  }
  await expect(year).toHaveText(String(y));
  await pop.getByRole("button", { name: MONTHS[m - 1], exact: true }).click();
}

async function download(page: Page, click: () => Promise<void>): Promise<Buffer> {
  const [d] = await Promise.all([page.waitForEvent("download"), click()]);
  return readFileSync((await d.path()) as string);
}

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "lucauc");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office?.dispose();
});

test("telefondan ve WhatsApp'tan gelen fiş ve Z raporu ofise ulaşır", async () => {
  test.setTimeout(120_000);
  const tp = await Api.login(tpLogin.email, tpLogin.password);
  const headers = { "X-Taxpayer": taxpayerUuid };
  const phone = async (form: Record<string, string | ReturnType<typeof png>>, key: string) => {
    const res = await tp.postForm("/v1/taxpayer/receipts", form, { ...headers, "Idempotency-Key": `lucauc-${key}-${run}` });
    expect(res.status(), await res.text()).toBe(201);
    return (await res.json()).data as Capture;
  };
  const phoneExpense = await phone({ image: png("fis.png"), kind: "expense_receipt" }, "fis");
  // Uzun Z raporu iki parça fotoğraf.
  const phoneZ = await phone({ image: png("z-1.png"), image_2: png("z-2.png"), kind: "z_report" }, "z");
  await tp.dispose();

  const whatsapp = async (target: string, name: string) => {
    const res = await office.postForm(`${base()}/whatsapp/captures`, {
      target,
      chat_key: `90555${run.padStart(7, "0")}@c.us`,
      file: png(name),
    });
    expect(res.status(), await res.text()).toBe(201);
    const v = (await res.json()).data as { receipt_uuid: string | null };
    expect(v.receipt_uuid, `${target} fotoğrafı fiş okumaya gitti`).toBeTruthy();
    return v.receipt_uuid as string;
  };
  const waExpense = await whatsapp("expense", "wa-fis.png");
  const waZ = await whatsapp("income", "wa-z.png");

  // Dördü de ofisin fiş listesinde (onay / okunamayan).
  const list = await office.json<{ items: Capture[] }>(await office.get(`${base()}/receipts?limit=100`));
  const byId = new Map(list.items.map((c) => [c.uuid, c]));
  for (const id of [phoneExpense.uuid, phoneZ.uuid, waExpense, waZ]) {
    expect(byId.has(id), `ofiste ${id}`).toBe(true);
  }

  aiRead = phoneExpense.status === "draft" && phoneExpense.draft?.type === "expense";
  if (!aiRead) {
    test.info().annotations.push({
      type: "not",
      description: `Yapay zekâ bu ortamda yok (yükleme: ${phoneExpense.status}); Luca adımları ofisin elle girdiği aynı fişlerle.`,
    });
    for (const id of [phoneExpense.uuid, phoneZ.uuid, waExpense, waZ]) {
      expect(byId.get(id)?.status).toBe("failed");
    }
    return;
  }
  // AI ile: taslakların kendisi Luca'ya gider.
  const captures = [phoneExpense, phoneZ, byId.get(waExpense)!, byId.get(waZ)!];
  for (const c of captures) {
    expect(c.status, `yükleme ${c.uuid}`).toBe("draft");
    if (c.draft?.type === "expense") {
      const e = await office.json<Expense & { vat_lines: { tax_amount: string }[]; tax_amount: string | null }>(
        await office.get(`${base()}/expenses/${c.draft.uuid}`),
      );
      const vat = e.vat_lines.length ? e.vat_lines.map((l) => Number(l.tax_amount)) : [Number(e.tax_amount)];
      expenses.push({ uuid: e.uuid, no: e.receipt_number!, period: e.period!, total: Number(e.amount), vat });
    } else {
      const z = await office.json<RetailSale & { vat_lines: { tax_amount: string }[] }>(
        await office.get(`${base()}/retail-sales/${c.draft!.uuid}`),
      );
      zReports.push({
        uuid: z.uuid,
        no: z.z_report_number!,
        period: z.period!,
        total: Number(z.total_amount),
        vat: z.vat_lines.map((l) => Number(l.tax_amount)),
      });
    }
  }
  expect(expenses).toHaveLength(2);
  expect(zReports).toHaveLength(2);
});

test("AI yoksa ofis aynı fişleri elle girer, Z raporlarını Excel'le yükler", async () => {
  test.skip(aiRead, "Fotoğraflar okundu; taslaklar kullanılıyor.");
  // Telefondaki market fişi: %1 / %10 / %20.
  const multi = await office.json<Expense>(
    await office.post(`${base()}/expenses`, {
      expense_date: "2026-10-14",
      vendor_name: "Demo Market Gıda Ltd.",
      vendor_tax_number: "4560012345",
      receipt_number: `M${run}`,
      payment_method: "cash",
      vat_lines: [
        { tax_rate: "1", tax_base: "100.00", tax_amount: "1.00" },
        { tax_rate: "10", tax_base: "200.00", tax_amount: "20.00" },
        { tax_rate: "20", tax_base: "500.00", tax_amount: "100.00" },
      ],
    }),
  );
  // WhatsApp'taki akaryakıt fişi: tek oran %20.
  const fuel = await office.json<Expense>(
    await office.post(`${base()}/expenses`, {
      expense_date: "2026-10-14",
      vendor_name: "Örnek Akaryakıt A.Ş.",
      vendor_tax_number: "1234567890",
      receipt_number: `W${run}`,
      payment_method: "cash",
      vat_lines: [{ tax_rate: "20", tax_base: "1041.67", tax_amount: "208.33" }],
    }),
  );
  expenses.push(
    { uuid: multi.uuid, no: `M${run}`, period: PERIOD, total: 921, vat: [1, 20, 100] },
    { uuid: fuel.uuid, no: `W${run}`, period: PERIOD, total: 1250, vat: [208.33] },
  );

  // İki yazarkasanın aynı günkü Z raporları (telefon ve WhatsApp).
  const z = await office.upload(`${base()}/retail-sales/import`, {
    name: `z-${run}.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(
      "Tarih;ÖKC Seri No;Z No;Nakit;Kart;%1 Matrah;%1 KDV;%10 Matrah;%10 KDV;%20 Matrah;%20 KDV;Toplam\n" +
        `15.10.2026;TR0000001;1${run.slice(-3)};2130,00;3300,00;1000,00;10,00;1000,00;100,00;2618,00;702,00;5430,00\n` +
        `15.10.2026;TR0000002;2${run.slice(-3)};1000,00;1000,00;;;;;1666,67;333,33;2000,00\n`,
      "utf8",
    ),
  });
  expect(z.status(), await z.text()).toBeLessThan(300);
  const sales = await office.json<{ items: RetailSale[] }>(
    await office.get(`${base()}/retail-sales?period=${PERIOD}&limit=100`),
  );
  const one = sales.items.find((s) => s.z_report_number === `1${run.slice(-3)}`);
  const two = sales.items.find((s) => s.z_report_number === `2${run.slice(-3)}`);
  expect(one && two, "iki Z raporu").toBeTruthy();
  zReports.push(
    { uuid: one!.uuid, no: one!.z_report_number!, period: PERIOD, total: 5430, vat: [10, 100, 702] },
    { uuid: two!.uuid, no: two!.z_report_number!, period: PERIOD, total: 2000, vat: [333.33] },
  );
});

test("Masraflar: Luca'ya aktar (Excel) KDV oranlarını ayrı satırda yazar, ikinci aktarım boş", async ({ page }) => {
  test.setTimeout(120_000);
  await uiLogin(page, DEMO.owner);
  for (const period of [...new Set(expenses.map((e) => e.period))]) {
    await page.goto(officePath(`/engagements/${engagement}/expenses`));
    await page.getByTestId("expense-luca-open").click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await pickMonth(page, "expense-luca-period", period);
    const lines = lucaLines(await download(page, () => dialog.getByTestId("expense-luca-download").click()));
    for (const doc of expenses.filter((e) => e.period === period)) {
      const own = expectVoucher(lines, doc, "191");
      // Okunan / girilen oran: "kırılım yok" işareti yok.
      for (const l of own) expect(l.detail).not.toContain("kırılımı yok");
    }
    // İkinci aktarım: aynı masraflar yeniden girmez, dosya inmez.
    await page.getByTestId("expense-luca-open").click();
    await pickMonth(page, "expense-luca-period", period);
    await page.getByRole("dialog").getByTestId("expense-luca-download").click();
    await expect(page.getByText("Bu dönemde Luca'ya aktarılacak yeni masraf yok.", { exact: false })).toBeVisible();
    await page.keyboard.press("Escape");
  }
});

test("Z raporları: Luca'ya aktar (Excel) yazarkasa başına fiş, oran başına satır; ikinci aktarım boş", async ({ page }) => {
  test.setTimeout(120_000);
  await uiLogin(page, DEMO.owner);
  for (const period of [...new Set(zReports.map((z) => z.period))]) {
    await page.goto(officePath(`/engagements/${engagement}/z-reports?period=${period}`));
    await page.getByTestId("luca-list-open").click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const lines = lucaLines(await download(page, () => dialog.getByTestId("luca-list-download").click()));
    for (const doc of zReports.filter((z) => z.period === period)) {
      const own = expectVoucher(lines, doc, "391");
      // Satış (600) da oran başına bir satır.
      expect(own.filter((l) => l.account.startsWith("600"))).toHaveLength(doc.vat.length);
    }
    await expect(dialog.getByTestId("luca-list-result")).toBeVisible();
    await dialog.getByTestId("luca-list-download").click();
    await expect(dialog.getByTestId("luca-list-empty")).toBeVisible();
  }
});

test("fiş dosyası: listeyle giden belge DOCUMENT_ALREADY_EXPORTED ile durur, yeniden dahil edilince aynı satırlarla gider", async () => {
  test.setTimeout(120_000);
  const docs = [
    { doc: expenses[0], kind: "expenses", key: "exp", vat: "191" },
    { doc: zReports[0], kind: "retail-sales", key: "z", vat: "391" },
  ];
  const drafts: string[] = [];
  for (const d of docs) {
    const ok = await office.post(`${base()}/${d.kind}/${d.doc.uuid}/approve`, {});
    expect(ok.status(), await ok.text()).toBeLessThan(300);
    const got = await office.json<{ journal: JournalRef }>(await office.get(`${base()}/${d.kind}/${d.doc.uuid}`));
    expect(got.journal, `${d.doc.no} yevmiye fişi`).toBeTruthy();
    const draft = got.journal!.uuid;
    let res = await office.post(`${base()}/journal-drafts/${draft}/approve`, {
      confirm_warnings: true,
      note: "uçtan uca test",
    });
    if (res.status() === 409) {
      res = await office.post(`${base()}/journal-drafts/${draft}/approve`, {
        confirm_warnings: true,
        note: "uçtan uca test",
        send_duplicate: true,
        reason: "uçtan uca test",
      });
    }
    expect(res.status(), await res.text()).toBeLessThan(300);
    drafts.push(draft);
  }

  // Liste dosyasıyla gitti: koruma durdurur.
  const blocked = await office.post(`${base()}/journal-exports`, { driver: "luca", drafts });
  const body = await blocked.text();
  expect(blocked.status(), body).toBe(422);
  const err = JSON.parse(body).error as { code: string; details?: { code: string }[] };
  expect(err.code).toBe("EXPORT_BLOCKED");
  expect(err.details?.map((x) => x.code)).toContain("DOCUMENT_ALREADY_EXPORTED");

  // Yeniden dahil edilince gider; dosyada aynı KDV satırları.
  const batch = await office.json<{ uuid: string; files: { name: string }[] }>(
    await office.post(`${base()}/journal-exports`, {
      driver: "luca",
      drafts,
      confirm_documents: docs.map((d) => `${d.key}:${d.doc.uuid}`),
    }),
  );
  expect(batch.files).toHaveLength(1);
  const file = await office.get(`${base()}/journal-exports/${batch.uuid}/files/${encodeURIComponent(batch.files[0].name)}`);
  expect(file.status()).toBe(200);
  const lines = lucaLines(Buffer.from(await file.body()));
  for (const d of docs) expectVoucher(lines, d.doc, d.vat);
});
