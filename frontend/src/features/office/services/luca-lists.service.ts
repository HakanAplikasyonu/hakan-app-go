import type { components } from "@/generated/api";

import { downloadPost } from "./download";

export type LucaListExportInput =
  components["schemas"]["LucaListExportInput"];
export type LucaListReport = components["schemas"]["LucaListReport"];
export type LucaListReportRow = components["schemas"]["LucaListReportRow"];

/** Luca'ya aktarılabilen listeler (Oğuz'un geri bildirimi). */
export const LUCA_LISTS = [
  "incoming-invoices",
  "outgoing-invoices",
  "bank-transactions",
  "z-reports",
] as const;
export type LucaList = (typeof LUCA_LISTS)[number];

const fallbackPrefix: Record<LucaList, string> = {
  "incoming-invoices": "gelen_fatura",
  "outgoing-invoices": "giden_fatura",
  "bank-transactions": "banka",
  "z-reports": "z_raporu",
};

/**
 * Sunucunun X-Luca-Report başlığı (base64url JSON): kontrol edilecek
 * satırlar, alınmayan ve daha önce aktarılan belgeler. Okunamazsa null.
 */
export function decodeLucaReport(
  header: string | null | undefined,
): LucaListReport | null {
  if (!header) return null;
  try {
    const b64 = header.replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const bytes = Uint8Array.from(atob(padded), (c) => c.charCodeAt(0));
    return JSON.parse(new TextDecoder().decode(bytes)) as LucaListReport;
  } catch {
    return null;
  }
}

export const lucaListsService = {
  /**
   * Dönemin listesi Luca fiş dosyası olarak (.xlsx ya da .zip) iner.
   * Daha önce aktarılan belge, include_exported olmadıkça dosyaya girmez.
   * Dönen başlıklar sayıları ve raporu taşır.
   */
  export(
    slug: string,
    engagement: string,
    list: LucaList,
    input: LucaListExportInput,
  ) {
    return downloadPost(
      slug,
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/luca-lists/${list}`,
      input,
      `${fallbackPrefix[list]}_luca_fis_${input.period}.xlsx`,
    );
  },
};
