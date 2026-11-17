import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import {
  TAXPAYER_HEADER,
  taxpayerRequest,
} from "@/features/taxpayer/services/taxpayer.service";

export type TaxpayerPayrollDocument = components["schemas"]["PayrollDocument"];
export type TaxpayerPayrollDocumentPage =
  components["schemas"]["PayrollDocumentPage"];

export const taxpayerPayrollKeys = {
  documents: (taxpayer: string, period?: string) =>
    ["taxpayer", taxpayer.toLowerCase(), "payroll", period ?? "all"] as const,
};

/**
 * Mükellef paneli (#382): ofisin açtığı bordro PDF'leri, salt okuma. Yalnızca
 * şirketin sahip rolü görür; başkası 403 alır.
 */
export const taxpayerPayrollService = {
  documents: (taxpayer: string, period?: string) =>
    taxpayerRequest<TaxpayerPayrollDocumentPage>(
      "GET",
      `/v1/taxpayer/payroll/documents?limit=200${period ? `&period=${encodeURIComponent(period)}` : ""}`,
      { taxpayer },
    ),
  async download(taxpayer: string, doc: TaxpayerPayrollDocument) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}/v1/taxpayer/payroll/documents/${encodeURIComponent(doc.uuid)}/file`,
      { credentials: "include", headers: { [TAXPAYER_HEADER]: taxpayer } },
    );
    if (!response.ok) throw new Error(`download ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = doc.file_name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
