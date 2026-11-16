import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ReviewThresholdImpact =
  components["schemas"]["ReviewThresholdImpact"];
export type AccountRuleSettings = components["schemas"]["AccountRuleSettings"];
export type AccountRuleSettingsInput =
  components["schemas"]["AccountRuleSettingsInput"];
export type AccountRuleSettingsPreview =
  components["schemas"]["EnvelopeAccountRuleSettingsPreview"]["data"];
export type ReviewSettingInput = components["schemas"]["ReviewSettingInput"];

const base = (slug: string) =>
  ["tenant", slug.toLowerCase(), "office-settings"] as const;

export const officeSettingsKeys = {
  all: base,
  impact: (slug: string, threshold: string, months: number) =>
    [...base(slug), "threshold-impact", threshold, months] as const,
  rules: (slug: string) => [...base(slug), "account-rule-settings"] as const,
  rulesPreview: (slug: string, input: AccountRuleSettingsInput) =>
    [...base(slug), "account-rule-settings", "preview", input] as const,
};

/**
 * Ayarlar → Yapay zekâ ve onay (#330): otomatik onay eşiği ve geçmişe dönük
 * etkisi (#333), kural adayı eşiği ve etkisi (#333).
 */
export const officeSettingsService = {
  putReview(slug: string, input: ReviewSettingInput) {
    return tenantRequest<unknown>(slug, "PUT", "/v1/tenant/review/settings", {
      body: input,
    });
  },
  thresholdImpact(slug: string, threshold: string, months: number) {
    return tenantRequest<ReviewThresholdImpact>(
      slug,
      "GET",
      "/v1/tenant/review/threshold-impact",
      { query: { threshold, months, kind: "account_code" }, silent: true },
    );
  },
  ruleSettings(slug: string) {
    return tenantRequest<AccountRuleSettings>(
      slug,
      "GET",
      "/v1/tenant/account-rule-settings",
      { silent: true },
    );
  },
  putRuleSettings(slug: string, input: AccountRuleSettingsInput) {
    return tenantRequest<AccountRuleSettings>(
      slug,
      "PUT",
      "/v1/tenant/account-rule-settings",
      { body: input },
    );
  },
  previewRuleSettings(slug: string, input: AccountRuleSettingsInput) {
    return tenantRequest<AccountRuleSettingsPreview>(
      slug,
      "GET",
      "/v1/tenant/account-rule-settings/preview",
      { query: { ...input }, silent: true },
    );
  },
};
