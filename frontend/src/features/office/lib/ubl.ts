/**
 * Tarayıcı içi UBL-TR özet ayrıştırıcı. YALNIZCA demo veri kaynağı için:
 * gerçek ayrıştırma, doğrulama (XSD/Schematron) ve görüntü backend'de
 * go-ubltr ile yapılacak (W2 #9–#12). Burada sadece listeleme ve temel
 * tutarlılık kontrolleri (paket 31 §6) için gereken alanlar okunur.
 */

import { absDiff, fromMinor, sum, toMinor } from "./decimal";
import type { Invoice, InvoiceLine, TaxBreakdown } from "../types";

const KNOWN_TYPE_CODES = [
  "SATIS",
  "IADE",
  "TEVKIFAT",
  "ISTISNA",
  "OZELMATRAH",
  "IHRACKAYITLI",
  "SGK",
  "KOMISYONCU",
  "TEVKIFATIADE",
];
const KNOWN_PROFILES = [
  "TEMELFATURA",
  "TICARIFATURA",
  "EARSIVFATURA",
  "IHRACAT",
  "YOLCUBERABERFATURA",
  "KAMU",
  "HKS",
  "ENERJI",
];
const TOLERANCE = BigInt(5); // ±0,05 (paket 31 §6)

function kids(el: Element | null | undefined, name: string): Element[] {
  if (!el) return [];
  return Array.from(el.children).filter((c) => c.localName === name);
}
function kid(
  el: Element | null | undefined,
  ...path: string[]
): Element | null {
  let cur: Element | null | undefined = el;
  for (const p of path) {
    cur = kids(cur, p)[0];
    if (!cur) return null;
  }
  return cur ?? null;
}
function text(el: Element | null | undefined, ...path: string[]): string {
  return kid(el, ...path)?.textContent?.trim() ?? "";
}

function partyOf(party: Element | null) {
  const ids = kids(party, "PartyIdentification").map((p) => kid(p, "ID"));
  const idEl =
    ids.find((i) => /^(VKN|TCKN)$/.test(i?.getAttribute("schemeID") ?? "")) ??
    ids[0];
  const name =
    text(party, "PartyName", "Name") ||
    [text(party, "Person", "FirstName"), text(party, "Person", "FamilyName")]
      .filter(Boolean)
      .join(" ");
  return { title: name, taxId: idEl?.textContent?.trim() ?? "" };
}

export type ParsedUbl = {
  invoice: Omit<
    Invoice,
    "id" | "taxpayerId" | "direction" | "status" | "source" | "uploadedAt"
  >;
  supplier: { title: string; taxId: string };
  customer: { title: string; taxId: string };
  /** Faturanın gömülü XSLT'si (varsa), görüntü için. */
  xslt: string | null;
  warnings: { code: string; params: Record<string, string>; lineId?: string }[];
};

export class NotUblError extends Error {}

