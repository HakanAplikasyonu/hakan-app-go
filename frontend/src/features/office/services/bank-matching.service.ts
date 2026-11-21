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

/**
 * Elle fatura araması süzgeci (#565): terim (fatura no, unvan, VKN), düzenleme
 * tarihi aralığı ve tutar aralığı; boş olan gönderilmez.
 */
export type BankInvoiceSearch = {
  q?: string;
  from?: string;
  to?: string;
  min?: string;
  max?: string;
};

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
  search: (
    slug: string,
    engagement: string,
    transaction: string,
    filter: BankInvoiceSearch,
  ) =>
    [
      "tenant",
      slug.toLowerCase(),
      "bank-matching",
      engagement,
      transaction,
      "search",
      filter,
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
    payrollRunUuids: string[] = [],
    sgkPremiumUuids: string[] = [],
    financialUuids: string[] = [],
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/matches`,
      {
        body: {
          invoice_uuids: invoiceUuids,
          ...(journalLines.length > 0 ? { journal_lines: journalLines } : {}),
          // Bordroya bağlama (#710): net maaş ödemesi.
          ...(payrollRunUuids.length > 0
            ? { payroll_run_uuids: payrollRunUuids }
            : {}),
          // SGK primine bağlama (#565): aylık prim ödemesi.
          ...(sgkPremiumUuids.length > 0
            ? { sgk_premium_uuids: sgkPremiumUuids }
            : {}),
          // Çek / senede bağlama (#565): entegrasyondan gelen finansal işlem.
          ...(financialUuids.length > 0
            ? { financial_transaction_uuids: financialUuids }
            : {}),
        },
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
  search(
    slug: string,
    engagement: string,
    transaction: string,
    filter: BankInvoiceSearch,
  ) {
    const query: Record<string, string> = {};
    for (const [k, v] of Object.entries(filter)) if (v) query[k] = v;
    return tenantRequest<{ items: BankMatchCandidate[] }>(
      slug,
      "GET",
      `${tx(engagement, transaction)}/invoice-search`,
      { query },
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
  /**
   * POS komisyonu (#565): banka tahsilattan komisyonu düşüp neti yatırdı;
   * komisyon 653 / 780'e, karşı hesaba brüt yazılır. null siler.
   */
  /** Döviz hareketinin ödeme günü kuru; kur farkı hesaplanır (#565). */
  setFxRate(
    slug: string,
    engagement: string,
    transaction: string,
    rate: string | null,
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/fx-rate`,
      { body: { rate } },
    );
  },
  setCommission(
    slug: string,
    engagement: string,
    transaction: string,
    amount: string | null,
    accountCode: string,
  ) {
    return tenantRequest<void>(
      slug,
      "PUT",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-postings/${encodeURIComponent(transaction)}/commission`,
      {
        body:
          amount === null
            ? { amount: null }
            : { amount, account_code: accountCode },
      },
    );
  },
  /**
   * Ofis içi virman (#710): hareketi başka banka hesabının ters yönlü
   * hareketine bağlar; iki taraf da eşleşir, karşı hesaplar birbirinin 102'si.
   */
  matchTransfer(
    slug: string,
    engagement: string,
    transaction: string,
    counterpart: string,
  ) {
    return tenantRequest<BankTransactionMatching>(
      slug,
      "PUT",
      `${tx(engagement, transaction)}/transfer`,
      { body: { counterpart_uuid: counterpart } },
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
