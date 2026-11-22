// KDV uyarıları (#799, mobilde #780): saf yardımcılar, node --test ile
// sınanır. Metinler ofis panelindeki "Son KDV uyarıları" ile aynıdır
// (frontend/src/locales/tr/tenant.json, vat_alert_*).

export const eventLabels: Record<string, string> = {
  'vat.threshold_exceeded': 'KDV eşiği aşıldı',
  'vat.status_changed': 'KDV durumu değişti',
  'vat.report_ready': 'KDV raporu hazır',
};

export function eventLabel(event: string): string {
  return eventLabels[event] ?? 'KDV uyarısı';
}

export function eventTone(event: string): 'danger' | 'warning' | 'brand' {
  if (event === 'vat.threshold_exceeded') return 'danger';
  if (event === 'vat.status_changed') return 'warning';
  return 'brand';
}

/** Olaydaki ödenecek KDV (payload.payable, ondalık metin); yoksa null. */
export function payableOf(payload: Record<string, unknown> | undefined): string | null {
  const v = payload?.payable;
  if (typeof v === 'string' && v !== '') return v;
  if (typeof v === 'number' && Number.isFinite(v)) return String(v);
  return null;
}

/** "22.11.2026 09:30" biçiminde yerel tarih. */
export function dateLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** Son `days` gün içindeki uyarı sayısı (ana ekrandaki rozet). */
export function recentCount(items: { created_at: string }[], now: Date, days = 7): number {
  const from = now.getTime() - days * 24 * 60 * 60 * 1000;
  return items.filter((e) => {
    const t = new Date(e.created_at).getTime();
    return Number.isFinite(t) && t >= from;
  }).length;
}
