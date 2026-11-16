/** Belge görevleri yardımcıları: saf modül, node --test ile sınanır. */

export type TaskStatus = 'open' | 'submitted' | 'matched' | 'cancelled';

export const statusLabel: Record<TaskStatus, string> = {
  open: 'Bekleniyor',
  submitted: 'Yüklendi, inceleyin',
  matched: 'Tamamlandı',
  cancelled: 'İptal edildi',
};

export const statusTone = (s: TaskStatus) =>
  s === 'submitted' ? 'warning' : s === 'matched' ? 'success' : s === 'cancelled' ? 'neutral' : 'brand';

/** Görev başlığı: elle açılan talepte başlık, banka görevinde türü. */
export function taskTitle(t: { kind: string; title?: string }): string {
  if (t.title) return t.title;
  if (t.kind === 'undocumented_payment') return 'Ödeme için belge';
  if (t.kind === 'uninvoiced_collection') return 'Tahsilat için fatura';
  return 'Eksik belge';
}

/** Hızlı son tarih seçenekleri (gün). */
export const dueChoices = [3, 7, 14, 30] as const;

/** Bugünden `days` gün sonrası, Türkiye saatiyle "YYYY-MM-DD". */
export function dueDateFromDays(days: number, now: Date = new Date()): string {
  const d = new Date(now.getTime() + days * 86_400_000);
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d);
}

/** "2026-11-20" → "20 Kasım 2026". */
export function dayLabel(iso: string): string {
  const d = new Date(iso.length === 10 ? `${iso}T12:00:00Z` : iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString('tr-TR', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Europe/Istanbul' });
}

export type RequestDraft = { title: string; note: string; dueDays: number };

/** Form denetimi: backend kurallarıyla aynı (başlık 1–200, not ≤1000, gün 1–90). */
export function validateRequest(d: RequestDraft): string | null {
  const title = d.title.trim();
  if (!title) return 'Ne istediğinizi yazın.';
  if (title.length > 200) return 'Başlık en çok 200 karakter olabilir.';
  if (d.note.trim().length > 1000) return 'Açıklama en çok 1000 karakter olabilir.';
  if (!Number.isInteger(d.dueDays) || d.dueDays < 1 || d.dueDays > 90) return 'Son tarih 1–90 gün içinde olmalı.';
  return null;
}

/** Hangi işlemler açık: tamamla (açık/yüklendi), yeniden iste (yalnızca yüklendi), iptal (açık/yüklendi). */
export function actions(status: TaskStatus) {
  const live = status === 'open' || status === 'submitted';
  return { complete: live, rerequest: status === 'submitted', cancel: live };
}

/** Sık istenen belgeler: formda tek dokunuşla başlık. */
export const commonRequests = ['Kira kontratı', 'Banka ekstresi', 'SGK bildirgesi', 'Gider faturası', 'Z raporu', 'Vergi levhası'];
