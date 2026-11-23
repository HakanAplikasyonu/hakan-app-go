/**
 * Mükellef özeti (Oğuz'un canlı geri bildirimi): bu ayın KDV hesabı (oran
 * kırılımıyla), gelen / giden toplamları ve "Eksik / gedik" kartı. Saf
 * modül, node --test ile sınanır. Tutarlar kuruş (tam sayı) olarak
 * toplanır; ondalık hatası olmaz.
 */
import { amountLabel, rateLabel } from '../month/format.ts';
import { periodLabel } from '../warnings/format.ts';

/** "1234.50" → 123450 kuruş; boş ya da biçimsizse 0. */
export function cents(v: string | null | undefined): number {
  if (v === null || v === undefined || v === '') return 0;
  const n = Number(v);
  return Number.isFinite(n) ? Math.round(n * 100) : 0;
}

/** Kuruş → "1.234,50 ₺". */
export function tl(c: number): string {
  return amountLabel((c / 100).toFixed(2));
}

export type VatRateInput = {
  tax_rate: string;
  output_base: string;
  output_vat: string;
  output_withholding: string;
  input_base: string;
  input_vat: string;
  input_withholding: string;
};

export type VatRow = {
  rate: string;
  label: string;
  /** Satış matrahı, hesaplanan KDV (tevkifat düşülmüş), alış matrahı, indirilecek KDV; kuruş. */
  outputBase: number;
  outputVat: number;
  inputBase: number;
  inputVat: number;
  /** Hesaplanan − indirilecek (bu oranın katkısı). */
  net: number;
};

/** Oran kırılımı: hepsi sıfır olan oran atlanır; yüksek oran önce. */
export function vatBreakdown(rates: readonly VatRateInput[] | null | undefined): VatRow[] {
  return (rates ?? [])
    .map((r) => {
      const row = {
        rate: r.tax_rate,
        label: `KDV ${rateLabel(r.tax_rate)}`,
        outputBase: cents(r.output_base),
        outputVat: cents(r.output_vat),
        inputBase: cents(r.input_base),
        inputVat: cents(r.input_vat),
        net: 0,
      };
      row.net = row.outputVat - row.inputVat;
      return row;
    })
    .filter((r) => r.outputBase || r.outputVat || r.inputBase || r.inputVat)
    .sort((a, b) => Number(b.rate) - Number(a.rate));
}

export type VatTotalsInput = { output_vat: string; input_vat: string; carried_in: string; payable: string; carry_out: string };

export type VatResult = { kind: 'payable' | 'carry' | 'zero'; amount: number; title: string; text: string };

/** Ayın sonucu: ödenecek KDV ya da sonraki aya devreden KDV. */
export function vatResult(t: VatTotalsInput | null | undefined): VatResult | null {
  if (!t) return null;
  const payable = cents(t.payable);
  const carry = cents(t.carry_out);
  if (payable > 0) return { kind: 'payable', amount: payable, title: 'Ödenecek KDV', text: `${tl(payable)} ödenecek` };
  if (carry > 0) return { kind: 'carry', amount: carry, title: 'Devreden KDV', text: `${tl(carry)} sonraki aya devreder` };
  return { kind: 'zero', amount: 0, title: 'KDV', text: 'Ödenecek ya da devreden KDV yok' };
}

export type SideTotal = { base: number; vat: number; total: number };

/** Gelen (alış) ve giden (satış) faturaların matrah, KDV ve toplamı. */
export function directionTotals(rates: readonly VatRateInput[] | null | undefined): { outgoing: SideTotal; incoming: SideTotal } {
  const out = { base: 0, vat: 0, total: 0 };
  const inc = { base: 0, vat: 0, total: 0 };
  for (const r of rates ?? []) {
    out.base += cents(r.output_base);
    out.vat += cents(r.output_vat) + cents(r.output_withholding);
    inc.base += cents(r.input_base);
    inc.vat += cents(r.input_vat);
  }
  out.total = out.base + out.vat;
  inc.total = inc.base + inc.vat;
  return { outgoing: out, incoming: inc };
}

/** "YYYY-MM-DD" (Türkiye saati) bugün. */
export function istanbulToday(now: Date = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit' }).format(now);
}

/**
 * Z raporu olmayan günler: dönemin ilk gününden dünü (bugünün raporu gün
 * sonunda gelir) ya da ay sonunu geçmeyen son güne kadar, `saleDates`
 * içinde olmayan gün numaraları. Gelecek dönemde boş.
 */
export function zGapDays(period: string, saleDates: readonly string[], today: string): number[] {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  const t = /^(\d{4})-(\d{2})-(\d{2})/.exec(today);
  if (!m || !t) return [];
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const ty = Number(t[1]);
  const tm = Number(t[2]);
  const td = Number(t[3]);
  if (y > ty || (y === ty && mo > tm)) return [];
  const monthDays = new Date(Date.UTC(y, mo, 0)).getUTCDate();
  const last = y === ty && mo === tm ? td - 1 : monthDays;
  const have = new Set<number>();
  for (const d of saleDates) {
    const x = /^(\d{4})-(\d{2})-(\d{2})/.exec(d);
    if (x && Number(x[1]) === y && Number(x[2]) === mo) have.add(Number(x[3]));
  }
  const out: number[] = [];
  for (let d = 1; d <= last; d++) if (!have.has(d)) out.push(d);
  return out;
}

