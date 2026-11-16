"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useTenant } from "@/features/tenant/providers/tenant-provider";

import {
  engagementKeys,
  engagementService,
} from "../services/engagements.service";
import {
  officeSummaryKeys,
  officeSummaryService,
} from "../services/office-summary.service";
import { dashboardKeys, dashboardService } from "../services/dashboard.service";
import { periodKeys, periodService } from "../services/periods.service";

/** Period matrix for the office (all pages). */
export function usePeriodMatrix(periods: number) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: periodKeys.matrixAll(slug, { periods }),
    queryFn: () => periodService.matrixAll(slug, { periods }),
    staleTime: 60_000,
  });
}

export function usePendingSuggestionsTotal(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSummaryKeys.pendingSuggestions(slug),
    queryFn: () => officeSummaryService.pendingSuggestions(slug),
    enabled,
  });
}

export function useOpenBlockersTotal(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSummaryKeys.openBlockers(slug),
    queryFn: () => officeSummaryService.openBlockers(slug),
    enabled,
  });
}

export function useOfficeVATForecast(
  period: string | undefined,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSummaryKeys.vatForecast(slug, period),
    queryFn: () => officeSummaryService.vatForecast(slug, period),
    enabled,
  });
}

export function useEngagementAssignments(engagement: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: engagementKeys.assignments(slug, engagement),
    queryFn: () => engagementService.assignments(slug, engagement),
    enabled,
    staleTime: 60_000,
  });
}

export function useToggleAssignment(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ user, assigned }: { user: string; assigned: boolean }) =>
      assigned
        ? engagementService.unassign(slug, engagement, user)
        : engagementService.assign(slug, engagement, user),
    onSettled: () =>
      qc.invalidateQueries({
        queryKey: engagementKeys.assignments(slug, engagement),
      }),
  });
}

/**
 * Ofis panosu sayaçları: kenar menüdeki sayaçlar ve genel bakış KPI'ları aynı
 * sorguyu paylaşır. Dakikada bir tazelenir.
 */
export function useOfficeDashboard(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: dashboardKeys.office(slug),
    queryFn: () => dashboardService.office(slug),
    enabled,
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: false,
  });
}

/** Tek dönemin matrisi (genel bakışın dönem tacı, #326). */
export function usePeriodMatrixAt(period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: periodKeys.matrixAll(slug, { until: period, periods: 1 }),
    queryFn: () => periodService.matrixAll(slug, { until: period, periods: 1 }),
    staleTime: 60_000,
  });
}

/**
 * Dönemin öneri sayıları: toplam ve bekleyen (onay oranı = karara bağlanan /
 * toplam). Liste uçlarından `total` okunur.
 */
export function useReviewPeriodTotals(period: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSummaryKeys.reviewTotals(slug, period),
    queryFn: () => officeSummaryService.reviewTotals(slug, period),
    enabled,
    staleTime: 60_000,
  });
}
