import { deflateSync } from "node:zlib";

import { expect, test, type Page } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Belge isteme akışları (#568).
 *
 * - #606 Aynı belge için açık (ya da gönderilmiş) talep varken yenisi
 *   açılmaz: `POST …/document-tasks` var olan talebi 200 ile döner (yeni
 *   talep 201). Başlık büyük/küçük harf duyarsız karşılaştırılır. Talep
 *   iptal edilince aynı belge yeniden istenebilir.
 * - #630 Okunamayan fiş fotoğrafları Masraflar ekranında görünür; ofis
 *   mükelleften yenisini ister ve yüklemeyi kapatır. Aynı gün yüklenen iki
 *   fotoğrafın isteği aynı başlıktadır ("Okunamayan fiş: net fotoğraf
 *   (<tarih>)"): ikinci düğme de, sayfa yenilenip tekrar basılan düğme de
 *   ikinci talep açmaz (#606).
 *
 * Okunamayan fiş nasıl oluşur: `POST …/receipts` fotoğrafı yapay zekâya
 * okutur; okuma hata verirse yükleme `failed` olarak kalır ve taslak
 * çıkmaz. e2e ortamında AI anahtarı yoktur (.env.example AI_API_KEY=), okuyucu
 * ErrAIUnavailable döner: her fotoğraf "okunamayan" olur. AI açık bir
 * ortamda fotoğraf okunursa senaryo atlanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; talepler ve fotoğraflar
 * API ile. Atlas Gıda'ya, Selin Yılmaz'a ya da başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Belge Talebi ${run} Ltd. Şti.` };
const UNREADABLE_TITLE = "Okunamayan fiş: net fotoğraf (";

type Task = { uuid: string; kind: string; status: string; title: string; note: string };
type Capture = { uuid: string; status: string; created_at: string };

test.describe("Belge isteme (#568)", () => {
  let engagement = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const requests = async (match: (title: string) => boolean) =>
    (
      await api.json<{ items: Task[] }>(await api.get(`${base()}/document-tasks?status=all&limit=100`))
    ).items.filter((t) => t.kind === "document_request" && match(t.title));
  const failedCaptures = async () =>
    api.json<{ items: Capture[]; total: number }>(await api.get(`${base()}/receipts?status=failed&limit=20`));

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "belge");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("#606 aynı belge talebi ikinci kez açılmaz; var olan talep döner", async () => {
    // Yalnız ASCII: lower() karşılaştırması harf tablosuna bağlı kalmasın.
    const title = `Kira sozlesmesi ${run}`;
    const same = (t: string) => t.toLowerCase() === title.toLowerCase();

    const first = await api.post(`${base()}/document-tasks`, {
      title,
      note: "Imzali kira sozlesmesinin taramasi",
      due_days: 3,
    });
    expect(first.status(), await first.text()).toBe(201);
    const created = (await first.json()).data as Task;
    expect(created.status).toBe("open");

    // Aynısı (büyük harfle) tekrar istenir: yeni talep yok, var olan 200 ile döner.
    const again = await api.post(`${base()}/document-tasks`, { title: title.toUpperCase(), due_days: 3 });
    expect(again.status(), await again.text()).toBe(200);
    const existing = (await again.json()).data as Task;
    expect(existing.uuid).toBe(created.uuid);
    expect(existing.title).toBe(title);
    expect(existing.status).toBe("open");

    let list = await requests(same);
    expect(list).toHaveLength(1);
    expect(list[0].uuid).toBe(created.uuid);

    // İptal edilen talep engel değildir: aynı belge yeniden istenebilir.
    const cancelled = await api.json<Task>(
      await api.post(`${base()}/document-tasks/${created.uuid}/cancel`, { reason: "e2e: yanlış dönem" }),
    );
    expect(cancelled.status).toBe("cancelled");
    const fresh = await api.post(`${base()}/document-tasks`, { title, due_days: 3 });
    expect(fresh.status(), await fresh.text()).toBe(201);
    const second = (await fresh.json()).data as Task;
    expect(second.uuid).not.toBe(created.uuid);

    list = await requests(same);
    expect(list).toHaveLength(2);
    expect(list.filter((t) => t.status === "open").map((t) => t.uuid)).toEqual([second.uuid]);
  });

  test("#630 okunamayan fiş Masraflar'da görünür, yeniden istenir (tek talep), kapatılır", async ({ page }) => {
    // İki bulanık "fiş" fotoğrafı; baytlar farklı (aynı görüntü `duplicate` olur).
    const uploaded: Capture[] = [];
    for (const seed of [1, 2]) {
      const res = await api.uploadField(`${base()}/receipts`, "image", {
        name: `fis-bulanik-${seed}.png`,
        mimeType: "image/png",
        buffer: blurryReceipt(seed),
      });
      uploaded.push(await api.json<Capture>(res));
    }
    test.skip(
      uploaded.some((c) => c.status !== "failed"),
      `AI açık, fotoğraf okundu (${uploaded.map((c) => c.status).join(", ")}); okunamayan fiş oluşturulamadı`,
    );
    expect((await failedCaptures()).total).toBe(2);
    // İsteğin başlığı yükleme gününü taşır; iki yükleme aynı gündeyse başlık aynıdır.
    const days = new Set(uploaded.map((c) => istanbulDay(c.created_at)));
    const sameDay = days.size === 1;

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/expenses`));
    const panel = unreadablePanel(page);
    await expect(panel).toBeVisible();
    await expect(panel.getByRole("heading", { name: "Okunamayan fiş fotoğrafları (2)" })).toBeVisible();
    const items = panel.getByRole("listitem");
    await expect(items).toHaveCount(2);
    await expect(items.first()).toContainText("yüklendi");
    await expect(items.first().getByRole("img", { name: /tarihli okunamayan fiş fotoğrafı$/ })).toBeVisible();

    // Mükelleften yeniden iste: belge talebi açılır.
    const ask = (i: number) => items.nth(i).getByRole("button", { name: "Mükelleften yeniden iste" });
    await ask(0).click();
    await expect(page.getByText("Mükellefe belge isteği gönderildi.").first()).toBeVisible();
    await expect(items.nth(0).getByRole("button", { name: "İstendi" })).toBeDisabled();
    const unreadable = (t: string) => t.startsWith(UNREADABLE_TITLE);
    await expect.poll(async () => (await requests(unreadable)).length).toBe(1);
    const [task] = await requests(unreadable);
    expect(task.status).toBe("open");
    expect(task.note).toContain("okunamadı");

    // Aynı günün ikinci fotoğrafı için istek aynı başlıkta: ikinci talep açılmaz.
    await ask(1).click();
    await expect(items.nth(1).getByRole("button", { name: "İstendi" })).toBeDisabled();
    await expect.poll(async () => (await requests(unreadable)).length).toBe(sameDay ? 1 : 2);

    // Sayfa yenilenince düğme yeniden basılabilir; yine yeni talep açılmaz.
    await page.reload();
    await expect(items).toHaveCount(2);
    await ask(0).click();
    await expect(items.nth(0).getByRole("button", { name: "İstendi" })).toBeDisabled();
    const after = await requests(unreadable);
    expect(after).toHaveLength(sameDay ? 1 : 2);
    expect(after.map((t) => t.uuid)).toContain(task.uuid);
    expect(after.every((t) => t.status === "open")).toBeTruthy();

    // Yüklemeleri kapat: liste boşalınca bölüm kaybolur; talep açık kalır.
    await items.first().getByRole("button", { name: "Kapat", exact: true }).click();
    await expect(page.getByText("Yükleme kapatıldı.").first()).toBeVisible();
    await expect(panel.getByRole("heading", { name: "Okunamayan fiş fotoğrafları (1)" })).toBeVisible();
    await expect(items).toHaveCount(1);
    await items.first().getByRole("button", { name: "Kapat", exact: true }).click();
    await expect(panel).toHaveCount(0);

    expect((await failedCaptures()).total).toBe(0);
    for (const c of uploaded) {
      const v = await api.json<Capture>(await api.get(`${base()}/receipts/${c.uuid}`));
      expect(v.status).toBe("rejected");
    }
    expect((await requests(unreadable)).every((t) => t.status === "open")).toBeTruthy();
  });
});

