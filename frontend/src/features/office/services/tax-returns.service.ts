import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type TaxReturnKind = "kdv1" | "kdv2" | "damga" | "muhsgk" | "babs";
export type TaxReturnPreview = components["schemas"]["TaxReturnPreview"];
export type TaxReturnSummary = components["schemas"]["TaxReturnSummary"];
export type TaxReturnSection = components["schemas"]["TaxReturnSection"];
export type TaxReturnCheckItem = components["schemas"]["TaxReturnCheckItem"];
export type TaxReturnKDV1 = components["schemas"]["TaxReturnKDV1"];
export type TaxReturnKDV2 = components["schemas"]["TaxReturnKDV2"];
export type TaxReturnStamp = components["schemas"]["TaxReturnStamp"];
export type TaxReturnMUHSGK = components["schemas"]["TaxReturnMUHSGK"];
export type TaxReturnBABS = components["schemas"]["TaxReturnBABS"];
export type TaxReturnBABSRow =
  components["schemas"]["TaxReturnBABSForm"]["rows"][number];
export type TaxReturnEmployee = TaxReturnMUHSGK["employees"][number];
export type TaxReturnAdjustments =
  components["schemas"]["TaxReturnAdjustments"];
export type TaxReturnStampLine = NonNullable<
  TaxReturnAdjustments["stamp_lines"]
>[number];
export type TaxReturnDraftInput = components["schemas"]["TaxReturnDraftInput"];
export type TaxReturnFileInput = components["schemas"]["TaxReturnFileInput"];
export type TaxReturnAmendment = components["schemas"]["TaxReturnAmendment"];
export type TaxReturnPayment = components["schemas"]["TaxReturnPayment"];
export type TaxReturnPaymentMovement =
  components["schemas"]["TaxReturnPaymentMovement"];

export const TAX_RETURN_KINDS: TaxReturnKind[] = [
  "kdv1",
  "kdv2",
  "damga",
  "muhsgk",
  "babs",
];

const root = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/tax-returns`;
const one = (engagement: string, kind: TaxReturnKind, period: string) =>
  `${root(engagement)}/${kind}/${encodeURIComponent(period)}`;

export const taxReturnKeys = {
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "tax-returns",
    ] as const,
  one: (slug: string, engagement: string, kind: string, period: string) =>
    [...taxReturnKeys.all(slug, engagement), kind, period] as const,
};

/** Beyanname önizlemesi ve beyan kayıtları (#390). */
export const taxReturnsService = {
  list(slug: string, engagement: string, year?: string) {
    return tenantRequest<{ items: TaxReturnSummary[] }>(
      slug,
      "GET",
      root(engagement),
      { query: { year } },
    );
  },
  get(slug: string, engagement: string, kind: TaxReturnKind, period: string) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "GET",
      one(engagement, kind, period),
    );
  },
  saveDraft(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    body: TaxReturnDraftInput,
  ) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "PUT",
      one(engagement, kind, period),
      { body, silent: true },
    );
  },
  prepare(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    acknowledged: string[],
  ) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "POST",
      `${one(engagement, kind, period)}/prepare`,
      { body: { acknowledged }, silent: true },
    );
  },
  reopen(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
  ) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "POST",
      `${one(engagement, kind, period)}/reopen`,
      { silent: true },
    );
  },
  /** Son kontrol (#669): yalnız sorumlu (tenant.tax_returns.approve). */
  check(slug: string, engagement: string, kind: TaxReturnKind, period: string) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "POST",
      `${one(engagement, kind, period)}/check`,
      { silent: true },
    );
  },
  file(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    body: TaxReturnFileInput,
  ) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "POST",
      `${one(engagement, kind, period)}/file`,
      { body, silent: true },
    );
  },
  /**
   * Tahakkuk ödemesi (#575): ödeme günü, istenirse ödeyen banka hareketi
   * (360'a işlenir). paid_on null ve hareket yoksa ödeme kaydı silinir.
   */
  setPayment(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    body: { paid_on: string | null; bank_transaction_uuid?: string | null },
  ) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "PUT",
      `${one(engagement, kind, period)}/payment`,
      { body, silent: true },
    );
  },
  /**
   * Düzeltme beyannamesi (#575): verilmiş beyan sürüm olarak arşivlenir,
   * beyan taslağa döner (sürüm + 1).
   */
  amend(slug: string, engagement: string, kind: TaxReturnKind, period: string) {
    return tenantRequest<TaxReturnPreview>(
      slug,
      "POST",
      `${one(engagement, kind, period)}/amend`,
      { silent: true },
    );
  },
  /** Tahakkuk PDF'i (#575): verilmiş beyana eklenir; yenisi eskisinin yerine geçer. */
  async uploadAccrual(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    file: File,
  ) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    const response = await fetch(
      `${base}${one(engagement, kind, period)}/accrual-document`,
      {
        method: "PUT",
        credentials: "include",
        headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
        body: form,
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<TaxReturnPreview>({ data: payload, response });
  },
  async downloadAccrual(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    fileName: string,
  ) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}${one(engagement, kind, period)}/accrual-document`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`accrual ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = fileName;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
  async exportFile(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
    format: "xlsx" | "pdf" = "xlsx",
  ) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}${one(engagement, kind, period)}/export?format=${format}`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`export ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${kind}-${period}.${format}`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
