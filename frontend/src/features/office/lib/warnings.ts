import type { Severity } from "../types";

/**
 * Uyarı kodlarının varsayılan seviyesi (kaynak paket 05 §3). Metin koddan
 * üretilir: `office.warning.<CODE>` (i18n), parametreler `params` içinde.
 */
export const warningSeverity: Record<string, Severity> = {
  PENDING_RESPONSE: "info",
  REJECTED: "warning",
  RETURNED: "warning",
  DELIVERY_ERROR: "blocker",
  PARTY_MISMATCH: "blocker",
  UNMAPPED_TYPE_CODE: "warning",
  UNMAPPED_PROFILE: "info",
  UNKNOWN_TAX_CODE: "warning",
  LINE_TAXABLE_MISMATCH: "warning",
  TAX_TOTAL_MISMATCH: "warning",
  BASE_TOTAL_MISMATCH: "warning",
  PAYABLE_MISMATCH: "warning",
  HEADER_ESTIMATE: "warning",
  POSSIBLE_DUPLICATE: "warning",
  INVALID_TAX_ID: "warning",
  UBL_SCHEMATRON_FAIL: "warning",
};

export const knownWarningCodes = Object.keys(warningSeverity);

export function severityOf(code: string): Severity {
  return warningSeverity[code] ?? "warning";
}
