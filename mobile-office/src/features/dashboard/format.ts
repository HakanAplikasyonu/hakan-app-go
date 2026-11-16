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
