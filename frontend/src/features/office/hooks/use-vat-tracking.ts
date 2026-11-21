"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type VATTrackingSettings = components["schemas"]["VATTrackingSettings"];
export type VATReportRule = components["schemas"]["VATReportRule"];
export type VATReportRules =
  components["schemas"]["EnvelopeVATReportRules"]["data"];
export type VATReportRun =
  components["schemas"]["EnvelopeVATReportRun"]["data"];

const rulesKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "vat-report-rules"] as const;

/** KDV raporu kuralları (#780): günler, kime, hangi mükellefler. */
export function useVATReportRules(slug: string) {
  return useQuery({
    queryKey: rulesKey(slug),
    queryFn: () =>
      tenantRequest<VATReportRules>(
        slug,
        "GET",
        "/v1/tenant/vat-report-rules",
        {
          silent: true,
        },
      ),
  });
}

export function useSaveVATReportRules(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (items: VATReportRule[]) =>
      tenantRequest<VATReportRules>(
        slug,
        "PUT",
        "/v1/tenant/vat-report-rules",
        {
          body: { items },
          silent: true,
        },
      ),
    onSuccess: (data) => qc.setQueryData(rulesKey(slug), data),
  });
}

/** Bir kuralı şimdi çalıştırır ("default": varsayılan kural). */
export function useRunVATReportRule(slug: string) {
  return useMutation({
    mutationFn: (rule: string) =>
      tenantRequest<VATReportRun>(
        slug,
        "POST",
        `/v1/tenant/vat-report-rules/${encodeURIComponent(rule)}/run`,
        { body: {}, silent: true },
      ),
  });
}

const key = (slug: string) =>
  ["tenant", slug.toLowerCase(), "vat-tracking-settings"] as const;

/**
 * KDV takibi ofis ayarları (#780): ay sonu KDV raporu açık/kapalı ve
 * hazır olduğu gün (varsayılan ayın 29'u).
 */
export function useVATTrackingSettings(slug: string, enabled = true) {
  return useQuery({
    queryKey: key(slug),
    queryFn: () =>
      tenantRequest<VATTrackingSettings>(
        slug,
        "GET",
        "/v1/tenant/vat-tracking-settings",
        { silent: true },
      ),
    enabled,
  });
}

export function useSaveVATTrackingSettings(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      report_enabled: boolean;
      report_day: number;
      watch_enabled?: boolean;
      watch_email?: boolean;
      /** Eşik (TL, "10000.00"); null siler, verilmezse değişmez. */
      payable_limit?: string | null;
    }) =>
      tenantRequest<VATTrackingSettings>(
        slug,
        "PUT",
        "/v1/tenant/vat-tracking-settings",
        { body },
      ),
    onSuccess: (data) => qc.setQueryData(key(slug), data),
  });
}

export type OfficeVATWatchEvent =
  components["schemas"]["OfficeVATWatchEvent"];

/** Son KDV uyarıları (#780 B): ofis genelindeki son nöbetçi olayları. */
export function useOfficeVATWatchEvents(slug: string, limit = 10) {
  return useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "vat-watch-events", limit],
    queryFn: () =>
      tenantRequest<{ items: OfficeVATWatchEvent[] }>(
        slug,
        "GET",
        "/v1/tenant/vat-forecast/watch-events",
        { query: { limit }, silent: true },
      ),
  });
}

/** Devreden KDV açılışı (#780: "devir KDV'yi ofis girer"). */
export function useSaveVATOpening(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      engagement: string;
      period: string;
      amount: string;
      note?: string;
    }) =>
      tenantRequest(
        slug,
        "PUT",
        `/v1/tenant/engagements/${encodeURIComponent(v.engagement)}/vat-forecast/openings/${v.period}`,
        { body: { amount: v.amount, note: v.note || undefined } },
      ),
    onSuccess: () =>
      qc.invalidateQueries({
        queryKey: [
          "tenant",
          slug.toLowerCase(),
          "office-summary",
          "vat-forecast",
        ],
      }),
  });
}
