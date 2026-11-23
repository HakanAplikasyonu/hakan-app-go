import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Gerçek (ikili) Excel 97-2003 .xls banka ekstresi doğrudan okunur
 * (Oğuz'un geri bildirimi).
 *
 * - Ziraat düzeninde bir ekstre, testin içinde küçük bir BIFF8 yazıcısıyla
 *   gerçek .xls olarak üretilir: tarihler Excel seri sayısı, tutarlar sayı
 *   (1.234,56 ve -0,10 dahil). Banka sayfasından yüklenir; önizleme bankayı
 *   tanır, IBAN eklenen hesabı seçer, içe aktarılan tutarlar kuruşu kuruşuna
 *   aynıdır (kayan nokta yuvarlaması yok), tarihler doğru güne düşer.
 * - Bozuk bir .xls (yalnız imza) anlaşılır bir uyarıyla reddedilir.
 *
 * Veri: demo ofisine her koşuda yeni mükellef ve sahte Ziraat IBAN'ı;
 * hareketler uydurmadır.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka XLS ${run} A.Ş.` };
const tag = `XLS${run}`;

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli sahte TR IBAN. */
function ziraatIban(): string {
  const bban = `000100${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

/** 1900 tarih sisteminde günün seri sayısı (1899-12-30 + gün). */
function serial(y: number, m: number, d: number): number {
  return Math.round((Date.UTC(y, m - 1, d) - Date.UTC(1899, 11, 30)) / 86_400_000);
}

type Cell = string | number | null;

/**
 * En küçük BIFF8 çalışma kitabı: tek "Workbook" akışı taşıyan OLE bileşik
 * dosyası (sürüm 3, tek FAT kesimi). Metin LABEL (UTF-16), sayı NUMBER
 * kaydıdır.
 */
function writeXls(sheetName: string, rows: Cell[][]): Buffer {
  const rec = (type: number, data: Buffer) => {
    const h = Buffer.alloc(4);
    h.writeUInt16LE(type, 0);
    h.writeUInt16LE(data.length, 2);
    return Buffer.concat([h, data]);
  };
  const bof = (dt: number) => {
    const b = Buffer.alloc(16);
    b.writeUInt16LE(0x0600, 0);
    b.writeUInt16LE(dt, 2);
    b.writeUInt16LE(0x0dbb, 4);
    b.writeUInt16LE(0x07cc, 6);
    b.writeUInt32LE(0x0006, 12);
    return rec(0x0809, b);
  };
  const eof = rec(0x000a, Buffer.alloc(0));
  const head = (r: number, c: number) => {
    const b = Buffer.alloc(6);
    b.writeUInt16LE(r, 0);
    b.writeUInt16LE(c, 2);
    b.writeUInt16LE(0x0f, 4);
    return b;
  };

  const sheet: Buffer[] = [bof(0x0010)];
  rows.forEach((row, r) =>
    row.forEach((v, c) => {
      if (v === null || v === "") return;
      if (typeof v === "number") {
        const n = Buffer.alloc(8);
        n.writeDoubleLE(v, 0);
        sheet.push(rec(0x0203, Buffer.concat([head(r, c), n])));
      } else {
        const len = Buffer.alloc(3);
        len.writeUInt16LE(v.length, 0);
        len[2] = 1; // UTF-16LE karakterler
        sheet.push(rec(0x0204, Buffer.concat([head(r, c), len, Buffer.from(v, "utf16le")])));
      }
    }),
  );
  sheet.push(eof);

  const name = Buffer.concat([Buffer.from([sheetName.length, 1]), Buffer.from(sheetName, "utf16le")]);
  const boundsheet = rec(0x0085, Buffer.concat([Buffer.alloc(6), name]));
  const globals = Buffer.concat([bof(0x0005), boundsheet, eof]);
  globals.writeUInt32LE(globals.length, bof(0x0005).length + 4); // çalışma sayfasının konumu
  let stream = Buffer.concat([globals, ...sheet]);
  // 4096 bayttan küçük akış mini akışa düşer: normal kesimlerde kalsın.
  if (stream.length < 4096) stream = Buffer.concat([stream, Buffer.alloc(4096 - stream.length)]);

  const S = 512;
  const n = Math.ceil(stream.length / S);
  const out = Buffer.alloc(S * (3 + n));
  Buffer.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1]).copy(out, 0);
  out.writeUInt16LE(0x003e, 24);
  out.writeUInt16LE(3, 26);
  out.writeUInt16LE(0xfffe, 28);
  out.writeUInt16LE(9, 30);
  out.writeUInt16LE(6, 32);
  out.writeUInt32LE(1, 44); // FAT kesimi sayısı
  out.writeUInt32LE(1, 48); // ilk dizin kesimi
  out.writeUInt32LE(4096, 56);
  out.writeUInt32LE(0xfffffffe, 60);
  out.writeUInt32LE(0xfffffffe, 68);
  for (let i = 0; i < 109; i++) out.writeUInt32LE(0xffffffff, 76 + 4 * i);
  out.writeUInt32LE(0, 76); // FAT kesim 0'da

  const fat = S;
  for (let i = 0; i < S / 4; i++) out.writeUInt32LE(0xffffffff, fat + 4 * i);
  out.writeUInt32LE(0xfffffffd, fat);
  out.writeUInt32LE(0xfffffffe, fat + 4);
  for (let i = 0; i < n; i++) out.writeUInt32LE(i === n - 1 ? 0xfffffffe : 2 + i + 1, fat + 4 * (2 + i));

  const dir = 2 * S;
  const entry = (at: number, entryName: string, type: number, child: number, start: number, size: number) => {
    Buffer.from(entryName, "utf16le").copy(out, at);
    out.writeUInt16LE(2 * (entryName.length + 1), at + 64);
    out[at + 66] = type;
    out[at + 67] = 1;
    out.writeUInt32LE(0xffffffff, at + 68);
    out.writeUInt32LE(0xffffffff, at + 72);
    out.writeUInt32LE(child, at + 76);
    out.writeUInt32LE(start, at + 116);
    out.writeUInt32LE(size, at + 120);
  };
  entry(dir, "Root Entry", 5, 1, 0xfffffffe, 0);
  entry(dir + 128, "Workbook", 2, 0xffffffff, 2, stream.length);
  for (const at of [dir + 256, dir + 384]) {
    out.writeUInt32LE(0xffffffff, at + 68);
    out.writeUInt32LE(0xffffffff, at + 72);
    out.writeUInt32LE(0xffffffff, at + 76);
  }
  stream.copy(out, 3 * S);
  return out;
}

