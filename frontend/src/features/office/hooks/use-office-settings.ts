"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { useTenant } from "@/features/tenant/providers/tenant-provider";

import { reviewKeys } from "../services/review.service";
import {
  officeSettingsKeys,
  officeSettingsService,
  type AccountRuleSettingsInput,
  type ReviewSettingInput,
} from "../services/office-settings.service";

export function useThresholdImpact(
  threshold: string,
  months = 1,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSettingsKeys.impact(slug, threshold, months),
    queryFn: () =>
      officeSettingsService.thresholdImpact(slug, threshold, months),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
}

export function usePutReviewSetting() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ReviewSettingInput) =>
      officeSettingsService.putReview(slug, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: reviewKeys.all(slug) });
      void qc.invalidateQueries({ queryKey: officeSettingsKeys.all(slug) });
    },
  });
}

export function useRuleSettings(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSettingsKeys.rules(slug),
    queryFn: () => officeSettingsService.ruleSettings(slug),
    enabled,
  });
}

export function useRuleSettingsPreview(
  input: AccountRuleSettingsInput,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: officeSettingsKeys.rulesPreview(slug, input),
    queryFn: () => officeSettingsService.previewRuleSettings(slug, input),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
}

export function usePutRuleSettings() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AccountRuleSettingsInput) =>
      officeSettingsService.putRuleSettings(slug, input),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: officeSettingsKeys.all(slug) }),
  });
}
