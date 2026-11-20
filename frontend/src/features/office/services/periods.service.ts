import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type PeriodMatrix =
  components["schemas"]["EnvelopePeriodMatrix"]["data"];
export type PeriodMatrixRow = components["schemas"]["PeriodMatrixRow"];
export type PeriodMatrixCell = components["schemas"]["PeriodMatrixCell"];

export type PeriodMatrixQuery = {
  until?: string;
  periods?: number;
  limit?: number;
  offset?: number;
};

/** The API caps one page of the matrix at 100 engagements. */
export const MATRIX_PAGE = 100;

export const periodKeys = {
  matrix: (slug: string, query: PeriodMatrixQuery) =>
    ["tenant", slug.toLowerCase(), "periods", "matrix", query] as const,
  matrixAll: (
    slug: string,
    query: Omit<PeriodMatrixQuery, "limit" | "offset">,
  ) =>
    ["tenant", slug.toLowerCase(), "periods", "matrix", "all", query] as const,
};

/**
 * Period management (issue #32): active engagements of the office (assigned
 * ones only without `tenant.engagements.read_all`) × the last N months, with
 * readiness and lock state. It is also the one office-wide list that carries
 * the taxpayer title next to each engagement.
 */
export const periodService = {
  /**
   * Kilitli dönemi gerekçeyle açar (#669 A); gerekçe dönemin kilit
   * geçmişine yazılır. `tenant.periods.lock` ister.
   */
  unlock(slug: string, engagement: string, period: string, reason: string) {
    return tenantRequest<unknown>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/periods/${encodeURIComponent(period)}/unlock`,
      { body: { reason } },
    );
  },
  matrix(slug: string, query: PeriodMatrixQuery) {
    return tenantRequest<PeriodMatrix>(
      slug,
      "GET",
      "/v1/tenant/periods/matrix",
      {
        query,
      },
    );
  },
  /** Every page of the matrix (offices are at most a few hundred taxpayers). */
  async matrixAll(
    slug: string,
    query: Omit<PeriodMatrixQuery, "limit" | "offset">,
  ): Promise<PeriodMatrix> {
    const first = await periodService.matrix(slug, {
      ...query,
      limit: MATRIX_PAGE,
      offset: 0,
    });
    const items = [...first.items];
    for (
      let offset = MATRIX_PAGE;
      offset < first.total;
      offset += MATRIX_PAGE
    ) {
      const page = await periodService.matrix(slug, {
        ...query,
        limit: MATRIX_PAGE,
        offset,
      });
      items.push(...page.items);
    }
    return { ...first, items, limit: items.length, offset: 0 };
  },
};
