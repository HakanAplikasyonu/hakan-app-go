import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type Suggestion = Schemas['Suggestion'];
export type SuggestionContext = Schemas['SuggestionContext'];
type Page = { items: Suggestion[]; total: number; limit: number; offset: number };

const PAGE = 20;

/** Ofis inceleme kuyruğu (`GET /v1/tenant/review/suggestions`, #14): bekleyenler, en düşük güven önce. */
export function useReviewQueue() {
  const org = useOrganization();
  return useInfiniteQuery({
    queryKey: ['review', org],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.get<Page>('/v1/tenant/review/suggestions', { organization: org, query: { limit: PAGE, offset: pageParam } }),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
  });
}

export function useSuggestion(engagement: string, uuid: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['suggestion', org, uuid],
    queryFn: () => api.get<Suggestion>(`/v1/tenant/engagements/${engagement}/suggestions/${uuid}`, { organization: org }),
  });
}

export function useSuggestionContext(engagement: string, uuid: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['suggestion-context', org, uuid],
    queryFn: () => api.get<SuggestionContext>(`/v1/tenant/engagements/${engagement}/suggestions/${uuid}/context`, { organization: org }),
  });
}

export type Decision =
  | { action: 'accept'; reason?: string }
  | { action: 'reject'; reason: string }
  | { action: 'correct'; reason: string; accountCode: string };

/** Tek öneri kararı. Karar backend'de kayda geçer, ofis panelinde de görünür. */
export function decide(org: string, s: Pick<Suggestion, 'uuid' | 'engagement_uuid'>, d: Decision) {
  const body =
    d.action === 'correct' ? { reason: d.reason, value: { account_code: d.accountCode.trim() } } : { reason: d.reason ?? '' };
  return api.post<unknown>(`/v1/tenant/engagements/${s.engagement_uuid}/suggestions/${s.uuid}/${d.action}`, body, { organization: org });
}

export function useDecide() {
  const org = useOrganization();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ suggestion, decision }: { suggestion: Pick<Suggestion, 'uuid' | 'engagement_uuid'>; decision: Decision }) =>
      decide(org, suggestion, decision),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['review', org] });
      void qc.invalidateQueries({ queryKey: ['dashboard', org] });
    },
  });
}

export function useInvalidateReview() {
  const org = useOrganization();
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ['review', org] });
    void qc.invalidateQueries({ queryKey: ['dashboard', org] });
  };
}
