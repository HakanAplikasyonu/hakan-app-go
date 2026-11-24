import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type EngagementContacts = components["schemas"]["EngagementContacts"];
export type EngagementContactsInput =
  components["schemas"]["EngagementContactsInput"];

const path = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/contacts`;

export const contactKeys = {
  detail: (slug: string, engagement: string) =>
    ["tenant", slug.toLowerCase(), "engagement-contacts", engagement] as const,
};

/**
 * Mükellefin iletişim bilgisi (#780): telefon, e-posta ve mükellef hesapları
 * (sahibi yetkili kişi). Düzenleme tenant.engagements.write ister.
 */
export const contactService = {
  get(slug: string, engagement: string) {
    return tenantRequest<EngagementContacts>(slug, "GET", path(engagement));
  },
  update(slug: string, engagement: string, body: EngagementContactsInput) {
    return tenantRequest<EngagementContacts>(slug, "PUT", path(engagement), {
      body,
      silent: true,
    });
  },
};
