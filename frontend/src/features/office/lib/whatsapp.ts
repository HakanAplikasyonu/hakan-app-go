/** WhatsApp Web uzantısı (#388) için küçük biçimlendiriciler. */

/**
 * Sohbet anahtarı → okunur numara: 905321234567 → +90 532 123 45 67; grup
 * ve numarası gizli kişi için yalnızca tür.
 */
export function formatChatKey(key: string): string {
  if (key.startsWith("g:")) return "grup";
  if (key.startsWith("l:")) return "gizli numara";
  const m = /^90(\d{3})(\d{3})(\d{2})(\d{2})$/.exec(key);
  return m ? `+90 ${m[1]} ${m[2]} ${m[3]} ${m[4]}` : `+${key}`;
}

const RECEIPT_STATUS_KEYS: Record<string, string> = {
  draft: "office.whatsapp.receipt_draft",
  duplicate: "office.whatsapp.receipt_duplicate",
  failed: "office.whatsapp.receipt_failed",
  approved: "office.whatsapp.receipt_approved",
  rejected: "office.whatsapp.receipt_rejected",
};

/** Fiş yakalama durumunun çeviri anahtarı. */
export function receiptStatusKey(status: string): string {
  return RECEIPT_STATUS_KEYS[status] ?? "office.whatsapp.result_none";
}
