"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  historyShareKeys,
  historyShareService,
  type CreateHistoryShareBody,
} from "@/features/taxpayer/services/history-shares.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useHistoryShares(taxpayer: string) {
  return useQuery({
    queryKey: historyShareKeys.list(taxpayer),
    queryFn: () => historyShareService.list(taxpayer),
    retry: false,
    // Non-owners are refused: the card hides, no global toast.
    meta: { silent: true },
  });
}

export function useCreateHistoryShare(taxpayer: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: CreateHistoryShareBody) =>
      historyShareService.create(taxpayer, body),
    onSuccess: () => appToast.success(t("taxpayer.history_toast_shared")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: historyShareKeys.list(taxpayer),
      }),
  });
}

export function useRevokeHistoryShare(taxpayer: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => historyShareService.revoke(taxpayer, uuid),
    onSuccess: () => appToast.success(t("taxpayer.history_toast_revoked")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: historyShareKeys.list(taxpayer),
      }),
  });
}
