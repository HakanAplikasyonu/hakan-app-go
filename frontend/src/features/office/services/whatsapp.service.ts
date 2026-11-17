import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";
import { download } from "@/lib/utils/format";

export type WhatsAppPairing = components["schemas"]["WhatsAppPairing"];
export type WhatsAppCapture = components["schemas"]["WhatsAppCapture"];
export type WhatsAppChatLink = components["schemas"]["WhatsAppChatLink"];
export type WhatsAppTarget = WhatsAppCapture["target"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

export const whatsappKeys = {
  all: (slug: string) => ["tenant", slug.toLowerCase(), "whatsapp"] as const,
  captures: (slug: string, params: Record<string, unknown>) =>
    [...whatsappKeys.all(slug), "captures", params] as const,
  links: (slug: string, params: Record<string, unknown>) =>
    [...whatsappKeys.all(slug), "links", params] as const,
};

/** WhatsApp Web uzantısı (#388): eşleştirme kodu, kayıtlar, sohbet eşlemeleri. */
export const whatsappService = {
  createPairing(slug: string) {
    return tenantRequest<WhatsAppPairing>(
      slug,
      "POST",
      "/v1/tenant/whatsapp/pairings",
      { silent: true },
    );
  },
  captures(
    slug: string,
    q: { target?: WhatsAppTarget; limit: number; offset: number },
  ) {
    return tenantRequest<Page<WhatsAppCapture>>(
      slug,
      "GET",
      "/v1/tenant/whatsapp/captures",
      { query: q },
    );
  },
  links(slug: string, q: { limit: number; offset: number }) {
    return tenantRequest<Page<WhatsAppChatLink>>(
      slug,
      "GET",
      "/v1/tenant/whatsapp/chat-links",
      { query: q },
    );
  },
  deleteLink(slug: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/whatsapp/chat-links/${encodeURIComponent(uuid)}`,
    );
  },
  /** Görevsiz kaydedilen belgeyi indirir (BFF oturumu ekler). */
  async downloadFile(slug: string, capture: WhatsAppCapture) {
    const apiBase = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${apiBase}/v1/tenant/engagements/${encodeURIComponent(capture.taxpayer.engagement_uuid)}/whatsapp/captures/${encodeURIComponent(capture.uuid)}/file`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`download ${response.status}`);
    download(await response.blob(), capture.file_name || "belge");
  },
};
