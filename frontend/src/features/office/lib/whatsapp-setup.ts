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
export function stepStates(current: number, connected: boolean): StepState[] {
  return SETUP_STEPS.map((_, i) =>
    connected
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
