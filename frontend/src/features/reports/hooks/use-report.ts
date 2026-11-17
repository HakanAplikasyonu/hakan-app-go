"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import {
  reportKeys,
  reportService,
  type ReportRange,
} from "@/features/reports/services/reports.service";

export function useEngagementReport(
  slug: string,
  engagement: string,
  range: ReportRange,
) {
  return useQuery({
    queryKey: reportKeys.office(slug, engagement, range),
    queryFn: () => reportService.office(slug, engagement, range),
    placeholderData: keepPreviousData,
  });
}

export function useMyReport(taxpayer: string, range: ReportRange) {
  return useQuery({
    queryKey: reportKeys.mine(taxpayer, range),
    queryFn: () => reportService.mine(taxpayer, range),
    placeholderData: keepPreviousData,
  });
}
