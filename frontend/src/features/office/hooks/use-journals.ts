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
  };
}
