import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type OfficeInvoice = components["schemas"]["OfficeInvoice"];
export type OfficeInvoicePage = components["schemas"]["OfficeInvoicePage"];
export type OfficeInvoiceDetail = components["schemas"]["OfficeInvoiceDetail"];
export type OfficeInvoiceWarning =
  components["schemas"]["OfficeInvoiceWarning"];
export type UBLUploadJob = components["schemas"]["UBLUploadJob"];
export type UBLUploadItem = components["schemas"]["UBLUploadItem"];
export type EngagementPage = components["schemas"]["EngagementPage"];
export type EngagementTaxpayer = components["schemas"]["Taxpayer"];
export type InvoicePostingResult =
  components["schemas"]["InvoicePostingResult"];
export type InvoicePostingBatch = components["schemas"]["InvoicePostingBatch"];

export type InvoiceFilter = {
  direction?: "outgoing" | "incoming";
  period?: string;
  limit: number;
  offset: number;
};

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

export const officeInvoiceKeys = {
  engagements: (slug: string) =>
    ["tenant", slug.toLowerCase(), "engagements", "active"] as const,
  taxpayer: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "taxpayer",
    ] as const,
  invoices: (slug: string, engagement: string, filter: InvoiceFilter) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "invoices",
      filter,
    ] as const,
  invoice: (slug: string, engagement: string, invoice: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "invoice",
      invoice.toLowerCase(),
    ] as const,
  ubl: (slug: string, engagement: string, invoice: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "invoice",
      invoice.toLowerCase(),
      "ubl",
    ] as const,
  uploads: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "ubl-uploads",
    ] as const,
  upload: (slug: string, engagement: string, job: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "ubl-uploads",
      job.toLowerCase(),
    ] as const,
};

/**
 * Office panel invoices (issue #142): the engagement's invoices (office
 * period plus shared history), one invoice with its lines and warnings, its
 * stored UBL for the preview, and UBL / ZIP uploads.
 */
export const officeInvoiceService = {
  activeEngagements(slug: string) {
    return tenantRequest<EngagementPage>(
      slug,
      "GET",
      "/v1/tenant/engagements",
      {
        query: { status: "active", limit: 100 },
      },
    );
  },
  taxpayer(slug: string, engagement: string) {
    return tenantRequest<EngagementTaxpayer>(
      slug,
      "GET",
      `${eng(engagement)}/taxpayer`,
      { silent: true },
    );
  },
  invoices(slug: string, engagement: string, filter: InvoiceFilter) {
    return tenantRequest<OfficeInvoicePage>(
      slug,
      "GET",
      `${eng(engagement)}/invoices`,
      { query: filter },
    );
  },
  invoice(slug: string, engagement: string, invoice: string) {
    return tenantRequest<OfficeInvoiceDetail>(
      slug,
      "GET",
      `${eng(engagement)}/invoices/${encodeURIComponent(invoice)}`,
    );
  },
  /** Eksik kuru tamamlar (#571); faturanın yeni hâli döner. */
  setExchangeRate(
    slug: string,
    engagement: string,
    invoice: string,
    rate: string,
  ) {
    return tenantRequest<OfficeInvoiceDetail>(
      slug,
      "PUT",
      `${eng(engagement)}/invoices/${encodeURIComponent(invoice)}/exchange-rate`,
      { body: { rate }, silent: true },
    );
  },
  /**
   * KDV indirim dönemi (#640): alış faturasının KDV'si sonraki bir aya alınır;
   * `null` belge ayına döndürür. Faturanın yeni hâli döner.
   */
  setVatDeductionPeriod(
    slug: string,
    engagement: string,
    invoice: string,
    period: string | null,
  ) {
    const path = `${eng(engagement)}/invoices/${encodeURIComponent(invoice)}/vat-deduction-period`;
    return period
      ? tenantRequest<OfficeInvoiceDetail>(slug, "PUT", path, {
          body: { period },
          silent: true,
        })
      : tenantRequest<OfficeInvoiceDetail>(slug, "DELETE", path, {
          silent: true,
        });
  },
  /**
   * Faturadan fiş (#669): dönemin fişi olmayan alış / satış faturaları
   * taslak fişe; satış belge ayında, alış KDV indirim ayında.
   */
  postPeriod(slug: string, engagement: string, period: string) {
    return tenantRequest<InvoicePostingBatch>(
      slug,
      "POST",
      `${eng(engagement)}/invoice-postings`,
      { body: { period }, silent: true },
    );
  },
  /** The stored UBL-TR XML as text (not JSON, so not through tenantRequest). */
  async ubl(slug: string, engagement: string, invoice: string) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}${eng(engagement)}/invoices/${encodeURIComponent(invoice)}/ubl`,
      {
        credentials: "include",
        headers: { Accept: "application/xml", [ORGANIZATION_HEADER]: slug },
      },
    );
    if (!response.ok) throw new Error(`UBL ${response.status}`);
    return response.text();
  },
  uploads(slug: string, engagement: string) {
    return tenantRequest<{ items: UBLUploadJob[] }>(
      slug,
      "GET",
      `${eng(engagement)}/ubl-uploads`,
      { query: { limit: 20 } },
    );
  },
  upload(slug: string, engagement: string, job: string) {
    return tenantRequest<UBLUploadJob>(
      slug,
      "GET",
      `${eng(engagement)}/ubl-uploads/${encodeURIComponent(job)}`,
    );
  },
  /** Multipart upload; the browser sets the boundary header itself. */
  async createUpload(slug: string, engagement: string, file: File) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    const response = await fetch(`${base}${eng(engagement)}/ubl-uploads`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<UBLUploadJob>({ data: payload, response });
  },
};
