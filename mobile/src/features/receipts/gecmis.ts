/**
 * Geçmiş ekranı (MukellefMobil · 5): telefondaki kuyruk ve sunucudaki fişler
 * tek listede, aya göre gruplu; her satırda taç ve kuyruk etiketi.
 * Saf modül, node --test ile sınanır.
 */
import type { CrownState } from '@hakan/mobile-core/notifications/inbox';

import { receiptCrown } from '../home/crown.ts';
import { kindLabels, statusLabel } from './fields.ts';
import { historyStatusLabel } from './history.ts';
import type { QueueItem } from './queue.ts';

export type Ton = 'warn' | 'danger' | null;

export type GecmisSatiri = {
  key: string;
  baslik: string;
  alt: string;
  ton: Ton;
  tutar: number | null;
  tac: CrownState;
  /** epoch ms */
  zaman: number;
  /** telefondaki kuyruk öğesi: ayrıntı ekranı açılır */
  yerelId: string | null;
};

type SunucuFisi = { uuid: string; kind?: string; status: string; vendor_name?: string; date?: string; total_amount?: string; created_at: string };

const sayi = (v: string | null | undefined) => {
  const n = Number(v);
  return v && Number.isFinite(n) ? n : null;
};

export function yerelSatir(i: QueueItem): GecmisSatiri {
  const sent = i.status === 'uploaded';
  const tac: CrownState = sent ? receiptCrown(i.result?.status) : i.status === 'failed' ? 'sorun' : 'bekleniyor';
  const ton: Ton = tac === 'sorun' ? 'danger' : sent ? null : 'warn';
  return {
    key: `q:${i.id}`,
    baslik: i.result?.fields.vendor_name?.value || kindLabels[i.kind],
    alt: sent ? statusLabel(i) : `Telefonda · ${statusLabel(i).toLocaleLowerCase('tr')}`,
    ton,
    tutar: sayi(i.result?.fields.total_amount?.value),
    tac,
    zaman: i.createdAt,
    yerelId: i.id,
  };
}

export function sunucuSatiri(h: SunucuFisi): GecmisSatiri {
  const tac = receiptCrown(h.status);
  return {
    key: `s:${h.uuid}`,
    baslik: h.vendor_name || (h.kind && kindLabels[h.kind as keyof typeof kindLabels]) || 'Fiş',
    alt: historyStatusLabel(h.status),
    ton: tac === 'sorun' ? 'danger' : null,
    tutar: sayi(h.total_amount),
    tac,
    zaman: Date.parse(h.date ? `${h.date}T12:00:00Z` : h.created_at),
    yerelId: null,
  };
}

const AYLAR = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];

export function tl(n: number): string {
  return n.toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

/** Aya göre gruplar, yeniden eskiye. Başlık: "Ekim 2026 · 64 fiş · ₺ 41.208". */
export function ayGruplari(rows: readonly GecmisSatiri[]): { baslik: string; data: GecmisSatiri[] }[] {
  const sorted = [...rows].sort((a, b) => b.zaman - a.zaman);
  const groups = new Map<string, GecmisSatiri[]>();
  for (const r of sorted) {
    const d = new Date(r.zaman);
    const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit' }).format(d);
    const list = groups.get(p) ?? [];
    list.push(r);
    groups.set(p, list);
  }
  return [...groups.entries()].map(([p, data]) => {
    const [y, m] = p.split('-').map(Number);
    const toplam = data.reduce((s, r) => s + (r.tutar ?? 0), 0);
    const parts = [`${AYLAR[m - 1]} ${y}`, `${data.length} fiş`];
    if (toplam) parts.push(`₺ ${Math.round(toplam).toLocaleString('tr-TR')}`);
    return { baslik: parts.join(' · '), data };
  });
}
