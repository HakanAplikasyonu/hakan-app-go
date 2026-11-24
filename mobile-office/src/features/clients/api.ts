import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { env } from '@/core/config/env';
import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { session } from '@/core/auth/session';
import { useOrganization } from '@/core/auth/auth-provider';

import type { ClientFilter } from './format';

export type OfficeClient = Schemas['OfficeClient'];
export type ClientPage = Schemas['OfficeClientPage'];
export type Contacts = Schemas['EngagementContacts'];
export type PeriodReadiness = Schemas['PeriodReadiness'];
export type OfficeInvoice = Schemas['OfficeInvoice'];
export type ReceiptCapture = Schemas['ReceiptCapture'];
export type DocumentTask = Schemas['DocumentTask'];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

const PAGE = 30;

/** Mükellef listesi (`GET /v1/tenant/clients`), imleçli sayfalama (#182). */
export function useClients(search: string | undefined, filter: ClientFilter) {
  const org = useOrganization();
  return useInfiniteQuery({
    queryKey: ['clients', org, search ?? '', filter],
    initialPageParam: '' as string,
    queryFn: ({ pageParam }) =>
      api.get<ClientPage>('/v1/tenant/clients', {
        organization: org,
        query: { q: search, filter: filter === 'all' ? undefined : filter, cursor: pageParam || undefined, limit: PAGE },
      }),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 60_000,
  });
}

function useEngagementGet<T>(engagement: string, key: string, path: string, query?: Record<string, string | number>, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, key, query ?? {}],
    queryFn: () => api.get<T>(`/v1/tenant/engagements/${engagement}${path}`, { organization: org, query }),
    enabled,
  });
}

export const useContacts = (engagement: string) => useEngagementGet<Contacts>(engagement, 'contacts', '/contacts');

export const usePeriod = (engagement: string, period: string, enabled: boolean) =>
  useEngagementGet<PeriodReadiness>(engagement, `period-${period}`, `/periods/${period}`, undefined, enabled);

export const useRecentInvoices = (engagement: string, enabled: boolean) =>
  useEngagementGet<Page<OfficeInvoice>>(engagement, 'invoices', '/invoices', { limit: 5 }, enabled);

export const useRecentReceipts = (engagement: string, enabled: boolean) =>
  useEngagementGet<Page<ReceiptCapture>>(engagement, 'receipts', '/receipts', { limit: 5 }, enabled);

export const useOpenTasks = (engagement: string, enabled: boolean) =>
  useEngagementGet<Page<DocumentTask>>(engagement, 'tasks', '/document-tasks', { status: 'open', limit: 5 }, enabled);

/** expo-image kaynağı: fiş görseli, oturum tokenıyla. */
export function receiptImageSource(org: string, engagement: string, receipt: string) {
  const token = session.get()?.accessToken;
  return {
    uri: `${env.apiUrl}/v1/tenant/engagements/${engagement}/receipts/${receipt}/image`,
    headers: { 'X-Organization': org, ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    cacheKey: `receipt-${receipt}`,
  };
}

/**
 * Engagement → mükellef unvanı (tablet onay listesinin grup başlıkları).
 * Sunucunun izin verdiği en büyük sayfa (100); bulunamayan grup "Mükellef" olur.
 */
export function useClientTitles() {
  const org = useOrganization();
  const q = useQuery({
    queryKey: ['clients', org, 'titles'],
    queryFn: () => api.get<ClientPage>('/v1/tenant/clients', { organization: org, query: { limit: 100 } }),
    staleTime: 5 * 60_000,
  });
  return q.data?.items;
}

/**
 * Mükellefin unvanı (onay ekranı, Oğuz'un canlı geri bildirimi): önce
 * mükellef listesinden (önbellekte), bulunamazsa mükellefin iletişim
 * ucundan (`GET .../contacts`, `title`).
 */
export function useTaxpayerName(engagement: string | null | undefined): string | null {
  const org = useOrganization();
  const titles = useClientTitles();
  const fromList = engagement ? titles?.find((x) => x.engagement_uuid === engagement)?.title : undefined;
  const q = useQuery({
    queryKey: ['engagement', org, engagement ?? '', 'contacts', {}],
    queryFn: () => api.get<Contacts>(`/v1/tenant/engagements/${engagement}/contacts`, { organization: org }),
    enabled: !!engagement && !!titles && !fromList,
    staleTime: 5 * 60_000,
  });
  return fromList ?? q.data?.title ?? null;
}
