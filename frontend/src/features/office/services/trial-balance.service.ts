import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type TrialBalance = components["schemas"]["TrialBalance"];
export type TrialBalanceLine = components["schemas"]["TrialBalanceLine"];
export type TrialBalanceReport = components["schemas"]["TrialBalanceReport"];
export type LiveTrialBalanceReport =
  components["schemas"]["LiveTrialBalanceReport"];
export type CariBalanceReport = components["schemas"]["CariBalanceReport"];
export type CariBalanceGroup = components["schemas"]["CariBalanceGroup"];
export type CariBalanceAccount = components["schemas"]["CariBalanceAccount"];
export type AccountPlanImportResult =
  components["schemas"]["EnvelopeAccountPlanImportResult"]["data"];

export type TrialBalanceQuery = {
  /** `latest` or an import uuid. */
  which: string;
  compare: "previous" | "none";
  /** 0: all levels, 1: main accounts. */
  level: number;
};

/** Canlı mizan (#669 E): ofisin kendi fişlerinden. */
export type LiveTrialBalanceQuery = {
  /** `yyyy-MM`; Ocak'tan bu aya birikimli. */
  period: string;
  includeDrafts: boolean;
  compare: "none" | "luca";
  level: number;
};

/** Cari mizanı (#780 C): Ocak'tan bu aya birikimli. */
export type CariBalanceQuery = { period: string; includeDrafts: boolean };

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

export const trialBalanceKeys = {
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "trial-balances",
    ] as const,
  report: (slug: string, engagement: string, q: TrialBalanceQuery) =>
    [...trialBalanceKeys.all(slug, engagement), "report", q] as const,
  live: (slug: string, engagement: string, q: LiveTrialBalanceQuery) =>
    [...trialBalanceKeys.all(slug, engagement), "live", q] as const,
  cari: (slug: string, engagement: string, q: CariBalanceQuery) =>
    [...trialBalanceKeys.all(slug, engagement), "cari", q] as const,
};

function reportQuery(q: TrialBalanceQuery) {
  return {
    compare: q.compare,
    ...(q.level > 0 ? { level: String(q.level) } : {}),
  };
}

function liveQuery(q: LiveTrialBalanceQuery) {
  return {
    period: q.period,
    compare: q.compare,
    ...(q.includeDrafts ? { include_drafts: "true" } : {}),
    ...(q.level > 0 ? { level: String(q.level) } : {}),
  };
}

/** Dosyayı indirir; adı Content-Disposition'dan. */
async function downloadFile(url: string, slug: string, fallback: string) {
  const response = await fetch(url, {
    credentials: "include",
    headers: { [ORGANIZATION_HEADER]: slug },
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => undefined);
    return unwrap<never>({ data: payload, response });
  }
  const name =
    /filename="([^"]+)"/.exec(
      response.headers.get("Content-Disposition") ?? "",
    )?.[1] ?? fallback;
  const blob = await response.blob();
  const href = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = href;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(href);
}

/**
 * Mükellef mizanı (#669 E): Luca'dan yüklenen mizanlar, bir mizanın raporu
 * (önceki dönemle karşılaştırma) ve Excel / PDF dışa aktarımı.
 */
export const trialBalanceService = {
  /**
   * Cari mizanı (#780 C): her carinin borç / alacak / bakiyesi; ana hesap
   * toplamları canlı mizanla aynı.
   */
  cari(slug: string, engagement: string, q: CariBalanceQuery) {
    return tenantRequest<CariBalanceReport>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/cari-balances`,
      {
        query: {
          period: q.period,
          ...(q.includeDrafts ? { include_drafts: "true" } : {}),
        },
      },
    );
  },
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: TrialBalance[] }>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/trial-balances`,
    );
  },
  report(slug: string, engagement: string, q: TrialBalanceQuery) {
    return tenantRequest<TrialBalanceReport>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/trial-balances/${encodeURIComponent(q.which)}`,
      { query: reportQuery(q), silent: true },
    );
  },
  /** Luca mizanı yükler (hesap planı da güncellenir). */
  async upload(slug: string, engagement: string, file: File, period?: string) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    form.append("target", "luca");
    if (period) form.append("period", period);
    const response = await fetch(
      `${root}${eng(engagement)}/account-plan/imports`,
      {
        method: "POST",
        credentials: "include",
        headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
        body: form,
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<AccountPlanImportResult>({ data: payload, response });
  },
  /** Excel / PDF dosyasını indirir. */
  download(
    slug: string,
    engagement: string,
    q: TrialBalanceQuery,
    format: "xlsx" | "pdf",
    locale: string,
  ) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const params = new URLSearchParams({
      format,
      locale,
      ...reportQuery(q),
    });
    return downloadFile(
      `${root}${eng(engagement)}/account-plan/trial-balances/${encodeURIComponent(q.which)}/export?${params}`,
      slug,
      `mizan.${format}`,
    );
  },
  /** Canlı mizan: kendi fişlerimizden (onaylı ve aktarılmış). */
  live(slug: string, engagement: string, q: LiveTrialBalanceQuery) {
    return tenantRequest<LiveTrialBalanceReport>(
      slug,
      "GET",
      `${eng(engagement)}/account-plan/live-trial-balance`,
      { query: liveQuery(q), silent: true },
    );
  },
  /** Canlı mizanın Excel / PDF dosyası. */
  downloadLive(
    slug: string,
    engagement: string,
    q: LiveTrialBalanceQuery,
    format: "xlsx" | "pdf",
    locale: string,
  ) {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const params = new URLSearchParams({ format, locale, ...liveQuery(q) });
    return downloadFile(
      `${root}${eng(engagement)}/account-plan/live-trial-balance/export?${params}`,
      slug,
      `canli_mizan.${format}`,
    );
  },
};
