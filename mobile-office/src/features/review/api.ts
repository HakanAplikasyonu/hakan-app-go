import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, request } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

import { otherPendingForTarget } from './format';

export type Suggestion = Schemas['Suggestion'];
export type SuggestionContext = Schemas['SuggestionContext'];
export type ReceiptExpense = Schemas['Expense'];
type ReceiptCapture = Schemas['ReceiptCapture'];
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

export function useSuggestionContext(engagement: string, uuid: string, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['suggestion-context', org, uuid],
    queryFn: () => api.get<SuggestionContext>(`/v1/tenant/engagements/${engagement}/suggestions/${uuid}/context`, { organization: org }),
    enabled: enabled && !!engagement && !!uuid,
  });
}

/**
 * Fişin masraf taslağı (onay ekranı, Oğuz'un canlı geri bildirimi): fiş
 * yüklemesi (`GET .../receipts/{uuid}`) taslağın uuid'sini verir, masraf
 * (`GET .../expenses/{uuid}`) önerilen hesap kodunu. Z raporu fişinde
 * hesap kodu yok: null.
 */
export function useReceiptExpense(engagement: string | null | undefined, receipt: string | null | undefined) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement ?? '', 'receipt-expense', receipt ?? ''],
    queryFn: async () => {
      const base = `/v1/tenant/engagements/${encodeURIComponent(engagement ?? '')}`;
      const cap = await api.get<ReceiptCapture>(`${base}/receipts/${encodeURIComponent(receipt ?? '')}`, { organization: org });
      if (!cap.draft || cap.kind === 'z_report') return null;
      return api.get<ReceiptExpense>(`${base}/expenses/${encodeURIComponent(cap.draft.uuid)}`, { organization: org });
    },
    enabled: !!engagement && !!receipt,
    staleTime: 30_000,
  });
}

export type ReceiptFinish = 'approved' | 'waiting' | 'skipped';

/**
 * Fiş kartında Onayla'nın ikinci adımı: seçilen hesap kodu masrafa yazılır
 * (`PATCH .../expenses/{uuid}`, değiştiyse), fişin bekleyen başka alanı
 * kalmadıysa masraf onaylanır (`POST .../expenses/{uuid}/approve`, yevmiye
 * taslağı). Bekleyen alan varsa onay son alanda yapılır: onaylanmış
 * taslağa alan yazılamaz.
 */
export async function finishReceiptExpense(
  org: string,
  s: Pick<Suggestion, 'uuid' | 'engagement_uuid' | 'target_type' | 'target_id'>,
  expense: Pick<ReceiptExpense, 'uuid' | 'account_code' | 'approval_status'>,
  chosenCode: string | null,
): Promise<ReceiptFinish> {
  if (expense.approval_status !== 'pending') return 'skipped';
  const base = `/v1/tenant/engagements/${encodeURIComponent(s.engagement_uuid ?? '')}`;
  const code = chosenCode?.trim();
  if (code && code !== expense.account_code) {
    await request<ReceiptExpense>('PATCH', `${base}/expenses/${encodeURIComponent(expense.uuid)}`, { organization: org, body: { account_code: code } });
  }
  const page = await api.get<{ items: Suggestion[] }>(`${base}/suggestions`, { organization: org, query: { status: 'pending', limit: 100 } });
  if (otherPendingForTarget(page.items, s).length) return 'waiting';
  await api.post<ReceiptExpense>(`${base}/expenses/${encodeURIComponent(expense.uuid)}/approve`, undefined, { organization: org });
  return 'approved';
}

/** Fiş onayından sonra mükellefin dönem / masraf önbelleğini tazeler. */
export function useInvalidateEngagement() {
  const org = useOrganization();
  const qc = useQueryClient();
  return (engagement: string | null | undefined) => {
    if (engagement) void qc.invalidateQueries({ queryKey: ['engagement', org, engagement] });
  };
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
