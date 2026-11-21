import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type VATNotice = components["schemas"]["VATNotice"];
export type VATWatchEvent = components["schemas"]["VATWatchEvent"];
export type VATWatchResult = components["schemas"]["EnvelopeVATWatch"]["data"];
export type VATNoticeInput = { period?: string; message?: string };

function vatBase(engagement: string) {
  return `/v1/tenant/engagements/${encodeURIComponent(engagement)}/vat-forecast`;
}

function base(engagement: string) {
  return `${vatBase(engagement)}/notices`;
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
  watch: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "vat-watch",
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
  /** KDV nöbetçisinin bu mükellef için ürettiği olaylar. */
  watchEvents(slug: string, engagement: string) {
    return tenantRequest<VATWatchResult>(
      slug,
      "GET",
      `${vatBase(engagement)}/watch-events`,
    );
  },
  /** Nöbetçiyi bu mükellef için şimdi çalıştırır ("Şimdi kontrol et"). */
  runWatch(slug: string, engagement: string) {
    return tenantRequest<VATWatchResult>(
      slug,
      "POST",
      `${vatBase(engagement)}/watch`,
      { body: {}, silent: true },
    );
  },
};
