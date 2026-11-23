/**
 * Hesap planı seçici (onay ekranı, Oğuz'un canlı geri bildirimi): arama,
 * sıralama ve onayda gönderilecek karar. Saf modül, node --test ile sınanır.
 */
import { isValidAccountCode } from '../review/format.ts';

export type PlanAccount = { code: string; name: string; is_leaf: boolean };

/** Türkçe küçük harf; noktasız arama için "İ/I" doğru çevrilir. */
export function trLower(s: string): string {
  return s.toLocaleLowerCase('tr-TR');
}

/** Seçicinin açılış araması: önerilen kodun ana hesabı ("770.01.002" → "770"). */
export function initialQuery(code: string | null | undefined): string {
  const m = /^(\d{3})/.exec((code ?? '').trim());
  return m ? m[1] : '';
}

/**
 * Seçilebilir hesaplar: yalnız alt (kayıt atılabilen) hesaplar; kod öneki
 * ya da ad (Türkçe küçük harf) eşleşmesi. Kod önekiyle eşleşenler önce,
 * sonra koda göre. Arama boşsa liste boş (plan binlerce satır).
 */
export function filterAccounts<T extends PlanAccount>(items: readonly T[], query: string, limit = 20): T[] {
  const q = trLower(query.trim());
  if (!q) return [];
  const byCode: T[] = [];
  const byName: T[] = [];
  const seen = new Set<string>();
  for (const a of items) {
    if (!a.is_leaf || seen.has(a.code)) continue;
    if (a.code.startsWith(q)) {
      byCode.push(a);
      seen.add(a.code);
    } else if (trLower(a.name).includes(q)) {
      byName.push(a);
      seen.add(a.code);
    }
  }
  const cmp = (a: T, b: T) => a.code.localeCompare(b.code, 'tr-TR', { numeric: true });
  return [...byCode.sort(cmp), ...byName.sort(cmp)].slice(0, Math.max(0, limit));
}

/** Planda olmayan ama biçimi doğru kod: "elle gir" seçeneği; yoksa null. */
export function manualCode(query: string, shown: readonly PlanAccount[]): string | null {
  const t = query.trim();
  if (!t || !isValidAccountCode(t)) return null;
  return shown.some((a) => a.code === t) ? null : t;
}

export type ApproveDecision = { action: 'accept' } | { action: 'correct'; accountCode: string; reason: string };

/**
 * Onayla düğmesinin kararı: kod değişmediyse öneri olduğu gibi onaylanır
 * (`accept`); başka bir hesap seçildiyse seçilen kod `correct` ucuyla
 * gönderilir (gerekçe zorunlu, kendiliğinden yazılır). Öneri bir hesap
 * kodu değilse (ör. fişten okunan tutar) kod gönderilmez: `accept`.
 */
export function approveDecision(proposed: string | null | undefined, chosen: string | null | undefined): ApproveDecision {
  const p = (proposed ?? '').trim();
  const c = (chosen ?? '').trim();
  if (!p || !c || c === p) return { action: 'accept' };
  return { action: 'correct', accountCode: c, reason: `Onay ekranında hesap planından ${c} seçildi` };
}
