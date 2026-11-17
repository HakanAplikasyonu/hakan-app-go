"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  journalKeys,
  journalService,
  type JournalDraftInput,
  type JournalExportInput,
} from "@/features/office/services/journals.service";

export function useExportDrivers(slug: string) {
  return useQuery({
    queryKey: journalKeys.drivers(slug),
    queryFn: () => journalService.drivers(slug),
    staleTime: 10 * 60_000,
  });
}

export function useJournalDrafts(
  slug: string,
  engagement: string,
  period: string,
) {
  return useQuery({
    queryKey: journalKeys.drafts(slug, engagement, period),
    queryFn: () => journalService.drafts(slug, engagement, period),
    enabled: !!engagement,
  });
}

export function useJournalExports(slug: string, engagement: string) {
  return useQuery({
    queryKey: journalKeys.exports(slug, engagement),
    queryFn: () => journalService.exports(slug, engagement),
    enabled: !!engagement,
  });
}

/** Transfers to the accounting program (#391); refreshed every 30 s while
 * one waits for the desktop agent. */
export function useJournalTransfers(slug: string, engagement: string) {
  return useQuery({
    queryKey: journalKeys.transfers(slug, engagement),
    queryFn: () => journalService.transfers(slug, engagement),
    enabled: !!engagement,
    refetchInterval: (q) =>
      q.state.data?.items.some(
        (t) => t.status === "queued" || t.status === "claimed",
      )
        ? 30_000
        : false,
  });
}

/** Mutations of one engagement; every success refreshes its journal data. */
export function useJournalActions(slug: string, engagement: string) {
  const qc = useQueryClient();
  const refresh = () =>
    qc.invalidateQueries({ queryKey: journalKeys.all(slug, engagement) });
  return {
    create: useMutation({
      mutationFn: (input: JournalDraftInput) =>
        journalService.create(slug, engagement, input),
      onSuccess: refresh,
    }),
    approve: useMutation({
      mutationFn: (draft: string) =>
        journalService.approve(slug, engagement, draft),
      onSuccess: refresh,
    }),
    reverse: useMutation({
      mutationFn: (v: { draft: string; reason: string }) =>
        journalService.reverse(slug, engagement, v.draft, v.reason),
      onSuccess: refresh,
    }),
    export: useMutation({
      mutationFn: (input: JournalExportInput) =>
        journalService.export(slug, engagement, input),
      onSuccess: refresh,
    }),
    requestTransfer: useMutation({
      mutationFn: (batch: string) =>
        journalService.requestTransfer(slug, engagement, batch),
      onSuccess: refresh,
    }),
    completeTransfer: useMutation({
      mutationFn: (v: {
        transfer: string;
        outcome: "imported" | "failed";
        message?: string;
      }) =>
        journalService.completeTransfer(
          slug,
          engagement,
          v.transfer,
          v.outcome,
          v.message,
        ),
      onSuccess: refresh,
    }),
    cancelTransfer: useMutation({
      mutationFn: (transfer: string) =>
        journalService.cancelTransfer(slug, engagement, transfer),
      onSuccess: refresh,
    }),
    retryTransfer: useMutation({
      mutationFn: (transfer: string) =>
        journalService.retryTransfer(slug, engagement, transfer),
      onSuccess: refresh,
    }),
  };
}
