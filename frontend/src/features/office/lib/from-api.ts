import type { OfficeInvoiceDetail } from "../services/office-invoices.service";
import type { Invoice, TaxBreakdown } from "../types";
import { add, toMinor } from "./decimal";

/** API direction (UBL side of the taxpayer) → office panel wording. */
export const directionOf = (d: string): Invoice["direction"] =>
  d === "incoming" ? "purchase" : "sales";

/**
 * Maps an API invoice detail (#142) to the panel's Invoice shape, so the
 * amount waterfall and the UBL preview render API invoices as they did the
 * demo ones. KDV rows become code 0015; a withheld amount becomes 9015.
 */
export function toPanelInvoice(
  d: OfficeInvoiceDetail,
  engagement: string,
  xml?: string,
): Invoice {
  const taxes: TaxBreakdown[] = [];
  for (const row of d.tax_summary) {
    taxes.push({
      code: "0015",
      percent: row.tax_rate,
      taxable: row.base,
      amount: row.vat,
    });
    if (toMinor(row.withholding) !== BigInt(0)) {
      taxes.push({
        code: "9015",
        percent: row.tax_rate,
        taxable: row.vat,
        amount: row.withholding,
      });
    }
  }
  return {
    id: d.uuid,
    taxpayerId: engagement,
    ettn: d.ettn ?? "",
    number: d.invoice_number,
    issueDate: d.issue_date,
    profile: d.profile ?? "",
    typeCode: d.type,
    direction: directionOf(d.direction),
    currency: d.currency,
    exchangeRate: null,
    counterparty: {
      title: d.counterparty_name ?? "",
      taxId: d.counterparty_tax_id ?? "",
    },
    lines: d.items.map((it) => ({
      id: String(it.line_no),
      name: it.name,
      quantity: it.quantity,
      // Birim Türkçe adıyla (#378): API `unit_name` verir ("C62" → "adet").
      unit: it.unit_name || it.unit_code || "",
      unitPrice: it.unit_price,
      lineTotal: it.tax_exclusive_amount,
      vatPercent: it.tax_rate,
      vatAmount: it.tax_amount,
      accountCode: null,
    })),
    taxes,
    totals: {
      lineExtension: d.tax_exclusive_amount,
      taxExclusive: d.tax_exclusive_amount,
      taxInclusive: add(d.tax_exclusive_amount, d.tax_amount),
      payable: d.payable_amount,
    },
    status: "new",
    source: "upload",
    rawXml: xml,
    uploadedAt: d.sources[0]?.first_seen_at ?? "",
  };
}
