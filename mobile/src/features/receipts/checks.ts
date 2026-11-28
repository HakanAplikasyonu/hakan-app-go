/**
 * Fiş yapay zekâ kontrolleri (pure): fiş okunurken sunucunun bulduğu
 * uyarılar (`checks` kodları), mükellefe önizlemede ve fiş ekranında
 * gösterilir. Ofis bir kodu uyarı ayarlarından kapattıysa sunucu o kodu
 * göndermez.
 */
import type { ReceiptResult } from './queue';

export type ReceiptCheck = { code: string; text: string; hint: string };

const messages: Record<string, { text: string; hint: string }> = {
  RECEIPT_BUYER_MISMATCH: {
    text: 'Bu fiş başka biri adına kesilmiş',
    hint: 'Fişteki alıcı bilgisi şirketinizle uyuşmuyor. Fiş sizin değilse müşavirinize bildirin.',
  },
  PLATE_NOT_REGISTERED: {
    text: 'Plaka mükellefin araçlarında yok',
    hint: 'Araç sizinse Profil › Araçlarım’dan ekleyin; müşaviriniz de görür.',
  },
  PLATE_UNREADABLE: {
    text: 'Plaka okunamadı',
    hint: 'Akaryakıt fişinde plaka görünmüyor. Plakanın göründüğü bir fotoğraf çekin ya da müşavirinize yazın.',
  },
};

/** The warnings to show for a capture result, in the server's order. */
export function receiptChecks(result: Pick<ReceiptResult, 'checks' | 'plate_check'> | null | undefined): ReceiptCheck[] {
  if (!result) return [];
  // Eski sunucu: yalnız plate_check gelir.
  const codes = result.checks ?? (result.plate_check === 'not_registered' ? ['PLATE_NOT_REGISTERED'] : []);
  const out: ReceiptCheck[] = [];
  for (const code of codes) {
    const m = messages[code];
    if (m && !out.some((c) => c.code === code)) out.push({ code, ...m });
  }
  return out;
}
