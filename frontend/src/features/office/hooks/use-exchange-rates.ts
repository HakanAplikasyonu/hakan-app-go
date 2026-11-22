"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ExchangeRateSettings =
  components["schemas"]["ExchangeRateSettings"];
export type ExchangeRateKind = ExchangeRateSettings["rate_kind"];

const key = (slug: string) =>
  ["tenant", slug.toLowerCase(), "exchange-rate-settings"] as const;

/** Ofisin TCMB kur ayarı (#780): kur türü ve otomatik doldurma. */
export function useExchangeRateSettings(slug: string) {
  return useQuery({
    queryKey: key(slug),
    queryFn: () =>
      tenantRequest<ExchangeRateSettings>(
        slug,
        "GET",
        "/v1/tenant/exchange-rate-settings",
        { silent: true },
      ),
  });
}

/**
 * Kaydedince sunucu ayarı hemen uygular (eksik kurlar dolar, otomatik
 * kurlar yeni türe geçer); fatura ve beyan ekranları yeniden okunur.
 */
export function useSaveExchangeRateSettings(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ExchangeRateSettings) =>
      tenantRequest<ExchangeRateSettings>(
        slug,
        "PUT",
        "/v1/tenant/exchange-rate-settings",
        { body, silent: true },
      ),
    onSuccess: (data) => {
      qc.setQueryData(key(slug), data);
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "engagements"],
      });
    },
  });
}
