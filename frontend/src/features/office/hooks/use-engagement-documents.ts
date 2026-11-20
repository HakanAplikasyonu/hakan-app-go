"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import {
  engagementDocumentKeys,
  engagementDocumentService,
} from "@/features/office/services/engagement-documents.service";

export function useEngagementDocuments(
  slug: string,
  engagement: string,
  page: { limit: number; offset: number },
  enabled = true,
  period?: string,
) {
  return useQuery({
    queryKey: engagementDocumentKeys.documents(slug, engagement, page, period),
    queryFn: () =>
      engagementDocumentService.documents(slug, engagement, page, period),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useEngagementHistoryShares(
  slug: string,
  engagement: string,
  enabled = true,
) {
  return useQuery({
    queryKey: engagementDocumentKeys.historyShares(slug, engagement),
    queryFn: () => engagementDocumentService.historyShares(slug, engagement),
    enabled,
  });
}
