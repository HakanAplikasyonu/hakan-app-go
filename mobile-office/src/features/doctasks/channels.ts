/** Belge isteğinin gönderim kanalları ve hatırlatma: saf modül, node --test ile sınanır. */

export type Channel = 'push' | 'sms' | 'email';

export const channelOrder: readonly Channel[] = ['push', 'sms', 'email'];

export const channelLabel: Record<Channel, string> = {
  push: 'Uygulama bildirimi',
  sms: 'SMS',
  email: 'E-posta',
};

/** Uygulama bildirimi istek açılınca zaten gider (sunucu); formda hep seçili kalır. */
export const defaultChannels: Channel[] = ['push'];

/** Çoklu seçimde aç / kapat; uygulama bildirimi kapatılamaz. Sıra sabit. */
export function toggleChannel(list: readonly Channel[], ch: Channel): Channel[] {
  if (ch === 'push') return channelOrder.filter((c) => c === 'push' || list.includes(c));
  const next = list.includes(ch) ? list.filter((c) => c !== ch) : [...list, ch];
  return channelOrder.filter((c) => c === 'push' || next.includes(c));
}

/** İstek açılırken ayrıca gönderilmesi gereken kanallar (bildirim hariç). */
export function extraChannels(list: readonly Channel[]): Channel[] {
  return channelOrder.filter((c) => c !== 'push' && list.includes(c));
}

/** "SMS ve e-posta" gibi okunur liste. */
export function channelsText(list: readonly Channel[]): string {
  const names = channelOrder.filter((c) => list.includes(c)).map((c) => (c === 'email' ? 'e-posta' : c === 'push' ? 'uygulama bildirimi' : 'SMS'));
  if (names.length <= 1) return names[0] ?? '';
  return `${names.slice(0, -1).join(', ')} ve ${names[names.length - 1]}`;
}

/** Hatırlat yalnızca bekleyen (açık) istekte anlamlı. */
export function canRemind(status: string): boolean {
  return status === 'open';
}

/** Sunucuda henüz olmayan uç (404 / 405 / 501): ekranda "Bu özellik yakında". */
export function isComingSoon(e: unknown): boolean {
  if (typeof e !== 'object' || e === null || !('status' in e)) return false;
  const s = (e as { status: unknown }).status;
  return s === 404 || s === 405 || s === 501;
}

export const SOON_MESSAGE = 'Bu özellik yakında.';

/** Son günden kaç gün önce bir kez hatırlatılsın (null: ek hatırlatma yok). */
export const remindChoices: readonly (number | null)[] = [null, 1, 3];

/**
 * Seçilebilecek hatırlatma günleri: son güne kalan günden az olanlar
 * (sunucu geçmiş bir günü reddeder). Son gün bilinmiyorsa yalnızca "yok".
 */
export function remindOptions(dueDays: number | null): (number | null)[] {
  return remindChoices.filter((d) => d === null || (dueDays !== null && d < dueDays));
}

export function remindLabel(d: number | null): string {
  return d === null ? 'Yok' : `${d} gün önce`;
}
