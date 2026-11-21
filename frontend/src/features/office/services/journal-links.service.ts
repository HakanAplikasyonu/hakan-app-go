import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ManualLinkKind = "invoice" | "bank_transaction" | "customer";
type OfficeInvoicePage = components["schemas"]["OfficeInvoicePage"];
type BankTransactionPage = components["schemas"]["BankTransactionPage"];
type ManualLink = components["schemas"]["JournalDraftManualLink"];
type CustomerCandidate = components["schemas"]["JournalLinkCustomerCandidate"];

/** Bağlanabilecek kayıt: fişin dönemindeki fatura ya da banka hareketi. */
export type LinkCandidate = {
  uuid: string;
  date: string;
  amount: string;
  currency: string;
  label: string;
  detail: string;
  /** Cari kartının türü (customer, supplier, both); yalnız cari için. */
  cardType?: string;
};

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;
const segment: Record<ManualLinkKind, string> = {
  invoice: "invoices",
  bank_transaction: "bank-transactions",
  customer: "customers",
};

/** Fişe elle kayıt bağlama (#710). */
export const journalLinksService = {
  set(
    slug: string,
    engagement: string,
    draft: string,
    kind: ManualLinkKind,
    uuid: string,
    on: boolean,
  ) {
    return tenantRequest<ManualLink>(
      slug,
      on ? "PUT" : "DELETE",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/links/${segment[kind]}/${encodeURIComponent(uuid)}`,
    );
  },
  async candidates(
    slug: string,
    engagement: string,
    kind: ManualLinkKind,
    period: string,
  ): Promise<LinkCandidate[]> {
    if (kind === "customer") {
      const page = await tenantRequest<{ items: CustomerCandidate[] }>(
        slug,
        "GET",
        `${eng(engagement)}/link-candidates/customers`,
      );
      return page.items.map((c) => ({
        uuid: c.uuid,
        date: "",
        amount: "",
        currency: "TRY",
        label: c.name,
        detail: "",
        cardType: c.type,
      }));
    }
    if (kind === "invoice") {
      const page = await tenantRequest<OfficeInvoicePage>(
        slug,
        "GET",
        `${eng(engagement)}/invoices`,
        { query: { period, limit: 200 } },
      );
      return page.items.map((i) => ({
        uuid: i.uuid,
        date: i.issue_date,
        amount: i.payable_amount,
        currency: i.currency,
        label: i.invoice_number,
        detail: i.counterparty_name ?? "",
      }));
    }
    const page = await tenantRequest<BankTransactionPage>(
      slug,
      "GET",
      `${eng(engagement)}/bank-transactions`,
      { query: { period, limit: 200 } },
    );
    return page.items.map((b) => ({
      uuid: b.uuid,
      date: b.transaction_date,
      amount: b.amount,
      currency: b.currency,
      label: b.description,
      detail: b.counterparty_name ?? "",
    }));
  },
};
