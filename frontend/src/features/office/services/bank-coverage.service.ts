import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type BankStatementStatus = components["schemas"]["BankStatementStatus"];
export type BankStatementAccountStatus =
  components["schemas"]["BankStatementAccountStatus"];
export type BankStatementMonth = components["schemas"]["BankStatementMonth"];
export type BankStatementGaps = components["schemas"]["BankStatementGaps"];
export type BankStatementGap = components["schemas"]["BankStatementGap"];
export type BankStatementRequestInput =
  components["schemas"]["BankStatementRequestInput"];
export type BankStatementRequestResult =
  components["schemas"]["BankStatementRequestResult"];

/** organizations.settings anahtarı; yoksa açık (backend ile aynı). */
export const BANK_STATEMENT_TRACKING_SETTING = "bank_statement_tracking";

/** Sorgu anahtarları: hesap durumu ve ofis yapılacakları. */
export const bankCoverageKeys = {
  status: (slug: string, engagement: string) => [
    "tenant",
    slug.toLowerCase(),
    "bank-statement-status",
    engagement,
  ],
  gaps: (slug: string) => ["tenant", slug.toLowerCase(), "bank-statement-gaps"],
};

/**
 * Eksik ekstre takibi (Oğuz'un geri bildirimi): mükellefin kullanımdaki
 * banka hesaplarında kapanmış her ay ekstre geldi mi, eksik ay için hazır
 * başlıklı belge isteği ve ofisin "Eksik ekstreler" listesi.
 */
export const bankCoverageService = {
  status(slug: string, engagement: string) {
    return tenantRequest<BankStatementStatus>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-statement-status`,
      { silent: true },
    );
  },
  request(slug: string, engagement: string, body: BankStatementRequestInput) {
    return tenantRequest<BankStatementRequestResult>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-statement-status/requests`,
      { body, silent: true },
    );
  },
  mark(
    slug: string,
    engagement: string,
    body: { cash_box_uuid: string; period: string; received: boolean },
  ) {
    return tenantRequest<BankStatementStatus>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-statement-status/marks`,
      { body, silent: true },
    );
  },
  gaps(slug: string, limit: number, offset: number) {
    return tenantRequest<BankStatementGaps>(
      slug,
      "GET",
      "/v1/tenant/bank-statement-gaps",
      { query: { limit, offset }, silent: true },
    );
  },
};