/** [1,2,3,5,8,9] → "1–3, 5, 8–9". */
export function dayRanges(days: readonly number[]): string {
  const sorted = [...new Set(days)].sort((a, b) => a - b);
  const parts: string[] = [];
  let i = 0;
  while (i < sorted.length) {
    let j = i;
    while (j + 1 < sorted.length && sorted[j + 1] === sorted[j] + 1) j++;
    parts.push(j > i ? `${sorted[i]}–${sorted[j]}` : String(sorted[i]));
    i = j + 1;
  }
  return parts.join(', ');
}

type ConnStatus = 'pending' | 'active' | 'auth_failed' | 'rate_limited' | 'disabled' | string;

export type IntegrationLists = {
  einvoice?: readonly { integrator_name: string; state: string; error?: { message: string } | null }[];
  bank?: readonly { bank_name: string; status: ConnStatus; error?: { message: string } | null }[];
  easytrade?: readonly { status: ConnStatus; error?: string | null }[];
  kolaybi?: readonly { status: ConnStatus; company_name?: string | null; error?: string | null }[];
  parasut?: readonly { status: ConnStatus; company_name?: string | null; error?: string | null }[];
};

export type IntegrationIssue = { name: string; problem: string };

function problemOf(status: string, hasError: boolean): string | null {
  if (status === 'disabled') return null;
  if (status === 'auth_failed') return 'Giriş bilgileri reddedildi';
  if (status === 'rate_limited') return 'İstek sınırına takıldı';
  return hasError ? 'Son eşitleme hata verdi' : null;
}

/**
 * Eşitleme hatası olan bağlantılar. Sağlayıcının ham hata metni
 * gösterilmez (İngilizce / teknik olabilir); durumdan Türkçe neden üretilir.
 * Kapatılmış bağlantı sayılmaz.
 */
export function integrationIssues(l: IntegrationLists): IntegrationIssue[] {
  const out: IntegrationIssue[] = [];
  const add = (name: string, p: string | null) => {
    if (p) out.push({ name, problem: p });
  };
  for (const c of l.einvoice ?? []) add(`e-Fatura · ${c.integrator_name}`, problemOf(c.state, !!c.error?.message));
  for (const c of l.bank ?? []) add(`Banka · ${c.bank_name}`, problemOf(c.status, !!c.error?.message));
  for (const c of l.easytrade ?? []) add('EasyTrade', problemOf(c.status, !!c.error));
  for (const c of l.kolaybi ?? []) add(c.company_name ? `KolayBi · ${c.company_name}` : 'KolayBi', problemOf(c.status, !!c.error));
  for (const c of l.parasut ?? []) add(c.company_name ? `Paraşüt · ${c.company_name}` : 'Paraşüt', problemOf(c.status, !!c.error));
  return out;
}

export type MissingItem = { kind: string; name: string; requested: boolean };

export type GapLine = { key: string; title: string; detail: string; tone: 'warning' | 'danger' };

const MISSING_TITLE: Record<string, string> = {
  bank_statement: 'Banka ekstresi eksik',
  z_reports: 'Z raporu gelmedi',
  expenses: 'Masraf fişi gelmedi',
  purchase_invoices: 'Alış faturası gelmedi',
};

const MISSING_ORDER = ['bank_statement', 'z_reports', 'expenses', 'purchase_invoices'];

/**
 * "Eksik / gedik" kartının satırları: eksik banka ekstreleri (ve dönemin
 * gelmeyen diğer belgeleri), Z raporu olmayan günler, eşitleme hatası
 * olan bağlantılar. Kaynağı alınamayan bölüm atlanır.
 */
export function gapLines(input: {
  missing?: { period: string; items: readonly MissingItem[] } | null;
  zGaps?: { period: string; days: readonly number[] } | null;
  integrations?: readonly IntegrationIssue[] | null;
}): GapLine[] {
  const out: GapLine[] = [];
  if (input.missing) {
    const p = periodLabel(input.missing.period) ?? input.missing.period;
    const items = [...input.missing.items].sort((a, b) => MISSING_ORDER.indexOf(a.kind) - MISSING_ORDER.indexOf(b.kind));
    items.forEach((it, i) => {
      const title = MISSING_TITLE[it.kind];
      if (!title) return;
      const who = it.kind === 'bank_statement' ? `${it.name || 'Banka hesabı'} · ` : '';
      out.push({
        key: `missing-${it.kind}-${i}`,
        title,
        detail: `${who}${p}${it.requested ? ' · mükelleften istendi' : ' · henüz istenmedi'}`,
        tone: it.kind === 'bank_statement' ? 'danger' : 'warning',
      });
    });
  }
  if (input.zGaps && input.zGaps.days.length) {
    const p = periodLabel(input.zGaps.period) ?? input.zGaps.period;
    out.push({
      key: 'z-gaps',
      title: `Z raporu olmayan ${input.zGaps.days.length} gün`,
      detail: `${p}: ${dayRanges(input.zGaps.days)}`,
      tone: 'warning',
    });
  }
  (input.integrations ?? []).forEach((x, i) => out.push({ key: `integration-${i}`, title: 'Eşitleme hatası', detail: `${x.name}: ${x.problem}`, tone: 'danger' }));
  return out;
}

/**
 * Eksik belge sorgusunun dönemi: eksik belge kapanmış ay içindir; bu ay
 * seçiliyse bir önceki ay, değilse seçili ay.
 */
export function missingPeriod(selected: string, current: string): string {
  if (selected !== current) return selected;
  const m = /^(\d{4})-(\d{2})$/.exec(current);
  if (!m) return current;
  let y = Number(m[1]);
  let mo = Number(m[2]) - 1;
  if (mo === 0) {
    mo = 12;
    y -= 1;
  }
  return `${y}-${String(mo).padStart(2, '0')}`;
}
