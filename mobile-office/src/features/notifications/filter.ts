/** Müşavir bildirim listesi süzgeçleri (MusavirMobil · 5): saf modül, node --test ile sınanır. */
import type { InboxItem } from '@hakan/mobile-core/notifications/inbox';

export type InboxFilter = 'all' | 'blocker' | 'upload';

export const inboxFilters: { key: InboxFilter; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  { key: 'blocker', label: 'Engel' },
  { key: 'upload', label: 'Yükleme' },
];

export function filterInbox(items: readonly InboxItem[], f: InboxFilter): InboxItem[] {
  if (f === 'all') return [...items];
  return items.filter((i) => i.kind === f);
}
