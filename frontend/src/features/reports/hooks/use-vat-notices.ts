"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  vatNoticeKeys,
  vatNoticeService,
  type VATNoticeInput,
} from "@/features/reports/services/vat-notice.service";

export function useVATNotices(slug: string, engagement: string) {
  return useQuery({
    queryKey: vatNoticeKeys.list(slug, engagement),
    queryFn: () => vatNoticeService.list(slug, engagement),
    // Kuyruktaki gönderim varken liste kendini tazeler.
    refetchInterval: (q) =>
      q.state.data?.items.some((n) =>
        n.deliveries.some(
          (d) => d.status === "queued" || d.status === "processing",
        ),
      )
        ? 10_000
        : false,
  });
}

export function useSendVATNotice(slug: string, engagement: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VATNoticeInput) =>
      vatNoticeService.send(slug, engagement, body),
    onSuccess: () =>
      void qc.invalidateQueries({
        queryKey: vatNoticeKeys.list(slug, engagement),
      }),
  });
}
