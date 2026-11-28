/**
 * Gönderim durumu (#780 Senaryo B): telefondan gönderilen her fiş ve belge
 * için mükellefin gördüğü aşama, Türkçe etiket, renk ve neden. Onay ekranı
 * ("Müşavirinize ulaştı ✓" / "Telefonda sırada") ve Gönderdiklerim aynı
 * kuralları kullanır. Saf modül: React Native yok, node --test ile sınanır.
 */
import type { QueueItem } from './queue.ts';

export type Asama =
  | 'telefonda' // kuyrukta; bağlanınca ya da engel kalkınca gider
  | 'gonderiliyor' // şu an gönderiliyor ve okunuyor
  | 'ulasti' // ofise ulaştı, onay bekliyor
  | 'okunamadi'
  | 'mukerrer'
  | 'onaylandi'
  | 'reddedildi'
  | 'kapatildi';

export type Ton = 'neutral' | 'brand' | 'success' | 'warning' | 'danger';

/** `GET /v1/taxpayer/submissions` satırı (TaxpayerSubmission). */
export type SunucuGonderimi = {
  uuid: string;
  source: 'receipt' | 'document_request';
  kind: string;
  title: string;
  status: 'received' | 'duplicate' | 'unreadable' | 'approved' | 'rejected' | 'cancelled';
  reason: string;
  media_type: string;
  vendor_name: string;
  total_amount: string;
  office_name: string;
  task_uuid: string | null;
  created_at: string;
  updated_at: string;
};

const ETIKET: Record<Asama, string> = {
  telefonda: 'Telefonda',
  gonderiliyor: 'Gönderiliyor',
  ulasti: 'Müşavirinize ulaştı',
  okunamadi: 'Okunamadı',
  mukerrer: 'Daha önce gönderilmiş',
  onaylandi: 'Onaylandı',
  reddedildi: 'Reddedildi',
  kapatildi: 'İstek kapatıldı',
};

const TON: Record<Asama, Ton> = {
  telefonda: 'warning',
  gonderiliyor: 'brand',
  ulasti: 'brand',
  okunamadi: 'danger',
  mukerrer: 'neutral',
  onaylandi: 'success',
  reddedildi: 'danger',
  kapatildi: 'neutral',
};

export const asamaEtiketi = (a: Asama): string => ETIKET[a];
export const asamaTonu = (a: Asama): Ton => TON[a];

const TUR: Record<string, string> = { expense_receipt: 'Gider fişi', fuel_receipt: 'Akaryakıt fişi', z_report: 'Z raporu' };
export const turEtiketi = (kind: string | null | undefined): string => TUR[kind ?? ''] ?? 'Fiş';

/** Sunucudaki durum → aşama. */
export function sunucuAsamasi(status: SunucuGonderimi['status']): Asama {
  switch (status) {
    case 'approved':
      return 'onaylandi';
    case 'rejected':
      return 'reddedildi';
    case 'duplicate':
      return 'mukerrer';
    case 'unreadable':
      return 'okunamadi';
    case 'cancelled':
      return 'kapatildi';
    default:
      return 'ulasti';
  }
}

/** Kuyruktaki öğenin aşaması (sunucu satırı yoksa). */
export function kuyrukAsamasi(item: QueueItem): Asama {
  if (item.status === 'uploading') return 'gonderiliyor';
  if (item.status !== 'uploaded') return 'telefonda';
  switch (item.result?.status) {
    case 'approved':
      return 'onaylandi';
    case 'rejected':
      return 'reddedildi';
    case 'duplicate':
      return 'mukerrer';
    case 'failed':
      return 'okunamadi';
    default:
      return 'ulasti';
  }
}

/** Takılan öğe: elle "Tekrar dene" / "Kuyruktan kaldır" gösterilir. */
export function takiliMi(item: QueueItem): boolean {
  return item.status === 'failed' || item.status === 'blocked' || (item.status === 'pending' && item.attempts > 0);
}

/**
 * Telefonda bekleyen öğenin nedeni, mükellefin anlayacağı dille. Kod
 * sunucunun hata kodudur (queue.failureCode); kod yoksa (eski kayıt)
 * duruma göre.
 */
