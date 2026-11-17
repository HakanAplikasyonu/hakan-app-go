/** Bordro sekmesi yardımcıları (#393): saf modül, node --test ile sınanır. */

export type PayrollKind = 'payroll' | 'payslip' | 'sgk_accrual' | 'sgk_receipt' | 'other';

export const kindLabels: Record<PayrollKind, string> = {
  payroll: 'Bordro dökümü',
  payslip: 'Bordro pusulası',
  sgk_accrual: 'SGK tahakkuk fişi',
  sgk_receipt: 'SGK ödeme dekontu',
  other: 'Belge',
};

type Doc = { uuid: string; period: string; kind: PayrollKind; title: string; employee_uuid: string | null; employee_name: string | null };

export type EmployeeGroup<T extends Doc> = { key: string; title: string; items: T[] };

/**
 * Personel bazında gruplar: önce şirket geneli belgeler (bordro dökümü, SGK),
 * sonra personeller ada göre (tr-TR). Grup içinde yeni dönem önce.
 */
export function groupByEmployee<T extends Doc>(docs: readonly T[]): EmployeeGroup<T>[] {
  const general: T[] = [];
  const byEmployee = new Map<string, EmployeeGroup<T>>();
  for (const d of docs) {
    if (!d.employee_uuid) {
      general.push(d);
      continue;
    }
    const g = byEmployee.get(d.employee_uuid) ?? { key: d.employee_uuid, title: d.employee_name || 'Personel', items: [] };
    g.items.push(d);
    byEmployee.set(d.employee_uuid, g);
  }
  const newest = (a: T, b: T) => b.period.localeCompare(a.period) || a.title.localeCompare(b.title, 'tr');
  const groups = [...byEmployee.values()].sort((a, b) => a.title.localeCompare(b.title, 'tr'));
  for (const g of groups) g.items.sort(newest);
  general.sort(newest);
  return [...(general.length ? [{ key: 'general', title: 'Şirket geneli', items: general }] : []), ...groups];
}

/** Bordro dönemlerinden seçici seçenekleri, yeni dönem önce, tekil. */
export function periodsOf(runs: readonly { period: string }[], docs: readonly { period: string }[]): string[] {
  return [...new Set([...runs, ...docs].map((r) => r.period))].sort((a, b) => b.localeCompare(a));
}

export const paymentLabels: Record<'unpaid' | 'partial' | 'paid', { label: string; tone: 'danger' | 'warning' | 'success' }> = {
  unpaid: { label: 'Net maaş ödenmedi', tone: 'danger' },
  partial: { label: 'Kısmen ödendi', tone: 'warning' },
  paid: { label: 'Net maaş ödendi', tone: 'success' },
};
