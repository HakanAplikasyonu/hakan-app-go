// Saf yardımcılar (#780): KDV durumu ve yabancı fatura kartları için metin
// üretimi. node --test ile sınanır, React Native'e bağlı değil.

/** "1234.5" / "-12" → "₺1.234,50" / "-₺12,00". Geçersiz girdi "₺0,00". */
export function tl(money: string | null | undefined): string {
  const n = Number(money ?? '0');
  const v = Number.isFinite(n) ? n : 0;
  const [tam, kurus] = Math.abs(v).toFixed(2).split('.');
  const gruplu = tam.replace(/\B(?=(\d{3})+(?!\d))/g, '.');
  return `${v < 0 ? '-' : ''}₺${gruplu},${kurus}`;
}

const AYLAR = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];

/** "2026-12-26" → "26 Aralık 2026". */
export function tarih(iso: string | null | undefined): string {
  if (!iso) return '';
  const [y, m, d] = iso.slice(0, 10).split('-');
  const ay = AYLAR[Number(m) - 1];
  return ay ? `${Number(d)} ${ay} ${y}` : iso;
}

/** "2026-11" → "Kasım 2026". */
export function ayAdi(period: string): string {
  const [y, m] = period.split('-');
  const ay = AYLAR[Number(m) - 1];
  return ay ? `${ay} ${y}` : period;
}

/** KDV1 son günü satırı; `yakin` son üç gün. */
export function sonGun(cal: { due_date: string; days_left: number }): { metin: string; gecti: boolean; yakin: boolean } {
  const gun = cal.days_left < 0 ? 'son gün geçti' : cal.days_left === 0 ? 'bugün son gün' : `${cal.days_left} gün kaldı`;
  return {
    metin: `Son ödeme günü ${tarih(cal.due_date)} · ${gun}`,
    gecti: cal.days_left < 0,
    yakin: cal.days_left >= 0 && cal.days_left <= 3,
  };
}

/** Ödenecek mi devreden mi: kartın ana satırı. */
export function kdvSonucu(totals: { payable: string; carry_out: string }): { etiket: string; tutar: string; odenecek: boolean } {
  const odenecek = Number(totals.payable) > 0 || !(Number(totals.carry_out) > 0);
  return odenecek
    ? { etiket: 'Ödenecek KDV (tahmini)', tutar: tl(totals.payable), odenecek: true }
    : { etiket: 'Sonraki aya devredecek KDV', tutar: tl(totals.carry_out), odenecek: false };
}

type HataBilgisi = { status?: number; code?: string } | null | undefined;

/**
 * KDV kartı gizlenir mi: ofis paneli kapattı (403 TAXPAYER_VAT_PANEL_DISABLED)
 * ya da şirketin etkin ofisi yok (404).
 */
export function kdvGizli(err: unknown): boolean {
  const e = err as HataBilgisi;
  return !!e && (e.code === 'TAXPAYER_VAT_PANEL_DISABLED' || e.status === 404);
}

/** "Bu ay N yabancı faturanız alındı" ya da boş durum metni. */
export function alindiMetni(count: number): string {
  return count > 0 ? `Bu ay ${count} yabancı faturanız alındı` : 'Bu ay henüz yabancı fatura göndermediniz.';
}

export const DURUM: Record<'in_review' | 'processed', string> = {
  in_review: 'Ofiste inceleniyor',
  processed: 'İşlendi',
};

/** Yükleme hatasını kullanıcıya söylenecek Türkçe cümleye çevirir. */
export function yuklemeHatasi(err: unknown): string {
  const e = err as HataBilgisi;
  if (e?.code === 'NO_ACTIVE_ENGAGEMENT') return 'Şirketinizin bağlı bir müşavirlik ofisi yok; fatura gönderilemedi.';
  if (e?.status === 409) return 'Ofisiniz yabancı fatura gönderimini kapattı. Faturayı ofisinize başka yoldan iletin.';
  if (e?.status === 400) return 'Yalnızca fotoğraf (JPEG, PNG, WebP, HEIC) ya da PDF gönderebilirsiniz, en çok 20 MB.';
  if (e?.status === 0) return 'Bağlantı yok. İnternete bağlanınca yeniden deneyin.';
  return 'Fatura gönderilemedi. Biraz sonra yeniden deneyin.';
}

/** Sunucu sınırı: en çok 20 MB. */
export const AZAMI_BAYT = 20 * 1024 * 1024;
