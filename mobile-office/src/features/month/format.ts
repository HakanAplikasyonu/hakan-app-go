/**
 * Mükellef ayrıntısındaki dönem kartları (Fatura, Z raporu, Masraf, Banka;
 * TestFlight geri bildirimi, #780) ve açtıkları liste / belge ekranlarının
 * yardımcıları: saf modül, node --test ile sınanır.
 */

export type MonthKind = 'invoices' | 'zreports' | 'expenses' | 'bank';

export const MONTH_KINDS: readonly MonthKind[] = ['invoices', 'zreports', 'expenses', 'bank'];

/** Kartın başlığı (web'deki dört kartla aynı). */
export const kindTitle: Record<MonthKind, string> = {
  invoices: 'Fatura',
  zreports: 'Z raporu',
  expenses: 'Masraf',
  bank: 'Banka',
};

/** Liste ekranının başlığı. */
export const kindListTitle: Record<MonthKind, string> = {
  invoices: 'Faturalar',
  zreports: 'Z raporları',
  expenses: 'Masraflar',
  bank: 'Banka hareketleri',
};

/** Belge ekranının başlığı. */
export const kindItemTitle: Record<MonthKind, string> = {
  invoices: 'Fatura',
  zreports: 'Z raporu',
  expenses: 'Masraf',
  bank: 'Banka hareketi',
};

export function isMonthKind(v: unknown): v is MonthKind {
  return typeof v === 'string' && (MONTH_KINDS as readonly string[]).includes(v);
}

export function isPeriod(v: unknown): v is string {
  if (typeof v !== 'string') return false;
  const m = /^(\d{4})-(\d{2})$/.exec(v);
  return !!m && Number(m[2]) >= 1 && Number(m[2]) <= 12;
}

/** Bu ay ve geriye doğru `count` ay (Türkiye saati), yeniden eskiye, "YYYY-MM". */
export function pastPeriods(count = 12, now: Date = new Date()): string[] {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit' }).formatToParts(now);
  let y = Number(parts.find((p) => p.type === 'year')?.value);
  let m = Number(parts.find((p) => p.type === 'month')?.value);
  const out: string[] = [];
  for (let i = 0; i < Math.max(1, count); i++) {
    out.push(`${y}-${String(m).padStart(2, '0')}`);
    m -= 1;
    if (m === 0) {
      m = 12;
      y -= 1;
    }
  }
  return out;
}

const SHORT_MONTHS = ['Oca', 'Şub', 'Mar', 'Nis', 'May', 'Haz', 'Tem', 'Ağu', 'Eyl', 'Eki', 'Kas', 'Ara'];

/** Dönem çipi: "2026-11" → "Kas 2026". */
export function shortPeriodLabel(period: string): string {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  const i = Number(m[2]) - 1;
  return i >= 0 && i < 12 ? `${SHORT_MONTHS[i]} ${m[1]}` : period;
}

/** Fatura kartının durum satırı: "Gelen 3 · Giden 5". */
export function invoiceStatus(total: number | undefined, incoming: number | undefined): string {
  if (total === undefined || incoming === undefined) return 'Yükleniyor…';
  if (total === 0) return 'Bu ay kayıt yok';
  return `Gelen ${incoming} · Giden ${Math.max(total - incoming, 0)}`;
}

/** Masraf / Z raporu kartının durum satırı. */
export function receiptStatus(page: { total: number; pending: number } | undefined): string {
  if (!page) return 'Yükleniyor…';
  if (page.total === 0) return 'Bu ay kayıt yok';
  if (page.pending > 0) return `${page.pending} onay bekliyor · ${page.total - page.pending} işlendi`;
  return 'Hepsi işlendi';
}

/** Banka kartının durum satırı: eşleşmeyen ve öneri bekleyen hareketler. */
export function bankStatus(summary: { total: number; matched: number; unmatched: number; suggested: number } | undefined): string {
  if (!summary) return 'Yükleniyor…';
  if (summary.total === 0) return 'Bu ay kayıt yok';
  const waiting = summary.unmatched + summary.suggested;
  if (waiting > 0) return `${waiting} hareket bekliyor · ${summary.matched} eşleşti`;
  return 'Hepsi işlendi';
}

