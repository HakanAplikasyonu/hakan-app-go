/**
 * WhatsApp uzantısı kolay kurulum sihirbazı (#780): adımlar, paket bilgisi ve
 * uzantının bağlantı durumu. Saf yardımcılar; testleri whatsapp-setup.test.ts.
 */

/** Sabit indirme adresi (sürüm değişince de aynı kalır). */
export const EXTENSION_ZIP_URL = "/indir/easytrade-whatsapp.zip";
/** Paket bilgisi: frontend/scripts/whatsapp-extension.mjs yazar. */
export const EXTENSION_INFO_URL = "/indir/easytrade-whatsapp.json";
/** Bağlantı olarak açılamaz; kopyalanıp adres çubuğuna yapıştırılır. */
export const CHROME_EXTENSIONS_URL = "chrome://extensions";

export const SETUP_STEPS = ["download", "install", "pair", "test"] as const;
export type SetupStep = (typeof SETUP_STEPS)[number];

/** localStorage'daki adım (bozuk / eski değer → ilk adım). */
export function clampStep(raw: unknown): number {
  const n = typeof raw === "number" ? raw : Number(raw);
  if (!Number.isInteger(n)) return 0;
  return Math.min(Math.max(n, 0), SETUP_STEPS.length - 1);
}

export type StepState = "done" | "current" | "pending";

/**
 * Adım kartlarının durumu: seçili adım "current", öncekiler "done". Uzantı
 * bağlıysa bütün adımlar tamamlanmıştır (kurulum bitti).
 */
export function stepStates(
  current: number,
  connected: boolean,
  installed = false,
): StepState[] {
  return SETUP_STEPS.map((key, i) =>
    connected
      ? "done"
      : // Uzantı kuruluysa indirme ve ekleme adımları kendiliğinden biter.
        installed && (key === "download" || key === "install")
        ? "done"
        : i < current
          ? "done"
          : i === current
            ? "current"
            : "pending",
  );
}

export type ExtensionPackage = { version: string; size: number };

/** easytrade-whatsapp.json → paket bilgisi; beklenmeyen içerik → null. */
export function parsePackageInfo(raw: unknown): ExtensionPackage | null {
  if (!raw || typeof raw !== "object") return null;
  const { version, size } = raw as { version?: unknown; size?: unknown };
  if (typeof version !== "string" || !version) return null;
  if (typeof size !== "number" || !Number.isFinite(size) || size <= 0)
    return null;
  return { version, size };
}

/** Uzantının oturumunu tanıtan cihaz etiketi (backend whatsapp.SessionUserAgent). */
const EXTENSION_SESSION_MARK = "WhatsApp Web uzantısı";

type SessionLike = { user_agent?: string | null; created_at: string };

export type ExtensionStatus =
  { connected: true; lastSeen: string; devices: number } | { connected: false };

/**
 * Hesabın açık oturumlarından uzantının durumu: uzantı oturumu varsa bağlı,
 * "son görülme" en yeni oturumun zamanıdır (oturum belirteci yenilenince
 * güncellenir).
 */
export function extensionStatus(sessions: SessionLike[]): ExtensionStatus {
  const ext = sessions.filter((s) =>
    (s.user_agent ?? "").includes(EXTENSION_SESSION_MARK),
  );
  if (ext.length === 0) return { connected: false };
  const lastSeen = ext
    .map((s) => s.created_at)
    .reduce((a, b) => (Date.parse(b) > Date.parse(a) ? b : a));
  return { connected: true, lastSeen, devices: ext.length };
}

/**
 * Paketlenmemiş paketin sabit kimliği: extensions/whatsapp/scripts/
 * extension-key.txt genel anahtarından (tests/manifest.test.ts denetler).
 * Mağaza kimliği NEXT_PUBLIC_WHATSAPP_EXTENSION_IDS ile eklenir.
 */
export const DEFAULT_EXTENSION_IDS = ["fbcngkcogndfncloaedcfpkofebbfhco"];

const EXTENSION_ID_RE = /^[a-p]{32}$/;

/** "id1, id2" → geçerli kimlikler + sabit kimlik (tekrarsız). */
export function extensionIds(raw: string | undefined): string[] {
  const extra = (raw ?? "")
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter((s) => EXTENSION_ID_RE.test(s));
  return [...new Set([...extra, ...DEFAULT_EXTENSION_IDS])];
}

/** Mağaza bağlantısı yalnızca https ise kullanılır. */
export function storeUrl(raw: string | undefined): string {
  const s = (raw ?? "").trim();
  try {
    return new URL(s).protocol === "https:" ? s : "";
  } catch {
    return "";
  }
}

export type InstalledExtension = {
  id: string;
  version: string;
  paired: boolean;
};

/** Sayfadaki chrome.runtime'ın kullanılan kısmı (testte sahtesi verilir). */
export type ExtensionRuntime = {
  sendMessage(
    id: string,
    message: unknown,
    callback: (reply: unknown) => void,
  ): void;
  lastError?: unknown;
};

function pingOne(
  rt: ExtensionRuntime,
  id: string,
  timeoutMs: number,
): Promise<InstalledExtension | null> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve(null), timeoutMs);
    try {
      rt.sendMessage(id, { type: "ping" }, (reply) => {
        clearTimeout(timer);
        // Kurulu değilse Chrome lastError koyar; okunmazsa konsola uyarı düşer.
        if (rt.lastError || !reply || typeof reply !== "object") {
          resolve(null);
          return;
        }
        const r = reply as {
          ok?: unknown;
          version?: unknown;
          paired?: unknown;
        };
        resolve(
          r.ok === true && typeof r.version === "string"
            ? { id, version: r.version, paired: r.paired === true }
            : null,
        );
      });
    } catch {
      clearTimeout(timer);
      resolve(null);
    }
  });
}

/**
 * Uzantı kurulu mu? Panel alan adı uzantının externally_connectable
 * listesindeyse sayfada chrome.runtime.sendMessage vardır; her kimliğe "ping"
 * gönderilir, ilk yanıt veren kazanır. Chrome dışı / kurulu değil → null.
 */
export async function detectExtension(
  rt: ExtensionRuntime | undefined,
  ids: string[],
  timeoutMs = 1500,
): Promise<InstalledExtension | null> {
  if (!rt || typeof rt.sendMessage !== "function") return null;
  const found = await Promise.all(ids.map((id) => pingOne(rt, id, timeoutMs)));
  return found.find((x) => x !== null) ?? null;
}
