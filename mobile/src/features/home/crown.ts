/**
 * Mükellef ekranlarında durum dili (tasarım: MukellefMobil): her fiş ve istek
 * bir taç rozetiyle gösterilir. Saf modül, node --test ile sınanır.
 */
import type { CrownState } from '@hakan/mobile-core/notifications/inbox';

export type { CrownState };

/**
 * Sunucudaki fiş durumu → taç. `draft` müşavir onayında (sol kristal),
 * `approved` onaylandı; okunamayan, reddedilen ve mükerrer fiş "sorun".
 * Fiş ucu "Luca'ya aktarıldı" aşamasını döndürmüyor; üç kristalin dolduğu
 * durum fişlerde şimdilik görünmez.
 */
export function receiptCrown(status: string | null | undefined): CrownState {
  switch (status) {
    case 'processing': // alındı, sunucuda okunuyor (#780)
    case 'draft':
      return 'yuklendi';
    case 'approved':
      return 'onaylandi';
    case 'failed':
    case 'rejected':
    case 'duplicate':
      return 'sorun';
    default:
      return 'bekleniyor';
  }
}

/** Belge isteği → taç: gönderildiyse yüklendi, tamamlandıysa onaylandı, gecikmişse sorun. */
export function taskCrown(status: string, overdue?: boolean): CrownState {
  if (status === 'matched') return 'onaylandi';
  if (status === 'submitted') return 'yuklendi';
  if (overdue) return 'sorun';
  return 'bekleniyor';
}

function istanbulDay(d: Date): number {
  const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d);
  return Math.round(Date.parse(`${p}T00:00:00Z`) / 86_400_000);
}

/** Son gün metni: "2 gün gecikti", "bugün son gün", "5 gün kaldı". */
export function dueLine(due: string | null, now: Date = new Date()): { text: string; late: boolean } | null {
  if (!due) return null;
  const d = new Date(due.length === 10 ? `${due}T12:00:00Z` : due);
  if (Number.isNaN(d.getTime())) return null;
  const diff = istanbulDay(d) - istanbulDay(now);
  if (diff < 0) return { text: `${-diff} gün gecikti`, late: true };
  if (diff === 0) return { text: 'bugün son gün', late: false };
  return { text: `${diff} gün kaldı`, late: false };
}

/** "Müşavirinin istedikleri" başlığındaki rozet: gecikmiş istek sayısı. */
export function lateBadge(tasks: readonly { overdue?: boolean }[]): string | null {
  const n = tasks.filter((t) => t.overdue).length;
  return n ? `${n} gecikti` : null;
}

/** Çevrimdışı kuyruk kartı: "2 fiş telefonda bekliyor". */
export function waitingLine(waiting: number): string | null {
  return waiting > 0 ? `${waiting} fiş telefonda bekliyor` : null;
}
