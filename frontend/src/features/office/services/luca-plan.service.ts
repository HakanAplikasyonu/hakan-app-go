import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

import { accountMemoryKeys } from "./account-memory.service";

export type CariSubaccount = components["schemas"]["CariSubaccount"];
export type LucaPlanFile = components["schemas"]["LucaPlanFile"];
export type CariSubaccountGenerate =
  components["schemas"]["EnvelopeCariSubaccountGenerate"]["data"];
export type LucaPlanFileKind = LucaPlanFile["kind"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

export const lucaPlanKeys = {
  subaccounts: (slug: string, engagement: string) =>
    [...accountMemoryKeys.all(slug, engagement), "subaccounts"] as const,
  files: (slug: string, engagement: string) =>
    [...accountMemoryKeys.all(slug, engagement), "luca-files"] as const,
};

/**
 * Luca "Hesap Planı Aktar" file (#464): cari sub account codes, the CSV of
 * new accounts or the whole plan, and the download history.
 */
export const lucaPlanService = {
  subaccounts(slug: string, engagement: string) {
    return tenantRequest<{ items: CariSubaccount[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/subaccounts`,
    );
  },
  generate(slug: string, engagement: string) {
    return tenantRequest<CariSubaccountGenerate>(
      slug,
      "POST",
      `${eng(engagement)}/account-plan/subaccounts/generate`,
      { body: {}, silent: true },
    );
  },
  files(slug: string, engagement: string) {
    return tenantRequest<{ items: LucaPlanFile[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/luca-files`,
    );
  },
  /** Builds and downloads the CSV (Windows-1254, as Luca reads it). */
  async download(slug: string, engagement: string, kind: LucaPlanFileKind) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${root}${eng(engagement)}/account-plan/luca-files`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          Accept: "text/csv, application/json",
          "Content-Type": "application/json",
          [ORGANIZATION_HEADER]: slug,
        },
        body: JSON.stringify({ kind }),
      },
    );
    if (!response.ok) {
      const payload = await response.json().catch(() => undefined);
      return unwrap<never>({ data: payload, response });
    }
    const name =
      /filename="([^"]+)"/.exec(
        response.headers.get("Content-Disposition") ?? "",
      )?.[1] ?? `luca_hesap_plani_${kind}.csv`;
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = name;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
