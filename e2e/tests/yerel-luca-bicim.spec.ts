import { inflateRawSync } from "node:zlib";

import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Luca biçimleri (Oğuz'un geri bildirimi):
 * - "Luca'ya aktar" fiş dosyası .xlsx'tir ve tek başlık satırı Luca'nın fiş
 *   Excel'indeki 15 sütundur (Fiş No … TC/Vergi No). Dosya testte açılıp
 *   başlık satırı okunur (e2e'de xlsx kütüphanesi yok; küçük bir zip okuyucu
 *   yeter). Fiş / KDV satırı / denge ayrıntısı Go testlerinde.
 * - Defterler: Luca mizanı (5 satır üst bilgi, 6. satır başlık) yüklenince
 *   kendiliğinden "Mizan" olur; dönem üst bilgiden, toplamlar ana hesaplardan
 *   okunur. Dosya testte üretilir; firma ve tutarlar uydurmadır.
 *
 * Veri: her koşuda yeni mükellef; fiş, onay, aktarım ve yükleme API ile.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Luca Biçim ${run} Ltd. Şti.` };
const PERIOD = "2026-10";

const LUCA_HEADER = [
  "Fiş No",
  "Fiş Tarihi",
  "Fiş Açıklama",
  "Hesap Kodu",
  "Evrak No",
  "Evrak Tarihi",
  "Detay Açıklama",
  "Borç",
  "Alacak",
  "Miktar",
  "Belge Türü",
  "Para Birimi",
  "Kur",
  "Döviz Tutar",
  "TC/Vergi No",
];

// --- küçük zip / xlsx yardımcıları (yalnız bu test için) ---

const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(b: Buffer): number {
  let c = 0xffffffff;
  for (const x of b) c = CRC_TABLE[(c ^ x) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

/** Sıkıştırmasız (stored) zip. */
function zipStored(files: [string, string][]): Buffer {
  const locals: Buffer[] = [];
  const centrals: Buffer[] = [];
  let offset = 0;
  for (const [name, text] of files) {
    const data = Buffer.from(text, "utf8");
    const n = Buffer.from(name, "utf8");
    const crc = crc32(data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0x0800, 6);
    local.writeUInt16LE(0, 8);
    local.writeUInt16LE(0, 10);
    local.writeUInt16LE(0x21, 12);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(n.length, 26);
    local.writeUInt16LE(0, 28);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(0x0800, 8);
    central.writeUInt16LE(0, 10);
    central.writeUInt16LE(0, 12);
    central.writeUInt16LE(0x21, 14);
    central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(n.length, 28);
    central.writeUInt32LE(offset, 42);
    locals.push(local, n, data);
    centrals.push(central, n);
    offset += 30 + n.length + data.length;
  }
  const cd = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(files.length, 8);
  end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(cd.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, cd, end]);
}

/** Zip'in dosyaları (stored ya da deflate). */
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

const escapeXml = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

/** xlsx'in ilk sayfasının ilk satırı (paylaşılan ya da satır içi metin). */
function firstRow(xlsx: Buffer): string[] {
  const files = unzip(xlsx);
  const shared: string[] = [];
  const sst = files.get("xl/sharedStrings.xml")?.toString("utf8") ?? "";
  for (const si of sst.match(/<si>[\s\S]*?<\/si>/g) ?? []) {
    const parts = [...si.matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)].map((m) =>
      xmlText(m[1]),
    );
    shared.push(parts.join(""));
  }
  const sheetName = [...files.keys()].find((k) =>
    /^xl\/worksheets\/sheet\d+\.xml$/.test(k),
  );
  expect(sheetName, "sayfa").toBeTruthy();
  const sheet = files.get(sheetName as string)!.toString("utf8");
  const row = sheet.match(/<row[^>]*\br="1"[^>]*>([\s\S]*?)<\/row>/);
  expect(row, "başlık satırı").toBeTruthy();
  const cells = [
    ...(row as RegExpMatchArray)[1].matchAll(
      /<c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/c>)/g,
    ),
  ];
  return cells.map((c) => {
    const attrs = c[1];
    const body = c[2] ?? "";
    const t = /\bt="([^"]+)"/.exec(attrs)?.[1];
    const v = /<v>([\s\S]*?)<\/v>/.exec(body)?.[1] ?? "";
    if (t === "s") return shared[Number(v)] ?? "";
    if (t === "inlineStr")
      return xmlText(/<t[^>]*>([\s\S]*?)<\/t>/.exec(body)?.[1] ?? "");
    return xmlText(v);
  });
}

const colName = (i: number) => String.fromCharCode(65 + i);

/** Tek sayfalık en küçük xlsx: metin satır içi, sayı sayı hücresi. */
function xlsxOf(rows: (string | number)[][]): Buffer {
  const main = "http://schemas.openxmlformats.org/spreadsheetml/2006/main";
  const rel =
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships";
  const sheetData = rows
    .map((r, i) => {
      const cells = r
        .map((v, j) => {
          const ref = `${colName(j)}${i + 1}`;
          if (v === "") return "";
          return typeof v === "number"
            ? `<c r="${ref}"><v>${v}</v></c>`
            : `<c r="${ref}" t="inlineStr"><is><t>${escapeXml(v)}</t></is></c>`;
        })
        .join("");
      return `<row r="${i + 1}">${cells}</row>`;
    })
    .join("");
  return zipStored([
    [
      "[Content_Types].xml",
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
    ],
    [
      "_rels/.rels",
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${rel}/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
    ],
    [
      "xl/workbook.xml",
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="${main}" xmlns:r="${rel}"><sheets><sheet name="Mizan" sheetId="1" r:id="rId1"/></sheets></workbook>`,
    ],
    [
      "xl/_rels/workbook.xml.rels",
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${rel}/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
    ],
    [
      "xl/worksheets/sheet1.xml",
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="${main}"><sheetData>${sheetData}</sheetData></worksheet>`,
    ],
  ]);
}

/** Luca mizanı: 5 satır üst bilgi, 6. satır başlık; uydurma tutarlar. */
const mizanFile = () => ({
  name: `mizan-${run}.xlsx`,
  mimeType:
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  buffer: xlsxOf([
    [`ÖRNEK ${run} TİCARET LTD. ŞTİ.`],
    ["MİZAN"],
    ["Dönem: 2026"],
    ["Tarih Aralığı: 01.01.2026 - 31.10.2026"],
    ["Para Birimi: TL"],
    [
      "HESAP KODU",
      "HESAP ADI",
      "",
      "BORÇ",
      "ALACAK",
      "BORÇ BAKİYESİ",
      "ALACAK BAKİYESİ",
    ],
    ["100", "KASA", "", 5000, 3000, 2000, 0],
    ["100.01", "MERKEZ KASA", "", 5000, 3000, 2000, 0],
    ["320", "SATICILAR", "", 1000, 4000, 0, 3000],
    ["600", "YURT İÇİ SATIŞLAR", "", 0, 1000, 0, 1000],
    ["770", "GENEL YÖNETİM GİDERLERİ", "", 2000, 0, 2000, 0],
    [],
    ["TOPLAM", "", "", 8000, 8000, 4000, 4000],
  ]),
});

test.describe("Luca biçimleri", () => {
  let api: Api;
  let engagement = "";
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "lucabicim");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Luca'ya aktar: .xlsx, tek başlık satırı Luca'nın 15 sütunu", async () => {
    const draft = await api.json<{ uuid: string }>(
      await api.post(`${base()}/journal-drafts`, {
        entry_type: "mahsup",
        entry_date: `${PERIOD}-12T00:00:00Z`,
        description: `E2E Luca biçim ${run}`,
        lines: [
          { account: "770.01", debit: "1000.00", credit: "0" },
          { account: "191.20", debit: "200.00", credit: "0" },
          { account: "320.01.001", debit: "0", credit: "1200.00" },
        ],
      }),
    );
    const approved = await api.post(
      `${base()}/journal-drafts/${draft.uuid}/approve`,
    );
    expect(approved.status(), await approved.text()).toBeLessThan(300);

    const batch = await api.json<{
      uuid: string;
      template_version: string;
      template_verified: boolean;
      files: { name: string; content_type: string }[];
    }>(
      await api.post(`${base()}/journal-exports`, {
        driver: "luca",
        drafts: [draft.uuid],
      }),
    );
    expect(batch.template_version).toBe("verified-1");
    expect(batch.template_verified).toBe(true);
    expect(batch.files).toHaveLength(1);
    const file = batch.files[0];
    expect(file.name).toMatch(/^luca_fis.*\.xlsx$/);

    const res = await api.get(
      `${base()}/journal-exports/${batch.uuid}/files/${encodeURIComponent(file.name)}`,
    );
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain(
      "spreadsheetml.sheet",
    );
    const body = await res.body();
    expect(body.subarray(0, 2).toString("latin1")).toBe("PK");
    expect(firstRow(body)).toEqual(LUCA_HEADER);
  });

  test("Defterler: Luca mizanı tanınır, dönem ve toplamlar okunur", async ({
    page,
  }) => {
    const last = (
      await api.json<{ last_completed: string }>(
        await api.get(`${base()}/ledgers`),
      )
    ).last_completed;
    const res = await api.uploadWith(`${base()}/ledgers`, mizanFile(), {
      period: last,
    });
    expect(res.status(), await res.text()).toBe(201);
    const item = (await res.json()).data.item;
    expect(item).toMatchObject({ kind: "mizan", readable: true });
    expect(item.summary.mizan).toMatchObject({
      header_row: 6,
      period: "2026-10",
      period_start: "2026-01-01",
      period_end: "2026-10-31",
      accounts: 6,
      debit_total: "8000.00",
      credit_total: "8000.00",
      debit_balance: "4000.00",
      credit_balance: "4000.00",
      balanced: true,
    });

    // Ekran: tür "Mizan", özet dengeyi ve toplamları söyler.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/ledgers`));
    await expect(page.getByText("Nasıl yüklenir?")).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText(/Mizan ve ücret bordrosu/)).toBeVisible();
    const files = page.getByRole("table", { name: "Yüklenen dosyalar" });
    const row = files.getByRole("row", { name: new RegExp(`mizan-${run}`) });
    await expect(row).toContainText("Mizan");
    await expect(row).toContainText("Okundu");
    await row.getByRole("button", { name: "Özet" }).click();
    const dialog = page.getByRole("dialog", { name: "Defter özeti" });
    await expect(dialog.getByTestId("ledger-mizan")).toBeVisible();
    await expect(dialog).toContainText("Mizan dengede");
    await expect(dialog).toContainText("8.000,00");
    await expect(dialog).toContainText("başlık 6. satırda");
    await expect(
      dialog.getByRole("table", { name: "Ana hesaplar" }),
    ).toContainText("SATICILAR");
  });
});
