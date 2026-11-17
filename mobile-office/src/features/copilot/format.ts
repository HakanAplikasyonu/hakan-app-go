// Co-pilot "Sıradaki iş" (#392, mobilde #454): saf yardımcılar, node --test
// ile sınanır. Metinler ofis panelinin tr çevirisiyle aynıdır
// (frontend/src/locales/tr/office.json, copilot.*).

export type StepKey = 'documents' | 'bank' | 'review' | 'journals' | 'transfer' | 'report';
export type StepStatus = 'done' | 'todo' | 'waiting' | 'blocked' | 'na';
export type Action =
  | 'bank_suggest'
  | 'bank_approve'
  | 'bank_drafts'
  | 'journals_approve'
  | 'export_transfer'
  | 'report_send'
  | 'open_documents'
  | 'open_review'
  | 'open_journals';

export type Step = { key: StepKey; status: StepStatus; count: number; action?: Action | null };
export type Row = { next_step?: StepKey | null; steps: Step[]; done_steps: number; active_steps: number };

export const stepLabels: Record<StepKey, string> = {
  documents: 'Belgeler',
  bank: 'Banka',
  review: 'Öneriler',
  journals: 'Fişler',
  transfer: 'Aktarım',
  report: 'Rapor',
};

export const statusLabels: Record<StepStatus, { label: string; tone: 'neutral' | 'success' | 'warning' | 'danger' | 'brand' }> = {
  done: { label: 'Tamam', tone: 'success' },
  todo: { label: 'Yapılacak', tone: 'brand' },
  waiting: { label: 'Bekleniyor', tone: 'warning' },
  blocked: { label: 'Sorun var', tone: 'danger' },
  na: { label: 'Bu dönem yok', tone: 'neutral' },
};

const details: Record<`${StepKey}_${StepStatus}`, string> = {
  documents_done: 'Eksik belge yok',
  documents_todo: '{count} belge talebi açık',
  documents_waiting: 'Sonuç bekleniyor',
  documents_blocked: 'Kontrol edin',
  documents_na: 'Bu dönem yok',
  bank_done: '{count} hareketin hesabı onaylı',
  bank_todo: '{count} hareket bekliyor',
  bank_waiting: 'Sonuç bekleniyor',
  bank_blocked: 'Kontrol edin',
  bank_na: 'Bu dönem banka hareketi yok',
  review_done: 'Bekleyen öneri yok',
  review_todo: '{count} öneri onay bekliyor',
  review_waiting: 'Sonuç bekleniyor',
  review_blocked: 'Kontrol edin',
  review_na: 'Bu dönem yok',
  journals_done: '{count} fiş onaylı',
  journals_todo: '{count} kayıt fişe hazır',
  journals_waiting: 'Sonuç bekleniyor',
  journals_blocked: 'Kontrol edin',
  journals_na: 'Bu dönem fiş yok',
  transfer_done: '{count} fiş programa aktarıldı',
  transfer_todo: '{count} onaylı fiş aktarılmadı',
  transfer_waiting: 'Aktarım masaüstü ajanında bekliyor',
  transfer_blocked: 'Son aktarım başarısız oldu',
  transfer_na: 'Aktarılacak fiş yok',
  report_done: 'Aylık rapor gönderildi',
  report_todo: 'Aylık rapor gönderilmedi',
  report_waiting: 'Rapor e-postası kuyrukta',
  report_blocked: 'Rapor gönderilemedi',
  report_na: 'Bu dönem yok',
};

/** "2 öneri onay bekliyor": adımın durumu bir cümlede. */
export function stepDetail(key: StepKey, status: StepStatus, count: number): string {
  return (details[`${key}_${status}`] ?? '').replace('{count}', count.toLocaleString('tr-TR'));
}

export const actionLabels: Record<Action, string> = {
  bank_suggest: 'Banka hareketlerine hesap öner',
  bank_approve: 'Banka eşleştirmelerini onayla',
  bank_drafts: 'Banka hareketlerinden fiş oluştur',
  journals_approve: 'Taslak fişleri onayla',
  export_transfer: 'Fişleri muhasebe programına aktar',
  report_send: 'Aylık raporu gönder',
  open_documents: 'Eksik belgelere bak',
  open_review: 'Önerileri incele',
  open_journals: 'Aktarımı kontrol et',
};

/** Satırın sıradaki adımı (ilk `todo` / `blocked`), yoksa null. */
export function nextStepOf(row: Row): Step | null {
  if (!row.next_step) return null;
  return row.steps.find((s) => s.key === row.next_step) ?? null;
}

/**
 * Mobilde sıradaki işin nereye götürdüğü: ekran açan eylemler uygulamada
 * karşılığı olan ekrana gider; yazan eylemler (plan adımları) onay
 * gerektirdiği için ofis panelinde çalışır (mobilde yalnızca görünür).
 */
export function mobileTarget(action: Action | null | undefined): 'tasks' | 'review' | 'web' | null {
  if (!action) return null;
  if (action === 'open_documents') return 'tasks';
  if (action === 'open_review') return 'review';
  return 'web';
}

/** "3/5 adım tamam" (ofis panelindeki gibi). */
export function progressLabel(row: Row): string {
  return `${row.done_steps}/${row.active_steps} adım tamam`;
}

/** Sıradaki işi olmayan satır: bekleyen adım varsa "Sonuç bekleniyor", yoksa "Dönem kapandı". */
export function idleLabel(row: Row): string {
  return row.steps.some((s) => s.status === 'waiting') ? 'Sonuç bekleniyor' : 'Dönem kapandı';
}

/** Dönem kapanışında işi kalan satırlar önce (sunucu sırası korunur). */
export function openFirst<T extends Row>(rows: T[]): T[] {
  return [...rows.filter((r) => !!r.next_step), ...rows.filter((r) => !r.next_step)];
}
