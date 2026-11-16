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
export type AccountPlanAccountPage =
  components["schemas"]["EnvelopeAccountPlanAccountPage"]["data"];

/** Server page size cap for list endpoints (`apiquery.MaxLimit`). */
export const REVIEW_PAGE_MAX = 100;

export type ReviewListFilter = {
  /** Empty = the office-wide queue; otherwise one engagement. */
  engagement?: string;
  status?: ReviewSuggestionStatus | "all";
  kind?: string;
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
        limit: filter.limit,
        offset: filter.offset,
      },
    });
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
  settings(slug: string) {
    return tenantRequest<ReviewSettings>(
      slug,
      "GET",
      "/v1/tenant/review/settings",
      { silent: true },
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
