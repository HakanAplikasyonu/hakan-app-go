/** Pano metinleri: saf modül, node --test ile sınanır. */
export type UploadKind = 'receipt' | 'invoice' | 'document';

export const uploadKindLabel: Record<UploadKind, string> = {
  receipt: 'Fiş',
  invoice: 'e-Fatura (UBL)',
  document: 'Görev belgesi',
};

/** "14:05" (Europe/Istanbul) — panodaki yüklemeler bugüne ait. */
export function clock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Istanbul' });
}

/** Sayı rozetleri: 0 → "Yok", 1000+ → "999+". */
export function countLabel(n: number | undefined | null): string {
  if (!n) return 'Yok';
  return n > 999 ? '999+' : String(n);
}

export function greeting(now: Date = new Date()): string {
  const h = Number(now.toLocaleString('en-US', { hour: 'numeric', hour12: false, timeZone: 'Europe/Istanbul' }));
  if (h < 6) return 'İyi geceler';
  if (h < 12) return 'Günaydın';
  if (h < 18) return 'İyi günler';
  return 'İyi akşamlar';
}

function istanbulYearMonthDay(now: Date): [number, number, number] {
  const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(now);
  const n = (t: string) => Number(p.find((x) => x.type === t)?.value);
  return [n('year'), n('month'), n('day')];
}

/** Beyan dönemi: bir önceki ay, "YYYY-MM" (Türkiye saati). */
export function declarationPeriod(now: Date = new Date()): string {
  const [y, m] = istanbulYearMonthDay(now);
  const py = m === 1 ? y - 1 : y;
  const pm = m === 1 ? 12 : m - 1;
  return `${py}-${String(pm).padStart(2, '0')}`;
}

/** KDV beyannamesi son günü: dönemi izleyen ayın 28'i. Bugünden kalan gün (geçtiyse 0). */
export function daysUntilVatDue(now: Date = new Date()): number {
  const [, , d] = istanbulYearMonthDay(now);
  return Math.max(0, 28 - d);
}

/** Dönem tacı: matristeki tek dönem hücrelerinden beyana hazır / toplam. */
export function periodCrown(rows: readonly { cells: readonly { period: string; ready: boolean }[] }[], period: string) {
  let ready = 0;
  let total = 0;
  for (const r of rows) {
    const cell = r.cells.find((c) => c.period === period);
    if (!cell) continue;
    total++;
    if (cell.ready) ready++;
  }
  return { ready, total };
}

/** "12 gün kaldı", "bugün son gün". */
export function dueText(days: number): string {
  return days <= 0 ? 'bugün son gün' : `${days} gün kaldı`;
}
