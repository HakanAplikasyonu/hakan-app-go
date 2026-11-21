import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type CommunicationItem =
  components["schemas"]["CommunicationHistoryItem"];
export type CommunicationChannel = CommunicationItem["channel"];

export const communicationKeys = {
  history: (slug: string, engagement: string, channel: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagement",
      engagement,
      "communications",
      channel,
    ] as const,
};

/**
 * İletişim geçmişi (#780 D): mükellefe giden SMS ve e-postalar, yeniden
 * eskiye. SMS'te NetGSM iş no ve iletim zamanı, e-postada açılma ve indirme
 * zamanı gelir.
 */
export const communicationService = {
  history(slug: string, engagement: string, channel?: CommunicationChannel) {
    return tenantRequest<{ items: CommunicationItem[] }>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/communications`,
      { query: { channel, limit: 200 }, silent: true },
    );
  },
};
