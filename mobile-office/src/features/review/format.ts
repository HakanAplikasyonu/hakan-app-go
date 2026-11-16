/** İnceleme kuyruğu yardımcıları: saf modül, node --test ile sınanır. */

export type SuggestionLike = {
  uuid: string;
  engagement_uuid?: string;
  kind: string;
  target_field: string;
  proposed_value: unknown;
  confidence: string;
  status: string;
};

/** Önerilen hesap kodu (`{"account_code": "770.01"}`), yoksa null. */
export function proposedCode(v: unknown): string | null {
  if (v && typeof v === 'object' && 'account_code' in v) {
    const c = (v as { account_code: unknown }).account_code;
    return typeof c === 'string' && c ? c : null;
  }
  return null;
}

/**
 * Hesap kodu biçimi, backend ile aynı (accountrules/engine.ValidCode):
 * 3 haneli ana hesap, noktayla ayrılmış alt kırılımlar, en çok 64 karakter.
 */
export function isValidAccountCode(code: string): boolean {
  const c = code.trim();
  return c.length <= 64 && /^[0-9]{3}(\.[0-9A-Za-z]+)*$/.test(c);
}

/** "0.8700" → "%87". */
export function confidencePercent(c: string): string {
  const n = Number(c);
  return Number.isFinite(n) ? `%${Math.round(n * 100)}` : '—';
}

export type ConfidenceBand = 'low' | 'medium' | 'high';
export function confidenceBand(c: string): ConfidenceBand {
  const n = Number(c);
  if (!(n >= 0.6)) return 'low';
  return n >= 0.85 ? 'high' : 'medium';
}

export const fieldLabel = (f: string) => (f === 'vat_account_code' ? 'KDV hesabı' : f === 'account_code' ? 'Hesap kodu' : f);

/** Toplu onay: yalnızca bekleyen ve engagement'ı bilinen öneriler. */
export function bulkTargets<T extends SuggestionLike>(list: readonly T[], selected: ReadonlySet<string>): T[] {
  return list.filter((s) => selected.has(s.uuid) && s.status === 'pending' && !!s.engagement_uuid);
}

/** Sıralı çalıştırma: her biri ayrı karar (backend kaydı), hata diğerlerini durdurmaz. */
export async function runSequential<T>(items: readonly T[], fn: (item: T) => Promise<unknown>): Promise<{ ok: T[]; failed: { item: T; error: unknown }[] }> {
  const ok: T[] = [];
  const failed: { item: T; error: unknown }[] = [];
  for (const item of items) {
    try {
      await fn(item);
      ok.push(item);
    } catch (error) {
      failed.push({ item, error });
    }
  }
  return { ok, failed };
}

/** Kaydırma hareketi: eşik aşılınca karar (sağa onay, sola reddet, yukarı düzelt). */
export type SwipeAction = 'accept' | 'reject' | 'correct' | null;
export function swipeAction(dx: number, width: number, threshold = 0.35, dy = 0, height = 0): SwipeAction {
  'worklet';
  const limit = Math.max(60, width * threshold);
  const up = Math.max(80, height * 0.25);
  // Yukarı hareket yataydan baskınsa düzelt; aksi halde yatay eşik.
  if (height > 0 && dy <= -up && Math.abs(dy) > Math.abs(dx)) return 'correct';
  if (dx >= limit) return 'accept';
  if (dx <= -limit) return 'reject';
  return null;
}

/** Kart yığınında kartın eğimi (derece): yatay kaymayla en çok ±8°. */
export function cardTilt(dx: number, width: number): number {
  'worklet';
  if (!width) return 0;
  return Math.max(-8, Math.min(8, (dx / width) * 16));
}

/** Kuyruk ilerlemesi: bu oturumda karar verilen / (karar + kalan), 0..1. */
export function queueProgress(decided: number, remaining: number): number {
  const all = decided + remaining;
  return all > 0 ? Math.min(1, decided / all) : 0;
}

/** Tahmini süre: öneri başına ortalama 6 sn (tasarım). "~5 dk", "1 dk'dan az". */
export function queueEta(count: number, secondsEach = 6): string {
  if (count <= 0) return '';
  const secs = count * secondsEach;
  return secs < 60 ? "1 dk'dan az" : `~${Math.round(secs / 60)} dk`;
}