export type Approval = 'pending' | 'approved' | 'rejected';

export const approvalLabel: Record<Approval, string> = {
  pending: 'Onay bekliyor',
  approved: 'Onaylı',
  rejected: 'Reddedildi',
};

export const approvalTone: Record<Approval, 'warning' | 'success' | 'danger'> = {
  pending: 'warning',
  approved: 'success',
  rejected: 'danger',
};

export type ReceiptRead = 'pending' | 'read' | 'mismatch' | 'unreadable';

/**
 * Ön muhasebe programından gelen masrafın KDV oranları fiş görselinden
 * okundu mu (#780); fiş görseli yoksa null.
 */
export function receiptReadChip(status: ReceiptRead | null | undefined): { label: string; tone: 'success' | 'warning' | 'neutral' } | null {
  switch (status) {
    case 'read':
      return { label: 'Fişten okundu', tone: 'success' };
    case 'pending':
      return { label: 'Fiş okunuyor', tone: 'neutral' };
    case 'mismatch':
    case 'unreadable':
      return { label: 'Fiş okunamadı / tutmadı', tone: 'warning' };
    default:
      return null;
  }
}

/** Birden çok KDV oranlı masrafın oranları: "%1 + %20"; tek oranda boş. */
export function vatRatesLabel(lines: readonly { tax_rate: string }[]): string {
  if (lines.length < 2) return '';
  return lines.map((l) => rateLabel(l.tax_rate)).join(' + ');
}

/** Okunamayan / emin olunamayan alanların Türkçe adları (web: receipts-ui). */
const REVIEW_FIELDS: Record<string, string> = {
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
  cash_amount: 'Nakit',
  card_amount: 'Kart',
  receipt_count: 'Fiş sayısı',
};

/** "vat_20_tax" → "%20 KDV"; bilinmeyen alan "Diğer alan" olur (ham ad gösterilmez). */
export function reviewFieldLabel(field: string): string {
  const vat = /^vat_(\d+)_(tax|total)$/.exec(field);
  if (vat) return vat[2] === 'tax' ? `%${vat[1]} KDV` : `%${vat[1]} toplam`;
  return REVIEW_FIELDS[field] ?? 'Diğer alan';
}

/** "Kontrol et" satırı: alanlar virgülle; kontrol edilecek alan yoksa null. */
export function reviewSummary(fields: readonly string[] | null | undefined): string | null {
  if (!fields?.length) return null;
  const names = [...new Set(fields.map(reviewFieldLabel))];
  return `Kontrol et · ${names.join(', ')}`;
}

/** Masrafın hesap kodunun nereden geldiği. */
export function accountSourceLabel(src: string | null | undefined): string | null {
  switch (src) {
    case 'category':
      return 'Kategorinin hesabı';
    case 'plan':
      return 'Mizana eşlenmiş kategori hesabı';
    case 'vendor_memory':
      return 'Bu satıcının son onaylı masrafındaki hesap';
    case 'taxpayer_memory':
      return 'Mükellefin son onaylı masrafındaki hesap';
    case 'manual':
      return 'Elle girildi';
    default:
      return null;
  }
}

/** Masrafın ödeme şekli (onayda alacak hesabını belirler). */
export function paymentMethodLabel(m: string | null | undefined): string | null {
  if (!m) return null;
  const labels: Record<string, string> = {
    cash: 'Nakit',
    transfer: 'Havale / EFT',
    card: 'Kart',
    check: 'Çek',
    note: 'Senet',
    credit: 'Veresiye',
  };
  return labels[m] ?? 'Diğer';
}

export type MatchStatus = 'unmatched' | 'suggested' | 'matched' | 'ignored';

