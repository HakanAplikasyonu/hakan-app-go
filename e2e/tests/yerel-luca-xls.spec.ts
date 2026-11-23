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
 * Luca eski .xls (Oğuz'un geri bildirimi): Luca ücret bordrosunu ve mizanı
 * Excel 97-2003 (.xls, BIFF8) olarak verir. Defterler bu dosyaları
 * çevirmeden okur: tür dosyanın içindeki başlıktan (ad bir şey söylemez),
 * bordroda alt özet toplamları, T.C. kimlik no'nun ".0"sı kırpılır, metin
 * olarak yazılmış tutar ("1.400,00") da okunur. Dosyalar testte üretilir
 * (küçük bir BIFF8 + OLE yazıcısı); firma, kişi ve tutarlar uydurmadır.
 *
 * Veri: her koşuda yeni mükellef; yükleme API ile, özet ekranda.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Luca XLS ${run} Ltd. Şti.` };

// --- küçük .xls (BIFF8 içinde OLE bileşik dosya) yazıcısı ---

type Cell = string | number | null;

const END = 0xfffffffe;
const FREE = 0xffffffff;
const NOSTREAM = 0xffffffff;

function rec(type: number, data: Buffer): Buffer {
  const h = Buffer.alloc(4);
  h.writeUInt16LE(type, 0);
  h.writeUInt16LE(data.length, 2);
  return Buffer.concat([h, data]);
}

function bof(dt: number): Buffer {
  const b = Buffer.alloc(16);
  b.writeUInt16LE(0x0600, 0); // BIFF8
  b.writeUInt16LE(dt, 2);
  b.writeUInt16LE(0x0dbb, 4);
  b.writeUInt16LE(0x07cc, 6);
  b.writeUInt32LE(6, 12);
  return b;
}

/** Unicode (16 bit) karakterli dize; kısa: 8 bitlik uzunluk. */
function xlString(s: string, short: boolean): Buffer {
  const chars = Buffer.from(s, "utf16le");
  const n = chars.length / 2;
  const head = Buffer.alloc(short ? 2 : 3);
  if (short) head.writeUInt8(n, 0);
  else head.writeUInt16LE(n, 0);
  head.writeUInt8(1, short ? 1 : 2);
  return Buffer.concat([head, chars]);
}

function cellHead(r: number, c: number): Buffer {
  const b = Buffer.alloc(6);
  b.writeUInt16LE(r, 0);
  b.writeUInt16LE(c, 2);
  b.writeUInt16LE(0x0f, 4);
  return b;
}

function dirEntry(
  out: Buffer,
  at: number,
  name: string,
  type: number,
  child: number,
  start: number,
  size: number,
) {
  Buffer.from(name, "utf16le").copy(out, at);
  out.writeUInt16LE((name.length + 1) * 2, at + 64);
  out.writeUInt8(type, at + 66);
  out.writeUInt8(1, at + 67);
  out.writeUInt32LE(NOSTREAM, at + 68);
  out.writeUInt32LE(NOSTREAM, at + 72);
  out.writeUInt32LE(child, at + 76);
  out.writeUInt32LE(start, at + 116);
  out.writeUInt32LE(size, at + 120);
}

/** OLE bileşik dosya (sürüm 3): sektör 0 FAT, 1 dizin, sonra Workbook. */
function compound(stream: Buffer): Buffer {
  const S = 512;
  const n = Math.ceil(stream.length / S);
  if (2 + n > S / 4) throw new Error("xls fixture too large");
  const out = Buffer.alloc(S * (3 + n));
  Buffer.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1]).copy(out, 0);
  out.writeUInt16LE(0x3e, 24);
  out.writeUInt16LE(3, 26);
  out.writeUInt16LE(0xfffe, 28);
  out.writeUInt16LE(9, 30);
  out.writeUInt16LE(6, 32);
  out.writeUInt32LE(1, 44); // FAT sektör sayısı
  out.writeUInt32LE(1, 48); // ilk dizin sektörü
  out.writeUInt32LE(4096, 56);
  out.writeUInt32LE(END, 60);
  out.writeUInt32LE(END, 68);
  for (let i = 0; i < 109; i++) out.writeUInt32LE(FREE, 76 + 4 * i);
  out.writeUInt32LE(0, 76);
  const fat = S;
  for (let i = 0; i < S / 4; i++) out.writeUInt32LE(FREE, fat + 4 * i);
  out.writeUInt32LE(0xfffffffd, fat);
  out.writeUInt32LE(END, fat + 4);
  for (let i = 0; i < n; i++) {
    out.writeUInt32LE(i === n - 1 ? END : 3 + i, fat + 4 * (2 + i));
  }
  const dir = 2 * S;
  dirEntry(out, dir, "Root Entry", 5, 1, END, 0);
  dirEntry(out, dir + 128, "Workbook", 2, NOSTREAM, 2, stream.length);
  for (const e of [dir + 256, dir + 384]) {
    out.writeUInt32LE(NOSTREAM, e + 68);
    out.writeUInt32LE(NOSTREAM, e + 72);
    out.writeUInt32LE(NOSTREAM, e + 76);
  }
  stream.copy(out, 3 * S);
  return out;
}

/** Tek sayfalık .xls: metin LABEL, sayı NUMBER hücresi. */
function xlsOf(rows: Cell[][]): Buffer {
  const parts: Buffer[] = [rec(0x0809, bof(0x10))];
  rows.forEach((row, r) =>
    row.forEach((v, c) => {
      if (v === null || v === "") return;
      if (typeof v === "number") {
        const d = Buffer.alloc(8);
        d.writeDoubleLE(v, 0);
        parts.push(rec(0x0203, Buffer.concat([cellHead(r, c), d])));
      } else {
        parts.push(
          rec(0x0204, Buffer.concat([cellHead(r, c), xlString(v, false)])),
        );
      }
    }),
  );
  parts.push(rec(0x000a, Buffer.alloc(0)));
  const sheet = Buffer.concat(parts);
  const bound = Buffer.concat([Buffer.alloc(6), xlString("Sayfa1", true)]);
  const globals = [rec(0x0809, bof(0x05)), rec(0x0085, bound), rec(0x000a, Buffer.alloc(0))];
  // BOUNDSHEET: sayfanın akıştaki yeri (genel bölümün hemen ardı).
  bound.writeUInt32LE(
    globals.reduce((n, b) => n + b.length, 0),
    0,
  );
  globals[1] = rec(0x0085, bound);
  let stream = Buffer.concat([...globals, sheet]);
  if (stream.length < 4096) {
    stream = Buffer.concat([stream, Buffer.alloc(4096 - stream.length)]);
  }
  return compound(stream);
}

const XLS = "application/vnd.ms-excel";

/** Luca ücret bordrosu: üst bilgi, başlık, iki kişi, alt özet. */
function bordroFile() {
  const pad = (n: number): Cell[] => Array(n).fill(null);
  const rows: Cell[][] = [
    [],
    [...pad(7), "EKİM 2026 ÜCRET BORDROSU", ...pad(11), "Tarih: 05/11/2026"],
    ["İşyeri:", `ÖRNEK ${run} TİCARET LTD. ŞTİ.`],
    ["Vergi D./No:", "ÖRNEK VD / 1234567890"],
    [],
    [
      "ADI SOYADI", "GİRİŞ TARİHİ", "ÇIKIŞ TARİHİ", "T.C. KİMLİK NO", "SSK SİCİL NO", "GÜN", "",
      "NORMAL KAZANÇ", "DİĞER KAZANÇ", "TOPLAM KAZANÇ", "SSK MATRAH", "SSK PRİMİ", "İŞSİZLİK PRİMİ",
      "G.V.M. (AYLIK)", "GELİR VERGİSİ", "ASGARİ GEÇ.İND.", "KALAN G.VER.", "DAMGA VERGİSİ",
      "ÖZEL KESİNTİ", "AGI Hariç Ücret", "NET İSTİHKAK", "İMZA",
    ],
    [],
    ["AYŞE ÖRNEK", "01.01.2025", "", 10000000146, "", 30, "Gün", 30000, 0, 30000, ...pad(10), 25000],
    ["Normal Gün :", "30 Gün", 30000, "---"],
    [],
    ["MEHMET DENEME", "01.03.2025", "", "20000000046.0", "", 30, "Gün", 40000, 0, 40000, ...pad(10), 33000],
    ["Normal Gün :", "30 Gün", 40000, "---"],
    [],
    ["", "", "", "", "", 60, "", 70000, 0, 70000],
    ["Listelenen 2 Personel"],
    [],
    ["BRÜT ÖDEMELER", "", "Gün/Saat", "Tutar", "NET ÖDEMELER", ...pad(4), "Tutar"],
    ["Normal Kazançlar", "", 60, 70000, "Gelir Vergisi", ...pad(4), 4000],
    ["Toplam Kişi Sayısı", "", 2, "", "Net GV", ...pad(4), 3000],
    ["Toplam SGK Matrahı", "", "", 70000, "Damga Vergisi", ...pad(4), 531.3],
    ["Brüt Kazançlar Toplamı", "", "", 70000, "Net DV", ...pad(4), 300],
    ["Sigorta İşveren Hissesi", "", "", 15050, "Sigorta Toplam Prim (Emekli/Normal)", ...pad(4), 9800],
    ["SGK İşveren Teşvikleri", "", "", 3500, "İşsizlik Sigortası", ...pad(4), 700],
    ["Net Sigorta İşveren Hissesi", "", "", 11550, "Net Ödemeler Toplam", ...pad(4), 56200],
    ["İşsizlik İşveren Hissesi", "", "", "1.400,00", "Özel Kesintiler", ...pad(4), 0],
    ["TOPLAM", "", "", 82950, "", ...pad(4), ""],
  ];
  return { name: `luca-${run}-a.xls`, mimeType: XLS, buffer: xlsOf(rows) };
}

/** Luca mizanı: 4 satır üst bilgi, 5. satır başlık. */
function mizanFile() {
  return {
    name: `luca-${run}-b.xls`,
    mimeType: XLS,
    buffer: xlsOf([
      [`ÖRNEK ${run} TİCARET LTD. ŞTİ.`],
      ["MİZAN"],
      ["Tarih Aralığı: 01.01.2026 - 31.10.2026"],
      ["Para Birimi: TL"],
      ["HESAP KODU", "HESAP ADI", "", "BORÇ", "ALACAK", "BORÇ BAKİYESİ", "ALACAK BAKİYESİ"],
      ["100", "KASA", "", 5000, 3000, 2000, 0],
      ["320", "SATICILAR", "", 1000, 4000, 0, 3000],
      ["600", "YURT İÇİ SATIŞLAR", "", 0, 1000, 0, 1000],
      ["770", "GENEL YÖNETİM GİDERLERİ", "", 2000, 0, 2000, 0],
      [],
      ["TOPLAM", "", "", 8000, 8000, 4000, 4000],
    ]),
  };
}

test.describe("Luca eski .xls", () => {
  let api: Api;
  let engagement = "";
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "lucaxls");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Defterler: .xls bordro ve mizan çevirmeden okunur", async ({ page }) => {
    const last = (
      await api.json<{ last_completed: string }>(
        await api.get(`${base()}/ledgers`),
      )
    ).last_completed;

    const bordro = await api.uploadWith(`${base()}/ledgers`, bordroFile(), {
      period: last,
    });
    expect(bordro.status(), await bordro.text()).toBe(201);
    const b = (await bordro.json()).data.item;
    expect(b).toMatchObject({ kind: "bordro", readable: true });
    expect(b.summary.format).toBe("xls");
    expect(b.summary.bordro).toMatchObject({
      period: "2026-10",
      persons: 2,
      from_totals: true,
      gross: "70000.00",
      sgk_employee: "9800.00",
      sgk_employer: "11550.00",
      unemployment_employee: "700.00",
      unemployment_employer: "1400.00",
      income_tax: "3000.00",
      stamp_tax: "300.00",
      net: "56200.00",
      employer_cost: "82950.00",
    });
    expect(
      b.summary.bordro.employees.map((e: { national_id: string }) => e.national_id),
    ).toEqual(["10000000146", "20000000046"]);

    const mizan = await api.uploadWith(`${base()}/ledgers`, mizanFile(), {
      period: last,
    });
    expect(mizan.status(), await mizan.text()).toBe(201);
    const m = (await mizan.json()).data.item;
    expect(m).toMatchObject({ kind: "mizan", readable: true });
    expect(m.summary.mizan).toMatchObject({
      header_row: 5,
      period: "2026-10",
      accounts: 4,
      debit_total: "8000.00",
      credit_total: "8000.00",
      balanced: true,
    });

    // Ekran: "Nasıl yüklenir?" .xls'i de sayar; bordro satırı okundu.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/ledgers`));
    await expect(page.getByText("Nasıl yüklenir?")).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText(/xlsx olarak kaydedip/)).toHaveCount(0);
    const files = page.getByRole("table", { name: "Yüklenen dosyalar" });
    const row = files.getByRole("row", { name: new RegExp(`luca-${run}-a`) });
    await expect(row).toContainText("Ücret bordrosu");
    await expect(row).toContainText("Okundu");
    await row.getByRole("button", { name: "Özet" }).click();
    const dialog = page.getByRole("dialog", { name: "Defter özeti" });
    await expect(dialog.getByTestId("ledger-bordro")).toBeVisible();
    await expect(dialog).toContainText("56.200,00");
  });
});
