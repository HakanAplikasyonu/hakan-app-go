// HAKAN API istemcisi (arka plan hizmet çalışanında çalışır).
//
// Kimlik: panelde üretilen tek kullanımlık eşleştirme kodu
// /v1/auth/whatsapp/pair ile normal bir oturuma çevrilir. Yenileme belirteci
// chrome.storage.local'da (bu tarayıcı profili), erişim belirteci
// chrome.storage.session'da (tarayıcı kapanınca silinir) durur. Oturum
// panelde Hesap → Oturumlar listesinde görünür ve oradan kapatılabilir.

import type { Capture, ChatLookup, OpenTask, Status, Target, Taxpayer } from "./messages";
import {
  guessWebBase,
  links,
  SUMMARY_LIMIT,
  taskItems,
  vatItems,
  type ClientWithTasks,
  type InboxCounts,
  type Summary,
  type VatWatchEvent,
} from "./summary";

export interface KV {
  get(key: string): Promise<unknown>;
  set(key: string, value: unknown): Promise<void>;
  remove(key: string): Promise<void>;
}

interface Session {
  apiBase: string;
  refreshToken: string;
  organization: { uuid: string; slug: string; name: string };
  user: { uuid: string; email: string; name: string };
}

interface Access {
  token: string;
  expiresAt: number;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code = "",
  ) {
    super(message);
  }
}

/** Kullanıcıya gösterilen Türkçe hata metni. */
export function describeError(e: unknown): { message: string; code?: string } {
  if (!(e instanceof ApiError)) {
    return { message: "Sunucuya ulaşılamadı. İnternet bağlantınızı kontrol edin; sürerse ofis yöneticinize haber verin.", code: "UNREACHABLE" };
  }
  const byCode: Record<string, string> = {
    PAIRING_CODE_INVALID:
      "Kod tutmadı: yanlış yazılmış, süresi dolmuş (10 dakika) ya da daha önce kullanılmış. Panelden yeni kod alıp yeniden deneyin.",
    PAIRING_UNAVAILABLE: "Sunucuda eşleştirme şu an kapalı. Ofis yöneticinize haber verin.",
    NOT_A_SERVER: "Bu adreste Easytrade sunucusu yanıt vermedi. Paketi paneldeki WhatsApp sayfasından yeniden indirin.",
    IMAGE_REQUIRED: "Gider ve gelir fotoğraftan okunur. PDF'i belge olarak kaydedin.",
    CHAT_LINKED_ELSEWHERE: "Bu sohbet göremediğiniz bir mükellefe eşlenmiş. Ofis yöneticinize danışın.",
    TASK_CLOSED: "Belge görevi kapanmış. Görevsiz kaydedin.",
    AI_UNAVAILABLE: "Fiş okuma şu an kullanılamıyor. Biraz sonra yeniden deneyin.",
    RATE_LIMITED: "Çok fazla deneme yapıldı. Birkaç dakika sonra yeniden deneyin.",
    UPLOAD_TOO_LARGE: "Dosya çok büyük (en fazla 20 MB).",
    SIGNED_OUT: "Uzantının oturumu kapandı. Panelden yeni kodla yeniden eşleştirin.",
  };
  if (byCode[e.code]) return { message: byCode[e.code]!, code: e.code };
  if (e.status === 403) return { message: "Bu işlem için yetkiniz yok.", code: e.code };
  if (e.status === 404) return { message: "Kayıt bulunamadı ya da bu mükellefe erişiminiz yok.", code: e.code };
  return { message: e.message || "İşlem tamamlanamadı.", code: e.code };
}

/** Kullanıcı girdisini API köküne çevirir: uzak sunucuda https zorunlu. */
export function normalizeBase(raw: string): string {
  let s = raw.trim().replace(/\/+$/, "");
  s = s.replace(/\/v1$/, "");
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    throw new ApiError("Sunucu adresi geçerli bir adres olmalı (https://…).", 0, "INVALID_BASE");
  }
  const local = ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname);
  if (u.protocol !== "https:" && !(u.protocol === "http:" && local)) {
    throw new ApiError("Uzak sunucu için https gerekli.", 0, "INVALID_BASE");
  }
  return u.origin + u.pathname.replace(/\/+$/, "");
}

export class Api {
  private refreshing: Promise<string> | null = null;

  constructor(
    private readonly local: KV,
    private readonly session: KV,
    private readonly fetchFn: typeof fetch = (...a) => fetch(...a),
    private readonly now: () => number = () => Date.now(),
  ) {}

