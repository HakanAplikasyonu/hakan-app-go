"use client";

import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { keepPreviousWithin } from "@/lib/query/keep-previous";

import { useEngagementDirectory } from "./use-engagement-directory";

import {
  officeInvoiceKeys,
  officeInvoiceService,
  type InvoiceFilter,
  type UBLUploadJob,
} from "@/features/office/services/office-invoices.service";

/** An active engagement with its taxpayer's identity, for pickers. */
export type EngagementOption = {
  engagement: string;
  title: string;
  taxId: string;
};

/**
 * Active engagements the caller may see, each with its taxpayer title and
 * VKN/TCKN. The engagement list gives the order (newest first, the picker's
 * default); names come from the office engagement directory (one shared
 * request) instead of one request per engagement (#639). Only an engagement
 * the directory does not know is asked one by one.
 */
export function useEngagementOptions(slug: string) {
  const list = useQuery({
    queryKey: officeInvoiceKeys.engagements(slug),
    queryFn: () => officeInvoiceService.activeEngagements(slug),
  });
  const directory = useEngagementDirectory();
  const items = list.data?.items ?? [];
  // Dizin gelmediyse (hata) eski yola düşülür: her mükellef tek tek.
  const missing = directory.isSuccess
    ? items.filter((e) => !directory.entries.has(e.uuid))
    : directory.isError
      ? items
      : [];
  const taxpayers = useQueries({
    queries: missing.map((e) => ({
      queryKey: officeInvoiceKeys.taxpayer(slug, e.uuid),
      queryFn: () => officeInvoiceService.taxpayer(slug, e.uuid),
      staleTime: 5 * 60_000,
    })),
  });
  const fallback = new Map(
    missing.map((e, i) => [e.uuid, taxpayers[i]?.data] as const),
  );
  const options: EngagementOption[] = items.map((e) => {
    const known = directory.entries.get(e.uuid);
    const extra = fallback.get(e.uuid);
    return {
      engagement: e.uuid,
      title: known?.title ?? extra?.title ?? "",
      taxId: known?.taxId ?? extra?.tax_id ?? "",
    };
  });
  return {
    options,
    isLoading:
      list.isLoading ||
      directory.isLoading ||
      taxpayers.some((q) => q.isLoading),
    isError: list.isError,
  };
}

export function useEngagementInvoices(
  slug: string,
  engagement: string,
  filter: InvoiceFilter,
) {
  return useQuery({
    queryKey: officeInvoiceKeys.invoices(slug, engagement, filter),
    queryFn: () => officeInvoiceService.invoices(slug, engagement, filter),
    placeholderData: keepPreviousWithin(engagement),
    enabled: !!engagement,
  });
}

export function useEngagementInvoice(
  slug: string,
  engagement: string,
  invoice: string,
) {
  return useQuery({
    queryKey: officeInvoiceKeys.invoice(slug, engagement, invoice),
    queryFn: () => officeInvoiceService.invoice(slug, engagement, invoice),
    enabled: !!engagement && !!invoice,
  });
}

export function useInvoiceUBL(
  slug: string,
  engagement: string,
  invoice: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: officeInvoiceKeys.ubl(slug, engagement, invoice),
    queryFn: () => officeInvoiceService.ubl(slug, engagement, invoice),
    enabled: enabled && !!engagement && !!invoice,
    staleTime: Infinity,
  });
}

const pending = (j?: UBLUploadJob) =>
  j?.status === "queued" || j?.status === "processing";

export function useUBLUploads(slug: string, engagement: string) {
  return useQuery({
    queryKey: officeInvoiceKeys.uploads(slug, engagement),
    queryFn: () => officeInvoiceService.uploads(slug, engagement),
    enabled: !!engagement,
    // Jobs run in the background: poll while one is still working.
    refetchInterval: (q) =>
      q.state.data?.items.some((j) => pending(j)) ? 2000 : false,
  });
}

export function useUBLUpload(slug: string, engagement: string, job: string) {
  return useQuery({
    queryKey: officeInvoiceKeys.upload(slug, engagement, job),
    queryFn: () => officeInvoiceService.upload(slug, engagement, job),
    enabled: !!engagement && !!job,
    refetchInterval: (q) => (pending(q.state.data) ? 1500 : false),
  });
}

/** Uploads the files one by one (the endpoint takes one file per job). */
export function useCreateUBLUploads(slug: string, engagement: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (files: File[]) => {
      const jobs: UBLUploadJob[] = [];
      for (const f of files) {
        jobs.push(await officeInvoiceService.createUpload(slug, engagement, f));
      }
      return jobs;
    },
    onSettled: () => {
      void qc.invalidateQueries({
        queryKey: officeInvoiceKeys.uploads(slug, engagement),
      });
      void qc.invalidateQueries({
        queryKey: [
          "tenant",
          slug.toLowerCase(),
          "engagements",
          engagement.toLowerCase(),
          "invoices",
        ],
      });
    },
  });
}
