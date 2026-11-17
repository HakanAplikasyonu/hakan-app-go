import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type TaxReturnKind = "kdv1" | "muhsgk" | "babs";
export type TaxReturnPreview = components["schemas"]["TaxReturnPreview"];
export type TaxReturnSummary = components["schemas"]["TaxReturnSummary"];
export type TaxReturnCheckItem = components["schemas"]["TaxReturnCheckItem"];
export type TaxReturnKDV1 = components["schemas"]["TaxReturnKDV1"];
export type TaxReturnMUHSGK = components["schemas"]["TaxReturnMUHSGK"];
export type TaxReturnBABS = components["schemas"]["TaxReturnBABS"];
export type TaxReturnBABSRow =
  components["schemas"]["TaxReturnBABSForm"]["rows"][number];
export type TaxReturnEmployee = TaxReturnMUHSGK["employees"][number];
export type TaxReturnAdjustments =
  components["schemas"]["TaxReturnKDV1Adjustments"];
export type TaxReturnDraftInput = components["schemas"]["TaxReturnDraftInput"];
export type TaxReturnFileInput = components["schemas"]["TaxReturnFileInput"];

export const TAX_RETURN_KINDS: TaxReturnKind[] = ["kdv1", "muhsgk", "babs"];

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
  async exportFile(
    slug: string,
    engagement: string,
    kind: TaxReturnKind,
    period: string,
  ) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}${one(engagement, kind, period)}/export`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) throw new Error(`export ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${kind}-${period}.xlsx`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
