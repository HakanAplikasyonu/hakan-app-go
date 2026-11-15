import type { ChipTone } from '@/components/ui';

import type { QueueItem } from './queue';

export function statusTone(item: QueueItem): ChipTone {
  switch (item.status) {
    case 'uploaded':
      return item.result?.status === 'failed' || item.result?.status === 'rejected'
        ? 'danger'
        : item.result?.status === 'approved'
          ? 'success'
          : 'brand';
    case 'failed':
      return 'danger';
    case 'blocked':
      return 'warning';
    default:
      return 'neutral';
  }
}

/** "15 Kas 14:32" on the Türkiye clock. */
export function shotAt(ms: number): string {
  return new Intl.DateTimeFormat('tr-TR', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Istanbul',
  }).format(new Date(ms));
}
