"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  reportKeys,
  reportService,
  type PageParams,
  type ReportRange,
} from "@/features/reports/services/reports.service";

/** Kuyruktaki gönderim varken liste kendini tazeler. */
function refetchWhilePending(items: { status: string }[] | undefined) {
  return items?.some((d) => d.status === "queued" || d.status === "sending")
    ? 10_000
    : false;
}

export function useReportDeliveries(
  slug: string,
  engagement: string,
  page: PageParams,
) {
  return useQuery({
    queryKey: [...reportKeys.deliveries(slug, engagement), page],
    queryFn: () => reportService.deliveries(slug, engagement, page),
    placeholderData: keepPreviousData,
    refetchInterval: (q) => refetchWhilePending(q.state.data?.items),
  });
}

export function useOfficeReportDeliveries(slug: string, page: PageParams) {
  return useQuery({
    queryKey: [...reportKeys.officeDeliveries(slug), page],
    queryFn: () => reportService.officeDeliveries(slug, page),
    placeholderData: keepPreviousData,
    refetchInterval: (q) => refetchWhilePending(q.state.data?.items),
  });
}

export function useSendReport(slug: string, engagement: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { range: ReportRange; recipient?: string }) =>
      reportService.send(slug, engagement, v.range, v.recipient),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: reportKeys.deliveries(slug, engagement),
      });
      void qc.invalidateQueries({
        queryKey: reportKeys.officeDeliveries(slug),
      });
    },
  });
}

export function useBulkSendReports(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { engagements: string[]; range: ReportRange }) =>
      reportService.bulk(slug, v.engagements, v.range),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: ["tenant", slug.toLowerCase()] }),
  });
}
