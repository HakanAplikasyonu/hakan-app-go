/** İnceleme kuyruğu yardımcıları: saf modül, node --test ile sınanır. */

export type SuggestionLike = {
  uuid: string;
  engagement_uuid?: string;
  kind: string;
  target_field: string;
  proposed_value: unknown;
  confidence: string;
  status: string;
};

/** Önerilen hesap kodu (`{"account_code": "770.01"}`), yoksa null. */
export function proposedCode(v: unknown): string | null {
  if (v && typeof v === 'object' && 'account_code' in v) {
    const c = (v as { account_code: unknown }).account_code;
    return typeof c === 'string' && c ? c : null;
  }
  return null;
}

/**
 * Hesap kodu biçimi, backend ile aynı (accountrules/engine.ValidCode):
 * 3 haneli ana hesap, noktayla ayrılmış alt kırılımlar, en çok 64 karakter.
 */
export function isValidAccountCode(code: string): boolean {
  const c = code.trim();
  return c.length <= 64 && /^[0-9]{3}(\.[0-9A-Za-z]+)*$/.test(c);
}

/** "0.8700" → "%87". */
export function confidencePercent(c: string): string {
  const n = Number(c);
  return Number.isFinite(n) ? `%${Math.round(n * 100)}` : '—';
}

export type ConfidenceBand = 'low' | 'medium' | 'high';
export function confidenceBand(c: string): ConfidenceBand {
  const n = Number(c);
  if (!(n >= 0.6)) return 'low';
  return n >= 0.85 ? 'high' : 'medium';
}

/** Öneri alanlarının Türkçe adları (#375): ham alan adı gösterilmez. */
const FIELD_LABELS: Record<string, string> = {
  account_code: 'Hesap kodu',
  vat_account_code: 'KDV hesabı',
  date: 'Fiş tarihi',
  vendor_tax_number: 'Satıcı VKN',
  vendor_name: 'Satıcı adı',
  receipt_number: 'Fiş no',
  total_amount: 'Fiş toplamı',
  tax_amount: 'KDV tutarı',
  tax_rate: 'KDV oranı',
  vehicle_plate: 'Plaka',
  z_report_number: 'Z raporu no',
  okc_serial_number: 'ÖKC sicil no',
  payment_method: 'Ödeme şekli',
  match: 'Banka eşleşmesi',
};

/** "total_amount" → "Total amount": çevirisi olmayan alan için son çare. */
export function humanize(f: string): string {
  const s = f.replace(/[_.]+/g, ' ').trim();
  return s ? s[0].toLocaleUpperCase('tr-TR') + s.slice(1) : f;
}

export const fieldLabel = (f: string) => FIELD_LABELS[f] ?? humanize(f);

type MatchValue = { score?: number; invoices?: { number?: string }[] };
function asMatch(v: unknown): MatchValue | null {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return null;
  const m = v as MatchValue;
  return Array.isArray(m.invoices) ? m : null;
}

/** Listede ve kartta başlık: banka eşleşmesinde fatura numaraları, yoksa alanın adı. */
export function suggestionLabel(s: Pick<SuggestionLike, 'kind' | 'target_field' | 'proposed_value'>): string {
  const m = s.kind === 'bank_match' ? asMatch(s.proposed_value) : null;
  if (m) {
    const nums = (m.invoices ?? []).map((i) => i.number).filter(Boolean) as string[];
    return nums.length === 1 ? `Banka eşleşmesi: Fatura ${nums[0]}` : `Banka eşleşmesi: ${nums.length} fatura`;
  }
  return fieldLabel(s.target_field);
}

const trMoney = new Intl.NumberFormat('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/**
 * Önerilen değerin kısa gösterimi: hesap kodu, tutar (tr-TR), tarih
 * (GG.AA.YYYY), eşleşme puanı (%73). Nesne asla JSON olarak yazılmaz.
 */
export function proposedText(v: unknown): string {
  const code = proposedCode(v);
  if (code) return code;
  if (v === null || v === undefined) return '';
  if (typeof v === 'number') return trMoney.format(v);
  if (typeof v === 'string') {
    const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v);
    if (d) return `${d[3]}.${d[2]}.${d[1]}`;
    return /^-?\d+\.\d+$/.test(v) ? trMoney.format(Number(v)) : v;
  }
  const m = asMatch(v);
  if (m) return typeof m.score === 'number' ? `%${m.score}` : '';
  return '—';
}

/** Toplu onay: yalnızca bekleyen ve engagement'ı bilinen öneriler. */
export function bulkTargets<T extends SuggestionLike>(list: readonly T[], selected: ReadonlySet<string>): T[] {
  return list.filter((s) => selected.has(s.uuid) && s.status === 'pending' && !!s.engagement_uuid);
}

