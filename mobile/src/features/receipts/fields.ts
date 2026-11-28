/** OCR field presentation (pure). */
import type { QueueItem, ReceiptField, ReceiptKind } from './queue';

/** Below this the field is shown yellow and goes to the müşavir's review queue (#14 default 0.80). */
export const LOW_CONFIDENCE = 0.8;

export const kindLabels: Record<ReceiptKind, string> = {
  expense_receipt: 'Masraf fişi',
  fuel_receipt: 'Yakıt fişi',
  z_report: 'Z raporu',
};

// Keys of backend/internal/modules/receipts/extract (Field* constants).
const labels: Record<string, string> = {
  date: 'Tarih',
  vendor_name: 'Satıcı',
  vendor_tax_number: 'Satıcı VKN',
  receipt_number: 'Fiş no',
  total_amount: 'Toplam',
  tax_amount: 'KDV',
  tax_rate: 'KDV oranı',
  vehicle_plate: 'Plaka',
  z_report_number: 'Z no',
  okc_serial_number: 'ÖKC sicil no',
  payment_method: 'Ödeme şekli',
  buyer_tax_number: 'Alıcı VKN/TCKN',
  buyer_name: 'Alıcı',
};

/**
 * Fişteki alıcı (Fiş yapay zekâ kontrolleri): yalnız "başka biri adına
 * kesilmiş" uyarısı için okunur; düzeltilecek alanlar arasında gösterilmez.
 */
const infoOnly = new Set(['buyer_tax_number', 'buyer_name']);

export function fieldLabel(key: string): string {
  const vat = /^vat_(\d+)_(total|tax)$/.exec(key);
  if (vat) return vat[2] === 'total' ? `%${vat[1]} KDV dahil` : `%${vat[1]} KDV`;
  return labels[key] ?? key.replace(/_/g, ' ');
}

/** Display order: known fields first in a fixed order, then the rest alphabetically. */
export function orderedFields(fields: Record<string, ReceiptField>): [string, ReceiptField][] {
  const order = Object.keys(labels);
  return Object.entries(fields)
    .filter(([k]) => !infoOnly.has(k))
    .sort(([a], [b]) => {
      const ia = order.indexOf(a);
      const ib = order.indexOf(b);
      if (ia !== -1 || ib !== -1) return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib);
      return a.localeCompare(b, 'tr');
    });
}

export function needsAttention(f: ReceiptField): boolean {
  return !f.valid || f.confidence < LOW_CONFIDENCE;
}

/** Fields of an uploaded receipt the user should look at (not already corrected). */
export function fieldsToCheck(item: QueueItem): string[] {
  if (!item.result) return [];
  const corrected = new Set(item.corrections.map((c) => c.field));
  return orderedFields(item.result.fields)
    .filter(([k, f]) => needsAttention(f) && !corrected.has(k))
    .map(([k]) => k);
}

export function statusLabel(item: QueueItem): string {
  switch (item.status) {
    case 'pending':
      return item.attempts > 0 ? 'Bağlantı bekleniyor' : 'Sırada';
    case 'uploading':
      return 'Gönderiliyor';
    case 'blocked':
      return 'Telefonda bekliyor';
    case 'failed':
      return 'Gönderilemedi';
    case 'uploaded':
      if (item.result?.status === 'processing') return 'Okunuyor';
      if (item.result?.status === 'duplicate') return 'Daha önce gönderilmiş';
      if (item.result?.status === 'failed') return 'Okunamadı';
      if (item.result?.status === 'approved') return 'Onaylandı';
      if (item.result?.status === 'rejected') return 'Reddedildi';
      return 'Müşavir onayında';
  }
}
