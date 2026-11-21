"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type LateDocument = components["schemas"]["LateDocument"];
export type LateDocumentList = components["schemas"]["LateDocumentList"];
export type LateDocumentSummary = components["schemas"]["LateDocumentSummary"];
export type LateDocumentDecision =
  components["schemas"]["LateDocumentDecision"];

export type LateDocumentQuery = {
  status?: string;
  period?: string;
  document_uuid?: string;
};

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/late-documents`;

const rootKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "late-documents"] as const;

/**
 * Mükellefin geç gelen belgeleri (#780 C): dönemi kapandıktan sonra gelen
 * fatura, masraf ve Z raporları, ofisin kararıyla.
 */
export function useLateDocuments(
  engagement: string,
  query: LateDocumentQuery = {},
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...rootKey(slug), engagement, query],
    queryFn: () =>
      tenantRequest<LateDocumentList>(slug, "GET", base(engagement), {
        query,
      }),
    enabled: enabled && !!engagement,
  });
}

/** Ofisin kararı: düzeltmeye al, cari döneme al, yok say, geri al. */
export function useDecideLateDocument(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      uuid,
      decision,
    }: {
      uuid: string;
      decision: LateDocumentDecision;
    }) =>
      tenantRequest<LateDocument>(
        slug,
        "POST",
        `${base(engagement)}/${encodeURIComponent(uuid)}/decision`,
        { body: decision },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rootKey(slug) });
    },
  });
}

/** Ofis geneli sayaç: mükellef başına bekleyen geç belge. */
export function useLateDocumentSummary(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...rootKey(slug), "summary"],
    queryFn: () =>
      tenantRequest<LateDocumentSummary>(
        slug,
        "GET",
        "/v1/tenant/late-documents/summary",
      ),
    enabled,
  });
}