const unreadablePanel = (page: Page) => page.getByRole("region", { name: /^Okunamayan fiş fotoğrafları/ });

/** Yüklemenin İstanbul saatine göre günü (istek başlığındaki tarih). */
function istanbulDay(iso: string): string {
  return new Date(iso).toLocaleDateString("en-CA", { timeZone: "Europe/Istanbul" });
}

/**
 * Küçük, bulanık bir "fiş" PNG'si: soluk şerit üzerinde gri bantlar (demo
 * seed'in blurryPhoto'su gibi). `seed` her fotoğrafı farklı kılar; koşu
 * damgası da eklenir, önceki koşunun görüntüsüyle çakışmaz.
 */
function blurryReceipt(seed: number): Buffer {
  const w = 120;
  const h = 220;
  const stamp = Number(run) % 7;
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++) {
    const row = y * (w * 3 + 1);
    raw[row] = 0; // filtre yok
    const band = 18 * (1 + (Math.floor(y / 14) % 2));
    for (let x = 0; x < w; x++) {
      let v = 236 - band + ((x * 7 + y * 3 + seed * 11 + stamp) % 9);
      if (x < 12 || x > w - 12) v = 170 + seed;
      raw.set([v, v, v - 6], row + 1 + x * 3);
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr.set([8, 2, 0, 0, 0], 8); // 8 bit, RGB
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk("IHDR", ihdr),
    pngChunk("tEXt", Buffer.from(`Comment\0e2e ${run}-${seed}`, "latin1")),
    pngChunk("IDAT", deflateSync(raw)),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

function pngChunk(type: string, data: Buffer): Buffer {
  const head = Buffer.alloc(8);
  head.writeUInt32BE(data.length, 0);
  head.write(type, 4, "latin1");
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([head.subarray(4), data])), 0);
  return Buffer.concat([head, data, crc]);
}

function crc32(buf: Buffer): number {
  let c = 0xffffffff;
  for (const b of buf) {
    c ^= b;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  }
  return (c ^ 0xffffffff) >>> 0;
}
