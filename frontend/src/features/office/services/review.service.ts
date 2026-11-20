import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ReviewSuggestion = components["schemas"]["Suggestion"];
export type ReviewSuggestionStatus = ReviewSuggestion["status"];
export type ReviewSuggestionDecision =
  components["schemas"]["SuggestionDecision"];
export type ReviewSuggestionDetail = ReviewSuggestion & {
  decisions: ReviewSuggestionDecision[];
};
export type ReviewSuggestionPage =
  components["schemas"]["EnvelopeSuggestionPage"]["data"];
export type ReviewSettings = components["schemas"]["ReviewSettings"];
export type AccountPlanAccount = components["schemas"]["AccountPlanAccount"];
export type CounterpartyCodeUse = components["schemas"]["CounterpartyCodeUse"];
export type AccountSuggestionList =
  components["schemas"]["EnvelopeAccountSuggestionList"]["data"];
export type AccountPlanAccountPage =
  components["schemas"]["EnvelopeAccountPlanAccountPage"]["data"];

/** Server page size cap for list endpoints (`apiquery.MaxLimit`). */
export const REVIEW_PAGE_MAX = 100;

export type ReviewListFilter = {
  /** Empty = the office-wide queue; otherwise one engagement. */
  engagement?: string;
  status?: ReviewSuggestionStatus | "all";
  kind?: string;
  /** Global dönem seçici (#326): YYYY-MM; boş = bütün dönemler. */
  period?: string;
  limit: number;
  offset: number;
};

export type ReviewAction = "accept" | "reject" | "correct" | "revert";

const enc = encodeURIComponent;

export const reviewKeys = {
  /** Prefix of every review query of the organization (invalidation). */
  all: (slug: string) => ["tenant", slug.toLowerCase(), "review"] as const,
  list: (slug: string, filter: ReviewListFilter) =>
    [...reviewKeys.all(slug), "suggestions", filter] as const,
  detail: (slug: string, engagement: string, uuid: string) =>
    [
      ...reviewKeys.all(slug),
      "suggestion",
      engagement.toLowerCase(),
      uuid.toLowerCase(),
    ] as const,
  settings: (slug: string) => [...reviewKeys.all(slug), "settings"] as const,
  history: (slug: string, engagement: string, counterparty: string) =>
    [
      ...reviewKeys.all(slug),
      "history",
      engagement.toLowerCase(),
      counterparty,
    ] as const,
  engagementSettings: (slug: string, engagement: string) =>
    [...reviewKeys.all(slug), "settings", engagement] as const,
  accounts: (slug: string, engagement: string, q: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "account-plan",
      "accounts",
      q,
    ] as const,
};

function suggestionPath(engagement: string, uuid: string) {
  return `/v1/tenant/engagements/${enc(engagement)}/suggestions/${enc(uuid)}`;
}

/**
 * Review queue (W4): suggestions waiting for an office decision. Office-wide
 * list is permission-scoped on the server; decisions go through the
 * engagement-scoped routes.
 */
export const reviewService = {
  list(slug: string, filter: ReviewListFilter) {
    const path = filter.engagement
      ? `/v1/tenant/engagements/${enc(filter.engagement)}/suggestions`
      : "/v1/tenant/review/suggestions";
    return tenantRequest<ReviewSuggestionPage>(slug, "GET", path, {
      query: {
        status: filter.status,
        kind: filter.kind,
        period: filter.period,
        limit: filter.limit,
        offset: filter.offset,
      },
    });
  },
  /** Karşı tarafın geçmiş satır kodları, bütün kararlardan (#669 L). */
  history(slug: string, engagement: string, counterparty: string) {
    return tenantRequest<{ items: CounterpartyCodeUse[] }>(
      slug,
      "GET",
      `/v1/tenant/engagements/${enc(engagement)}/suggestions/history`,
      { query: { counterparty, limit: 4 } },
    );
  },
  get(slug: string, engagement: string, uuid: string) {
    return tenantRequest<ReviewSuggestionDetail>(
      slug,
      "GET",
      suggestionPath(engagement, uuid),
    );
  },
  decide(
    slug: string,
    engagement: string,
    uuid: string,
    action: ReviewAction,
    body: { reason?: string; value?: unknown },
  ) {
    return tenantRequest<ReviewSuggestion>(
      slug,
      "POST",
      `${suggestionPath(engagement, uuid)}/${action}`,
      { body, silent: true },
    );
  },
  /** Mükellefe özel eşik (#518 A5): ofis ayarını ezer. */
  engagementSettings(slug: string, engagement: string) {
    return tenantRequest<ReviewSettings>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/review-settings`,
    );
  },
  putEngagementSetting(
    slug: string,
    engagement: string,
    input: {
      kind?: string;
      auto_accept: boolean | null;
      threshold: string | null;
    },
  ) {
    return tenantRequest<ReviewSettings>(
      slug,
      "PUT",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/review-settings`,
      { body: input, silent: true },
    );
  },
  deleteEngagementSetting(slug: string, engagement: string, kind = "*") {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/review-settings/${encodeURIComponent(kind)}`,
      { silent: true },
    );
  },
  settings(slug: string) {
    return tenantRequest<ReviewSettings>(
      slug,
      "GET",
      "/v1/tenant/review/settings",
      { silent: true },
    );
  },
  /** Runs the account-code rules for an invoice (#160); results join the queue. */
  suggestInvoice(slug: string, engagement: string, invoice: string) {
    return tenantRequest<AccountSuggestionList>(
      slug,
      "POST",
      `/v1/tenant/engagements/${enc(engagement)}/invoices/${enc(invoice)}/account-suggestions`,
      { body: {}, silent: true },
    );
  },
  accounts(slug: string, engagement: string, q: string) {
    return tenantRequest<AccountPlanAccountPage>(
      slug,
      "GET",
      `/v1/tenant/engagements/${enc(engagement)}/account-plan/accounts`,
      { query: { q, limit: 50 }, silent: true },
    );
  },
};
