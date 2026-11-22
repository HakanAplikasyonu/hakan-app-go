/**
 * Gelen kutusu (#806, mobilde #780): saf yardımcılar, node --test ile
 * sınanır. Tür adları ofis panelindeki gelen kutusuyla aynıdır
 * (frontend/src/locales/tr/office.json, inbox.type_*).
 */

export type InboxType = 'invoice' | 'bank_transaction' | 'expense' | 'retail_sale' | 'foreign_invoice' | 'late_document';

type Item = {
  type: InboxType;
  amount: string | null;
  currency: string;
  date: string | null;
  description: string;
  reference: string | null;
  can_approve: boolean;
};

/** Ekrandaki sıra. */
export const inboxTypes: readonly InboxType[] = ['invoice', 'bank_transaction', 'expense', 'retail_sale', 'foreign_invoice', 'late_document'];

export const typeLabels: Record<InboxType, string> = {
  invoice: 'Fatura',
  bank_transaction: 'Banka hareketi',
  expense: 'Masraf fişi',
  retail_sale: 'Z raporu',
  foreign_invoice: 'Yabancı fatura',
  late_document: 'Geç belge',
};

export const typeHints: Record<InboxType, string> = {
  invoice: 'Kaydı ve önerisi olmayan faturalar',
  bank_transaction: 'Eşleşmemiş banka hareketleri',
  expense: 'Onay bekleyen masraf fişleri',
  retail_sale: 'Onay bekleyen Z raporları',
  foreign_invoice: 'Mükellefin gönderdiği yabancı faturalar',
  late_document: 'Karar bekleyen geç gelen belgeler',
};

export function typeLabel(t: string): string {
  return typeLabels[t as InboxType] ?? 'Kayıt';
}

/** Tek dokunuş onay yalnız masraf ve Z raporunda, onay yetkisi varsa. */
export function canApproveRow(item: Pick<Item, 'type' | 'can_approve'>, canApprove: boolean): boolean {
  return canApprove && item.can_approve && (item.type === 'expense' || item.type === 'retail_sale');
}

/** "2026-11-05" → "05.11.2026". */
export function dayLabel(date: string | null): string | null {
  if (!date) return null;
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(date);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : date;
}

/** Satır alt metni: tutar · tarih · açıklama (ve numara). */
export function rowSubtitle(item: Item, money: (amount: string, currency: string) => string): string {
  const desc = [item.description, item.reference].filter((s) => s && s.trim()).join(' · ');
  return [item.amount ? money(item.amount, item.currency) : null, dayLabel(item.date), desc || null].filter(Boolean).join(' · ');
}

/** Rozet metni: 0 → null, 99'dan büyükse "99+". */
export function badge(n: number | null | undefined): string | null {
  if (!n || n <= 0) return null;
  return n > 99 ? '99+' : String(n);
}
