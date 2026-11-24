/**
 * Fiş önizlemesi (saf; node --test ile denenir). Oğuz'un geri bildirimi:
 * fotoğraf çekilince okunan satıcı, tarih, toplam ve KDV oranı satırları
 * (%1 / %10 / %20 için ayrı ayrı matrah ve KDV) gösterilir; mükellef her
 * alanı düzeltip "Gönder"e basar. Düzeltmeler mevcut düzeltme ucuna gider
 * (`POST /v1/taxpayer/receipts/{uuid}/corrections`) ve ofisin inceleme
 * kuyruğunda "Mükellef düzeltmesi" olarak görünür.
 *
 * Sunucu alanları (backend/internal/modules/receipts/extract):
 * `vat_<oran>_total` KDV dahil tutar, `vat_<oran>_tax` KDV. Matrah = tutar − KDV.
 */
import type { Correction, ReceiptField } from './queue';

/** Rates the server reads (extract.ZRates). */
export const VAT_RATES = ['0', '1', '8', '10', '18', '20'] as const;
/** Rates offered when the taxpayer adds a row. */
export const QUICK_RATES = ['1', '10', '20'] as const;

/** One VAT rate row as shown (amounts in Turkish format, e.g. "1.234,56"). */
export type VatRow = {
  rate: string;
  base: string;
  tax: string;
  /** The receipt's VAT for this rate was not read: base and VAT were computed from the total. */
  estimated?: boolean;
};

export type Preview = { vendor: string; date: string; total: string; rows: VatRow[] };

/** Amounts closer than this (TL) are equal: rounding on the receipt. */
export const TOLERANCE = 0.05;

/** Reads "1.234,56", "1234.56", "1,234.56", "₺12,00" (like the server's parseAmount). */
export function parseAmount(input: string): number | null {
  let v = input.replace(/[*₺\s]|TL/g, '');
  if (!v) return null;
  const c = v.lastIndexOf(',');
  const d = v.lastIndexOf('.');
  if (c > d) v = v.replace(/\./g, '').replace(',', '.');
  else if (d > c && c >= 0) v = v.replace(/,/g, '');
  else if (d >= 0 && v.split('.').length > 2) v = v.replace(/\./g, '');
  if (!/^-?\d+(\.\d+)?$/.test(v)) return null;
  const n = Math.abs(Number(v));
  return Number.isFinite(n) ? n : null;
}

const round2 = (n: number) => Math.round(n * 100) / 100;

/** 1234.5 → "1.234,50". */
export function formatAmount(n: number): string {
  const [int, frac] = round2(n).toFixed(2).split('.');
  return `${int.replace(/\B(?=(\d{3})+(?!\d))/g, '.')},${frac}`;
}

/** The API's amount text: 1234.5 → "1234.50". */
export function apiAmount(n: number): string {
  return round2(n).toFixed(2);
}

/** "2026-10-14" → "14.10.2026"; anything else unchanged. */
export function isoToTr(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : iso;
}