export function bekleyisNedeni(item: QueueItem, online: boolean): string | null {
  if (item.status === 'uploaded' || item.status === 'uploading') return null;
  const code = item.errorCode ?? null;
  if (item.status === 'failed') {
    return `${item.lastError || 'Sunucu fotoğrafı kabul etmedi.'} Tekrar deneyin ya da kuyruktan kaldırın.`;
  }
  if (code === 'NO_ACTIVE_ENGAGEMENT') {
    return 'Şirketinizin bağlı bir müşavirlik ofisi yok. Ofis bağlanınca kendiliğinden gönderilecek.';
  }
  if (item.status === 'blocked') {
    return 'Sunucu şu an fiş kabul etmiyor. Birkaç saat içinde kendiliğinden yeniden denenecek; isterseniz şimdi tekrar deneyin.';
  }
  if (!online || code === 'NETWORK') return 'İnternet bağlantısı yok. Bağlanınca kendiliğinden gönderilecek.';
  if (item.attempts === 0) return null;
  if (code === 'UPLOAD_TIMEOUT' || code === 'HTTP_408') {
    return 'Bağlantı yavaş olduğu için fotoğraf yarıda kaldı; kendiliğinden yeniden gönderilecek.';
  }
  if (code === 'HTTP_401') return 'Oturumunuz yenileniyor; birazdan yeniden denenecek.';
  if (code === 'HTTP_429' || (code?.startsWith('HTTP_5') ?? false)) {
    return 'Sunucu şu an yoğun ya da ulaşılamıyor; kendiliğinden yeniden denenecek.';
  }
  return item.lastError ? `${item.lastError} Kendiliğinden yeniden denenecek.` : 'Kendiliğinden yeniden denenecek.';
}

/** Sunucu aşamasının açıklaması; ofisin gerekçesi varsa o. */
export function sunucuNedeni(s: Pick<SunucuGonderimi, 'status' | 'reason' | 'source'>): string | null {
  const reason = s.reason?.trim();
  switch (s.status) {
    case 'rejected':
      if (reason) return reason;
      return s.source === 'document_request'
        ? 'Müşaviriniz bu belgeyi yeniden istedi.'
        : "Müşaviriniz bu fişi kabul etmedi. Ayrıntı için Mesajlarım'dan yazabilirsiniz.";
    case 'unreadable':
      return 'Fotoğraf okunamadı; müşaviriniz bakacak. Gerekirse daha net bir fotoğrafla yeniden çekin.';
    case 'duplicate':
      return 'Bu fiş daha önce gönderilmiş; yeni kayıt açılmadı.';
    case 'cancelled':
      return reason || 'Müşaviriniz bu isteği kapattı.';
    default:
      return null;
  }
}

/** "15 Kas 14:32" (Türkiye saati). */
export function saat(ms: number): string {
  return new Intl.DateTimeFormat('tr-TR', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Istanbul',
  }).format(new Date(ms));
}

/** Görüntüleyicide açılacak dosya: telefondaki fotoğraf ya da sunucudaki dosya. */
export type GonderimBelgesi = { title: string; localUri?: string; path?: string; contentType: string; id: string; prefix: string };

export type GonderimSatiri = {
  key: string;
  baslik: string;
  alt: string;
  asama: Asama;
  etiket: string;
  ton: Ton;
  neden: string | null;
  zaman: number;
  /** Kuyruk kimliği: "Tekrar dene" / "Kuyruktan kaldır" bununla. */
  yerelId: string | null;
  takili: boolean;
  belge: GonderimBelgesi | null;
};

function serverUuid(item: QueueItem): string | null {
  return item.result?.uuid ?? item.taskUpload?.uploadUuid ?? null;
}

function serverBelge(s: SunucuGonderimi, title: string): GonderimBelgesi | null {
  if (s.source === 'receipt') {
    if (!s.media_type.toLowerCase().startsWith('image/')) return null;
    return { title, path: `/v1/taxpayer/receipts/${encodeURIComponent(s.uuid)}/image`, contentType: s.media_type, id: s.uuid, prefix: 'fis' };
  }
  if (!s.task_uuid) return null;
  return {
    title,
    path: `/v1/taxpayer/document-tasks/${encodeURIComponent(s.task_uuid)}/uploads/${encodeURIComponent(s.uuid)}/file`,
    contentType: s.media_type,
    id: s.uuid,
    prefix: 'istek',
  };
}

/**
 * Gönderdiklerim satırları, en yeni önce: telefondaki öğeler (sunucuya
 * ulaşmışsa sunucudaki durumuyla) ve yalnız sunucuda olanlar (başka
 * telefondan ya da kuyruktan kaldırılmış).
 */
