/** Belge arşivi ve İSG sekmeleri yardımcıları (#393): saf modül, node --test ile sınanır. */

export type Category = 'general' | 'contract' | 'tax' | 'sgk' | 'trade_registry' | 'isg' | 'other';
export type ISGKind = 'isg_training' | 'risk_assessment' | 'health_report' | 'other';
export type ExpiryStatus = 'none' | 'valid' | 'expiring' | 'expired';

export const categoryLabels: Record<Category, string> = {
  general: 'Genel',
  contract: 'Sözleşme',
  tax: 'Vergi',
  sgk: 'SGK',
  trade_registry: 'Ticaret sicil',
  isg: 'İSG',
  other: 'Diğer',
};

export const isgKindLabels: Record<ISGKind, string> = {
  isg_training: 'İSG eğitimi',
  risk_assessment: 'Risk değerlendirmesi',
  health_report: 'Sağlık raporu',
  other: 'Diğer İSG evrakı',
};

/** Arşiv sekmesinin kategori süzgeci (İSG kendi sekmesinde de var). */
export const archiveFilters: readonly { key: Category | 'all'; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  ...(Object.keys(categoryLabels) as Category[]).map((k) => ({ key: k, label: categoryLabels[k] })),
];

export const isgFilters: readonly { key: ISGKind | 'all'; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  { key: 'isg_training', label: 'Eğitim' },
  { key: 'risk_assessment', label: 'Risk' },
  { key: 'health_report', label: 'Sağlık' },
];

function trDate(iso: string): string {
  const [y, m, d] = iso.split('-');
  return `${d}.${m}.${y}`;
}

/** Geçerlilik çipi: süresi doldu / N gün kaldı / geçerli; bitiş tarihi yoksa null. */
export function expiryChip(d: { expiry_status: ExpiryStatus; days_left: number | null; valid_until: string | null }): {
  label: string;
  tone: 'danger' | 'warning' | 'success';
} | null {
  if (!d.valid_until || d.expiry_status === 'none') return null;
  const days = d.days_left ?? 0;
  switch (d.expiry_status) {
    case 'expired':
      return { label: days === -1 ? 'Dün doldu' : `Süresi doldu (${Math.abs(days)} gün)`, tone: 'danger' };
    case 'expiring':
      return { label: days === 0 ? 'Bugün bitiyor' : `${days} gün kaldı`, tone: 'warning' };
    default:
      return { label: `${trDate(d.valid_until)} tarihine kadar`, tone: 'success' };
  }
}

/** Satırın alt satırı: tür · belge tarihi · geçerlilik bitişi. */
export function documentSubtitle(d: {
  category: Category;
  isg_kind: ISGKind | null;
  document_date: string | null;
  valid_until: string | null;
}): string {
  const parts = [d.isg_kind ? isgKindLabels[d.isg_kind] : categoryLabels[d.category]];
  if (d.document_date) parts.push(trDate(d.document_date));
  if (d.valid_until) parts.push(`bitiş ${trDate(d.valid_until)}`);
  return parts.join(' · ');
}

/** Yaklaşan bitiş uyarısı: "1 İSG evrakının süresi doldu, 2 evrak 30 gün içinde bitiyor." */
export function expiryWarning(expired: number, expiring: number, days: number, noun = 'İSG evrakı'): string | null {
  const parts: string[] = [];
  if (expired > 0) parts.push(`${expired} ${noun}nın süresi doldu`);
  if (expiring > 0) parts.push(`${expiring} ${noun} ${days} gün içinde bitiyor`);
  if (parts.length === 0) return null;
  const s = parts.join(', ');
  return `${s[0].toLocaleUpperCase('tr')}${s.slice(1)}.`;
}
