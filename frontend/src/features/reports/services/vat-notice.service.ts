import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type VATNotice = components["schemas"]["VATNotice"];
export type VATNoticeInput = { period?: string; message?: string };

function base(engagement: string) {
  return `/v1/tenant/engagements/${encodeURIComponent(engagement)}/vat-forecast/notices`;
}

export const vatNoticeKeys = {
  list: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "vat-notices",
    ] as const,
};

/**
 * Mükellefe giden KDV bildirimi (#780): "bu ay ödeme çıkacak / çıkmayacak".
 * Mükellefe ne söylendiği ve ne zaman, o anki rakamlarla saklanır.
 */
export const vatNoticeService = {
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: VATNotice[] }>(slug, "GET", base(engagement));
  },
  send(slug: string, engagement: string, body: VATNoticeInput) {
    return tenantRequest<VATNotice>(slug, "POST", base(engagement), {
      body,
      silent: true,
    });
  },
};