export function gonderimSatirlari(
  queue: readonly QueueItem[],
  server: readonly SunucuGonderimi[],
  taxpayer: string,
  online: boolean,
  taskTitles: Readonly<Record<string, string>> = {},
): GonderimSatiri[] {
  const byUuid = new Map(server.map((s) => [s.uuid, s]));
  const used = new Set<string>();
  const rows: GonderimSatiri[] = [];
  for (const item of queue) {
    if (item.taxpayer !== taxpayer) continue;
    const su = serverUuid(item);
    const s = su ? byUuid.get(su) : undefined;
    if (su) used.add(su);
    const isTask = item.target?.type === 'task';
    const baslik = isTask
      ? (s?.title || taskTitles[item.target!.taskUuid] || 'Belge isteği')
      : s?.vendor_name || item.result?.fields?.vendor_name?.value || turEtiketi(item.kind);
    const asama = s ? sunucuAsamasi(s.status) : kuyrukAsamasi(item);
    const neden = s ? sunucuNedeni(s) : item.status === 'uploaded' ? null : bekleyisNedeni(item, online);
    const zaman = item.createdAt;
    const ofis = s?.office_name;
    rows.push({
      key: `q:${item.id}`,
      baslik,
      alt: [isTask ? 'Belge isteği' : turEtiketi(item.kind), saat(zaman), asama === 'telefonda' || asama === 'gonderiliyor' ? null : ofis]
        .filter(Boolean)
        .join(' · '),
      asama,
      etiket: ETIKET[asama],
      ton: TON[asama],
      neden,
      zaman,
      yerelId: item.id,
      takili: takiliMi(item),
      belge: { title: baslik, localUri: item.fileUri, contentType: item.mimeType, id: item.id, prefix: 'kuyruk' },
    });
  }
  for (const s of server) {
    if (used.has(s.uuid)) continue;
    const asama = sunucuAsamasi(s.status);
    const baslik = s.source === 'document_request' ? s.title || 'Belge isteği' : s.vendor_name || turEtiketi(s.kind);
    const zaman = Date.parse(s.created_at) || 0;
    rows.push({
      key: `s:${s.uuid}`,
      baslik,
      alt: [s.source === 'document_request' ? 'Belge isteği' : turEtiketi(s.kind), saat(zaman), s.office_name].filter(Boolean).join(' · '),
      asama,
      etiket: ETIKET[asama],
      ton: TON[asama],
      neden: sunucuNedeni(s),
      zaman,
      yerelId: null,
      takili: false,
      belge: serverBelge(s, baslik),
    });
  }
  return rows.sort((a, b) => b.zaman - a.zaman);
}

export type Teslim = {
  durum: 'gonderiliyor' | 'ulasti' | 'sirada' | 'gonderilemedi';
  baslik: string;
  aciklama: string;
};

/**
 * Onay ekranının durumu: gönderilen öğelerin hepsi ulaştıysa "Müşavirinize
 * ulaştı ✓" (ofis adı ve saatiyle), biri kesin reddedildiyse
 * "Gönderilemedi", biri telefonda bekliyorsa "Telefonda sırada" ve nedeni,
 * yoksa "Gönderiliyor".
 */
export function teslimDurumu(items: readonly QueueItem[], online: boolean, office: string | null | undefined): Teslim {
  const n = items.length;
  const what = n > 1 ? `${n} fotoğraf` : 'Fotoğraf';
  const failed = items.find((i) => i.status === 'failed');
  if (failed) {
    return { durum: 'gonderilemedi', baslik: 'Gönderilemedi', aciklama: bekleyisNedeni(failed, online) ?? 'Tekrar deneyin.' };
  }
  if (n > 0 && items.every((i) => i.status === 'uploaded')) {
    const at = Math.max(...items.map((i) => i.uploadedAt ?? i.createdAt));
    const to = office?.trim() ? office.trim() : 'Müşavir ofisiniz';
    return { durum: 'ulasti', baslik: 'Müşavirinize ulaştı ✓', aciklama: `${to} · ${saat(at)}` };
  }
  const waiting = items.find((i) => i.status === 'blocked' || (i.status === 'pending' && (i.attempts > 0 || !online)));
  if (waiting) {
    const why = bekleyisNedeni(waiting, online);
    return {
      durum: 'sirada',
      baslik: 'Telefonda sırada, bağlanınca gönderilecek',
      aciklama: `${what} telefonda güvende.${why ? ` ${why}` : ''}`,
    };
  }
  return { durum: 'gonderiliyor', baslik: 'Gönderiliyor…', aciklama: `${what} müşavirinize gönderiliyor.` };
}
