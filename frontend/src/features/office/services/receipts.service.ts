import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

import { downloadFile as download } from "./download";

export type Expense = components["schemas"]["Expense"];
export type ExpensePage = components["schemas"]["EnvelopeExpensePage"]["data"];
export type ExpenseCategory = components["schemas"]["ExpenseCategory"];
export type ExpenseCategoryInput =
  components["schemas"]["ExpenseCategoryInput"];
export type ExpensePostingInput = components["schemas"]["ExpensePostingInput"];
export type RetailSale = components["schemas"]["RetailSale"];
export type RetailSalePage =
  components["schemas"]["EnvelopeRetailSalePage"]["data"];
export type RetailSaleInput = components["schemas"]["RetailSaleInput"];
export type ReceiptImportResult =
  components["schemas"]["EnvelopeReceiptImportResult"]["data"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

const base = (slug: string, engagement: string) =>
  [
    "tenant",
    slug.toLowerCase(),
    "engagements",
    engagement.toLowerCase(),
  ] as const;

export const receiptKeys = {
  expenses: (slug: string, engagement: string) =>
    [...base(slug, engagement), "expenses"] as const,
  categories: (slug: string, engagement: string) =>
    [...base(slug, engagement), "expense-categories"] as const,
  retail: (slug: string, engagement: string) =>
    [...base(slug, engagement), "retail-sales"] as const,
};

export type ExpenseQuery = {
  period?: string;
  status?: string;
  category?: string;
  q?: string;
  limit: number;
  offset: number;
};

export type RetailQuery = {
  period?: string;
  status?: string;
  okc?: string;
  limit: number;
  offset: number;
};

/** Multipart upload; the browser sets the boundary header itself. */
async function upload(slug: string, path: string, file: File) {
  const root = apiConfig.baseUrl.replace(/\/$/, "");
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${root}${path}`, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
    body: form,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<ReceiptImportResult>({ data: payload, response });
}

/**
 * Masraflar, masraf kategorileri ve Z raporları (#384): ofis panelinde
 * mükellef bazında liste, kategori / hesap seçimi, onay (yevmiye taslağı),
 * Excel içe / dışa aktarma.
 */
export const receiptsService = {
  expenses(slug: string, engagement: string, q: ExpenseQuery) {
    return tenantRequest<ExpensePage>(
      slug,
      "GET",
      `${eng(engagement)}/expenses`,
      { query: q },
    );
  },
  setPosting(
    slug: string,
    engagement: string,
    expense: string,
    input: ExpensePostingInput,
  ) {
    return tenantRequest<Expense>(
      slug,
      "PATCH",
      `${eng(engagement)}/expenses/${encodeURIComponent(expense)}`,
      {
        body: input,
        silent: true,
      },
    );
  },
  decideExpense(
    slug: string,
    engagement: string,
    expense: string,
    approve: boolean,
  ) {
    return tenantRequest<Expense>(
      slug,
      "POST",
      `${eng(engagement)}/expenses/${encodeURIComponent(expense)}/${approve ? "approve" : "reject"}`,
      { silent: true },
    );
  },
  categories(slug: string, engagement: string) {
    return tenantRequest<{ items: ExpenseCategory[] }>(
      slug,
      "GET",
      `${eng(engagement)}/expense-categories`,
    );
  },
  saveCategory(
    slug: string,
    engagement: string | null,
    input: ExpenseCategoryInput,
  ) {
    const path = engagement
      ? `${eng(engagement)}/expense-categories`
      : "/v1/tenant/expense-categories";
    return tenantRequest<ExpenseCategory>(slug, "PUT", path, {
      body: input,
      silent: true,
    });
  },
  deleteCategory(slug: string, engagement: string | null, category: string) {
    const path = engagement
      ? `${eng(engagement)}/expense-categories/${encodeURIComponent(category)}`
      : `/v1/tenant/expense-categories/${encodeURIComponent(category)}`;
    return tenantRequest<void>(slug, "DELETE", path, { silent: true });
  },
  exportExpenses(
    slug: string,
    engagement: string,
    q: Omit<ExpenseQuery, "limit" | "offset">,
  ) {
    return download(
      slug,
      `${eng(engagement)}/expenses/export`,
      q,
      "masraflar.xlsx",
    );
  },
  importExpenses(slug: string, engagement: string, file: File) {
    return upload(slug, `${eng(engagement)}/expenses/import`, file);
  },
  retailSales(slug: string, engagement: string, q: RetailQuery) {
    return tenantRequest<RetailSalePage>(
      slug,
      "GET",
      `${eng(engagement)}/retail-sales`,
      { query: q },
    );
  },
  updateRetailSale(
    slug: string,
    engagement: string,
    sale: string,
    input: RetailSaleInput,
  ) {
    return tenantRequest<RetailSale>(
      slug,
      "PATCH",
      `${eng(engagement)}/retail-sales/${encodeURIComponent(sale)}`,
      {
        body: input,
        silent: true,
      },
    );
  },
  decideRetailSale(
    slug: string,
    engagement: string,
    sale: string,
    approve: boolean,
  ) {
    return tenantRequest<RetailSale>(
      slug,
      "POST",
      `${eng(engagement)}/retail-sales/${encodeURIComponent(sale)}/${approve ? "approve" : "reject"}`,
      { silent: true },
    );
  },
  runContinuity(slug: string, engagement: string) {
    return tenantRequest<{ reports_with_findings: number }>(
      slug,
      "POST",
      `${eng(engagement)}/retail-sales/continuity`,
      { silent: true },
    );
  },
  exportRetailSales(
    slug: string,
    engagement: string,
    q: Omit<RetailQuery, "limit" | "offset">,
  ) {
    return download(
      slug,
      `${eng(engagement)}/retail-sales/export`,
      q,
      "z-raporlari.xlsx",
    );
  },
  importRetailSales(slug: string, engagement: string, file: File) {
    return upload(slug, `${eng(engagement)}/retail-sales/import`, file);
  },
};
