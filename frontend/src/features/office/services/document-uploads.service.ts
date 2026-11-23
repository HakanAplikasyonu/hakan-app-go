import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type DocumentUpload = components["schemas"]["DocumentUpload"];
export type DocumentUploadType = DocumentUpload["doc_type"];
export type DocumentUploadSettings =
  components["schemas"]["DocumentUploadSettings"];
export type LockChoice = "reject" | "correction" | "target_period";

/** Yüklemede seçilen evrak türleri, ekrandaki sırayla. */
export const DOCUMENT_UPLOAD_TYPES: DocumentUploadType[] = [
  "invoice",
  "z_report",
  "expense",
  "bank_statement",
  "other",
];

export type DocumentUploadInput = {
  file: File;
  docType: DocumentUploadType;
  period: string;
  /** Kullanıcı önerilen ayı değiştirmedi. */
  periodAuto: boolean;
  documentDate?: string;
  lockChoice?: LockChoice;
  targetPeriod?: string;
};

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-uploads`;
const root = () => apiConfig.baseUrl.replace(/\/$/, "");

export const documentUploadKeys = {
  list: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "document-uploads",
    ] as const,
  settings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "document-upload-settings"] as const,
};

/**
 * Tür ve dönem seçerek evrak yükleme (Oğuz'un demo geri bildirimi): fatura,
 * Z raporu, masraf, banka ekstresi, diğer; dönem belge tarihinden önerilir,
 * kilitli ayda üç seçenek sorulur.
 */
export const documentUploadsService = {
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: DocumentUpload[] }>(
      slug,
      "GET",
      base(engagement),
    );
  },
  async create(slug: string, engagement: string, u: DocumentUploadInput) {
    const form = new FormData();
    form.append("file", u.file);
    form.append("doc_type", u.docType);
    form.append("period", u.period);
    form.append("period_auto", String(u.periodAuto));
    if (u.documentDate) form.append("document_date", u.documentDate);
    if (u.lockChoice) form.append("lock_choice", u.lockChoice);
    if (u.lockChoice === "target_period" && u.targetPeriod)
      form.append("target_period", u.targetPeriod);
    const response = await fetch(`${root()}${base(engagement)}`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    // Kilitli ay ekranda seçeneklerle sorulur; genel hata bildirimi çıkmaz.
    const locked =
      response.status === 409 &&
      (payload as { error?: { code?: string } } | undefined)?.error?.code ===
        "PERIOD_LOCKED";
    return unwrap<DocumentUpload>(
      { data: payload, response },
      { silent: locked },
    );
  },
  getSettings: (slug: string) =>
    tenantRequest<DocumentUploadSettings>(
      slug,
      "GET",
      "/v1/tenant/document-upload-settings",
      { silent: true },
    ),
  saveSettings: (slug: string, enabled: boolean) =>
    tenantRequest<DocumentUploadSettings>(
      slug,
      "PUT",
      "/v1/tenant/document-upload-settings",
      { body: { enabled } },
    ),
};
