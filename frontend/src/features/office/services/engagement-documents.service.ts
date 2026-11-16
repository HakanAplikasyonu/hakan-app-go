import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type EngagementDocument = components["schemas"]["EngagementDocument"];
export type EngagementDocumentList =
  components["schemas"]["EngagementDocumentList"];
export type EngagementHistoryShareList =
  components["schemas"]["HistoryShareList"];

export const engagementDocumentKeys = {
  documents: (
    slug: string,
    engagement: string,
    page: { limit: number; offset: number },
  ) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "documents",
      page,
    ] as const,
  historyShares: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "history-shares",
    ] as const,
};

/**
 * Office side (issue #8): documents this office may see for one engagement
 * (its own period plus whatever the owner shared from before it), and the
 * owner's history consents for that engagement.
 */
export const engagementDocumentService = {
  documents(
    slug: string,
    engagement: string,
    page: { limit: number; offset: number },
  ) {
    return tenantRequest<EngagementDocumentList>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/documents`,
      { query: page },
    );
  },
  historyShares(slug: string, engagement: string) {
    return tenantRequest<EngagementHistoryShareList>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/history-shares`,
    );
  },
};

/** Ofisin elle açtığı belge isteği (`POST …/document-tasks`, #240). */
export type DocumentRequestInput = {
  title: string;
  note?: string;
  due_days?: number;
};

export function createDocumentRequest(
  slug: string,
  engagement: string,
  body: DocumentRequestInput,
) {
  return tenantRequest<{ uuid: string }>(
    slug,
    "POST",
    `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks`,
    { body, silent: true },
  );
}
