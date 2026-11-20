import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type BankTransactionMatching =
  components["schemas"]["BankTransactionMatching"];
export type BankMatchCandidate = components["schemas"]["BankMatchCandidate"];
export type BankMatchJournalLineCandidate =
  components["schemas"]["BankMatchJournalLineCandidate"];
export type BankMatchedJournalLine =
  components["schemas"]["BankMatchedJournalLine"];
/** Fiş satırı seçimi: fiş + satır no (#669 G). */
export type BankMatchLineRef = { journal_draft_uuid: string; line_no: number };
export type BankTransactionHistoryEntry =
  components["schemas"]["BankTransactionHistoryEntry"];

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
  history: (slug: string, engagement: string, transaction: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "bank-matching",
      engagement,
      transaction,
      "history",
    ] as const,
  search: (slug: string, engagement: string, transaction: string, q: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "bank-matching",
      engagement,
      transaction,
      "search",
      q,
    ] as const,
  lineSearch: (
    slug: string,
    engagement: string,
    transaction: string,
    q: string,
    cari: boolean,
  ) =>
    [
      "tenant",
      slug.toLowerCase(),
      "bank-matching",
      engagement,
      transaction,
      "line-search",
      q,
      cari,
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
  /**
   * Hareketin eşleşmelerini seçilen faturalar ve / veya fiş satırlarıyla
   * tek adımda değiştirir (#669 G): yeniden eşleştirmede önce geri almak
   * gerekmez.
   */
  match(
    slug: string,
    engagement: string,
    transaction: string,
    invoiceUuids: string[],
    journalLines: BankMatchLineRef[] = [],
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/matches`,
      {
        body:
          journalLines.length > 0
            ? { invoice_uuids: invoiceUuids, journal_lines: journalLines }
            : { invoice_uuids: invoiceUuids },
      },
    );
  },
  /**
   * Mevcut fiş satırı / cari hareketi araması (#669 G). Terim boşsa
   * tutarı tutan satırlar; cari: yalnız cari ana hesaplar (120, 320 …).
   */
  searchLines(
    slug: string,
    engagement: string,
    transaction: string,
    q: string,
    cari: boolean,
  ) {
    return tenantRequest<{ items: BankMatchJournalLineCandidate[] }>(
      slug,
      "GET",
      `${tx(engagement, transaction)}/journal-line-search`,
      { query: { q, cari: cari ? "1" : "0" } },
    );
  },
  /** Hareketi kim, ne zaman eşleştirdi / geri aldı / hesaba işledi (#565). */
  history(slug: string, engagement: string, transaction: string) {
    return tenantRequest<{ items: BankTransactionHistoryEntry[] }>(
      slug,
      "GET",
      `${tx(engagement, transaction)}/history`,
    );
  },
  /** Elle eşleştirme için açık fatura araması (#565). */
  search(slug: string, engagement: string, transaction: string, q: string) {
    return tenantRequest<{ items: BankMatchCandidate[] }>(
      slug,
      "GET",
      `${tx(engagement, transaction)}/invoice-search`,
      { query: { q } },
    );
  },
  /**
   * Hareketi seçilen mizan hesabına işler (#565): cari, avans, gider…
   * Dönemin banka uyarıları yenilenir.
   */
  setAccount(
    slug: string,
    engagement: string,
    transaction: string,
    accountCode: string,
    accountName: string,
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/account`,
      { body: { account_code: accountCode, account_name: accountName } },
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
