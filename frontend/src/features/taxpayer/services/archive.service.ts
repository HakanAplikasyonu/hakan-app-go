import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import {
  TAXPAYER_HEADER,
  taxpayerRequest,
} from "@/features/taxpayer/services/taxpayer.service";

export type TaxpayerArchiveDocument = components["schemas"]["ArchiveDocument"];
export type TaxpayerArchivePage = components["schemas"]["ArchiveDocumentPage"];

export const taxpayerArchiveKeys = {
  documents: (taxpayer: string) =>
    ["taxpayer", taxpayer.toLowerCase(), "archive"] as const,
};

/**
 * Mükellef paneli (#682): ofisin belge arşivine "Mükellef de görsün"
 * işaretiyle koyduğu belgeler, salt okuma. Yalnızca şirketin sahip rolü
 * görür; başkası 403 alır.
 */
export const taxpayerArchiveService = {
  documents: (taxpayer: string) =>
    taxpayerRequest<TaxpayerArchivePage>(
      "GET",
      "/v1/taxpayer/archive-documents?limit=200",
      { taxpayer },
    ),
  async download(taxpayer: string, doc: TaxpayerArchiveDocument) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}/v1/taxpayer/archive-documents/${encodeURIComponent(doc.uuid)}/file`,
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
