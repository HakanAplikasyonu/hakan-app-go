/** Mükellef belgeleri ekranının sekmeleri (#393): saf modül, node --test ile sınanır. */

export type DocumentTab = 'payroll' | 'mizan';

export const documentTabs: readonly { key: DocumentTab; label: string; permission: string }[] = [
  { key: 'payroll', label: 'Bordro', permission: 'tenant.payroll.read' },
  { key: 'mizan', label: 'Mizan', permission: 'tenant.account_plan.read' },
];

/** Kullanıcının izinli sekmeleri; istenen sekme izinliyse o, değilse ilki seçilir. */
export function visibleTabs(can: (permission: string) => boolean, requested?: string) {
  const tabs = documentTabs.filter((t) => can(t.permission));
  const selected = tabs.find((t) => t.key === requested)?.key ?? tabs[0]?.key;
  return { tabs, selected };
}
