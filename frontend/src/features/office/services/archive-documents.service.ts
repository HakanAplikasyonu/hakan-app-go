import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type ArchiveDocument = components["schemas"]["ArchiveDocument"];
export type ArchiveDocumentPage = components["schemas"]["ArchiveDocumentPage"];
export type ArchiveCategory = ArchiveDocument["category"];
export type ArchiveIsgKind = NonNullable<ArchiveDocument["isg_kind"]>;
export type ArchiveImportantKind = NonNullable<
  ArchiveDocument["important_kind"]
>;
export type ArchiveDocumentPatch =
  components["schemas"]["ArchiveDocumentPatch"];
export type ImportantDocumentSettings =
  components["schemas"]["ImportantDocumentSettings"];

export const ARCHIVE_CATEGORIES: ArchiveCategory[] = [
  "general",
  "contract",
  "tax",
  "sgk",
  "trade_registry",
  "isg",
  "other",
];
export const ARCHIVE_ISG_KINDS: ArchiveIsgKind[] = [
  "isg_training",
  "risk_assessment",
  "health_report",
  "other",
];

/** Önemli belge türleri (#780 B). */
export const ARCHIVE_IMPORTANT_KINDS: ArchiveImportantKind[] = [
  "tax_certificate",
  "signature_circular",
  "trade_registry_gazette",
  "lease_contract",
  "license",
  "power_of_attorney",
  "other",
];

export type ArchiveQuery = {
  category?: ArchiveCategory;
  q?: string;
  expiring?: boolean;
  important?: boolean;
  limit: number;
  offset: number;
};

export type ArchiveUpload = {
  file: File;
  category: ArchiveCategory;
  isgKind?: ArchiveIsgKind;
  title?: string;
  note?: string;
  documentDate?: string;
  validUntil?: string;
  visibleToTaxpayer: boolean;
  importantKind?: ArchiveImportantKind;
};

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/archive-documents`;
const root = () => apiConfig.baseUrl.replace(/\/$/, "");

export const archiveKeys = {
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "archive-documents",
    ] as const,
  list: (slug: string, engagement: string, q: ArchiveQuery) =>
    [...archiveKeys.all(slug, engagement), q] as const,
};

async function fetchFile(slug: string, engagement: string, uuid: string) {
  const response = await fetch(
    `${root()}${base(engagement)}/${encodeURIComponent(uuid)}/file`,
    { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
  );
  if (!response.ok) {
    const payload = await response.json().catch(() => undefined);
    return unwrap<never>({ data: payload, response });
  }
  return response.blob();
}

/**
 * Mükellefin belge arşivi (#393 uçları, web ekranı #669 I): faaliyet
 * belgesi, ticaret sicil, sözleşme, İSG belgeleri … yükle / gör / indir.
 */
export const archiveService = {
  list(slug: string, engagement: string, q: ArchiveQuery) {
    return tenantRequest<ArchiveDocumentPage>(slug, "GET", base(engagement), {
      query: {
        category: q.category,
        q: q.q || undefined,
        expiring: q.expiring ? "true" : undefined,
        important: q.important ? "true" : undefined,
        limit: String(q.limit),
        offset: String(q.offset),
      },
    });
  },
  async upload(slug: string, engagement: string, u: ArchiveUpload) {
    const form = new FormData();
    form.append("file", u.file);
    form.append("category", u.category);
    if (u.category === "isg" && u.isgKind) form.append("isg_kind", u.isgKind);
    if (u.title?.trim()) form.append("title", u.title.trim());
    if (u.note?.trim()) form.append("note", u.note.trim());
    if (u.documentDate) form.append("document_date", u.documentDate);
    if (u.validUntil) form.append("valid_until", u.validUntil);
    form.append("visible_to_taxpayer", String(u.visibleToTaxpayer));
    if (u.importantKind) form.append("important_kind", u.importantKind);
    const response = await fetch(`${root()}${base(engagement)}`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<ArchiveDocument>({ data: payload, response });
  },
  /** Önemli işareti, "Mükellef de görsün", ad, not, geçerlilik (#780 B). */
  update(
    slug: string,
    engagement: string,
    uuid: string,
    patch: ArchiveDocumentPatch,
  ) {
    return tenantRequest<ArchiveDocument>(
      slug,
      "PATCH",
      `${base(engagement)}/${encodeURIComponent(uuid)}`,
      { body: patch },
    );
  },
  remove(slug: string, engagement: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${base(engagement)}/${encodeURIComponent(uuid)}`,
    );
  },
  /** Belgeyi yeni sekmede açar (PDF ve görseller tarayıcıda görünür). */
  async view(slug: string, engagement: string, d: ArchiveDocument) {
    // Sekme tıklamayla hemen açılır; açılır pencere engelleyicisi
    // beklemeden sonra açılanı durdurur.
    const tab = window.open("", "_blank");
    try {
      const blob = await fetchFile(slug, engagement, d.uuid);
      const url = URL.createObjectURL(
        new Blob([blob], { type: d.content_type }),
      );
      if (tab) tab.location.href = url;
      else window.location.assign(url);
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (err) {
      tab?.close();
      throw err;
    }
  },
  async download(slug: string, engagement: string, d: ArchiveDocument) {
    const blob = await fetchFile(slug, engagement, d.uuid);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = d.file_name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};

export const importantSettingsKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "important-document-settings"] as const;

/** "Önemli belgeler" bölümü ofis ayarı (#780 B); varsayılan açık. */
export const importantSettingsService = {
  get: (slug: string) =>
    tenantRequest<ImportantDocumentSettings>(
      slug,
      "GET",
      "/v1/tenant/important-document-settings",
      { silent: true },
    ),
  save: (slug: string, enabled: boolean) =>
    tenantRequest<ImportantDocumentSettings>(
      slug,
      "PUT",
      "/v1/tenant/important-document-settings",
      { body: { enabled } },
    ),
};
