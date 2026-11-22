"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

import { receiptsService } from "../services/receipts.service";

export type InboxItem = components["schemas"]["InboxItem"];
export type InboxList = components["schemas"]["InboxList"];
export type InboxType = InboxItem["type"];
export type InboxCounts = components["schemas"]["InboxCounts"];

/** Gelen kutusundaki türler, ekrandaki sırayla. */
export const INBOX_TYPES: readonly InboxType[] = [
  "invoice",
  "bank_transaction",
  "expense",
  "retail_sale",
  "foreign_invoice",
  "late_document",
];

export type InboxQuery = {
  period?: string;
  type?: InboxType;
  engagement?: string;
  limit?: number;
  offset?: number;
};

export const INBOX_PAGE = 50;

const rootKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "inbox"] as const;

/**
 * Gelen kutusu (#780 b): bu dönem müşavirin elini bekleyen her şey (önerisi
 * olmayan fatura, eşleşmemiş banka hareketi, onay bekleyen masraf ve Z
 * raporu, mükelleften gelen yabancı fatura, geç belge) ve türe göre sayılar.
 */
export function useInbox(query: InboxQuery, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...rootKey(slug), query],
    queryFn: () =>
      tenantRequest<InboxList>(slug, "GET", "/v1/tenant/inbox", { query }),
    placeholderData: keepPreviousData,
    enabled,
  });
}

/**
 * Tek tıkla onay: yalnız onay ucu olan türler (masraf fişi, Z raporu).
 * Onaydan sonra gelen kutusu ve ilgili listeler yenilenir.
 */
export function useApproveInboxItem() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (item: InboxItem): Promise<void> => {
      if (item.type === "expense")
        await receiptsService.decideExpense(
          slug,
          item.engagement_uuid,
          item.uuid,
          true,
        );
      else
        await receiptsService.decideRetailSale(
          slug,
          item.engagement_uuid,
          item.uuid,
          true,
        );
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rootKey(slug) });
      void qc.invalidateQueries({ queryKey: ["tenant", slug.toLowerCase()] });
    },
  });
}
