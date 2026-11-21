import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type SmsSettings = components["schemas"]["SmsSettings"];
export type SmsSettingsInput = components["schemas"]["SmsSettingsInput"];
export type SmsMessage = components["schemas"]["SmsMessage"];

export const smsKeys = {
  settings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "sms-settings"] as const,
  messages: (slug: string) =>
    ["tenant", slug.toLowerCase(), "sms-messages"] as const,
};

/**
 * İletişim merkezi (#780 A): ofisin NetGSM hesabı, bağlantı testi, deneme
 * SMS'i ve son gönderim kayıtları. Şifre sunucudan hiç gelmez; yalnız
 * `password_set`.
 */
export const smsService = {
  settings(slug: string) {
    return tenantRequest<SmsSettings>(slug, "GET", "/v1/tenant/sms-settings", {
      silent: true,
    });
  },
  save(slug: string, body: SmsSettingsInput) {
    return tenantRequest<SmsSettings>(slug, "PUT", "/v1/tenant/sms-settings", {
      body,
    });
  },
  test(slug: string) {
    return tenantRequest<SmsSettings>(
      slug,
      "POST",
      "/v1/tenant/sms-settings/test",
    );
  },
  messages(slug: string, limit = 10) {
    return tenantRequest<{ items: SmsMessage[] }>(
      slug,
      "GET",
      "/v1/tenant/sms-messages",
      { query: { limit: String(limit) }, silent: true },
    );
  },
  sendTest(slug: string, phone: string, text: string) {
    return tenantRequest<SmsMessage>(
      slug,
      "POST",
      "/v1/tenant/sms-messages/test",
      { body: { phone, text } },
    );
  },
};
