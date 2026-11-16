import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";

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
  admin: process.env.E2E_SA_EMAIL ?? "sa@e2e.local",
  adminPassword: process.env.E2E_SA_PASSWORD ?? "E2e-Admin-123!",
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

/**
 * Mailhog'da `to` adresine giden e-postalardaki `path` bağlantısının
 * belirteci (ör. "/consent/", "/invite/", "/invite/m/"). Belirteçler URL-güvenli
 * base64; quoted-printable satır kırılmaları önce birleştirilir.
 */
export async function mailToken(to: string, route: string): Promise<string> {
  const ctx = await pwRequest.newContext({ baseURL: MAIL_URL });
  const re = new RegExp(`${escapeRe(route)}([A-Za-z0-9_-]{20,})`);
  try {
    for (let i = 0; i < 45; i++) {
      const res = await ctx.get("/api/v2/search", { params: { kind: "to", query: to, limit: "10" } });
      if (res.ok()) {
        const body = await res.json();
        for (const m of body.items ?? []) {
          const raw: string = (m.Content?.Body ?? "").replace(/=\r?\n/g, "").replace(/=3D/g, "=");
          const hit = raw.match(re);
          if (hit) return hit[1];
        }
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
  } finally {
    await ctx.dispose();
  }
  throw new Error(`no ${route} mail for ${to}`);
}

/** Mükellef ekleme onay e-postasındaki belirteç. */
export const consentTokenFor = (to: string) => mailToken(to, "/consent/");

/**
 * Davet ya da onay belirteciyle hesap oluşturur ve `next` sayfasına döner
 * (`/platform/register?invitation=...`).
 */
export async function registerWithToken(
  page: Page,
  token: string,
  next: string,
  person: { name: string; surname: string; password: string },
) {
  await page.goto(`/platform/register?invitation=${token}&next=${encodeURIComponent(next)}`);
  await page.locator('input[name="name"]').fill(person.name);
  await page.locator('input[name="surname"]').fill(person.surname);
  await page.locator('input[type="password"]').first().fill(person.password);
  await expect(page.getByText("Kayıt bulunamadı")).toHaveCount(0); // #146
  await page.locator('button[type="submit"]').click();
  await page.waitForURL((u) => u.pathname === next);
}

/** Her koşuda farklı, kontrol hanesi geçerli bir VKN (9 hane + kontrol). */
export function uniqueVkn(): string {
  const base = `7${String(Date.now()).slice(-6)}${String(Math.floor(Math.random() * 100)).padStart(2, "0")}`;
  const d = base.split("").map(Number);
  let sum = 0;
  for (let i = 0; i < 9; i++) {
    const t = (d[i] + 9 - i) % 10;
    if (t === 0) continue;
    let v = (t * 2 ** (9 - i)) % 9;
    if (v === 0) v = 9;
    sum += v;
  }
  return base + String((10 - (sum % 10)) % 10);
}

/** Benzersiz e-posta (Mailhog'da aramak ve kayıt çakışmasını önlemek için). */
export const uniqueEmail = (tag: string) =>
  `e2e-${tag}-${Date.now()}-${Math.floor(Math.random() * 1e4)}@deneme.local`;

/**
 * Demo Atlas UBL'lerinden yeni bir mükellef için fatura üretir: VKN, unvan
 * ve ETTN değişir; tutarlar ve yapı aynı kalır (XSD + Schematron geçerli).
 */
export function deriveUbl(file: string, vkn: string, title: string): { name: string; mimeType: string; buffer: Buffer } {
  let xml = readFileSync(path.join(DEMO.ublDir, file), "utf8");
  // ETTN: ilk UUID öğesi (önekli ya da varsayılan ad alanlı yazılabilir).
  const ettn = xml.match(/<(?:cbc:)?UUID\b[^>]*>([^<]+)<\/(?:cbc:)?UUID>/)?.[1];
  if (!ettn) throw new Error(`no ETTN in ${file}`);
  xml = xml
    .split(DEMO.atlas.taxId).join(vkn)
    .split(DEMO.atlas.title).join(title)
    .split(ettn).join(randomUUID().toUpperCase());
  return { name: file.replace("atlas-", "e2e-"), mimeType: "application/xml", buffer: Buffer.from(xml, "utf8") };
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