/** "14.10.2026" (or 14/10/2026, 1.2.2026) → "2026-10-14"; null if not a real date. */
export function trToIso(text: string): string | null {
  const m = /^(\d{1,2})[./-](\d{1,2})[./-](\d{4})$/.exec(text.trim());
  if (!m) return null;
  const [day, month, year] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const d = new Date(Date.UTC(year, month - 1, day));
  if (d.getUTCFullYear() !== year || d.getUTCMonth() !== month - 1 || d.getUTCDate() !== day) return null;
  return `${year}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

type Fields = Record<string, ReceiptField | undefined>;

function text(f: ReceiptField | undefined): string {
  if (!f) return '';
  return (f.valid ? f.value : f.raw) || f.raw || '';
}

function amount(f: ReceiptField | undefined): number | null {
  if (!f) return null;
  return parseAmount(f.valid ? f.value : f.raw);
}

/** The VAT inside a total incl. VAT at this rate. */
export function taxInTotal(total: number, rate: string): number {
  const r = Number(rate);
  return r > 0 ? round2((total * r) / (100 + r)) : 0;
}

/** Rows read from the receipt (rates with a positive total). */
export function vatRowsFrom(fields: Fields): VatRow[] {
  const rows: VatRow[] = [];
  for (const rate of VAT_RATES) {
    const total = amount(fields[`vat_${rate}_total`]);
    if (total == null || total <= 0) continue;
    const tax = amount(fields[`vat_${rate}_tax`]);
    const t = tax ?? taxInTotal(total, rate);
    rows.push({ rate, base: formatAmount(total - t), tax: formatAmount(t), ...(tax == null ? { estimated: true } : null) });
  }
  if (rows.length) return rows;
  // Only one VAT read (tax_amount + tax_rate): one row of the whole total.
  const rate = text(fields.tax_rate).replace(/\.0+$/, '');
  const tax = amount(fields.tax_amount);
  const total = amount(fields.total_amount);
  if ((VAT_RATES as readonly string[]).includes(rate) && tax != null && total != null && total >= tax) {
    rows.push({ rate, base: formatAmount(total - tax), tax: formatAmount(tax) });
  }
  return rows;
}

/** What the preview shows first: the server's read. */
export function previewFrom(fields: Fields): Preview {
  const total = amount(fields.total_amount);
  const date = fields.date;
  return {
    vendor: text(fields.vendor_name),
    date: date?.valid ? isoToTr(date.value) : (date?.raw ?? ''),
    total: total == null ? text(fields.total_amount) : formatAmount(total),
    rows: vatRowsFrom(fields),
  };
}

/** Sum of the rows (matrah, KDV, KDV dahil); null if a row's amount is not a number. */
export function rowsSum(rows: readonly VatRow[]): { base: number; tax: number; total: number } | null {
  let base = 0;
  let tax = 0;
  for (const r of rows) {
    const b = parseAmount(r.base);
    const t = parseAmount(r.tax);
    if (b == null || t == null) return null;
    base += b;
    tax += t;
  }
  return { base: round2(base), tax: round2(tax), total: round2(base + tax) };
}

/** Edits a row's field. */
export function editRow(rows: readonly VatRow[], rate: string, patch: Partial<Pick<VatRow, 'base' | 'tax'>>): VatRow[] {
  return rows.map((r) => (r.rate === rate ? { rate, base: patch.base ?? r.base, tax: patch.tax ?? r.tax } : r));
}

/** Adds an empty row for a rate not yet in the list (rows stay ordered by rate). */
export function addRow(rows: readonly VatRow[], rate: string): VatRow[] {
  if (rows.some((r) => r.rate === rate)) return [...rows];
  return [...rows, { rate, base: '', tax: '' }].sort((a, b) => Number(a.rate) - Number(b.rate));
}

export function removeRow(rows: readonly VatRow[], rate: string): VatRow[] {
  return rows.filter((r) => r.rate !== rate);
}

/** Rates the taxpayer can still add. */
export function addableRates(rows: readonly VatRow[]): string[] {
  return QUICK_RATES.filter((r) => !rows.some((x) => x.rate === r));
}

/** Fills a row's VAT from its matrah at its rate (the "hesapla" help). */
export function fillTax(rows: readonly VatRow[], rate: string): VatRow[] {
  return rows.map((r) => {
    const b = r.rate === rate ? parseAmount(r.base) : null;
    return b == null ? r : { rate: r.rate, base: r.base, tax: formatAmount(round2((b * Number(rate)) / 100)) };
  });
}

export type PreviewCheck = {
  /** Field → Turkish message; sending waits until there is none. */
  errors: Record<string, string>;
  /** Rate → note (VAT does not match the rate); does not stop sending. */
  notes: Record<string, string>;
  sum: { base: number; tax: number; total: number } | null;
};

/** Checks the preview before "Gönder". */
export function checkPreview(p: Preview): PreviewCheck {
  const errors: Record<string, string> = {};
  const notes: Record<string, string> = {};
  if (!p.vendor.trim()) errors.vendor = 'Satıcının adını yazın.';
  if (!trToIso(p.date)) errors.date = 'Tarihi GG.AA.YYYY biçiminde yazın (ör. 14.10.2026).';
  const total = parseAmount(p.total);
  if (total == null || total <= 0) errors.total = 'Fişin toplam tutarını yazın.';
  for (const r of p.rows) {
    const b = parseAmount(r.base);
    const t = parseAmount(r.tax);
    if (b == null || t == null) {
      errors[`row_${r.rate}`] = `%${r.rate} satırında matrah ve KDV tutar olmalı.`;
      continue;
    }
    const expected = (b * Number(r.rate)) / 100;
    if (Math.abs(expected - t) > Math.max(TOLERANCE, expected * 0.02)) {
      notes[r.rate] = `%${r.rate} KDV, matrahın %${r.rate}'i olmalı (${formatAmount(expected)}). Fişle karşılaştırın.`;
    }
  }
  const sum = rowsSum(p.rows);
  if (p.rows.length && sum && total != null && Math.abs(sum.total - total) > TOLERANCE) {
    errors.sum = `KDV satırlarının toplamı ${formatAmount(sum.total)} TL, fişin toplamı ${formatAmount(total)} TL. Satırları ya da toplamı düzeltin.`;
  }
  return { errors, notes, sum };
}

export function canSend(p: Preview): boolean {
  return Object.keys(checkPreview(p).errors).length === 0;
}

function sameAmount(a: string, b: string): boolean {
  const x = parseAmount(a);
  const y = parseAmount(b);
  return x != null && y != null ? Math.abs(x - y) < 0.005 : a.trim() === b.trim();
}

/**
 * The corrections to send: every field the taxpayer changed. A changed
 * (or computed) row goes as its total incl. VAT and its VAT; a removed row
 * as total 0; the receipt's VAT (tax_amount) follows the rows' sum.
 */
export function previewCorrections(fields: Fields, original: Preview, edited: Preview): Correction[] {
  const out: Correction[] = [];
  if (edited.vendor.trim() !== original.vendor.trim()) out.push({ field: 'vendor_name', value: edited.vendor.trim() });
  if (edited.date.trim() !== original.date.trim()) {
    const iso = trToIso(edited.date);
    if (iso) out.push({ field: 'date', value: iso });
  }
  const total = parseAmount(edited.total);
  if (total != null && !sameAmount(edited.total, original.total)) out.push({ field: 'total_amount', value: apiAmount(total) });

  let rowsChanged = false;
  for (const row of edited.rows) {
    const before = original.rows.find((r) => r.rate === row.rate);
    const b = parseAmount(row.base);
    const t = parseAmount(row.tax);
    if (b == null || t == null) continue;
    if (before && !before.estimated && sameAmount(before.base, row.base) && sameAmount(before.tax, row.tax)) continue;
    rowsChanged = true;
    out.push({ field: `vat_${row.rate}_total`, value: apiAmount(b + t) }, { field: `vat_${row.rate}_tax`, value: apiAmount(t) });
  }
  for (const before of original.rows) {
    if (!edited.rows.some((r) => r.rate === before.rate)) {
      rowsChanged = true;
      out.push({ field: `vat_${before.rate}_total`, value: '0' });
    }
  }
  const sum = rowsSum(edited.rows);
  if (rowsChanged && sum && edited.rows.length) {
    const read = amount(fields.tax_amount);
    if (read == null || Math.abs(read - sum.tax) >= 0.005) out.push({ field: 'tax_amount', value: apiAmount(sum.tax) });
  }
  return out;
}
