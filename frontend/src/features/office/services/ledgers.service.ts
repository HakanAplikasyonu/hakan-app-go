import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type LedgerKind = components["schemas"]["LedgerKind"];
export type LedgerPeriodKind = components["schemas"]["LedgerPeriodKind"];
export type LedgerFile = components["schemas"]["LedgerFile"];
export type LedgerList = components["schemas"]["LedgerList"];
export type LedgerPeriod = components["schemas"]["LedgerPeriod"];
export type LedgerDetail = components["schemas"]["LedgerDetail"];
export type LedgerSummary = components["schemas"]["LedgerSummary"];
export type LedgerUpload = components["schemas"]["LedgerUpload"];

export const LEDGER_KINDS: LedgerKind[] = [
  "isletme_defteri",
  "e_defter_yevmiye",
  "e_defter_kebir",
  "berat",
  "mizan",
  "bordro",
  "other",
];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/ledgers`;
const root = () => apiConfig.baseUrl.replace(/\/$/, "");

export const ledgerKeys = {
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "ledgers",
    ] as const,
  detail: (slug: string, engagement: string, uuid: string) =>
    [...ledgerKeys.all(slug, engagement), uuid] as const,
};

/**
 * Mükellef defterleri (#780): Luca işletme defteri, mizan ve ücret bordrosu
 * dökümleri ve e-defter dosyaları; dönem listesi, eksik dönem, dosyadan
 * okunan özet.
 */
export const ledgersService = {
  list(slug: string, engagement: string) {
    return tenantRequest<LedgerList>(slug, "GET", base(engagement));
  },
  get(slug: string, engagement: string, uuid: string) {
    return tenantRequest<LedgerDetail>(
      slug,
      "GET",
      `${base(engagement)}/${encodeURIComponent(uuid)}`,
    );
  },
  async upload(
    slug: string,
    engagement: string,
    u: { file: File; period: string; kind?: LedgerKind },
  ) {
    const form = new FormData();
    form.append("file", u.file);
    form.append("period", u.period);
    if (u.kind) form.append("kind", u.kind);
    const response = await fetch(`${root()}${base(engagement)}`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<LedgerUpload>({ data: payload, response }, { silent: true });
  },
  saveSettings(slug: string, engagement: string, kind: LedgerPeriodKind) {
    return tenantRequest<{ period_kind: LedgerPeriodKind }>(
      slug,
      "PUT",
      `${base(engagement)}/settings`,
      { body: { period_kind: kind } },
    );
  },
  remove(slug: string, engagement: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${base(engagement)}/${encodeURIComponent(uuid)}`,
    );
  },
  async download(slug: string, engagement: string, f: LedgerFile) {
    const response = await fetch(
      `${root()}${base(engagement)}/${encodeURIComponent(f.uuid)}/file`,
      { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
    );
    if (!response.ok) {
      const payload = await response.json().catch(() => undefined);
      return unwrap<never>({ data: payload, response });
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = f.file_name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  },
};
