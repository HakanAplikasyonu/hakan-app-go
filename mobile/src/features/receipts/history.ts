/**
 * Sunucudaki fiş geçmişi ile telefondaki kuyruğun birleşimi (#199; saf, node --test ile denenir).
 *
 * Telefonda olmayan (başka cihazdan çekilmiş ya da uygulama yeniden kurulmuş)
 * fişler geçmişten gösterilir; telefondaki kuyrukta zaten olanlar tekrarlanmaz.
 */
import type { QueueItem } from './queue';

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