/** Sıralı çalıştırma: her biri ayrı karar (backend kaydı), hata diğerlerini durdurmaz. */
export async function runSequential<T>(items: readonly T[], fn: (item: T) => Promise<unknown>): Promise<{ ok: T[]; failed: { item: T; error: unknown }[] }> {
  const ok: T[] = [];
  const failed: { item: T; error: unknown }[] = [];
  for (const item of items) {
    try {
      await fn(item);
      ok.push(item);
    } catch (error) {
      failed.push({ item, error });
    }
  }
  return { ok, failed };
}

/** Kaydırma hareketi: eşik aşılınca karar (sağa onay, sola reddet, yukarı düzelt). */
export type SwipeAction = 'accept' | 'reject' | 'correct' | null;
export function swipeAction(dx: number, width: number, threshold = 0.35, dy = 0, height = 0): SwipeAction {
  'worklet';
  const limit = Math.max(60, width * threshold);
  const up = Math.max(80, height * 0.25);
  // Yukarı hareket yataydan baskınsa düzelt; aksi halde yatay eşik.
  if (height > 0 && dy <= -up && Math.abs(dy) > Math.abs(dx)) return 'correct';
  if (dx >= limit) return 'accept';
  if (dx <= -limit) return 'reject';
  return null;
}

/** Kart yığınında kartın eğimi (derece): yatay kaymayla en çok ±8°. */
export function cardTilt(dx: number, width: number): number {
  'worklet';
  if (!width) return 0;
  return Math.max(-8, Math.min(8, (dx / width) * 16));
}

/** Kuyruk ilerlemesi: bu oturumda karar verilen / (karar + kalan), 0..1. */
export function queueProgress(decided: number, remaining: number): number {
  const all = decided + remaining;
  return all > 0 ? Math.min(1, decided / all) : 0;
}

/** Tahmini süre: öneri başına ortalama 6 sn (tasarım). "~5 dk", "1 dk'dan az". */
export function queueEta(count: number, secondsEach = 6): string {
  if (count <= 0) return '';
  const secs = count * secondsEach;
  return secs < 60 ? "1 dk'dan az" : `~${Math.round(secs / 60)} dk`;
}

export type QueueGrouping = 'mukellef' | 'guven';
export type QueueGroup<T> = { key: string; title: string; data: T[] };

const BAND_TITLE: Record<ConfidenceBand, string> = { low: 'Düşük güven', medium: 'Orta güven', high: 'Yüksek güven' };

/**
 * Tablet onay listesi (MusavirTablet): mükellefe göre (kuyruktaki ilk
 * görünüş sırasıyla) ya da güvene göre (önce düşük: dikkat isteyenler üstte).
 * Grup içindeki sıra kuyruk sırasıdır.
 */
export function groupQueue<T extends Pick<SuggestionLike, 'uuid' | 'engagement_uuid' | 'confidence'>>(
  list: readonly T[],
  mode: QueueGrouping,
  titleOf: (engagement: string) => string | undefined,
): QueueGroup<T>[] {
  const groups = new Map<string, QueueGroup<T>>();
  if (mode === 'guven') {
    for (const band of ['low', 'medium', 'high'] as const) groups.set(band, { key: band, title: BAND_TITLE[band], data: [] });
  }
  for (const s of list) {
    const key = mode === 'guven' ? confidenceBand(s.confidence) : (s.engagement_uuid ?? '');
    let g = groups.get(key);
    if (!g) {
      g = { key, title: (key && titleOf(key)) || 'Mükellef', data: [] };
      groups.set(key, g);
    }
    g.data.push(s);
  }
  return [...groups.values()].filter((g) => g.data.length > 0);
}

/**
 * Aynı belgenin (hedef tür + kimlik) bekleyen diğer önerileri: fişin
 * okunan alanlarının hepsi karara bağlanmadan masraf onaylanmaz.
 */
export function otherPendingForTarget<T extends { uuid: string; status: string; target_type: string; target_id?: number | null }>(
  list: readonly T[],
  s: { uuid: string; target_type: string; target_id?: number | null },
): T[] {
  if (s.target_id === null || s.target_id === undefined) return [];
  return list.filter((x) => x.uuid !== s.uuid && x.status === 'pending' && x.target_type === s.target_type && x.target_id === s.target_id);
}

/** Fiş kartında Onayla sonrası bildirim. */
export function receiptApproveMessage(result: 'approved' | 'waiting' | 'skipped', code: string | null | undefined): string {
  const c = code ? ` (${code})` : '';
  if (result === 'approved') return `Fiş onaylandı${c}; yevmiye taslağı oluşturuldu.`;
  if (result === 'waiting') return `Fiş alanı onaylandı${c}; fişin diğer alanları da onaylanınca fiş muhasebeleşir.`;
  return 'Fiş alanı onaylandı.';
}
