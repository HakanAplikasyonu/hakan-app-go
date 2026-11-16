import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Page = { total: number };
export type VATForecast = components["schemas"]["VATForecast"];
type VATForecastPage =
  components["schemas"]["EnvelopeVATForecastOfficePage"]["data"];

export const officeSummaryKeys = {
  pendingSuggestions: (slug: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "office-summary",
      "pending-suggestions",
    ] as const,
  openBlockers: (slug: string) =>
    ["tenant", slug.toLowerCase(), "office-summary", "open-blockers"] as const,
  vatForecast: (slug: string, period?: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "office-summary",
      "vat-forecast",
      period ?? "",
    ] as const,
};

/**
 * Totals for the office home. Each asks the list endpoint for one row and
 * reads `total`, so the numbers match what the list screens show.
 */
export const officeSummaryService = {
  async pendingSuggestions(slug: string) {
    const page = await tenantRequest<Page>(
      slug,
      "GET",
      "/v1/tenant/review/suggestions",
      {
        query: { status: "pending", limit: 1 },
        silent: true,
      },
    );
    return page.total;
  },
  async openBlockers(slug: string) {
    const page = await tenantRequest<Page>(slug, "GET", "/v1/tenant/warnings", {
      query: { status: "open", severity: "blocker", limit: 1 },
      silent: true,
    });
    return page.total;
  },
  vatForecast(slug: string, period?: string) {
    return tenantRequest<VATForecastPage>(
      slug,
      "GET",
      "/v1/tenant/vat-forecast",
      {
        query: { period, limit: 100 },
        silent: true,
      },
    );
  },
};
