/**
 * Ret gerekçesi (#780): ofis masrafı ya da Z raporunu reddederken hazır
 * gerekçelerden birini seçer, isterse açıklama yazar ("Diğer"de zorunlu).
 * Mükellef bunu Gönderdiklerim'de görür. Kurallar sunucuyla aynı.
 */
export type RejectReasonCode = 'okunaksiz' | 'eksik_bilgi' | 'kisisel_harcama' | 'mukerrer' | 'yanlis_mukellef' | 'diger';

export type Rejection = { reason_code: RejectReasonCode; reason_text?: string };

export const MAX_REJECT_TEXT = 500;

/** Gerekçeler, ekrandaki sırayla, Türkçe etiketleriyle. */
export const REJECT_REASONS: readonly { code: RejectReasonCode; label: string }[] = [
  { code: 'okunaksiz', label: 'Fotoğraf okunmuyor' },
  { code: 'eksik_bilgi', label: 'Bilgi eksik (VKN, tarih ya da tutar)' },
  { code: 'kisisel_harcama', label: 'İşletme gideri değil' },
  { code: 'mukerrer', label: 'Daha önce gönderilmiş' },
  { code: 'yanlis_mukellef', label: 'Başka bir şirkete ait' },
  { code: 'diger', label: 'Diğer' },
];

/** Formun gövdesi ya da neden gönderilemediği (Türkçe). */
export function rejectionBody(code: RejectReasonCode | null, text: string): { body: Rejection } | { error: string } {
  const clean = text.trim().replace(/\s+/g, ' ');
  if (!code) return { error: 'Bir gerekçe seçin.' };
  if (code === 'diger' && !clean) return { error: 'Diğer seçildiğinde açıklama yazın.' };
  if ([...clean].length > MAX_REJECT_TEXT) return { error: `Açıklama en fazla ${MAX_REJECT_TEXT} karakter olabilir.` };
  return { body: clean ? { reason_code: code, reason_text: clean } : { reason_code: code } };
}
