import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type JournalDraft = components["schemas"]["JournalDraft"];
export type JournalDraftPage = components["schemas"]["JournalDraftPage"];
export type JournalDraftDetail = components["schemas"]["JournalDraftDetail"];
export type JournalDraftInput = components["schemas"]["JournalDraftInput"];
export type JournalExport = components["schemas"]["JournalExport"];
export type JournalExportSummary =
  components["schemas"]["JournalExportSummary"];
export type JournalExportInput = components["schemas"]["JournalExportInput"];
export type ExportDriver = components["schemas"]["ExportDriver"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

const base = (slug: string, engagement: string) =>
  [
    "tenant",
    slug.toLowerCase(),
    "engagements",
    engagement.toLowerCase(),
  ] as const;

export const journalKeys = {
  drivers: (slug: string) =>
    ["tenant", slug.toLowerCase(), "export-drivers"] as const,
  all: (slug: string, engagement: string) =>
    [...base(slug, engagement), "journals"] as const,
  drafts: (slug: string, engagement: string, period: string) =>
    [...base(slug, engagement), "journals", "drafts", period] as const,
  draft: (slug: string, engagement: string, draft: string) =>
    [...base(slug, engagement), "journals", "draft", draft] as const,
  exports: (slug: string, engagement: string) =>
    [...base(slug, engagement), "journals", "exports"] as const,
};

/**
 * Journal drafts (yevmiye taslağı) and their export to accounting programs
 * (issue #143): create, approve, export to Luca / Zirve / Excel, download,
 * reverse.
 */
export const journalService = {
  drivers(slug: string) {
    return tenantRequest<{ items: ExportDriver[] }>(
      slug,
      "GET",
      "/v1/tenant/export-drivers",
    );
  },
  drafts(slug: string, engagement: string, period: string) {
    return tenantRequest<JournalDraftPage>(
      slug,
      "GET",
      `${eng(engagement)}/journal-drafts`,
      { query: { period: period || undefined, limit: 200 } },
    );
  },
  draft(slug: string, engagement: string, draft: string) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "GET",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}`,
    );
  },
  create(slug: string, engagement: string, input: JournalDraftInput) {
    return tenantRequest<JournalDraft>(
      slug,
      "POST",
      `${eng(engagement)}/journal-drafts`,
      { body: input },
    );
  },
  approve(slug: string, engagement: string, draft: string) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "POST",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/approve`,
    );
  },
  reverse(slug: string, engagement: string, draft: string, reason: string) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "POST",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/reverse`,
      { body: { reason } },
    );
  },
  exports(slug: string, engagement: string) {
    return tenantRequest<{ items: JournalExportSummary[] }>(
      slug,
      "GET",
      `${eng(engagement)}/journal-exports`,
    );
  },
  export(slug: string, engagement: string, input: JournalExportInput) {
    return tenantRequest<JournalExport>(
      slug,
      "POST",
      `${eng(engagement)}/journal-exports`,
      { body: input, silent: true },
    );
  },
  /** Link for a stored export file (a plain GET; the BFF adds the session). */
  async download(
    slug: string,
    engagement: string,
    batch: string,
    file: string,
  ) {
    const apiBase = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${apiBase}${eng(engagement)}/journal-exports/${encodeURIComponent(batch)}/files/${encodeURIComponent(file)}`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`download ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = file;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
