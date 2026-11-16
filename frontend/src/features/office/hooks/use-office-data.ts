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
