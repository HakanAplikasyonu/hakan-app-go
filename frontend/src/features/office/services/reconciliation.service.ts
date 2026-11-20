import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

type S = components["schemas"];
export type ReconciliationRun = S["ReconciliationRun"];
export type ReconciliationRunDetail = S["ReconciliationRunDetail"];
/** `runs/latest`: `run` null ise dönem henüz karşılaştırılmadı (#639). */
export type ReconciliationLatestRunDetail = S["ReconciliationLatestRunDetail"];
export type ReconciliationItem = S["ReconciliationItem"];
export type ReconciliationCounterparty = S["ReconciliationCounterparty"];
export type ReconciliationCounterpartyPage =
  S["ReconciliationCounterpartyPage"];
export type BabsDeclaration = S["BabsDeclaration"];
export type BabsDeclarationInput = S["BabsDeclarationInput"];
export type BabsImportResult = S["BabsImportResult"];
export type BabsResult = S["BabsResult"];
export type ReconciliationLetter = S["ReconciliationLetter"];
export type LetterResponseInput = S["ReconciliationLetterResponseInput"];
export type DecisionStatus = "open" | "explained" | "resolved" | "ignored";

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/reconciliation`;

const base = (slug: string, engagement: string) =>
  [
    "tenant",
    slug.toLowerCase(),
    "engagements",
    engagement.toLowerCase(),
    "reconciliation",
  ] as const;

export const reconciliationKeys = {
  all: (slug: string, engagement: string) => base(slug, engagement),
  run: (slug: string, engagement: string, period: string) =>
    [...base(slug, engagement), "run", period] as const,
  latestByPeriod: (
    slug: string,
    engagement: string,
    from: string,
    to: string,
  ) => [...base(slug, engagement), "latest-by-period", from, to] as const,
  counterparties: (slug: string, engagement: string, period: string) =>
    [...base(slug, engagement), "counterparties", period] as const,
  babs: (slug: string, engagement: string, period: string) =>
    [...base(slug, engagement), "babs", period] as const,
  letters: (slug: string, engagement: string) =>
    [...base(slug, engagement), "letters"] as const,
};

/**
 * Mutabakat (#389): mizan – kaynak karşılaştırması, cari mutabakat, BA/BS
 * beyan satırları ve mutabakat mektupları.
 */
export const reconciliationService = {
  latest(slug: string, engagement: string, period: string) {
    return tenantRequest<ReconciliationLatestRunDetail>(
      slug,
      "GET",
      `${eng(engagement)}/runs/latest`,
      // Dönem henüz karşılaştırılmadıysa 200 + run: null (#639).
      { query: { period }, silent: true },
    );
  },
  /** Penceredeki her ayın son koşusu tek istekte; karşılaştırılmamış ay null (#639). */
  latestByPeriod(slug: string, engagement: string, from: string, to: string) {
    return tenantRequest<{ runs: Record<string, ReconciliationRun | null> }>(
      slug,
      "GET",
      `${eng(engagement)}/runs/latest-by-period`,
      { query: { from, to }, silent: true },
    );
  },
  run(slug: string, engagement: string, period: string) {
    return tenantRequest<ReconciliationRunDetail>(
      slug,
      "POST",
      `${eng(engagement)}/runs`,
      { body: { period }, silent: true },
    );
  },
  decideItem(
    slug: string,
    engagement: string,
    item: string,
    status: DecisionStatus,
    note?: string,
  ) {
    return tenantRequest<ReconciliationItem>(
      slug,
      "PATCH",
      `${eng(engagement)}/items/${encodeURIComponent(item)}`,
      { body: { status, note: note ?? null }, silent: true },
    );
  },
  reviewItem(slug: string, engagement: string, item: string, note?: string) {
    return tenantRequest<ReconciliationItem>(
      slug,
      "POST",
      `${eng(engagement)}/items/${encodeURIComponent(item)}/review`,
      { body: { note: note ?? null }, silent: true },
    );
  },
  counterparties(slug: string, engagement: string, period: string) {
    return tenantRequest<ReconciliationCounterpartyPage>(
      slug,
      "GET",
      `${eng(engagement)}/runs/latest/counterparties`,
      { query: { period }, silent: true },
    );
  },
  decideCounterparty(
    slug: string,
    engagement: string,
    uuid: string,
    status: DecisionStatus,
    note?: string,
  ) {
    return tenantRequest<ReconciliationCounterparty>(
      slug,
      "PATCH",
      `${eng(engagement)}/counterparties/${encodeURIComponent(uuid)}`,
      { body: { status, note: note ?? null }, silent: true },
    );
  },
  reviewCounterparty(
    slug: string,
    engagement: string,
    uuid: string,
    note?: string,
  ) {
    return tenantRequest<ReconciliationCounterparty>(
      slug,
      "POST",
      `${eng(engagement)}/counterparties/${encodeURIComponent(uuid)}/review`,
      { body: { note: note ?? null }, silent: true },
    );
  },
  declarations(slug: string, engagement: string, period: string) {
    return tenantRequest<{ items: BabsDeclaration[] }>(
      slug,
      "GET",
      `${eng(engagement)}/babs`,
      { query: { period } },
    );
  },
  saveDeclaration(
    slug: string,
    engagement: string,
    input: BabsDeclarationInput,
  ) {
    return tenantRequest<BabsDeclaration>(
      slug,
      "POST",
      `${eng(engagement)}/babs`,
      { body: input, silent: true },
    );
  },
  deleteDeclaration(slug: string, engagement: string, uuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `${eng(engagement)}/babs/${encodeURIComponent(uuid)}`,
    );
  },
  /** Multipart upload; the browser sets the boundary header itself. */
  async importDeclarations(
    slug: string,
    engagement: string,
    file: File,
    period: string,
    form: "ba" | "bs",
  ) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const body = new FormData();
    body.append("file", file);
    body.append("period", period);
    body.append("form", form);
    const response = await fetch(`${root}${eng(engagement)}/babs/imports`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<BabsImportResult>(
      { data: payload, response },
      { silent: true },
    );
  },
  letters(slug: string, engagement: string) {
    return tenantRequest<{ items: ReconciliationLetter[] }>(
      slug,
      "GET",
      `${eng(engagement)}/letters`,
    );
  },
  createLetter(
    slug: string,
    engagement: string,
    input: {
      counterparty_uuid: string;
      email?: string | null;
      note?: string | null;
      send: boolean;
    },
  ) {
    return tenantRequest<ReconciliationLetter>(
      slug,
      "POST",
      `${eng(engagement)}/letters`,
      { body: input, silent: true },
    );
  },
  sendLetter(slug: string, engagement: string, uuid: string, email?: string) {
    return tenantRequest<ReconciliationLetter>(
      slug,
      "POST",
      `${eng(engagement)}/letters/${encodeURIComponent(uuid)}/send`,
      { body: { email: email ?? null }, silent: true },
    );
  },
  recordResponse(
    slug: string,
    engagement: string,
    uuid: string,
    input: LetterResponseInput,
  ) {
    return tenantRequest<ReconciliationLetter>(
      slug,
      "POST",
      `${eng(engagement)}/letters/${encodeURIComponent(uuid)}/response`,
      { body: input, silent: true },
    );
  },
  /** Opens the letter PDF in a new tab. */
  async openLetterPdf(slug: string, engagement: string, uuid: string) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${root}${eng(engagement)}/letters/${encodeURIComponent(uuid)}/pdf`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`pdf ${response.status}`);
    const url = URL.createObjectURL(await response.blob());
    window.open(url, "_blank", "noopener");
    setTimeout(() => URL.revokeObjectURL(url), 60_000);
  },
};
