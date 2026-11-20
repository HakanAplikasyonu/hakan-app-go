import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type JournalDraft = components["schemas"]["JournalDraft"];
export type JournalDraftPage = components["schemas"]["JournalDraftPage"];
export type JournalDraftDetail = components["schemas"]["JournalDraftDetail"];
export type JournalDraftInput = components["schemas"]["JournalDraftInput"];
export type JournalDraftIssue = components["schemas"]["JournalDraftIssue"];
export type JournalExport = components["schemas"]["JournalExport"];
export type JournalExportSummary =
  components["schemas"]["JournalExportSummary"];
export type JournalExportInput = components["schemas"]["JournalExportInput"];
export type ExportDriver = components["schemas"]["ExportDriver"];
export type JournalTransfer = components["schemas"]["JournalTransfer"];
export type JournalTransferPage = components["schemas"]["JournalTransferPage"];
export type JournalTransferStatus =
  components["schemas"]["JournalTransferStatus"];
export type JournalExportCheck = components["schemas"]["JournalExportCheck"];
export type OfficeJournalTransfer =
  components["schemas"]["OfficeJournalTransfer"];
export type OfficeJournalTransferPage =
  components["schemas"]["OfficeJournalTransferPage"];
export type PendingJournalTransfer =
  components["schemas"]["PendingJournalTransfer"];
export type PendingJournalTransferPage =
  components["schemas"]["PendingJournalTransferPage"];
export type JournalExportDocument =
  components["schemas"]["JournalExportDocument"];
export type JournalExportSettings =
  components["schemas"]["JournalExportSettings"];
export type OfficeJournalExportSettings =
  components["schemas"]["OfficeJournalExportSettings"];
export type ExportIssue = components["schemas"]["ExportIssue"];

/** Integrators Luca can fetch e-invoices from itself (#466). */
export const LUCA_INTEGRATORS = ["isnet", "turmob", "gib"] as const;
export type LucaIntegrator = (typeof LUCA_INTEGRATORS)[number];

