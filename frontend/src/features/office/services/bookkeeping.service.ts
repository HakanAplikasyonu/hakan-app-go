import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

import type { LedgerType } from "../lib/ledger-type";

export type OperatingAccount = components["schemas"]["OperatingAccount"];
export type OperatingAccountRow = components["schemas"]["OperatingAccountRow"];
export type OperatingAccountCategory =
  components["schemas"]["OperatingAccountCategory"];
export type OperatingAccountInventory =
  components["schemas"]["OperatingAccountInventory"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

/** Dönem (YYYY-MM) ya da yıl (YYYY) görünümü. */
export type BookScope = { period: string } | { year: string };

export const bookkeepingKeys = {
  all: (slug: string, engagement: string) =>
    ["tenant", slug.toLowerCase(), "operating-account", engagement] as const,
  book: (slug: string, engagement: string, scope: BookScope) =>
    [...bookkeepingKeys.all(slug, engagement), scope] as const,
};

/** Defter türü ve işletme hesabı (#780). */
export const bookkeepingService = {
  setLedgerType(slug: string, engagement: string, ledgerType: LedgerType) {
    return tenantRequest<{ ledger_type: LedgerType }>(
      slug,
      "PUT",
      `${eng(engagement)}/ledger-type`,
      { body: { ledger_type: ledgerType } },
    );
  },
  book(slug: string, engagement: string, scope: BookScope) {
    return tenantRequest<OperatingAccount>(
      slug,
      "GET",
      `${eng(engagement)}/operating-account`,
      { query: scope },
    );
  },
  setCategory(
    slug: string,
    engagement: string,
    body: {
      source: OperatingAccountRow["source"];
      record_uuid: string;
      category: OperatingAccountCategory | "";
    },
  ) {
    return tenantRequest<void>(
      slug,
      "PUT",
      `${eng(engagement)}/operating-account/categories`,
      { body },
    );
  },
  setInventory(
    slug: string,
    engagement: string,
    body: { year: number; opening: string; closing: string | null },
  ) {
    return tenantRequest<OperatingAccountInventory>(
      slug,
      "PUT",
      `${eng(engagement)}/operating-account/inventory`,
      { body },
    );
  },
};
