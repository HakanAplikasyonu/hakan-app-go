"use client";

import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  officeTaxpayerRequestService,
  onboardingKeys,
  taxpayerOnboardingService,
  type CreateTaxpayerRequestBody,
  type PrivacyNoticeAcceptance,
} from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

// --- Office side ---

export function useOfficeTaxpayerRequests(
  slug: string,
  page: { limit: number; offset: number },
  enabled = true,
) {
  return useQuery({
    queryKey: onboardingKeys.officeRequests(slug, page),
    queryFn: () => officeTaxpayerRequestService.list(slug, page),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useCreateTaxpayerRequest(slug: string) {
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: CreateTaxpayerRequestBody) =>
      officeTaxpayerRequestService.create(slug, body),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: onboardingKeys.officeRequestsAll(slug),
      }),
  });
}

export function useCancelTaxpayerRequest(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) =>
      officeTaxpayerRequestService.cancel(slug, uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: onboardingKeys.officeRequestsAll(slug),
      });
      appToast.success(t("onboarding.toast_cancelled"));
    },
  });
}

// --- Mükellef panel ---

export function usePendingOnboardingRequests(taxpayer: string) {
  return useQuery({
    queryKey: onboardingKeys.pendingForTaxpayer(taxpayer),
    queryFn: () => taxpayerOnboardingService.listPending(taxpayer),
    retry: false,
    // Non-owners are refused: the card hides, no global toast.
    meta: { silent: true },
  });
}

export function useApproveOnboardingRequest(taxpayer: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: ({
      uuid,
      notice,
    }: {
      uuid: string;
      notice: PrivacyNoticeAcceptance;
    }) => taxpayerOnboardingService.approve(taxpayer, uuid, notice),
    onSuccess: () => appToast.success(t("onboarding.panel_toast_approved")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: onboardingKeys.pendingForTaxpayer(taxpayer),
      }),
  });
}

export function useRejectOnboardingRequest(taxpayer: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) =>
      taxpayerOnboardingService.reject(taxpayer, uuid),
    onSuccess: () => appToast.success(t("onboarding.panel_toast_rejected")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: onboardingKeys.pendingForTaxpayer(taxpayer),
      }),
  });
}
