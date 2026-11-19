import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ApiDataWarning = components["schemas"]["DataWarning"];
export type ApiDataWarningPage =
  components["schemas"]["EnvelopeDataWarningPage"]["data"];
export type ApiWarningSeverity = components["schemas"]["WarningSeverity"];
export type ApiWarningStatus = ApiDataWarning["status"];
export type ApiWarningRule = components["schemas"]["WarningRule"];
export type ApiWarningDecision =
  components["schemas"]["EnvelopeWarningDecision"]["data"];

export type ApiWarningGroup = components["schemas"]["WarningGroup"];
export type WarningGroupQuery = {
  period?: string;
  severity?: ApiWarningSeverity;
  status?: ApiWarningStatus | "all";
};

export type WarningListFilters = {
  period?: string;
  code?: string;
  severity?: ApiWarningSeverity;
  status?: ApiWarningStatus;
  entity_type?: string;
};

export type WarningListQuery = WarningListFilters & {
  limit: number;
  offset: number;
};

export const warningKeys = {
  /** Prefix of every warning list of the office (invalidate after a decision). */
  all: (slug: string) => ["tenant", slug.toLowerCase(), "warnings"] as const,
  office: (slug: string, query: WarningListQuery) =>
    [...warningKeys.all(slug), "office", query] as const,
  engagement: (slug: string, engagement: string, query: WarningListQuery) =>
    [
      ...warningKeys.all(slug),
      "engagement",
      engagement.toLowerCase(),
      query,
    ] as const,
  groups: (slug: string, query: WarningGroupQuery, engagement?: string) =>
    [
      ...warningKeys.all(slug),
      "groups",
      engagement?.toLowerCase() ?? "office",
      query,
    ] as const,
  rules: (slug: string) =>
    ["tenant", slug.toLowerCase(), "warning-rules"] as const,
};

export type WarningDecisionAction = "acknowledge" | "resolve";

/** Warning engine (05-uyari-kodlari.md): office-wide and per-engagement lists. */
export const warningService = {
  listOffice(slug: string, query: WarningListQuery) {
    return tenantRequest<ApiDataWarningPage>(
      slug,
      "GET",
      "/v1/tenant/warnings",
      { query },
    );
  },
  listEngagement(slug: string, engagement: string, query: WarningListQuery) {
    return tenantRequest<ApiDataWarningPage>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/warnings`,
      { query },
    );
  },
  /** Every warning code with its effective configuration (no titles). */
  rules(slug: string) {
    return tenantRequest<{ items: ApiWarningRule[] }>(
      slug,
      "GET",
      "/v1/tenant/warning-rules",
    );
  },
  /** Only an open warning can be decided; `note` is the reason (#529). */
  decide(
    slug: string,
    engagement: string,
    warning: string,
    action: WarningDecisionAction,
    note: string,
  ) {
    return tenantRequest<ApiWarningDecision>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/warnings/${encodeURIComponent(warning)}/${action}`,
      { body: { note }, silent: true },
    );
  },
  /** Koda göre özet (arayüz 6/9, #329): ofis geneli ya da tek engagement. */
  groups(slug: string, query: WarningGroupQuery, engagement?: string) {
    const base = engagement
      ? `/v1/tenant/engagements/${encodeURIComponent(engagement)}/warnings/groups`
      : "/v1/tenant/warnings/groups";
    return tenantRequest<{ items: ApiWarningGroup[] }>(slug, "GET", base, {
      query,
    });
  },
  /** "Tümünü gördüm": bir koddaki bütün açık uyarıları görüldü yapar. */
  acknowledgeBulk(
    slug: string,
    body: { code: string; period?: string; note: string },
    engagement?: string,
  ) {
    const base = engagement
      ? `/v1/tenant/engagements/${encodeURIComponent(engagement)}/warnings/acknowledge-bulk`
      : "/v1/tenant/warnings/acknowledge-bulk";
    return tenantRequest<{ acknowledged: number }>(slug, "POST", base, {
      body,
      silent: true,
    });
  },
};
