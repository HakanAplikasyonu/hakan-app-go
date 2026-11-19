import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

import { downloadFile } from "./download";

export type PrivacyRequest = components["schemas"]["PrivacyRequest"];
export type PrivacyRequestInput = components["schemas"]["PrivacyRequestInput"];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/privacy-requests`;

export const privacyKeys = {
  list: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagement",
      engagement,
      "privacy-requests",
    ] as const,
};

/** KVKK ilgili kişi talepleri (#193, ekran #518 B3). */
export const privacyService = {
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: PrivacyRequest[] }>(
      slug,
      "GET",
      base(engagement),
      { query: { limit: 100 } },
    );
  },
  create(slug: string, engagement: string, input: PrivacyRequestInput) {
    return tenantRequest<PrivacyRequest>(slug, "POST", base(engagement), {
      body: input,
      silent: true,
    });
  },
  decide(
    slug: string,
    engagement: string,
    request: string,
    action: "approve" | "reject",
    note?: string,
  ) {
    return tenantRequest<PrivacyRequest>(
      slug,
      "POST",
      `${base(engagement)}/${encodeURIComponent(request)}/${action}`,
      { body: note ? { note } : {}, silent: true },
    );
  },
  download(slug: string, engagement: string, request: string) {
    return downloadFile(
      slug,
      `${base(engagement)}/${encodeURIComponent(request)}/export`,
      {},
      "kvkk-disa-aktarim.zip",
    );
  },
};
