/** Mükellef listesi ve ayrıntısı yardımcıları: saf modül, node --test ile sınanır. */

export type ClientFilter = 'all' | 'warnings' | 'tasks' | 'mine';

export const filterLabels: { key: ClientFilter; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  { key: 'mine', label: 'Bana atanan' },
  { key: 'warnings', label: 'Uyarısı olan' },
  { key: 'tasks', label: 'Açık görevi olan' },
];

/** Arama terimi: kırpılır, 2 karakterden kısaysa aramaz (tüm liste). */
export function normalizeSearch(raw: string): string | undefined {
  const t = raw.trim().replace(/\s+/g, ' ');
  return t.length >= 2 ? t.slice(0, 100) : undefined;
}

/** "tel:" bağlantısı: yalnızca rakam ve baştaki +. */
export function telHref(phone: string | null | undefined): string | null {
  if (!phone) return null;
  const digits = phone.replace(/[^\d+]/g, '').replace(/(?!^)\+/g, '');
  return digits.replace(/\D/g, '').length >= 7 ? `tel:${digits}` : null;
}

export function mailHref(email: string | null | undefined): string | null {
  if (!email || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) return null;
  return `mailto:${encodeURIComponent(email).replace('%40', '@')}`;
}

/** Bu ay ve önceki ay (Türkiye saati), "YYYY-MM". */
export function recentPeriods(now: Date = new Date()): [string, string] {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit' }).formatToParts(now);
  const y = Number(parts.find((p) => p.type === 'year')?.value);
  const m = Number(parts.find((p) => p.type === 'month')?.value);
  const cur = `${y}-${String(m).padStart(2, '0')}`;
  const py = m === 1 ? y - 1 : y;
  const pm = m === 1 ? 12 : m - 1;
  return [cur, `${py}-${String(pm).padStart(2, '0')}`];
}

/** "1234.5" → "1.234,50 ₺" (TRY dışı para birimi kodla yazılır). */
export function money(amount: string, currency = 'TRY'): string {
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  const s = n.toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return currency === 'TRY' ? `${s} ₺` : `${s} ${currency}`;
}

/** Liste satırının alt satırı: VKN/TCKN · uyarı · görev. */
export function clientSubtitle(c: { tax_id: string; kind: string; open_warnings: number; open_blockers: number; open_tasks: number }): string {
  const parts = [`${c.kind === 'legal' ? 'VKN' : 'TCKN'} ${c.tax_id}`];
  if (c.open_blockers) parts.push(`${c.open_blockers} engelleyici`);
  else if (c.open_warnings) parts.push(`${c.open_warnings} uyarı`);
  if (c.open_tasks) parts.push(`${c.open_tasks} açık görev`);
  return parts.join(' · ');
}
