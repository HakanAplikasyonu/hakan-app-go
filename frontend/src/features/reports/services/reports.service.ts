import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { parseApiError } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";
import { taxpayerRequest } from "@/features/taxpayer/services/taxpayer.service";

export type TaxpayerReport = components["schemas"]["TaxpayerReport"];
export type ReportMonth = components["schemas"]["ReportMonth"];
export type ReportCategory = components["schemas"]["ReportCategory"];
export type ReportTotals = components["schemas"]["ReportTotals"];
export type ReportDelivery = components["schemas"]["ReportDelivery"];
export type ReportDeliveryPage =
  components["schemas"]["EnvelopeReportDeliveryPage"]["data"];
export type ReportBulkResult =
  components["schemas"]["EnvelopeReportBulkResult"]["data"];
export type PageParams = { limit: number; offset: number };
export type ReportSchedule = components["schemas"]["ReportSchedule"];
export type ReportScheduleInput = {
  enabled: boolean;
  day_of_month: number;
  recipient?: string;
};

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
  deliveries: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "report-deliveries",
    ] as const,
  schedule: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "report-schedule",
    ] as const,
  officeDeliveries: (slug: string) =>
    ["tenant", slug.toLowerCase(), "report-deliveries"] as const,
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
  /** The PDF needs `X-Organization`: fetched as a Blob, not a plain link. */
  pdf(slug: string, engagement: string, range: ReportRange): Promise<Blob> {
    return withStepUpRetry(async () => {
      const base = apiConfig.baseUrl.replace(/\/$/, "");
      const q = new URLSearchParams(rangeQuery(range));
      const url = `${base}/v1/tenant/engagements/${encodeURIComponent(engagement)}/report/pdf?${q.toString()}`;
      const response = await fetch(url, {
        credentials: "include",
        headers: { Accept: "application/pdf", [ORGANIZATION_HEADER]: slug },
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => undefined);
        throw parseApiError(response.status, payload);
      }
      return response.blob();
    });
  },
  send(
    slug: string,
    engagement: string,
    range: ReportRange,
    recipient?: string,
  ) {
    return tenantRequest<ReportDelivery>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/report/deliveries`,
      {
        body: {
          ...rangeQuery(range),
          ...(recipient ? { recipient } : {}),
        },
        silent: true,
      },
    );
  },
  deliveries(slug: string, engagement: string, page: PageParams) {
    return tenantRequest<ReportDeliveryPage>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/report/deliveries`,
      { query: page },
    );
  },
  officeDeliveries(slug: string, page: PageParams) {
    return tenantRequest<ReportDeliveryPage>(
      slug,
      "GET",
      "/v1/tenant/report-deliveries",
      { query: page },
    );
  },
  bulk(slug: string, engagements: string[], range: ReportRange) {
    return tenantRequest<ReportBulkResult>(
      slug,
      "POST",
      "/v1/tenant/report-deliveries",
      { body: { engagement_uuids: engagements, ...rangeQuery(range) } },
    );
  },
  schedule(slug: string, engagement: string) {
    return tenantRequest<ReportSchedule>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/report/schedule`,
    );
  },
  saveSchedule(slug: string, engagement: string, body: ReportScheduleInput) {
    return tenantRequest<ReportSchedule>(
      slug,
      "PUT",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/report/schedule`,
      { body, silent: true },
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
