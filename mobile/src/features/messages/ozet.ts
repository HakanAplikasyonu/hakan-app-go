// Saf yardımcılar (#780 Mesajlarım): ofisten gelen SMS ve e-posta listesi
// için metin üretimi. node --test ile sınanır, React Native'e bağlı değil.

export type MesajDurumu = 'pending' | 'sent' | 'delivered' | 'failed';
export type MesajKanali = 'sms' | 'email' | 'push';

/** Durum kodunun ekrandaki adı ve rengi. */
export const DURUM: Record<MesajDurumu, { metin: string; ton: 'notr' | 'ok' | 'danger' | 'brand' }> = {
  pending: { metin: 'Gönderiliyor', ton: 'notr' },
  sent: { metin: 'Gönderildi', ton: 'brand' },
  delivered: { metin: 'İletildi', ton: 'ok' },
  failed: { metin: 'Ulaşmadı', ton: 'danger' },
};

export function durum(s: string): { metin: string; ton: 'notr' | 'ok' | 'danger' | 'brand' } {
  return DURUM[s as MesajDurumu] ?? DURUM.pending;
}

export function kanalAdi(k: string): string {
  if (k === 'push') return 'Uygulama bildirimi';
  return k === 'email' ? 'E-posta' : 'SMS';
}

/** Satır başlığı: e-postada ve uygulama bildiriminde konu, SMS'te metnin ilk satırı; en çok 60 harf. */
export function baslik(m: { channel: string; subject: string; body: string }): string {
  const ham = (m.channel === 'email' || m.channel === 'push') && m.subject.trim() ? m.subject.trim() : (m.body.trim().split('\n')[0] ?? '');
  return kisalt(ham || kanalAdi(m.channel), 60);
}

/** Satırın altındaki önizleme: metin tek satıra indirgenmiş, en çok 90 harf. */
export function onizleme(body: string): string {
  return kisalt(body.replace(/\s+/g, ' ').trim(), 90);
}

function kisalt(s: string, n: number): string {
  const harfler = Array.from(s);
  return harfler.length > n ? `${harfler.slice(0, n - 1).join('').trimEnd()}…` : s;
}

const AYLAR = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];

/** ISO zaman → "19 Kasım 2026 10:00" (cihazın saat diliminde). */
export function zaman(iso: string | null | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const iki = (n: number) => String(n).padStart(2, '0');
  return `${d.getDate()} ${AYLAR[d.getMonth()]} ${d.getFullYear()} ${iki(d.getHours())}:${iki(d.getMinutes())}`;
}
