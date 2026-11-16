/**
 * Sunucudaki fiş geçmişi ile telefondaki kuyruğun birleşimi (#199; saf, node --test ile denenir).
 *
 * Telefonda olmayan (başka cihazdan çekilmiş ya da uygulama yeniden kurulmuş)
 * fişler geçmişten gösterilir; telefondaki kuyrukta zaten olanlar tekrarlanmaz.
 */
import type { QueueItem, QueueSummary } from './queue';

/** Minimal shape of a server-side receipt (api.ts ServerReceipt). */
export type HistoryReceipt = { uuid: string; created_at: string };

/** Server receipts not already in this company's queue, newest first. */
export function serverOnly<T extends HistoryReceipt>(history: readonly T[], items: readonly QueueItem[], taxpayer: string): T[] {
  const local = new Set(items.filter((i) => i.taxpayer === taxpayer && i.result).map((i) => i.result!.uuid));
  return history.filter((h) => !local.has(h.uuid)).sort((a, b) => b.created_at.localeCompare(a.created_at));
}

const statusLabels: Record<string, string> = {
  draft: 'Müşavir onayında',
  approved: 'Onaylandı',
  rejected: 'Reddedildi',
  duplicate: 'Daha önce gönderilmiş',
  failed: 'Okunamadı',
};

/** Label of a server-side receipt status. */
export function historyStatusLabel(status: string): string {
  return statusLabels[status] ?? status;
}

/**
 * Ana sayfadaki "Fişlerim" satırı (#283): telefondaki kuyruk ve sunucuda olup
 * telefonda olmayan fişler birlikte sayılır. Yeni cihazda ya da yeniden
 * kurulumdan sonra kuyruk boş olsa da daha önce gönderilenler görünür.
 */
export function receiptsSummaryLine(s: QueueSummary, earlier: number): string {
  const sent = s.uploaded + earlier;
  const waiting = s.waiting + s.blocked;
  if (sent + waiting + s.failed === 0) return 'Henüz fiş çekmediniz';
  const parts: string[] = [];
  if (sent || !(waiting || s.failed)) parts.push(`${sent} gönderildi`);
  if (waiting) parts.push(`${waiting} bekliyor`);
  if (s.failed) parts.push(`${s.failed} hatalı`);
  return parts.join(' · ');
}
