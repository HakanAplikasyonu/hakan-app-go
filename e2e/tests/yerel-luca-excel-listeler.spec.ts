import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import { inflateRawSync } from "node:zlib";

import { expect, test, type Page } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Listelerde "Luca'ya aktar (Excel)" (Oğuz'un geri bildirimi): Gelen
 * faturalar, Giden faturalar, Banka hareketleri ve Z raporları listesinde
 * düğme dönemin Luca fiş dosyasını (.xlsx) indirir; dosyanın tek başlık
 * satırı Luca'nın fiş Excel'indeki 15 sütundur. Aynı dönem ikinci kez
 * aktarılınca daha önce aktarılan belge dosyaya girmez: pencere "aktarılacak
 * yeni belge yok" der (çift aktarım koruması). Fiş / KDV satırı / denge /
 * hesap kodu ayrıntısı Go testlerinde.
 *
 * Veri: her koşuda yeni mükellef (uydurma). Giden fatura Atlas'ın temel
 * satış UBL'inden, gelen fatura Atlas'ın ticari satışında alıcı bu
 * mükellefe çevrilerek türetilir; banka ekstresi Ziraat örneği
 * (fixtures/atlas-ziraat-2026-10.sta); Z raporu CSV ile içe aktarılır.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Luca Liste ${run} A.Ş.` };
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

/**
 * Atlas'ın ticari satış faturası, alıcısı bu mükellef olacak şekilde:
 * mükellef için gelen (alış) fatura. ETTN yenilenir.
 */
function incomingUbl(vkn: string) {
  const file = "atlas-02-ticari-satis.xml";
  let xml = readFileSync(path.join(DEMO.ublDir, file), "utf8");
  const ettn = xml.match(/<(?:cbc:)?UUID\b[^>]*>([^<]+)<\/(?:cbc:)?UUID>/)?.[1];
  if (!ettn) throw new Error(`no ETTN in ${file}`);
  xml = xml
    .split("9100000021")
    .join(vkn)
    .split(ettn)
    .join(randomUUID().toUpperCase());
  return {
    name: `e2e-gelen-${run}.xml`,
    mimeType: "application/xml",
    buffer: Buffer.from(xml, "utf8"),
  };
}

/**
 * Pencerede dosyayı indirir ve başlık satırını denetler; ikinci deneme
 * daha önce aktarılanı almaz ("yeni belge yok").
 */
async function exportTwice(page: Page, list?: string) {
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  if (list) await dialog.getByTestId("luca-list-kind").selectOption(list);
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    dialog.getByTestId("luca-list-download").click(),
  ]);
  expect(download.suggestedFilename()).toMatch(/_luca_fis_2026-10\.xlsx$/);
  const saved = await download.path();
  const buf = readFileSync(saved as string);
  expect(buf.subarray(0, 2).toString()).toBe("PK");
  expect(firstRow(buf).slice(0, LUCA_HEADER.length)).toEqual(LUCA_HEADER);
  await expect(dialog.getByTestId("luca-list-result")).toBeVisible();

  // İkinci çıktı: daha önce aktarılan belge girmez, dosya inmez.
  await dialog.getByTestId("luca-list-download").click();
  await expect(dialog.getByTestId("luca-list-empty")).toBeVisible();
}

test.describe("Luca'ya aktar (Excel) listeleri", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "lucaliste");

    // Giden ve gelen fatura (Ekim 2026).
    for (const file of [
      deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
      incomingUbl(taxpayer.vkn),
    ]) {
      const up = await api.upload(`${base()}/ubl-uploads`, file);
      expect(up.status(), await up.text()).toBeLessThan(300);
    }
    await waitForUploads(api, engagement, 2);

    // Banka hareketleri (Ekim 2026).
    await api.json(
      await api.post(`${base()}/cash-boxes`, {
        iban: "TR76 0001 0005 1978 6457 8413 26",
        name: "Ziraat TL",
      }),
    );
    await api.json(
      await api.upload(`${base()}/bank-statements`, {
        name: "atlas-ziraat-2026-10.sta",
        mimeType: "text/plain",
        buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
      }),
    );

    // Z raporu (Ekim 2026).
    const z = await api.upload(`${base()}/retail-sales/import`, {
      name: `z-${run}.csv`,
      mimeType: "text/csv",
      buffer: Buffer.from(
        "Tarih;ÖKC Seri No;Z No;Nakit;Kart;%20 Matrah;%20 KDV;Toplam\n" +
          "15.10.2026;TR0000001;0001;200,00;400,00;500,00;100,00;600,00\n",
        "utf8",
      ),
    });
    expect(z.status(), await z.text()).toBeLessThan(300);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("gelen ve giden faturalar Luca dosyası olarak iner, ikinci aktarım engellenir", async ({
    page,
  }) => {
    test.setTimeout(180_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/engagements/${engagement}/invoices?period=${PERIOD}`),
    );
    await page.getByTestId("luca-list-open").click();
    await expect(page.getByRole("dialog")).toContainText(
      "Luca'nın API'si yok, dosya iner, Luca'ya siz yüklersiniz.",
    );
    await exportTwice(page, "incoming-invoices");
    await exportTwice(page, "outgoing-invoices");
  });

  test("banka hareketleri Luca dosyası olarak iner, ikinci aktarım engellenir", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/engagements/${engagement}/bank?period=${PERIOD}`),
    );
    await page.getByTestId("luca-list-open").click();
    await exportTwice(page);
  });

  test("Z raporları Luca dosyası olarak iner, ikinci aktarım engellenir", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/engagements/${engagement}/z-reports?period=${PERIOD}`),
    );
    await page.getByTestId("luca-list-open").click();
    await exportTwice(page);
  });

  test("API: ikinci aktarım 422, yeniden dahil et dosyayı yine verir", async () => {
    const again = await api.post(`${base()}/luca-lists/z-reports`, {
      period: PERIOD,
    });
    expect(again.status(), await again.text()).toBe(422);
    const re = await api.post(`${base()}/luca-lists/z-reports`, {
      period: PERIOD,
      include_exported: true,
    });
    expect(re.status(), await re.text()).toBe(200);
    expect(re.headers()["x-luca-exported"]).toBe("1");
    const body = Buffer.from(await re.body());
    expect(firstRow(body).slice(0, LUCA_HEADER.length)).toEqual(LUCA_HEADER);
  });
});
