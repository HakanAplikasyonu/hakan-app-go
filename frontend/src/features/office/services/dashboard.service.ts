import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type OfficeDashboard = components["schemas"]["OfficeDashboard"];
export type OfficeDashboardUpload =
  components["schemas"]["OfficeDashboardUpload"];

export const dashboardKeys = {
  office: (slug: string) =>
    ["tenant", slug.toLowerCase(), "dashboard"] as const,
};

/**
 * Ofis panosu sayaçları (`GET /v1/tenant/dashboard`): bekleyen öneri, belge
 * görevi, açık uyarılar (seviyeye göre), bugün gelenler ve son yüklemeler.
 * Okuma izni olmayan bölüm `null` gelir.
 */
export const dashboardService = {
  office(slug: string) {
    return tenantRequest<OfficeDashboard>(slug, "GET", "/v1/tenant/dashboard", {
      silent: true,
    });
  },
};
