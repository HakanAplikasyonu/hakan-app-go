// Saf yardımcılar (#382): node --test ile sınanır, React Native'e bağlı değil.

/** Dönemlere göre gruplar (yeniden eskiye, sunucunun sırası korunur). */
export function groupByPeriod<T extends { period: string }>(items: T[]): { period: string; items: T[] }[] {
  const out: { period: string; items: T[] }[] = [];
  for (const d of items) {
    const last = out[out.length - 1];
    if (last && last.period === d.period) last.items.push(d);
    else out.push({ period: d.period, items: [d] });
  }
  return out;
}

const AYLAR = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];

/** "2026-10" → "Ekim 2026". */
export function periodLabel(period: string): string {
  const [y, m] = period.split('-');
  const i = Number(m) - 1;
  return AYLAR[i] ? `${AYLAR[i]} ${y}` : period;
}