  async status(): Promise<Status> {
    const s = (await this.local.get("session")) as Session | undefined;
    const base = ((await this.local.get("apiBase")) as string | undefined) ?? "";
    return {
      paired: !!s,
      apiBase: s?.apiBase ?? base,
      organization: s ? { slug: s.organization.slug, name: s.organization.name } : null,
      user: s ? { email: s.user.email, name: s.user.name } : null,
    };
  }

  async pair(apiBase: string, code: string, deviceName: string): Promise<Status> {
    const base = normalizeBase(apiBase);
    await this.local.set("apiBase", base);
    const res = await this.fetchFn(base + "/v1/auth/whatsapp/pair", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, device_name: deviceName }),
    });
    const data = await envelope<{
      access_token: string;
      refresh_token: string;
      expires_in: number;
      user: Session["user"];
      organization: Session["organization"];
    }>(res);
    await this.local.set("session", {
      apiBase: base,
      refreshToken: data.refresh_token,
      organization: data.organization,
      user: data.user,
    } satisfies Session);
    await this.setAccess(data.access_token, data.expires_in);
    return this.status();
  }

  async unpair(): Promise<void> {
    const s = (await this.local.get("session")) as Session | undefined;
    if (s) {
      try {
        // Oturumu sunucuda da kapat (yenileme belirteci iptal).
        await this.request("POST", "/v1/auth/logout", { json: { refresh_token: s.refreshToken } });
      } catch {
        // Ağ yoksa yerel oturum yine silinir; sunucudaki oturum panelden kapatılır.
      }
    }
    await this.local.remove("session");
    await this.session.remove("access");
  }

  /** "Bağlantıyı test et": oturumla kimlikli bir istek (yenileme dahil). */
  async ping(): Promise<void> {
    await this.request("GET", "/v1/auth/me");
  }

  lookup(chatKey: string): Promise<ChatLookup> {
    return this.request("GET", "/v1/tenant/whatsapp/chat-lookup?chat_key=" + encodeURIComponent(chatKey));
  }

  async search(q: string): Promise<Taxpayer[]> {
    const page = await this.request<{ items: Array<Taxpayer & { title: string }> }>(
      "GET",
      "/v1/tenant/clients?limit=20&q=" + encodeURIComponent(q.trim().slice(0, 100)),
    );
    return page.items.map((c) => ({ engagement_uuid: c.engagement_uuid, taxpayer_uuid: c.taxpayer_uuid, title: c.title, tax_id: c.tax_id }));
  }

  link(engagementUuid: string, chatKey: string, label: string): Promise<ChatLookup> {
    return this.request("PUT", `/v1/tenant/engagements/${encodeURIComponent(engagementUuid)}/whatsapp/chat-link`, {
      json: { chat_key: chatKey, label },
    });
  }

  async openTasks(engagementUuid: string): Promise<OpenTask[]> {
    const page = await this.request<{ items: OpenTask[] }>(
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagementUuid)}/document-tasks?status=open&limit=20`,
    );
    return page.items ?? [];
  }

  capture(engagementUuid: string, target: Target, chatKey: string, file: Blob, fileName: string, taskUuid?: string): Promise<Capture> {
    const form = new FormData();
    form.set("target", target);
    if (chatKey) form.set("chat_key", chatKey);
    if (taskUuid) form.set("task_uuid", taskUuid);
    form.set("file", file, fileName);
    return this.request("POST", `/v1/tenant/engagements/${encodeURIComponent(engagementUuid)}/whatsapp/captures`, { form });
  }

  /** Panel adresi: kullanıcının yazdığı, yoksa API adresinden tahmin. */
  async webBase(): Promise<string> {
    const saved = (await this.local.get("webBase")) as string | undefined;
    if (saved) return saved;
    const s = (await this.local.get("session")) as Session | undefined;
    return s ? guessWebBase(s.apiBase) : "";
  }

  async setWebBase(raw: string): Promise<string> {
    if (!raw.trim()) {
      await this.local.remove("webBase");
    } else {
      await this.local.set("webBase", normalizeBase(raw));
    }
    return this.webBase();
  }

  /**
   * Ofis özeti (#780): Gelen kutusu sayıları, son KDV uyarıları ve açık belge
   * isteği olan mükellefler. Okunamayan bölüm (yetki yok, uç nokta henüz
   * sunucuda yok) null döner; oturum kapandıysa hata yükselir.
   */
  async summary(): Promise<Summary & { webBase: string }> {
    const s = (await this.local.get("session")) as Session | undefined;
    if (!s) throw new ApiError("Uzantı eşleştirilmemiş.", 401, "SIGNED_OUT");
    const webBase = await this.webBase();
    const l = links(webBase, s.organization.slug);
    const soft = <T>(p: Promise<T>): Promise<T | null> =>
      p.catch((e: unknown) => {
        if (e instanceof ApiError && e.code === "SIGNED_OUT") throw e;
        return null;
      });
    const [inbox, vat, clients] = await Promise.all([
      soft(this.request<{ counts: InboxCounts }>("GET", "/v1/tenant/inbox?limit=1")),
      soft(this.request<{ items: VatWatchEvent[] }>("GET", `/v1/tenant/vat-forecast/watch-events?limit=${SUMMARY_LIMIT}`)),
      soft(this.request<{ items: ClientWithTasks[] }>("GET", `/v1/tenant/clients?filter=tasks&limit=${SUMMARY_LIMIT}`)),
    ]);
    return {
      webBase,
      inbox: inbox?.counts ? { counts: inbox.counts, url: l.inbox } : null,
      vat: vat ? { items: vatItems(vat.items ?? [], l), url: l.reports } : null,
      tasks: clients ? { items: taskItems(clients.items ?? [], l), url: l.documentCollection } : null,
      fetchedAt: this.now(),
    };
  }

  /** Kimlikli istek: 401'de bir kez yenileyip yeniden dener. */
  async request<T>(method: string, path: string, body?: { json?: unknown; form?: FormData }): Promise<T> {
    const s = (await this.local.get("session")) as Session | undefined;
    if (!s) throw new ApiError("Uzantı eşleştirilmemiş.", 401, "SIGNED_OUT");
    const send = async (token: string) => {
      const headers: Record<string, string> = { Authorization: "Bearer " + token, "X-Organization": s.organization.slug };
      let payload: BodyInit | undefined;
      if (body?.json !== undefined) {
        headers["Content-Type"] = "application/json";
        payload = JSON.stringify(body.json);
      } else if (body?.form) {
        payload = body.form;
      }
      return this.fetchFn(s.apiBase + path, { method, headers, body: payload });
    };
    let res = await send(await this.accessToken());
    if (res.status === 401) {
      res = await send(await this.refresh());
    }
    return envelope<T>(res);
  }

  private async accessToken(): Promise<string> {
    const a = (await this.session.get("access")) as Access | undefined;
    if (a && a.expiresAt > this.now()) return a.token;
    return this.refresh();
  }

  /** Aynı anda gelen yenilemeler tek istekte birleşir (belirteç döner). */
  private refresh(): Promise<string> {
    if (!this.refreshing) {
      this.refreshing = this.doRefresh().finally(() => {
        this.refreshing = null;
      });
    }
    return this.refreshing;
  }

  private async doRefresh(): Promise<string> {
    const s = (await this.local.get("session")) as Session | undefined;
    if (!s) throw new ApiError("Uzantı eşleştirilmemiş.", 401, "SIGNED_OUT");
    const res = await this.fetchFn(s.apiBase + "/v1/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: s.refreshToken }),
    });
    if (res.status === 401 || res.status === 400) {
      await this.local.remove("session");
      await this.session.remove("access");
      throw new ApiError("Oturum kapandı.", 401, "SIGNED_OUT");
    }
    const t = await envelope<{ access_token: string; refresh_token: string; expires_in: number }>(res);
    await this.local.set("session", { ...s, refreshToken: t.refresh_token });
    await this.setAccess(t.access_token, t.expires_in);
    return t.access_token;
  }

  private async setAccess(token: string, expiresIn: number): Promise<void> {
    // Süre dolmadan bir dakika önce yenile.
    await this.session.set("access", { token, expiresAt: this.now() + Math.max(0, expiresIn - 60) * 1000 } satisfies Access);
  }
}

async function envelope<T>(res: Response): Promise<T> {
  let body: { success?: boolean; data?: T; error?: { code?: string; message?: string } } = {};
  try {
    body = await res.json();
  } catch {
    // Boş gövde (204) ya da JSON olmayan yanıt. Hata durumunda JSON yoksa
    // adres bir Easytrade sunucusu değildir (ör. HTML 404 sayfası).
    if (!res.ok && res.status !== 401) throw new ApiError(`HTTP ${res.status}`, res.status, "NOT_A_SERVER");
  }
  if (!res.ok || body.success === false) {
    throw new ApiError(body.error?.message ?? `HTTP ${res.status}`, res.status, body.error?.code ?? "");
  }
  return body.data as T;
}