/** Programs whose exports the desktop agent can hand to the program (#391). */
export const TRANSFER_DRIVERS = new Set(["luca", "zirve"]);

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
  transfers: (slug: string, engagement: string) =>
    [...base(slug, engagement), "journals", "transfers"] as const,
  documents: (slug: string, engagement: string, q: string, batch: string) =>
    [...base(slug, engagement), "journals", "documents", q, batch] as const,
  settings: (slug: string, engagement: string) =>
    [...base(slug, engagement), "journals", "settings"] as const,
  officeSettings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "journal-export-settings"] as const,
  /** Ofis geneli aktarım görünümü (#574). */
  officeTransfers: (slug: string) =>
    ["tenant", slug.toLowerCase(), "journal-transfers"] as const,
  officePending: (slug: string) =>
    ["tenant", slug.toLowerCase(), "journal-transfers", "pending"] as const,
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
  /**
   * Uyarılı fiş (eksik belge / mükerrer olabilir) yalnız `confirm` ile
   * onaylanır; yoksa 409 JOURNAL_NEEDS_CONFIRMATION (#669).
   */
  approve(
    slug: string,
    engagement: string,
    draft: string,
    opts?: { confirm?: boolean; note?: string },
  ) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "POST",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/approve`,
      opts?.confirm
        ? { body: { confirm_warnings: true, note: opts.note || undefined } }
        : undefined,
    );
  },
  /** Kontrol ekranında satırın hesap kodu düzeltilir (#566). */
  setLineAccount(
    slug: string,
    engagement: string,
    draft: string,
    line: number,
    account: string,
  ) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "PATCH",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/lines/${line}`,
      { body: { account } },
    );
  },
  /** Onaylı, henüz aktarılmamış fişin onayını gerekçeyle geri alır (#566). */
  unapprove(slug: string, engagement: string, draft: string, reason: string) {
    return tenantRequest<JournalDraftDetail>(
      slug,
      "POST",
      `${eng(engagement)}/journal-drafts/${encodeURIComponent(draft)}/unapprove`,
      { body: { reason } },
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
  /** Pre-export check (#466): issues and entries left out; nothing written. */
  check(slug: string, engagement: string, input: JournalExportInput) {
    return tenantRequest<JournalExportCheck>(
      slug,
      "POST",
      `${eng(engagement)}/journal-exports/check`,
      { body: input, silent: true },
    );
  },
  /** Document → file trail of the export history (#466). */
  documents(slug: string, engagement: string, q: string, batch: string) {
    return tenantRequest<{ items: JournalExportDocument[] }>(
      slug,
      "GET",
      `${eng(engagement)}/journal-export-documents`,
      { query: { q: q || undefined, batch: batch || undefined, limit: 100 } },
    );
  },
  settings(slug: string, engagement: string) {
    return tenantRequest<JournalExportSettings>(
      slug,
      "GET",
      `${eng(engagement)}/journal-export-settings`,
    );
  },
  /** Saves the taxpayer's export settings; `luca_pull_integrators: null`
   * returns to the office setting. */
  saveSettings(
    slug: string,
    engagement: string,
    input: Omit<JournalExportSettings, "effective_luca_pull_integrators">,
  ) {
    return tenantRequest<JournalExportSettings>(
      slug,
      "PUT",
      `${eng(engagement)}/journal-export-settings`,
      { body: input, silent: true },
    );
  },
  officeSettings(slug: string) {
    return tenantRequest<OfficeJournalExportSettings>(
      slug,
      "GET",
      "/v1/tenant/journal-export-settings",
    );
  },
  saveOfficeSettings(slug: string, input: OfficeJournalExportSettings) {
    return tenantRequest<OfficeJournalExportSettings>(
      slug,
      "PUT",
      "/v1/tenant/journal-export-settings",
      { body: input, silent: true },
    );
  },
  /** Ofis genelinde her çıktının son aktarımı ve mizan durumu (#574). */
  officeTransfers(slug: string) {
    return tenantRequest<OfficeJournalTransferPage>(
      slug,
      "GET",
      "/v1/tenant/journal-transfers",
      { query: { limit: 200 } },
    );
  },
  /** Ofis genelinde programa aktarılmamış onaylı fişler (#574). */
  officePending(slug: string) {
    return tenantRequest<PendingJournalTransferPage>(
      slug,
      "GET",
      "/v1/tenant/journal-transfers/pending",
      { query: { limit: 200 } },
    );
  },
  transfers(slug: string, engagement: string) {
    return tenantRequest<JournalTransferPage>(
      slug,
      "GET",
      `${eng(engagement)}/journal-transfers`,
      { query: { limit: 200 } },
    );
  },
  /** Queue an export for the desktop agent (#391). */
  requestTransfer(slug: string, engagement: string, batch: string) {
    return tenantRequest<JournalTransfer>(
      slug,
      "POST",
      `${eng(engagement)}/journal-exports/${encodeURIComponent(batch)}/transfers`,
      { body: { method: "agent_folder" }, silent: true },
    );
  },
  completeTransfer(
    slug: string,
    engagement: string,
    transfer: string,
    outcome: "imported" | "failed",
    message?: string,
  ) {
    return tenantRequest<JournalTransfer>(
      slug,
      "POST",
      `${eng(engagement)}/journal-transfers/${encodeURIComponent(transfer)}/complete`,
      { body: { outcome, message }, silent: true },
    );
  },
  cancelTransfer(slug: string, engagement: string, transfer: string) {
    return tenantRequest<JournalTransfer>(
      slug,
      "POST",
      `${eng(engagement)}/journal-transfers/${encodeURIComponent(transfer)}/cancel`,
      { silent: true },
    );
  },
  retryTransfer(slug: string, engagement: string, transfer: string) {
    return tenantRequest<JournalTransfer>(
      slug,
      "POST",
      `${eng(engagement)}/journal-transfers/${encodeURIComponent(transfer)}/retry`,
      { silent: true },
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
