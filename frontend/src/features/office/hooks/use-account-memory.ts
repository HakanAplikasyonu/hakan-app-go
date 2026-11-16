"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  accountMemoryKeys,
  accountMemoryService,
  type AccountRulePromoteInput,
  type MissingPlanAccount,
} from "../services/account-memory.service";
import { reviewKeys } from "../services/review.service";

export function useAccountPlanImports(slug: string, engagement: string) {
  return useQuery({
    queryKey: accountMemoryKeys.imports(slug, engagement),
    queryFn: () => accountMemoryService.imports(slug, engagement),
    enabled: !!engagement,
  });
}

export function useRuleCandidates(slug: string, engagement: string) {
  return useQuery({
    queryKey: accountMemoryKeys.candidates(slug, engagement),
    queryFn: () => accountMemoryService.candidates(slug, engagement),
    enabled: !!engagement,
  });
}

export function useEngagementRules(slug: string, engagement: string) {
  return useQuery({
    queryKey: accountMemoryKeys.rules(slug, engagement),
    queryFn: () => accountMemoryService.rules(slug, engagement),
    enabled: !!engagement,
  });
}

export function useMissingPlanAccounts(slug: string, engagement: string) {
  return useQuery({
    queryKey: accountMemoryKeys.missing(slug, engagement),
    queryFn: () => accountMemoryService.missing(slug, engagement),
    enabled: !!engagement,
  });
}

export function useAccountMemoryActions(slug: string, engagement: string) {
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({
      queryKey: accountMemoryKeys.all(slug, engagement),
    });
    void qc.invalidateQueries({ queryKey: reviewKeys.all(slug) });
  };
  return {
    importPlan: useMutation({
      mutationFn: (file: File) =>
        accountMemoryService.importPlan(slug, engagement, file),
      onSuccess: refresh,
    }),
    retireRule: useMutation({
      mutationFn: (ruleKey: string) =>
        accountMemoryService.retireRule(slug, engagement, ruleKey),
      onSuccess: refresh,
    }),
    downloadTemplate: useMutation({
      mutationFn: (accounts: MissingPlanAccount[]) =>
        accountMemoryService.downloadMissingTemplate(
          slug,
          engagement,
          accounts,
        ),
    }),
    promote: useMutation({
      mutationFn: (input: AccountRulePromoteInput) =>
        accountMemoryService.promote(slug, engagement, input),
      onSuccess: refresh,
    }),
  };
}
