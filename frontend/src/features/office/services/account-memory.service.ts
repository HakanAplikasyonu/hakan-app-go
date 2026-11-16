import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type AccountPlanImport = components["schemas"]["AccountPlanImport"];
export type AccountPlanImportResult =
  components["schemas"]["EnvelopeAccountPlanImportResult"]["data"];
export type AccountRuleCandidate =
  components["schemas"]["AccountRuleCandidate"];
export type AccountRulePromoteInput =
  components["schemas"]["AccountRulePromoteInput"];
export type AccountRule = components["schemas"]["AccountRule"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

const base = (slug: string, engagement: string) =>
  [
    "tenant",
    slug.toLowerCase(),
    "engagements",
    engagement.toLowerCase(),
    "account-memory",
  ] as const;

export const accountMemoryKeys = {
  all: (slug: string, engagement: string) => base(slug, engagement),
  imports: (slug: string, engagement: string) =>
    [...base(slug, engagement), "imports"] as const,
  candidates: (slug: string, engagement: string) =>
    [...base(slug, engagement), "candidates"] as const,
  rules: (slug: string, engagement: string) =>
    [...base(slug, engagement), "rules"] as const,
};

/**
 * Mizan hafızası (#310): the taxpayer's chart of accounts (Luca mizan import),
 * rule candidates learned from the office's decisions and the rules promoted
 * from them.
 */
export const accountMemoryService = {
  imports(slug: string, engagement: string) {
    return tenantRequest<{ items: AccountPlanImport[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/imports`,
    );
  },
  /** Multipart upload; the browser sets the boundary header itself. */
  async importPlan(slug: string, engagement: string, file: File) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    form.append("target", "luca");
    const response = await fetch(
      `${root}${eng(engagement)}/account-plan/imports`,
      {
        method: "POST",
        credentials: "include",
        headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
        body: form,
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<AccountPlanImportResult>({ data: payload, response });
  },
  candidates(slug: string, engagement: string) {
    return tenantRequest<{ items: AccountRuleCandidate[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-rule-candidates`,
    );
  },
  promote(slug: string, engagement: string, input: AccountRulePromoteInput) {
    return tenantRequest<AccountRule>(
      slug,
      "POST",
      `${eng(engagement)}/account-rule-candidates/promote`,
      { body: input },
    );
  },
  /** Retires the rule (versions stay in history). */
  retireRule(slug: string, engagement: string, ruleKey: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${eng(engagement)}/account-rules/${encodeURIComponent(ruleKey)}`,
    );
  },
  rules(slug: string, engagement: string) {
    return tenantRequest<{ items: AccountRule[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-rules`,
    );
  },
};
