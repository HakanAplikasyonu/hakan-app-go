/** Yabancı faturalar / KDV2 bölümü yardımcıları (#780): saf modül, node --test ile sınanır. */

type Invoice = {
  amount: string;
  currency: string;
  treatment: 'kdv2' | 'kdv2_stopaj';
  kdv2_try: string | null;
  stopaj_try: string | null;
  status: 'draft' | 'confirmed';
  missing_rate: boolean;
  exchange_rate: string | null;
  exchange_rate_source: 'tcmb' | 'manual' | null;
  exchange_rate_date: string | null;
  source_kind: 'taxpayer_upload' | 'whatsapp' | 'email' | null;
};

type Totals = { count: number; confirmed: number; draft: number; missing_rate: number };

/** "1234.5" → "1.234,50 ₺"; TRY dışı para birimi kodla ("1.000,00 USD"). */
export function amountLabel(amount: string | null, currency = 'TRY'): string {
  if (amount === null) return '—';
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  const s = n.toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return currency === 'TRY' ? `${s} ₺` : `${s} ${currency}`;
}

/** İşlem türü: yalnız KDV2 ya da KDV2 ve stopaj. */
export function treatmentLabel(t: Invoice['treatment']): string {
  return t === 'kdv2_stopaj' ? 'KDV2 + stopaj' : 'Yalnız KDV2';
}

export function statusLabel(s: Invoice['status']): string {
  return s === 'confirmed' ? 'Onaylandı' : 'Taslak';
}

/** Mükellef panelinden ya da WhatsApp'tan gelen satır: "Mükelleften geldi". */
export function sourceLabel(s: Invoice['source_kind']): string | null {
  if (s === 'taxpayer_upload' || s === 'whatsapp') return 'Mükelleften geldi';
  if (s === 'email') return 'E-postayla geldi';
  return null;
}

/** Kur etiketi: "TCMB kuru 38,25 · 20.11.2026", "Elle girilen kur 38,25" ya da "Kur eksik". */
export function rateLabel(
  i: Pick<Invoice, 'currency' | 'exchange_rate' | 'exchange_rate_source' | 'exchange_rate_date' | 'missing_rate'>,
): string | null {
  if (i.currency === 'TRY') return null;
  if (i.missing_rate || i.exchange_rate === null) return 'Kur eksik';
  const rate = Number(i.exchange_rate).toLocaleString('tr-TR', { maximumFractionDigits: 6 });
  if (i.exchange_rate_source === 'tcmb') {
    const d = i.exchange_rate_date ? /^(\d{4})-(\d{2})-(\d{2})$/.exec(i.exchange_rate_date) : null;
    return `TCMB kuru ${rate}${d ? ` · ${d[3]}.${d[2]}.${d[1]}` : ''}`;
  }
  return `Elle girilen kur ${rate}`;
}

/** Satırın alt yazısı: tutar, işlem türü, KDV2 ve (varsa) stopaj, TL. */
export function rowSubtitle(i: Invoice): string {
  const parts = [amountLabel(i.amount, i.currency), treatmentLabel(i.treatment), `KDV2 ${amountLabel(i.kdv2_try)}`];
  if (i.treatment === 'kdv2_stopaj') parts.push(`Stopaj ${amountLabel(i.stopaj_try)}`);
  return parts.join(' · ');
}

/** Dönem özeti: fatura sayısı, onay durumu ve kur eksikleri. */
export function totalsSummary(t: Totals): string {
  if (t.count === 0) return 'Bu dönemde yabancı fatura yok.';
  const parts = [`${t.count} fatura`, `${t.confirmed} onaylı`];
  if (t.draft > 0) parts.push(`${t.draft} taslak`);
  if (t.missing_rate > 0) parts.push(`${t.missing_rate} faturada kur eksik (toplama girmedi)`);
  return parts.join(' · ');
}

/**
 * Elle girilen kur: "38,25" ya da "38.25" → "38.25". Binlik ayırıcı kabul
 * edilmez; geçersiz ya da sıfırsa null.
 */
export function parseRate(input: string): string | null {
  const s = input.trim().replace(',', '.');
  if (!/^\d{1,6}(\.\d{1,6})?$/.test(s)) return null;
  return Number(s) > 0 ? s : null;
}
