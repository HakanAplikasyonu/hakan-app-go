import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { inflateRawSync } from "node:zlib";

import { type APIRequestContext, type BrowserContext, chromium, expect, type Page } from "@playwright/test";

/**
 * Gerçek Chrome'da WhatsApp uzantısı (#780): paneldeki zip indirilir, açılır
 * ve `--load-extension` ile yüklenir. WhatsApp Web yerine uzantının fixture
 * sohbeti `web.whatsapp.com` adresinde sunulur; gerçek WhatsApp'a bağlanılmaz.
 */

export const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:3000";
/** Paketlenmemiş paketin sabit kimliği (extensions/whatsapp/scripts/extension-key.txt). */
export const EXTENSION_ID = "fbcngkcogndfncloaedcfpkofebbfhco";

/** Bağımlılıksız zip açıcı (paket uzantının kendi zip yazıcısıyla üretilir). */
export function unzip(buf: Buffer, dir: string): string[] {
  const eocd = buf.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  if (eocd < 0) throw new Error("zip: son kayıt yok");
  const count = buf.readUInt16LE(eocd + 10);
  let p = buf.readUInt32LE(eocd + 16);
  const names: string[] = [];
  for (let i = 0; i < count; i++) {
    if (buf.readUInt32LE(p) !== 0x02014b50) throw new Error("zip: merkezi dizin bozuk");
    const method = buf.readUInt16LE(p + 10);
    const compSize = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extraLen = buf.readUInt16LE(p + 30);
    const commentLen = buf.readUInt16LE(p + 32);
    const local = buf.readUInt32LE(p + 42);
    const name = buf.subarray(p + 46, p + 46 + nameLen).toString("utf8");
    const start = local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const body = buf.subarray(start, start + compSize);
    const data = method === 8 ? inflateRawSync(body) : body;
    const out = join(dir, name);
    mkdirSync(dirname(out), { recursive: true });
    writeFileSync(out, data);
    names.push(name);
    p += 46 + nameLen + extraLen + commentLen;
  }
  return names;
}

/** Panelden zip'i indirip açar; açılan klasör. */
export async function downloadExtension(request: APIRequestContext): Promise<string> {
  const res = await request.get(`${BASE_URL}/indir/easytrade-whatsapp.zip`);
  expect(res.status()).toBe(200);
  const dir = mkdtempSync(join(tmpdir(), "easytrade-wa-ext-"));
  expect(unzip(await res.body(), dir)).toContain("manifest.json");
  return dir;
}

/** Uzantı yüklü Chrome (yeni başsız kip uzantıları çalıştırır). */
export async function launchWithExtension(ext: string): Promise<BrowserContext> {
  const ctx = await chromium.launchPersistentContext(mkdtempSync(join(tmpdir(), "easytrade-wa-profile-")), {
    channel: "chromium",
    headless: true,
    baseURL: BASE_URL,
    locale: "tr-TR",
    timezoneId: "Europe/Istanbul",
    args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`],
  });
  let [sw] = ctx.serviceWorkers();
  if (!sw) sw = await ctx.waitForEvent("serviceworker");
  expect(new URL(sw.url()).host).toBe(EXTENSION_ID);
  return ctx;
}

/**
 * Sahte WhatsApp Web: uzantının fixture sohbeti; `phone` sohbetin numarası
 * (data-id içinde). Görseller indirilmiş bir fiş gibi gerçek blob olur.
 */
export async function fakeWhatsApp(ctx: BrowserContext, phone = "905321234567"): Promise<void> {
  const chat = readFileSync(join(process.cwd(), "../extensions/whatsapp/tests/fixtures/chat.html"), "utf8").replaceAll(
    "905321234567",
    phone,
  );
  const html = `<!doctype html><html lang="tr"><head><meta charset="utf-8"></head><body>${chat}<script>
for (const img of document.querySelectorAll('img[src^="blob:"]')) {
  const c = document.createElement('canvas'); c.width = 330; c.height = 440;
  const g = c.getContext('2d'); g.fillStyle = '#fafafa'; g.fillRect(0, 0, 330, 440);
  g.fillStyle = '#222'; g.font = 'bold 18px monospace'; g.fillText('FİŞ ' + Date.now(), 40, 50);
  c.toBlob((b) => { img.src = URL.createObjectURL(b); });
}
</script></body></html>`;
  await ctx.route("https://web.whatsapp.com/**", (r) =>
    r.fulfill({ status: 200, contentType: "text/html; charset=utf-8", body: html }),
  );
}

/** Açılır pencereyi sekmede açar ve panelden alınan kodla eşleştirir. */
export async function pairInPopup(ctx: BrowserContext, code: string): Promise<Page> {
  const pop = await ctx.newPage();
  await pop.goto(`chrome-extension://${EXTENSION_ID}/popup.html`);
  await pop.locator("#code").fill(code.toLowerCase());
  await pop.getByRole("button", { name: "Eşleştir", exact: true }).click();
  await expect(pop.getByText("Bağlandı ✓")).toBeVisible();
  return pop;
}
