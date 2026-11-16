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

/** Kaydırma hareketi: eşik aşılınca karar (sağa onay, sola reddet). */
export type SwipeAction = 'accept' | 'reject' | null;
export function swipeAction(dx: number, width: number, threshold = 0.35): SwipeAction {
  const limit = Math.max(60, width * threshold);
  if (dx >= limit) return 'accept';
  if (dx <= -limit) return 'reject';
  return null;
}
