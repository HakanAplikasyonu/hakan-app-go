import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Konsol ve ağ bekçisi (#639): ana ofis ekranları gezilirken
 * - 4xx/5xx yanıt olmaz (boş durum 404/503 ile bildirilmez),
 * - konsol hatası ve yakalanmamış sayfa hatası olmaz,
 * - bir sayfa açılışında aynı API isteği 3'ten fazla gitmez. "Aynı istek":
 *   yöntem + yol + sorgu; içindeki uuid ve sayılar soyutlanır. Böylece
 *   satır başına istek (N+1: /engagements/:id/taxpayer × satır sayısı) ve
 *   yeniden çekme döngüsü yakalanır; aynı ucu farklı süzgeçle isteyen ayrı
 *   sorgular (dönem matrisi 1 ay / 12 ay gibi) sayılmaz. Next.js'in bağlantı
 *   ön yüklemeleri (/t/... sayfa istekleri) API değildir, sayılmaz.
 * Bilinçli istisnalar aşağıdaki izin listelerinde gerekçesiyle durur.
 *
 * Yalnızca okur; demo verisini değiştirmez. Her ekran yumuşak denetlenir:
 * bir ekrandaki bulgu öbürlerinin taranmasını durdurmaz, CI hepsini tek
 * koşuda listeler.
 */

const MAX_SAME_REQUEST = 3;

const OFFICE = ["", "/taxpayers", "/command", "/warnings", "/journals", "/transfers"];

const ENGAGEMENT = [
  "",
  "/documents",
  "/archive",
  "/invoices",
  "/bank",
  "/review",
  "/warnings",
  "/journals",
  "/reconciliation",
  "/trial-balance",
  "/connections",
  "/expenses",
  "/z-reports",
  "/payroll",
  "/tax-returns",
  "/excel-imports",
  "/petitions",
  "/privacy",
  "/report",
];

/** İzin verilen hata yanıtları: [yöntem + uç kalıbı, durum, gerekçe]. */
const ALLOWED_ERRORS: { re: RegExp; status: number; why: string }[] = [];

/** Aynı isteğin 3'ten fazla gitmesine izin verilenler: [istek kalıbı, gerekçe]. */
const ALLOWED_REPEATS: { re: RegExp; why: string }[] = [];

const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;

/** "GET /api/v1/tenant/engagements/:id/bank-transactions" biçiminde uç adı. */
function endpoint(method: string, url: string): string {
  const u = new URL(url);
  return `${method} ${u.pathname.replace(UUID, ":id").replace(/\/\d+(?=\/|$)/g, "/:n")}`;
}

/** Uç adı + soyutlanmış, sıralı sorgu: "aynı istek" anahtarı. */
function requestKey(method: string, url: string): string {
  const u = new URL(url);
  const params = [...u.searchParams]
    .map(([k, v]) => `${k}=${v.replace(UUID, ":id")}`)
    .sort()
    .join("&");
  return params ? `${endpoint(method, url)}?${params}` : endpoint(method, url);
}

type Findings = { errors: string[]; console: string[]; requests: Map<string, number> };

function watch(page: Page, origin: string) {
  let f: Findings = { errors: [], console: [], requests: new Map() };
  page.on("request", (req) => {
    if (!req.url().startsWith(`${origin}/api/`) || !["fetch", "xhr"].includes(req.resourceType())) return;
    const key = requestKey(req.method(), req.url());
    f.requests.set(key, (f.requests.get(key) ?? 0) + 1);
  });
  page.on("response", (res) => {
    if (!res.url().startsWith(origin) || res.status() < 400) return;
    const key = endpoint(res.request().method(), res.url());
    if (ALLOWED_ERRORS.some((a) => a.status === res.status() && a.re.test(key))) return;
    f.errors.push(`${res.status()} ${key} (${new URL(res.url()).search})`);
  });
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    // Ağ hataları yukarıda yanıt olarak sayılır; tarayıcının kopyası değil.
    if (msg.text().startsWith("Failed to load resource")) return;
    f.console.push(msg.text().slice(0, 300));
  });
  page.on("pageerror", (err) => f.console.push(`pageerror: ${err.message.slice(0, 300)}`));
  return {
    reset() {
      f = { errors: [], console: [], requests: new Map() };
    },
    get: () => f,
  };
}

async function settle(page: Page) {
  await page.waitForLoadState("networkidle");
  await expect(page.locator("main").first()).toBeVisible();
  await expect
    .poll(async () => page.locator('[data-slot="skeleton"], .animate-pulse').count(), { timeout: 15_000 })
    .toBe(0)
    .catch(() => undefined);
  // Yükleme bittikten sonra gelen (yeniden deneme, yeniden çizim) istekler de sayılsın.
  await page.waitForTimeout(1_500);
  await page.waitForLoadState("networkidle");
}

async function scan(page: Page, w: ReturnType<typeof watch>, path: string) {
  await test.step(path, async () => {
    w.reset();
    await page.goto(path);
    await settle(page);
    const f = w.get();
    expect.soft(f.errors, `${path}: hata dönen istek\n${f.errors.join("\n")}`).toEqual([]);
    expect.soft(f.console, `${path}: konsol hatası\n${f.console.join("\n")}`).toEqual([]);
    const repeated = [...f.requests]
      .filter(([key, n]) => n > MAX_SAME_REQUEST && !ALLOWED_REPEATS.some((a) => a.re.test(key.split("?")[0])))
      .map(([key, n]) => `${n}× ${key}`);
    expect
      .soft(repeated, `${path}: aynı istek ${MAX_SAME_REQUEST}'ten fazla\n${repeated.join("\n")}`)
      .toEqual([]);
  });
}

test.describe("#639 konsol ve ağ bekçisi", () => {
  let atlas = "";

  test.beforeAll(async () => {
    const api = await Api.login(DEMO.owner);
    try {
      atlas = await engagementOf(api, DEMO.atlas.title);
    } finally {
      await api.dispose();
    }
  });

  test("ofis ekranlarında hata yanıtı, konsol hatası ve tekrarlı istek yok", async ({ page, baseURL }) => {
    test.setTimeout(300_000);
    await uiLogin(page, DEMO.owner);
    const w = watch(page, new URL(baseURL!).origin);
    for (const p of OFFICE) await scan(page, w, officePath(p));
  });

  test("mükellef sekmelerinde hata yanıtı, konsol hatası ve tekrarlı istek yok", async ({ page, baseURL }) => {
    test.setTimeout(420_000);
    await uiLogin(page, DEMO.owner);
    const w = watch(page, new URL(baseURL!).origin);
    for (const p of ENGAGEMENT) await scan(page, w, officePath(`/engagements/${atlas}${p}`));
  });
});