/** Ziraat "Hesap Hareketleri" dışa aktarımının .xls hâli (sentetik). */
function ziraatXls(iban: string): Buffer {
  return writeXls("Hesap Hareketleri", [
    ["T.C. ZİRAAT BANKASI A.Ş."],
    ["Hesap Hareketleri"],
    ["IBAN", iban],
    ["Dönem", "01.10.2026 - 31.10.2026"],
    [],
    ["Tarih", "Fiş No", "Açıklama", "İşlem Tutarı", "Bakiye"],
    [serial(2026, 10, 1), "770001", `FAST GELEN ${tag} YILDIZ YAZILIM`, 1234.56, 11234.56],
    [serial(2026, 10, 2), "770002", `HESAP ISLETIM UCRETI ${tag}`, -0.1, 11234.46],
    [serial(2026, 10, 5), "770003", `EFT GIDEN ${tag} KALEM KIRTASIYE`, -3250.75, 7983.71],
    [],
    ["Toplam"],
  ]);
}

test("gerçek Excel 97-2003 (.xls) banka ekstresi çevirmeden okunur; tutarlar kuruşu kuruşuna doğru", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bankaxls");
  const base = `/v1/tenant/engagements/${engagement}`;
  const iban = ziraatIban();
  const box = await api.json<{ uuid: string }>(await api.post(`${base}/cash-boxes`, { iban, name: `Ziraat TL ${tag}` }));

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  const input = page.locator('input[type="file"][aria-label="Ekstre yükle"]');
  await input.setInputFiles({ name: `ziraat-hesap-hareketleri-${run}.xls`, mimeType: "application/vnd.ms-excel", buffer: ziraatXls(iban) });
  const preview = page.getByTestId("bank-statement-preview");
  await expect(preview.getByRole("heading", { name: "Ekstre önizleme" })).toBeVisible();
  await expect(preview).toContainText("banka: Ziraat Bankası");
  await preview.getByRole("button", { name: "İçe aktar (3 hareket)" }).click();
  await expect(page.getByText("3 yeni hareket eklendi (banka: Ziraat Bankası).")).toBeVisible();
  const table = page.getByRole("table");
  await expect(table.getByRole("row").filter({ hasText: tag })).toHaveCount(3);
  await expect(table.getByRole("row").filter({ hasText: `FAST GELEN ${tag}` })).toContainText("1.234,56");
  await expect(table.getByRole("row").filter({ hasText: `HESAP ISLETIM UCRETI ${tag}` })).toContainText("0,10");

  // Tutarlar ve tarihler: seri sayılar doğru güne, sayılar tam ondalığa.
  const txs = await api.json<{ items: { transaction_date: string; amount: string; description: string; cash_box_uuid: string }[] }>(
    await api.get(`${base}/bank-transactions?period=2026-10&limit=50`),
  );
  const mine = txs.items
    .filter((x) => x.description.includes(tag))
    .map((x) => ({ date: x.transaction_date.slice(0, 10), amount: x.amount, box: x.cash_box_uuid }))
    .sort((a, b) => a.date.localeCompare(b.date));
  expect(mine).toEqual([
    { date: "2026-10-01", amount: "1234.56", box: box.uuid },
    { date: "2026-10-02", amount: "0.10", box: box.uuid },
    { date: "2026-10-05", amount: "3250.75", box: box.uuid },
  ]);

  // Bozuk .xls: yalnız uyarı, hareket eklenmez.
  const broken = Buffer.concat([Buffer.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1]), Buffer.alloc(512)]);
  const res = await api.upload(`${base}/bank-statements`, { name: `bozuk-${run}.xls`, mimeType: "application/vnd.ms-excel", buffer: broken });
  expect(res.status()).toBe(415);
  expect(await res.text()).toContain("STATEMENT_XLS_LEGACY");
  await input.setInputFiles({ name: `bozuk-${run}.xls`, mimeType: "application/vnd.ms-excel", buffer: broken });
  await expect(page.getByText("Bu Excel 97-2003 (.xls) dosyası okunamadı")).toBeVisible();
  await api.dispose();
});
