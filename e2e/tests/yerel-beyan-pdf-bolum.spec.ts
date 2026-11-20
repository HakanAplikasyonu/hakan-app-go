import { inflateRawSync } from "node:zlib";

import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf } from "./lib";

/**
 * Beyan özeti PDF'i ve Excel'i bölüm özetiyle biter (#710): Z raporları,
 * faturalar, banka hareketleri ve KDV'li fişler için toplam / işlenen / yok
 * sayılan / bekleyen. Excel'de "Bölüm özeti" sayfası ve satırları; PDF aynı
 * kayıttan çizilir.
 *
 * Veri: demo ofisi, Atlas Gıda, Ekim 2026; yalnız okur.
 */

/** ZIP içindeki bir dosyanın metni (merkezi dizinden, yalnız deflate/stored). */
function zipText(zip: Buffer, name: string): string {
  const end = zip.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  let at = zip.readUInt32LE(end + 16);
  const count = zip.readUInt16LE(end + 10);
  for (let i = 0; i < count; i++) {
    const method = zip.readUInt16LE(at + 10);
    const size = zip.readUInt32LE(at + 20);
    const nameLen = zip.readUInt16LE(at + 28);
    const extra = zip.readUInt16LE(at + 30);
    const comment = zip.readUInt16LE(at + 32);
    const local = zip.readUInt32LE(at + 42);
    const entry = zip.subarray(at + 46, at + 46 + nameLen).toString("utf8");
    if (entry === name) {
      const start =
        local +
        30 +
        zip.readUInt16LE(local + 26) +
        zip.readUInt16LE(local + 28);
      const data = zip.subarray(start, start + size);
      return (method === 8 ? inflateRawSync(data) : data).toString("utf8");
    }
    at += 46 + nameLen + extra + comment;
  }
  throw new Error(`${name} not in zip`);
}

test("beyan Excel'i ve PDF'i bölüm özetini taşır (#710)", async () => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}/tax-returns/kdv1/2026-10`;

  const preview = await api.json<{
    sections: { key: string; total: number; processed: number }[];
  }>(await api.get(base));
  expect(preview.sections.map((s) => s.key)).toEqual([
    "retail",
    "invoices",
    "bank",
    "expenses",
  ]);

  const xlsx = await api.get(`${base}/export`);
  expect(xlsx.status(), await xlsx.text()).toBe(200);
  const zip = Buffer.from(await xlsx.body());
  expect(zipText(zip, "xl/workbook.xml")).toContain('name="Bölüm özeti"');
  const strings = zipText(zip, "xl/sharedStrings.xml");
  for (const text of [
    "İşlenen",
    "Yok sayılan",
    "Bekleyen",
    "Z raporları",
    "Faturalar",
    "Banka hareketleri",
    "li fişler",
  ])
    expect(strings).toContain(text);

  const pdf = await api.get(`${base}/export?format=pdf`);
  expect(pdf.status(), await pdf.text()).toBe(200);
  expect((await pdf.body()).subarray(0, 4).toString("latin1")).toBe("%PDF");
  await api.dispose();
});
