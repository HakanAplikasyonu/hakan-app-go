"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  journalKeys,
  journalService,
  type JournalDraftInput,
  type JournalExportInput,
  type JournalExportSettings,
  type OfficeJournalExportSettings,
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

/** One draft with its lines; draft "" = nothing open. */
export function useJournalDraft(
  slug: string,
  engagement: string,
  draft: string,
) {
  return useQuery({
    queryKey: journalKeys.draft(slug, engagement, draft),
    queryFn: () => journalService.draft(slug, engagement, draft),
    enabled: !!engagement && !!draft,
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

/** Ofis geneli bekleyen aktarımlar ve son aktarımlar (#574). */
export function useOfficePendingTransfers(slug: string) {
  return useQuery({
    queryKey: journalKeys.officePending(slug),
    queryFn: () => journalService.officePending(slug),
  });
}

export function useOfficeTransfers(slug: string) {
  return useQuery({
    queryKey: journalKeys.officeTransfers(slug),
    queryFn: () => journalService.officeTransfers(slug),
    refetchInterval: (q) =>
      q.state.data?.items.some(
        (t) => t.status === "queued" || t.status === "claimed",
      )
        ? 30_000
        : false,
  });
}

/** Document → file trail of the export history (#466). */
export function useJournalDocuments(
  slug: string,
  engagement: string,
  q: string,
  batch = "",
) {
  return useQuery({
    queryKey: journalKeys.documents(slug, engagement, q, batch),
    queryFn: () => journalService.documents(slug, engagement, q, batch),
    enabled: !!engagement,
  });
}

export function useJournalSettings(slug: string, engagement: string) {
  return useQuery({
    queryKey: journalKeys.settings(slug, engagement),
    queryFn: () => journalService.settings(slug, engagement),
    enabled: !!engagement,
  });
}

/** Office-wide export settings; saving refreshes every engagement's view. */
export function useOfficeJournalSettings(slug: string) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: journalKeys.officeSettings(slug),
    queryFn: () => journalService.officeSettings(slug),
  });
  const save = useMutation({
    mutationFn: (input: OfficeJournalExportSettings) =>
      journalService.saveOfficeSettings(slug, input),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["tenant", slug.toLowerCase()] }),
  });
  return { query, save };
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
      mutationFn: (
        v:
          | string
          | {
              draft: string;
              confirm?: boolean;
              note?: string;
              sendDuplicate?: boolean;
              reason?: string;
            },
      ) =>
        typeof v === "string"
          ? journalService.approve(slug, engagement, v)
          : journalService.approve(slug, engagement, v.draft, v),
      onSuccess: refresh,
    }),
    setLineAccount: useMutation({
      mutationFn: (v: { draft: string; line: number; account: string }) =>
        journalService.setLineAccount(
          slug,
          engagement,
          v.draft,
          v.line,
          v.account,
        ),
      onSuccess: refresh,
    }),
    setLineAmounts: useMutation({
      mutationFn: async (v: {
        draft: string;
        lines: { line: number; debit: string; credit: string }[];
      }) => {
        for (const l of v.lines)
          await journalService.setLineAmounts(
            slug,
            engagement,
            v.draft,
            l.line,
            l.debit,
            l.credit,
          );
      },
      onSettled: refresh,
    }),
    unapprove: useMutation({
      mutationFn: (v: { draft: string; reason: string }) =>
        journalService.unapprove(slug, engagement, v.draft, v.reason),
      onSuccess: refresh,
    }),
    reverse: useMutation({
      mutationFn: (v: { draft: string; reason: string }) =>
        journalService.reverse(slug, engagement, v.draft, v.reason),
      onSuccess: refresh,
    }),
    cancelExport: useMutation({
      mutationFn: (v: { draft: string; reason: string }) =>
        journalService.cancelExport(slug, engagement, v.draft, v.reason),
      onSuccess: refresh,
    }),
    export: useMutation({
      mutationFn: (input: JournalExportInput) =>
        journalService.export(slug, engagement, input),
      onSuccess: refresh,
    }),
    check: useMutation({
      mutationFn: (input: JournalExportInput) =>
        journalService.check(slug, engagement, input),
    }),
    saveSettings: useMutation({
      mutationFn: (
        input: Omit<JournalExportSettings, "effective_luca_pull_integrators">,
      ) => journalService.saveSettings(slug, engagement, input),
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
