"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  warningKeys,
  warningService,
  type WarningDecisionAction,
  type WarningListQuery,
} from "@/features/office/services/warnings.service";

/**
 * Warnings list: office-wide, or one engagement's when `engagement` is set
 * (the office-wide endpoint has no engagement filter).
 */
export function useWarnings(
  slug: string,
  query: WarningListQuery,
  engagement?: string,
) {
  return useQuery({
    queryKey: engagement
      ? warningKeys.engagement(slug, engagement, query)
      : warningKeys.office(slug, query),
    queryFn: () =>
      engagement
        ? warningService.listEngagement(slug, engagement, query)
        : warningService.listOffice(slug, query),
    placeholderData: keepPreviousData,
  });
}

export function useWarningRules(slug: string) {
  return useQuery({
    queryKey: warningKeys.rules(slug),
    queryFn: () => warningService.rules(slug),
    staleTime: 10 * 60_000,
  });
}

/** Acknowledge / resolve one open warning, then refetch the lists. */
export function useDecideWarning(slug: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: {
      engagement: string;
      warning: string;
      action: WarningDecisionAction;
    }) =>
      warningService.decide(slug, vars.engagement, vars.warning, vars.action),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: warningKeys.all(slug) }),
  });
}
