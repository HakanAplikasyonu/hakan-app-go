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

export function useVATWatchEvents(slug: string, engagement: string) {
  return useQuery({
    queryKey: vatNoticeKeys.watch(slug, engagement),
    queryFn: () => vatNoticeService.watchEvents(slug, engagement),
  });
}

export function useRunVATWatch(slug: string, engagement: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => vatNoticeService.runWatch(slug, engagement),
    onSuccess: (data) =>
      qc.setQueryData(vatNoticeKeys.watch(slug, engagement), data),
  });
}
