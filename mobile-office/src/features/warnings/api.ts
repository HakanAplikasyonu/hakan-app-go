import { useInfiniteQuery, useMutation, useQueryClient, type InfiniteData } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useOrganization } from '@/core/auth/auth-provider';

import { applyDecision, type DataWarning, type WarningStatus } from './format';

export type WarningPage = { items: DataWarning[]; total: number; limit: number; offset: number };

const PAGE = 20;

export const warningsKey = (org: string, status: WarningStatus | 'all', engagement?: string) => ['warnings', org, status, engagement ?? ''] as const;

/**
 * Uyarılar, sayfalı: ofis çapı (`GET /v1/tenant/warnings`) ya da bir
 * mükellefin (`.../engagements/{uuid}/warnings`, kritik uyarı bildirimi #241).
 */
export function useWarnings(status: WarningStatus | 'all', engagement?: string) {
  const org = useOrganization();
  const path = engagement ? `/v1/tenant/engagements/${engagement}/warnings` : '/v1/tenant/warnings';
  return useInfiniteQuery({
    queryKey: warningsKey(org, status, engagement),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<WarningPage>(path, {
        organization: org,
        query: { status: status === 'all' ? undefined : status, limit: PAGE, offset: pageParam },
      }),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
  });
}

/** Önbellekteki listelerden bir uyarıyı bulur (ayrıntı ekranı için; tekil uç yok). */
export function useCachedWarning(uuid: string): DataWarning | undefined {
  const qc = useQueryClient();
  for (const [, data] of qc.getQueriesData<InfiniteData<WarningPage>>({ queryKey: ['warnings'] })) {
    for (const page of data?.pages ?? []) {
      const hit = page.items.find((w) => w.uuid === uuid);
      if (hit) return hit;
    }
  }
  return undefined;
}

/** Gördüm / Çözüldü (`POST .../warnings/{uuid}/acknowledge|resolve`). */
export function useDecideWarning() {
  const org = useOrganization();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ warning, decision }: { warning: DataWarning; decision: 'acknowledge' | 'resolve' }) =>
      api.post<{ uuid: string; status: 'acknowledged' | 'resolved' }>(
        `/v1/tenant/engagements/${warning.engagement_uuid}/warnings/${warning.uuid}/${decision}`,
        undefined,
        { organization: org },
      ),
    onSuccess: (res) => {
      qc.setQueriesData<InfiniteData<WarningPage>>({ queryKey: ['warnings', org] }, (data) =>
        data ? { ...data, pages: data.pages.map((p) => ({ ...p, items: applyDecision(p.items, res.uuid, res.status) })) } : data,
      );
      void qc.invalidateQueries({ queryKey: ['warnings', org] });
      void qc.invalidateQueries({ queryKey: ['dashboard', org] });
    },
  });
}
