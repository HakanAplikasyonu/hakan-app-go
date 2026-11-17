import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { taxpayerRequest } from "@/features/taxpayer/services/taxpayer.service";

export type TaxpayerReport = components["schemas"]["TaxpayerReport"];
export type ReportMonth = components["schemas"]["ReportMonth"];
export type ReportCategory = components["schemas"]["ReportCategory"];
export type ReportTotals = components["schemas"]["ReportTotals"];

/** Bir ay (YYYY-MM) ya da bir takvim yılı (YYYY). */
export type ReportRange =
  { kind: "month"; period: string } | { kind: "year"; year: string };

function rangeQuery(range: ReportRange): Record<string, string> {
  return range.kind === "month"
    ? { period: range.period }
    : { year: range.year };
}

function rangeKey(range: ReportRange) {
  return range.kind === "month" ? range.period : range.year;
}

export const reportKeys = {
  office: (slug: string, engagement: string, range: ReportRange) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "report",
      rangeKey(range),
    ] as const,
  mine: (taxpayer: string, range: ReportRange) =>
    ["taxpayer", taxpayer.toLowerCase(), "report", rangeKey(range)] as const,
};

/** Mükellef raporu (#386): ofis ve mükellef paneli aynı veriyi okur. */
export const reportService = {
  office(slug: string, engagement: string, range: ReportRange) {
    return tenantRequest<TaxpayerReport>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/report`,
      { query: rangeQuery(range) },
    );
  },
  mine(taxpayer: string, range: ReportRange) {
    const q = new URLSearchParams(rangeQuery(range));
    return taxpayerRequest<TaxpayerReport>(
      "GET",
      `/v1/taxpayer/report?${q.toString()}`,
      { taxpayer },
    );
  },
};
