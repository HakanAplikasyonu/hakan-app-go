import {
  expect,
  request as pwRequest,
  type APIRequestContext,
  type Page,
} from "@playwright/test";

export const API_URL = process.env.E2E_API_URL ?? "http://127.0.0.1:8080";
export const MAIL_URL = process.env.E2E_MAIL_URL ?? "http://127.0.0.1:8025";

/** Demo seed (backend/internal/demo): hesaplar, ofis ve mükellefler. */
export const DEMO = {
  password: process.env.DEMO_PASSWORD ?? "Demo1234!",
  org: "demo-smmm",
  owner: "sahip@demo.hakan.local",
  intern: "stajyer@demo.hakan.local",
  taxpayerUser: "mukellef@demo.hakan.local",
  atlas: { title: "Atlas Gıda Ticaret A.Ş.", taxId: "9000000017" },
  selin: { title: "Selin Yılmaz", taxId: "10000000146" },
  ublDir: "../backend/internal/demo/ubl",
} as const;

export const officePath = (p = "") => `/t/${DEMO.org}${p}`;

/** Arayüzden e-posta + şifre ile giriş. */
export async function uiLogin(page: Page, email: string, password = DEMO.password) {
  await page.goto("/login");
  await page.locator('input[name="email"], input[type="email"]').first().fill(email);
  await page.locator('input[type="password"]').first().fill(password);
  await page.locator('button[type="submit"]').first().click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 60_000 });
}

/** Go API'ye doğrudan erişen istemci (arayüzü olmayan adımlar ve denetimler). */
export class Api {
  private constructor(
    private readonly ctx: APIRequestContext,
    private readonly token: string,
  ) {}

  static async login(email: string, password = DEMO.password): Promise<Api> {
    const ctx = await pwRequest.newContext({ baseURL: API_URL });
    const res = await ctx.post("/v1/auth/login", { data: { email, password } });
    expect(res.status(), `login ${email}: ${await res.text()}`).toBe(200);
    const body = await res.json();
    return new Api(ctx, body.data.access_token as string);
  }

  private headers(extra: Record<string, string> = {}) {
    return { authorization: `Bearer ${this.token}`, "X-Organization": DEMO.org, ...extra };
  }

  async get(path: string, headers?: Record<string, string>) {
    return this.ctx.get(path, { headers: this.headers(headers) });
  }

  async post(path: string, data: unknown = {}, headers?: Record<string, string>) {
    return this.ctx.post(path, { headers: this.headers(headers), data });
  }

  async upload(path: string, file: { name: string; mimeType: string; buffer: Buffer }) {
    return this.ctx.post(path, { headers: this.headers(), multipart: { file } });
  }

  /** `data` of a 2xx JSON envelope; fails the test otherwise. */
  async json<T = any>(res: Awaited<ReturnType<APIRequestContext["get"]>>): Promise<T> {
    const text = await res.text();
    expect(res.ok(), `${res.url()} → ${res.status()} ${text}`).toBeTruthy();
    return JSON.parse(text).data as T;
  }

  async dispose() {
    await this.ctx.dispose();
  }
}

/** Ofisin `title` unvanlı mükellefle olan engagement'ı. */
export async function engagementOf(api: Api, title: string): Promise<string> {
  const page = await api.json<{ items: { uuid: string; taxpayer_uuid: string }[] }>(
    await api.get("/v1/tenant/engagements?limit=100"),
  );
  for (const e of page.items) {
    const tp = await api.json<{ title: string }>(
      await api.get(`/v1/tenant/engagements/${e.uuid}/taxpayer`),
    );
    if (tp.title === title) return e.uuid;
  }
  throw new Error(`engagement not found for ${title}`);
}

/** Mailhog'da `to` adresine giden son e-postadaki onay bağlantısının belirteci. */
export async function consentTokenFor(to: string): Promise<string> {
  const ctx = await pwRequest.newContext({ baseURL: MAIL_URL });
  try {
    for (let i = 0; i < 30; i++) {
      const res = await ctx.get("/api/v2/search", { params: { kind: "to", query: to, limit: "5" } });
      if (res.ok()) {
        const body = await res.json();
        for (const m of body.items ?? []) {
          const raw: string = (m.Content?.Body ?? "").replace(/=\r?\n/g, "");
          const hit = raw.match(/\/consent\/([A-Za-z0-9_-]{20,})/);
          if (hit) return hit[1];
        }
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
  } finally {
    await ctx.dispose();
  }
  throw new Error(`no consent mail for ${to}`);
}

/** Ofis ekranlarındaki mükellef seçimi (yerel <select>). */
export async function pickTaxpayer(page: Page, title: string) {
  const select = page.locator(`select:has(option:text-matches("^${escapeRe(title)}"))`).first();
  await expect(select).toBeVisible();
  const option = select.locator("option", { hasText: title }).first();
  const value = await option.getAttribute("value");
  await select.selectOption(value!);
  await page.waitForURL((u) => u.searchParams.get("taxpayer") === value);
}

function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
