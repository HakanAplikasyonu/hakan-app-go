/** Uyarı biçimlendirme: saf modül, node --test ile sınanır. */
import { warningText } from './catalog.ts';

export type Severity = 'blocker' | 'warning' | 'info';
export type WarningStatus = 'open' | 'acknowledged' | 'resolved' | 'auto_resolved';

export type DataWarning = {
  uuid: string;
  engagement_uuid: string;
  period: string | null;
  entity_type: string;
  entity_uuid: string | null;
  code: string;
  severity: Severity;
  params: Record<string, unknown>;
  producer: string;
  status: WarningStatus;
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
};

export const severityLabel: Record<Severity, string> = { blocker: 'Engelleyici', warning: 'Uyarı', info: 'Bilgi' };

export const statusLabel: Record<WarningStatus, string> = {
  open: 'Açık',
  acknowledged: 'Görüldü',
  resolved: 'Çözüldü',
  auto_resolved: 'Kendiliğinden kapandı',
};

export const severityTone = (s: Severity) => (s === 'blocker' ? 'danger' : s === 'warning' ? 'warning' : 'brand');

function paramText(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  // Ham JSON gösterilmez (#375): liste değerleri virgülle, nesneler boş.
  if (Array.isArray(v)) return v.map(paramText).filter(Boolean).join(', ');
  return '';
}

/** Kodun Türkçe metni, `{{ad}}` yer tutucuları params ile doldurulur. */
export function warningMessage(code: string, params: Record<string, unknown> = {}): string {
  const template = warningText[code];
  if (!template) return `Kontrol uyarısı (${code})`;
  return template.replace(/\{\{\s*(\w+)\s*\}\}/g, (_, key: string) => {
    const v = paramText(params[key]);
    return v || '—';
  });
}

/** "2026-10" → "Ekim 2026". */
export function periodLabel(period: string | null): string | null {
  if (!period) return null;
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  const months = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];
  const i = Number(m[2]) - 1;
  return i >= 0 && i < 12 ? `${months[i]} ${m[1]}` : period;
}

/** Yalnızca açık uyarı için karar verilebilir (backend kuralı). */
export const canDecide = (w: Pick<DataWarning, 'status'>) => w.status === 'open';

/** Liste sırası backend'deki gibi: engelleyici, uyarı, bilgi; sonra en yeni. */
export function sortWarnings(list: readonly DataWarning[]): DataWarning[] {
  const rank: Record<Severity, number> = { blocker: 0, warning: 1, info: 2 };
  return [...list].sort((a, b) => rank[a.severity] - rank[b.severity] || b.updated_at.localeCompare(a.updated_at));
}

/** Kararın listeye yansıması (iyimser güncelleme). */
export function applyDecision(list: readonly DataWarning[], uuid: string, status: 'acknowledged' | 'resolved', now = new Date().toISOString()): DataWarning[] {
  return list.map((w) => (w.uuid === uuid ? { ...w, status, resolved_at: status === 'resolved' ? now : w.resolved_at, updated_at: now } : w));
}
