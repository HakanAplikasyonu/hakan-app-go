import { createHmac, randomUUID } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

import {
  expect,
  request as pwRequest,
  type APIRequestContext,
  type Browser,
  type Locator,
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
    private readonly org: string,
  ) {}

  /** `org`: X-Organization (ofis kısa adı); varsayılan demo ofisi. */
  static async login(email: string, password = DEMO.password, org: string = DEMO.org): Promise<Api> {
    const ctx = await pwRequest.newContext({ baseURL: API_URL });
    const res = await ctx.post("/v1/auth/login", { data: { email, password } });
    expect(res.status(), `login ${email}: ${await res.text()}`).toBe(200);
    const body = await res.json();
    return new Api(ctx, body.data.access_token as string, org);
  }

  private headers(extra: Record<string, string> = {}) {
    return { authorization: `Bearer ${this.token}`, "X-Organization": this.org, ...extra };
  }

  async get(path: string, headers?: Record<string, string>) {
    return this.ctx.get(path, { headers: this.headers(headers) });
  }

  async post(path: string, data: unknown = {}, headers?: Record<string, string>) {
    return this.ctx.post(path, { headers: this.headers(headers), data });
  }

  async delete(path: string, headers?: Record<string, string>) {
    return this.ctx.delete(path, { headers: this.headers(headers) });
  }

  /** Dosya ve ek form alanları (ör. mizan yüklemesinde `period`). */
  async uploadWith(
    path: string,
    file: { name: string; mimeType: string; buffer: Buffer },
    fields: Record<string, string>,
  ) {
    return this.ctx.post(path, { headers: this.headers(), multipart: { ...fields, file } });
  }

  async put(path: string, data: unknown = {}, headers?: Record<string, string>) {
    return this.ctx.put(path, { headers: this.headers(headers), data });
  }

  async patch(path: string, data: unknown = {}, headers?: Record<string, string>) {
    return this.ctx.patch(path, { headers: this.headers(headers), data });
  }

  /** Çok parçalı PUT, alan adı `file` (ör. beyanın tahakkuk PDF'i). */
  async uploadPut(path: string, file: { name: string; mimeType: string; buffer: Buffer }) {
    return this.ctx.put(path, { headers: this.headers(), multipart: { file } });
  }

  async upload(path: string, file: { name: string; mimeType: string; buffer: Buffer }) {
    return this.ctx.post(path, { headers: this.headers(), multipart: { file } });
  }

  /** Çok parçalı yükleme, alan adı `field` (ör. fiş fotoğrafı: "image"). */
  async uploadField(path: string, field: string, file: { name: string; mimeType: string; buffer: Buffer }) {
    return this.ctx.post(path, { headers: this.headers(), multipart: { [field]: file } });
  }

  /** uploadField, ek başlıklarla (ör. mükellef uçlarında `X-Taxpayer`). */
  async uploadFieldWith(
    path: string,
    field: string,
    file: { name: string; mimeType: string; buffer: Buffer },
    headers: Record<string, string>,
  ) {
    return this.ctx.post(path, { headers: this.headers(headers), multipart: { [field]: file } });
  }

  /** Birden çok alanlı multipart form (dosya + metin alanları). */
  async postForm(
    path: string,
    form: Record<string, string | { name: string; mimeType: string; buffer: Buffer }>,
    headers?: Record<string, string>,
  ) {
    return this.ctx.post(path, { headers: this.headers(headers), multipart: form });
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

/**
 * Engagement'ın yükleme işleri bitene ve toplam `added` fatura eklenene kadar
 * bekler. Her dosya ayrı bir iş; ekranda ilk "Eklendi" görünmesi diğerlerinin
 * bittiği anlamına gelmez (#221).
 */
export async function waitForUploads(api: Api, engagement: string, added: number) {
  await expect
    .poll(
      async () => {
        const jobs = await api.json<{ items: { status: string; report?: { added: number } }[] }>(
          await api.get(`/v1/tenant/engagements/${engagement}/ubl-uploads?limit=50`),
        );
        if (jobs.items.some((j) => j.status !== "done")) return -1;
        return jobs.items.reduce((n, j) => n + (j.report?.added ?? 0), 0);
      },
      { timeout: 90_000 },
    )
    .toBe(added);
}

/** Ofisin `title` unvanlı mükellefle olan engagement'ı. */
export async function engagementOf(api: Api, title: string): Promise<string> {
  // Liste en yeniden eskiye sıralı ve sayfa en fazla 100. Akış testleri
  // mükellef ekledikçe demo mükellefleri (Atlas) ilk sayfadan düşer; bu
  // yüzden bütün sayfalar gezilir. Satır unvanı (taxpayer_title) taşır, her
  // satır için ayrı istek atılmaz (hız sınırı).
  const limit = 100;
  for (let offset = 0; ; offset += limit) {
    const page = await api.json<{
      items: { uuid: string; taxpayer_title: string }[];
      total: number;
    }>(await api.get(`/v1/tenant/engagements?limit=${limit}&offset=${offset}`));
    const hit = page.items.find((e) => e.taxpayer_title === title);
    if (hit) return hit.uuid;
    if (page.items.length < limit || offset + limit >= page.total) break;
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
  // Aranabilir seçici (#573): düğme "Mükellef: …", içinde arama ve liste.
  const picker = page.getByRole("combobox", { name: /^Mükellef:/ }).first();
  const select = page.locator(`select:has(option:text-matches("^${escapeRe(title)}"))`).first();
  await expect(picker.or(select)).toBeVisible();
  if (await picker.isVisible()) {
    const name = new RegExp(`^Mükellef: ${escapeRe(title)}`);
    if (name.test((await picker.getAttribute("aria-label")) ?? "")) return;
    const before = new URL(page.url()).searchParams.get("taxpayer");
    await picker.click();
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(title);
    await page.getByRole("option", { name: new RegExp(escapeRe(title)) }).first().click();
    await expect(picker).toHaveAccessibleName(name);
    await page.waitForURL((u) => {
      const now = u.searchParams.get("taxpayer");
      return !!now && now !== before;
    });
    return;
  }
  const option = select.locator("option", { hasText: title }).first();
  const value = await option.getAttribute("value");
  await select.selectOption(value!);
  await page.waitForURL((u) => u.searchParams.get("taxpayer") === value);
}

function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Mektubun yanıt belirteci (backend reconciliation.LetterToken): HMAC-SHA256,
 * anahtar "reconciliation-letter:" + APP_ENCRYPTION_KEY, ileti mektup
 * UUID'sinin 16 baytı. Anahtar scripts/e2e-up.sh'ın env dosyasından okunur.
 */
export function letterToken(letterUuid: string): string {
  const envFile = path.join(process.env.E2E_DIR ?? path.join(__dirname, "..", ".run"), "e2e.env");
  let key = process.env.APP_ENCRYPTION_KEY ?? "app-dev-encryption-key-32bytes!!";
  if (existsSync(envFile)) {
    for (const line of readFileSync(envFile, "utf8").split(/\r?\n/)) {
      const m = line.match(/^APP_ENCRYPTION_KEY=(.*)$/);
      if (m) key = m[1];
    }
  }
  const id = Buffer.from(letterUuid.replace(/-/g, ""), "hex");
  return createHmac("sha256", `reconciliation-letter:${key}`).update(id).digest("hex");
}

/**
 * Demo ofisine yeni bir mükellef: ofis onay ister, mükellef kaydolup onaylar.
 * Engagement uuid'ini döner. Dosyalar kendi mükellefine dokunur (#468, #469).
 */
export async function addConsentedTaxpayer(
  browser: Browser,
  api: Api,
  taxpayer: { vkn: string; title: string },
  tag: string,
): Promise<string> {
  const contact = uniqueEmail(tag);
  const office = await (await browser.newContext()).newPage();
  try {
    await uiLogin(office, DEMO.owner);
    await office.goto(officePath("/taxpayer-requests"));
    await office.locator('input[name="tax_id"]').fill(taxpayer.vkn);
    await office.locator('input[name="title"]').fill(taxpayer.title);
    await office.locator('input[name="contact_email"]').fill(contact);
    await office.getByRole("button", { name: "Onay iste" }).click();
    await expect(office.getByText(contact).first()).toBeVisible();
  } finally {
    await office.context().close();
  }
  const token = await consentTokenFor(contact);
  const tp = await (await browser.newContext()).newPage();
  try {
    await registerWithToken(tp, token, `/consent/${token}`, {
      name: "E2E",
      surname: "Mükellef",
      password: "E2e-Parola-123!",
    });
    await expect(tp.getByRole("checkbox").first()).toBeVisible();
    for (const box of await tp.getByRole("checkbox").all()) await box.check();
    await tp.getByRole("button", { name: "Onayla" }).click();
    await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
  } finally {
    await tp.context().close();
  }
  return engagementOf(api, taxpayer.title);
}

/**
 * addConsentedTaxpayer gibi, ama mükellef kullanıcısının giriş bilgisini de
 * döner: mükellef tarafındaki uçları (`/v1/taxpayer/...`, `X-Taxpayer`)
 * çağıran senaryolar için. `taxpayerUuid` X-Taxpayer başlığının değeridir.
 */
export async function addConsentedTaxpayerWithUser(
  browser: Browser,
  api: Api,
  taxpayer: { vkn: string; title: string },
  tag: string,
): Promise<{ engagement: string; taxpayerUuid: string; email: string; password: string }> {
  const email = uniqueEmail(tag);
  const password = "E2e-Parola-123!";
  const office = await (await browser.newContext()).newPage();
  try {
    await uiLogin(office, DEMO.owner);
    await office.goto(officePath("/taxpayer-requests"));
    await office.locator('input[name="tax_id"]').fill(taxpayer.vkn);
    await office.locator('input[name="title"]').fill(taxpayer.title);
    await office.locator('input[name="contact_email"]').fill(email);
    await office.getByRole("button", { name: "Onay iste" }).click();
    await expect(office.getByText(email).first()).toBeVisible();
  } finally {
    await office.context().close();
  }
  const token = await consentTokenFor(email);
  const tp = await (await browser.newContext()).newPage();
  try {
    await registerWithToken(tp, token, `/consent/${token}`, { name: "E2E", surname: "Mükellef", password });
    await expect(tp.getByRole("checkbox").first()).toBeVisible();
    for (const box of await tp.getByRole("checkbox").all()) await box.check();
    await tp.getByRole("button", { name: "Onayla" }).click();
    await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
  } finally {
    await tp.context().close();
  }
  const engagement = await engagementOf(api, taxpayer.title);
  const row = await api.json<{ uuid: string }>(await api.get(`/v1/tenant/engagements/${engagement}/taxpayer`));
  return { engagement, taxpayerUuid: row.uuid, email, password };
}

/**
 * Açık açılır pencerenin (Radix Popover) giriş animasyonu bitene ve yeri
 * iki karede sabit kalana kadar bekler; içindeki düğmeye tıklarken
 * "element is not stable" düşmelerini önler. Pencereyi döndürür.
 */
export async function settledPopover(page: Page) {
  const pop = page.locator('[data-slot="popover-content"][data-state="open"]').last();
  await expect(pop).toBeVisible();
  await expect
    .poll(
      () =>
        pop.evaluate(async (el) => {
          await Promise.all(el.getAnimations({ subtree: true }).map((a) => a.finished.catch(() => undefined)));
          const frame = () => new Promise((r) => requestAnimationFrame(() => r(el.getBoundingClientRect())));
          const a = (await frame()) as DOMRect;
          const b = (await frame()) as DOMRect;
          return a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height;
        }),
      { timeout: 10_000 },
    )
    .toBe(true);
  return pop;
}

/**
 * Ekranda ham kod kalmasın (#710): snake_case alan / kaynak adı
 * (`payable_amount`, `manual_upload`), çevrilmemiş i18n anahtarı
 * (`office.x.y`) ve UBL / yön enum'u (`SATIS`, `incoming`). E-posta ve
 * adresler önce çıkarılır. Bulunanlar hata iletisinde listelenir.
 */
export const RAW_TEXT =
  /\b[a-z]+(?:_[a-z0-9]+)+\b|\b(?:office|common|sources|taxpayer|setup)\.[a-z_]+(?:\.[a-z_]+)*\b|\b(?:SATIS|ALIS|IADE|TEVKIFAT|ISTISNA|OZELMATRAH|IHRACKAYITLI|TEMELFATURA|TICARIFATURA|EARSIVFATURA|incoming|outgoing)\b/g;

export async function expectNoRawText(target: Locator): Promise<void> {
  const text = (await target.innerText())
    .replace(/\S+@\S+/g, " ")
    .replace(/https?:\/\/\S+/g, " ");
  expect(text.match(RAW_TEXT) ?? [], text).toEqual([]);
}

/**
 * Fişler ekranında yeni fiş formunu açar (#710): form açılır pencerede
 * gelir; kaydedilince pencere kapanır.
 */
export async function openNewEntry(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Yeni fiş", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Yeni fiş" })).toBeVisible();
}
