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
 * Fişin masraf taslağı ve görseli (onay ekranı, Oğuz'un canlı geri
 * bildirimi). Bağlam ucu fiş alanı önerisinde masrafın uuid'sini verir
 * (`context.expense`); yoksa fiş yüklemesinin (`GET .../receipts/{uuid}`)
 * taslağından bulunur. Masraf (`GET .../expenses/{uuid}`) önerilen hesap
 * kodunu taşır. Görsel: bağlamdaki fiş, yoksa masrafa bağlı yükleme. Z
 * raporu fişinde hesap kodu yok: masraf null.
 */
export function useReceiptExpense(engagement: string | null | undefined, ctx: SuggestionContext | null | undefined) {
  const org = useOrganization();
  const base = `/v1/tenant/engagements/${encodeURIComponent(engagement ?? '')}`;
  const expenseUuid = ctx?.expense?.uuid ?? null;
  const receiptUuid = ctx?.receipt?.uuid ?? null;
  const expense = useQuery({
    queryKey: ['engagement', org, engagement ?? '', 'receipt-expense', expenseUuid ?? '', receiptUuid ?? ''],
    queryFn: async () => {
      let id = expenseUuid;
      if (!id && receiptUuid) {
        const cap = await api.get<ReceiptCapture>(`${base}/receipts/${encodeURIComponent(receiptUuid)}`, { organization: org });
        if (cap.draft && cap.kind !== 'z_report') id = cap.draft.uuid;
      }
      if (!id) return null;
      return api.get<ReceiptExpense>(`${base}/expenses/${encodeURIComponent(id)}`, { organization: org });
    },
    enabled: !!engagement && !!ctx && (!!expenseUuid || !!receiptUuid),
    staleTime: 30_000,
  });
  const e = expense.data ?? null;
  // Bağlamda fiş yoksa masrafa bağlı yükleme (taslağı bu masraf olan fiş).
  const capture = useQuery({
    queryKey: ['engagement', org, engagement ?? '', 'capture-of', e?.uuid ?? '', e?.period ?? ''],
    queryFn: async () => {
      const find = async (query: Record<string, string | number>) => {
        const page = await api.get<{ items: ReceiptCapture[] }>(`${base}/receipts`, { organization: org, query: { limit: 100, ...query } });
        return page.items.find((r) => r.draft?.uuid === e?.uuid) ?? null;
      };
      return (e?.period ? await find({ period: e.period }) : null) ?? (await find({}));
    },
    enabled: !!engagement && !receiptUuid && !!e?.has_attachment,
    staleTime: 5 * 60_000,
  });
  const image = ctx?.receipt ?? (capture.data ? { uuid: capture.data.uuid, media_type: capture.data.media_type } : null);
  return { expense: e, image };
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