function decodeBase64Utf8(b64: string) {
  const bin = atob(b64.replace(/\s+/g, ""));
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

export function parseUbl(xml: string): ParsedUbl {
  const doc = new DOMParser().parseFromString(xml, "application/xml");
  if (doc.getElementsByTagName("parsererror").length)
    throw new NotUblError("xml");
  const root = doc.documentElement;
  if (root.localName !== "Invoice") throw new NotUblError(root.localName);

  const supplier = partyOf(kid(root, "AccountingSupplierParty", "Party"));
  const customer = partyOf(kid(root, "AccountingCustomerParty", "Party"));

  const lines: InvoiceLine[] = kids(root, "InvoiceLine").map((l, i) => {
    const q = kid(l, "InvoicedQuantity");
    const sub = kid(l, "TaxTotal", "TaxSubtotal");
    return {
      id: text(l, "ID") || String(i + 1),
      name: text(l, "Item", "Name"),
      quantity: q?.textContent?.trim() ?? "0",
      unit: q?.getAttribute("unitCode") ?? "",
      unitPrice: text(l, "Price", "PriceAmount") || "0",
      lineTotal: text(l, "LineExtensionAmount") || "0",
      vatPercent: text(sub, "Percent") || "0",
      vatAmount: text(l, "TaxTotal", "TaxAmount") || "0",
      accountCode: null,
    };
  });

  const taxSubs = (container: string) =>
    kids(root, container).flatMap((tt) =>
      kids(tt, "TaxSubtotal").map<TaxBreakdown>((s) => ({
        code: text(s, "TaxCategory", "TaxScheme", "TaxTypeCode"),
        percent: text(s, "Percent") || "0",
        taxable: text(s, "TaxableAmount") || "0",
        amount: text(s, "TaxAmount") || "0",
      })),
    );
  const taxes = taxSubs("TaxTotal");
  const withholding = taxSubs("WithholdingTaxTotal");

  const lmt = kid(root, "LegalMonetaryTotal");
  const totals = {
    lineExtension: text(lmt, "LineExtensionAmount") || "0",
    taxExclusive: text(lmt, "TaxExclusiveAmount") || "0",
    taxInclusive: text(lmt, "TaxInclusiveAmount") || "0",
    payable: text(lmt, "PayableAmount") || "0",
  };

  let xslt: string | null = null;
  for (const ref of kids(root, "AdditionalDocumentReference")) {
    const bin = kid(ref, "Attachment", "EmbeddedDocumentBinaryObject");
    const fname = bin?.getAttribute("filename") ?? "";
    if (bin && /\.xslt?$/i.test(fname)) {
      try {
        xslt = decodeBase64Utf8(bin.textContent ?? "");
      } catch {
        xslt = null;
      }
      break;
    }
  }

  const warnings: ParsedUbl["warnings"] = [];
  const typeCode = text(root, "InvoiceTypeCode");
  const profile = text(root, "ProfileID");
  if (typeCode && !KNOWN_TYPE_CODES.includes(typeCode))
    warnings.push({ code: "UNMAPPED_TYPE_CODE", params: { value: typeCode } });
  if (profile && !KNOWN_PROFILES.includes(profile))
    warnings.push({ code: "UNMAPPED_PROFILE", params: { value: profile } });

  if (lines.length === 0) {
    warnings.push({ code: "HEADER_ESTIMATE", params: {} });
  } else {
    const lineVat = sum(lines.map((l) => l.vatAmount));
    const headerVat = sum(
      taxes.filter((t) => t.code === "0015").map((t) => t.amount),
    );
    if (absDiff(lineVat, headerVat) > TOLERANCE)
      warnings.push({
        code: "TAX_TOTAL_MISMATCH",
        params: { expected: headerVat, actual: lineVat },
      });
    const lineBase = sum(lines.map((l) => l.lineTotal));
    if (absDiff(lineBase, totals.lineExtension) > TOLERANCE)
      warnings.push({
        code: "BASE_TOTAL_MISMATCH",
        params: { expected: totals.lineExtension, actual: lineBase },
      });
  }
  const expectedPayable = fromMinor(
    toMinor(totals.taxExclusive) +
      toMinor(sum(taxes.map((t) => t.amount))) -
      toMinor(sum(withholding.map((t) => t.amount))),
  );
  if (absDiff(expectedPayable, totals.payable) > TOLERANCE)
    warnings.push({
      code: "PAYABLE_MISMATCH",
      params: { expected: expectedPayable, actual: totals.payable },
    });

  const rate = text(root, "PricingExchangeRate", "CalculationRate");

  return {
    invoice: {
      ettn: text(root, "UUID"),
      number: text(root, "ID"),
      issueDate: text(root, "IssueDate"),
      profile,
      typeCode,
      currency: text(root, "DocumentCurrencyCode") || "TRY",
      exchangeRate: rate || null,
      counterparty: { title: "", taxId: "" },
      lines,
      taxes: [...taxes, ...withholding],
      totals,
      rawXml: xml,
    },
    supplier,
    customer,
    xslt,
    warnings,
  };
}

export function isValidTaxId(id: string) {
  return /^\d{10,11}$/.test(id);
}