export const matchStatusLabel: Record<MatchStatus, string> = {
  unmatched: 'Eşleşmedi',
  suggested: 'Öneri var',
  matched: 'Eşleşti',
  ignored: 'Yok sayıldı',
};

export const matchStatusTone: Record<MatchStatus, 'warning' | 'brand' | 'success' | 'neutral'> = {
  unmatched: 'warning',
  suggested: 'brand',
  matched: 'success',
  ignored: 'neutral',
};

/** Banka hareketinin eşleşme etiketi: eşleşmemiş ama hesaba işlenmişse "Hesaba işlendi". */
export function bankMatchLabel(t: { match_status: MatchStatus; booked_account_code?: string | null }): string {
  if (t.match_status === 'unmatched' && t.booked_account_code) return 'Hesaba işlendi';
  return matchStatusLabel[t.match_status] ?? t.match_status;
}

/** "2026-11-20" → "20.11.2026"; boş ya da biçimsizse olduğu gibi / "—". */
export function dateLabel(d: string | null | undefined): string {
  if (!d) return '—';
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(d);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : d;
}

/** Tutar: "1234.5" → "1.234,50 ₺"; boşsa "—". */
export function amountLabel(amount: string | null | undefined, currency = 'TRY'): string {
  if (amount === null || amount === undefined || amount === '') return '—';
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  const s = n.toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return currency === 'TRY' ? `${s} ₺` : `${s} ${currency}`;
}

/** KDV oranı: "20.00" → "%20". */
export function rateLabel(rate: string | null | undefined): string {
  if (!rate) return '—';
  const n = Number(rate);
  return Number.isFinite(n) ? `%${n.toLocaleString('tr-TR', { maximumFractionDigits: 2 })}` : rate;
}

/** Hesap aramasında en az 2 karakter; kırpılmış terim ya da undefined. */
export function accountQuery(raw: string): string | undefined {
  const t = raw.trim();
  return t.length >= 2 ? t.slice(0, 50) : undefined;
}

export type InvoiceDirection = 'incoming' | 'outgoing';

export function isDirection(v: unknown): v is InvoiceDirection {
  return v === 'incoming' || v === 'outgoing';
}

/** Mükellef ayrıntısındaki kayıt sekmesi (Oğuz'un canlı geri bildirimi). */
export type RecordTab = 'incoming' | 'outgoing' | 'bank' | 'zreports' | 'expenses';

export type RecordTabDef = { key: RecordTab; label: string; kind: MonthKind; direction?: InvoiceDirection };

/** Sekmeler: gelen ve giden faturalar ayrı; her sekme bir liste ucuna gider. */
export const RECORD_TABS: readonly RecordTabDef[] = [
  { key: 'incoming', label: 'Gelen faturalar', kind: 'invoices', direction: 'incoming' },
  { key: 'outgoing', label: 'Giden faturalar', kind: 'invoices', direction: 'outgoing' },
  { key: 'bank', label: 'Banka hareketleri', kind: 'bank' },
  { key: 'zreports', label: 'Z raporları', kind: 'zreports' },
  { key: 'expenses', label: 'Masraflar / Fişler', kind: 'expenses' },
];

/** Okuma iznine göre görünen sekmeler (izin anahtarı → var mı). */
export function visibleRecordTabs(can: (perm: string) => boolean): RecordTabDef[] {
  const perm: Record<MonthKind, string> = {
    invoices: 'tenant.invoices.read',
    zreports: 'tenant.receipts.read',
    expenses: 'tenant.receipts.read',
    bank: 'tenant.bank.read',
  };
  return RECORD_TABS.filter((t) => can(perm[t.kind]));
}

/** Liste ekranının başlığı; faturada yöne göre "Gelen faturalar" / "Giden faturalar". */
export function listTitle(kind: MonthKind, direction?: InvoiceDirection | null): string {
  if (kind === 'invoices' && direction) return direction === 'incoming' ? 'Gelen faturalar' : 'Giden faturalar';
  return kindListTitle[kind];
}
