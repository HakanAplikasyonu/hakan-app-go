import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type SupportSettings = components["schemas"]["SupportSettings"];
export type ClickFixIdentityResponse =
  components["schemas"]["ClickFixIdentity"];

export const supportKeys = {
  settings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "support-settings"] as const,
  identity: (slug: string) =>
    ["tenant", slug.toLowerCase(), "support", "clickfix-identity"] as const,
};

/** Destek widget'ı ofis ayarı ve ofis kullanıcısının ClickFix kimliği (#387). */
export const supportService = {
  settings(slug: string) {
    return tenantRequest<SupportSettings>(
      slug,
      "GET",
      "/v1/tenant/support-settings",
      { silent: true },
    );
  },
  putSettings(slug: string, clickfixWidgetId: string) {
    return tenantRequest<SupportSettings>(
      slug,
      "PUT",
      "/v1/tenant/support-settings",
      { body: { clickfix_widget_id: clickfixWidgetId } },
    );
  },
  identity(slug: string) {
    return tenantRequest<ClickFixIdentityResponse>(
      slug,
      "GET",
      "/v1/tenant/support/clickfix-identity",
      { silent: true },
    );
  },
};
