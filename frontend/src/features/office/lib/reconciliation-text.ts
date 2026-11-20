import { formatMoney } from "./decimal";

/** Mutabakat satırının sonucu (OpenAPI ReconciliationItem.result). */
export type ReconciliationResult =
  "matched" | "difference" | "missing_in_ledger" | "missing_in_sources";

/**
 * Fark tablosunda bir hesabın sade anlatımı (#710): çeviri anahtarı ve
 * değişkenleri. Fark = mizan − uygulama (borç − alacak); artı fark mizanda
 * borç tarafının, eksi fark alacak tarafının fazla olduğunu söyler.
 */
export function differenceMeaning(item: {
  result: string;
  ledger_net: string;
  source_net: string;
  difference: string;
}): { key: string; vars: Record<string, string> } {
  const abs = (v: string) => formatMoney(v.replace(/^-/, ""));
  switch (item.result as ReconciliationResult) {
    case "matched":
      return { key: "office.reconciliation.meaning_matched", vars: {} };
    case "missing_in_ledger":
      return {
        key: "office.reconciliation.meaning_missing_in_ledger",
        vars: { amount: abs(item.source_net) },
      };
    case "missing_in_sources":
      return {
        key: "office.reconciliation.meaning_missing_in_sources",
        vars: { amount: abs(item.ledger_net) },
      };
    default: {
      const n = Number(item.difference);
      return {
        key:
          n >= 0
            ? "office.reconciliation.meaning_ledger_debit"
            : "office.reconciliation.meaning_ledger_credit",
        vars: {
          amount: abs(item.difference),
          ledger: formatMoney(item.ledger_net),
          sources: formatMoney(item.source_net),
        },
      };
    }
  }
}
