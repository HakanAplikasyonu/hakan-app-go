import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type CommunicationItem =
  components["schemas"]["CommunicationHistoryItem"];
export type CommunicationChannel = CommunicationItem["channel"];
export type TaxpayerGroup = components["schemas"]["TaxpayerGroup"];
export type TaxpayerGroupInput = components["schemas"]["TaxpayerGroupInput"];
export type CommunicationFilters =
  components["schemas"]["CommunicationFilters"];
export type BulkSendTarget = components["schemas"]["BulkSendTarget"];
export type BulkSendInput = components["schemas"]["BulkSendInput"];
export type BulkSendPreview = components["schemas"]["BulkSendPreview"];
export type BulkSend = components["schemas"]["BulkSend"];
export type BulkChannel = BulkSendInput["channel"];

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
  all: (slug: string) =>
    ["tenant", slug.toLowerCase(), "communication"] as const,
  groups: (slug: string) => [...communicationKeys.all(slug), "groups"] as const,
  group: (slug: string, uuid: string) =>
    [...communicationKeys.all(slug), "groups", uuid] as const,
  filters: (slug: string) =>
    [...communicationKeys.all(slug), "filters"] as const,
  bulkSends: (slug: string) =>
    [...communicationKeys.all(slug), "bulk-sends"] as const,
  bulkSend: (slug: string, uuid: string) =>
    [...communicationKeys.all(slug), "bulk-sends", uuid] as const,
};

/**
 * İletişim sayfası (#780 B): mükellef grupları, dinamik süzgeçler ve elle
 * toplu SMS / e-posta gönderimi. Gönderim arka planda yürür; ekran
 * `bulkSend` ile ilerlemeyi okur.
 */
export const communicationService = {
  /**
   * İletişim geçmişi (#780 D): mükellefe giden SMS ve e-postalar, yeniden
   * eskiye. SMS'te NetGSM iş no ve iletim zamanı, e-postada açılma ve indirme
   * zamanı gelir.
   */
  history(slug: string, engagement: string, channel?: CommunicationChannel) {
    return tenantRequest<{ items: CommunicationItem[] }>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/communications`,
      { query: { channel, limit: 200 }, silent: true },
    );
  },
  groups(slug: string) {
    return tenantRequest<{ items: TaxpayerGroup[] }>(
      slug,
      "GET",
      "/v1/tenant/communication/groups",
      { silent: true },
    );
  },
  group(slug: string, uuid: string) {
    return tenantRequest<TaxpayerGroup>(
      slug,
      "GET",
      `/v1/tenant/communication/groups/${encodeURIComponent(uuid)}`,
    );
  },
  createGroup(slug: string, body: TaxpayerGroupInput) {
    return tenantRequest<TaxpayerGroup>(
      slug,
      "POST",
      "/v1/tenant/communication/groups",
      { body },
    );
  },
  updateGroup(slug: string, uuid: string, body: TaxpayerGroupInput) {
    return tenantRequest<TaxpayerGroup>(
      slug,
      "PUT",
      `/v1/tenant/communication/groups/${encodeURIComponent(uuid)}`,
      { body },
    );
  },
  deleteGroup(slug: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/communication/groups/${encodeURIComponent(uuid)}`,
    );
  },
  filters(slug: string) {
    return tenantRequest<CommunicationFilters>(
      slug,
      "GET",
      "/v1/tenant/communication/filters",
      { silent: true },
    );
  },
  preview(slug: string, body: BulkSendInput) {
    return tenantRequest<BulkSendPreview>(
      slug,
      "POST",
      "/v1/tenant/communication/bulk-sends/preview",
      { body },
    );
  },
  start(slug: string, body: BulkSendInput) {
    return tenantRequest<BulkSend>(
      slug,
      "POST",
      "/v1/tenant/communication/bulk-sends",
      { body },
    );
  },
  recent(slug: string) {
    return tenantRequest<{ items: BulkSend[] }>(
      slug,
      "GET",
      "/v1/tenant/communication/bulk-sends",
      { silent: true },
    );
  },
  bulkSend(slug: string, uuid: string) {
    return tenantRequest<BulkSend>(
      slug,
      "GET",
      `/v1/tenant/communication/bulk-sends/${encodeURIComponent(uuid)}`,
      { silent: true },
    );
  },
};
