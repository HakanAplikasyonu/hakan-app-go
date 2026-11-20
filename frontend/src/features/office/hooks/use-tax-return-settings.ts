"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TaxReturnSettings = components["schemas"]["TaxReturnSettings"];

const key = (slug: string) =>
  ["tenant", slug.toLowerCase(), "tax-return-settings"] as const;

/**
 * Beyan son kontrolü ofis ayarı (#710): açıkken "kontrol edildi / verildi"
 * yalnız sorumluda; kapalıyken beyanı hazırlayan da işaretler.
 */
export function useTaxReturnSettings(slug: string, enabled = true) {
  return useQuery({
    queryKey: key(slug),
    queryFn: () =>
      tenantRequest<TaxReturnSettings>(
        slug,
        "GET",
        "/v1/tenant/tax-return-settings",
        { silent: true },
      ),
    enabled,
  });
}

export function useSaveTaxReturnSettings(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (finalCheckRequired: boolean) =>
      tenantRequest<TaxReturnSettings>(
        slug,
        "PUT",
        "/v1/tenant/tax-return-settings",
        { body: { final_check_required: finalCheckRequired } },
      ),
    onSuccess: (data) => qc.setQueryData(key(slug), data),
  });
}
