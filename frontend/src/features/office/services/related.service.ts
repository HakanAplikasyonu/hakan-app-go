import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type RelatedSourceKind = components["schemas"]["RelatedSourceKind"];
export type RelatedRecord = components["schemas"]["RelatedRecord"];
export type RelatedRecords = components["schemas"]["RelatedRecords"];

export const relatedKeys = {
  one: (slug: string, engagement: string, kind: string, uuid: string) =>
    ["tenant", slug.toLowerCase(), "related", engagement, kind, uuid] as const,
};

/** İlişkili kayıtlar (#469): GET .../related/{kind}/{recordUuid}. */
export const relatedService = {
  list: (
    slug: string,
    engagement: string,
    kind: RelatedSourceKind,
    uuid: string,
  ) =>
    tenantRequest<RelatedRecords>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/related/${encodeURIComponent(kind)}/${encodeURIComponent(uuid)}`,
    ),
};
