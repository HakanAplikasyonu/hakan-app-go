import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type BankTransactionMatching =
  components["schemas"]["BankTransactionMatching"];

const tx = (engagement: string, transaction: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-transactions/${encodeURIComponent(transaction)}`;

export const bankMatchingKeys = {
  one: (slug: string, engagement: string, transaction: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "bank-matching",
      engagement,
      transaction,
    ] as const,
};

/**
 * Tek banka hareketi ↔ fatura eşleştirme (#528): hareket, eşleşmeleri,
 * puanlı aday faturalar; elle eşleştir ve geri al.
 */
export const bankMatchingService = {
  get(slug: string, engagement: string, transaction: string) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "GET",
      `${tx(engagement, transaction)}/matching`,
    );
  },
  match(
    slug: string,
    engagement: string,
    transaction: string,
    invoiceUuids: string[],
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/matches`,
      { body: { invoice_uuids: invoiceUuids } },
    );
  },
  unmatch(slug: string, engagement: string, transaction: string) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "DELETE",
      `${tx(engagement, transaction)}/matches`,
    );
  },
};
