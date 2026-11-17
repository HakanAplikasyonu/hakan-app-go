"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";

import { useLocale } from "@/providers/locale-provider";

import {
  useEngagementOptions,
  type EngagementOption,
} from "../hooks/use-office-invoices";
import { FilterSelect } from "./office-ui";

/** URL parameter holding the selected engagement (taxpayer) uuid. */
export const ENGAGEMENT_PARAM = "taxpayer";

/**
 * The selected engagement of the office's active ones. It lives in the URL
 * (?taxpayer=<engagement uuid>) so links keep it; the first one is picked
 * when none is given.
 */
export function useSelectedEngagement(
  slug: string,
  /** Mükellef bağlamında sabit mükellef: URL'ye yazılmaz (#468). */
  fixed?: string | null,
) {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const { options, isLoading, isError } = useEngagementOptions(slug);
  const wanted = fixed || (params.get(ENGAGEMENT_PARAM) ?? "");
  const selected =
    options.find((o) => o.engagement === wanted) ?? options[0] ?? null;

  const select = (engagement: string) => {
    const next = new URLSearchParams(params);
    next.set(ENGAGEMENT_PARAM, engagement);
    // Mükellef değişince liste sayfası başa döner (#381).
    next.delete("page");
    next.delete("offset");
    router.replace(`${pathname}?${next.toString()}`);
  };

  useEffect(() => {
    if (!fixed && !wanted && selected) select(selected.engagement);
    // select depends on params; run only when the default appears.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fixed, wanted, selected?.engagement]);

  return { options, selected, select, isLoading, isError };
}

export function EngagementPicker({
  options,
  selected,
  onSelect,
}: {
  options: EngagementOption[];
  selected: EngagementOption | null;
  onSelect: (engagement: string) => void;
}) {
  const { t } = useLocale();
  return (
    <FilterSelect
      label={t("office.col_taxpayer")}
      value={selected?.engagement ?? ""}
      onChange={onSelect}
      options={options.map((o) => ({
        value: o.engagement,
        label: o.title ? `${o.title} · ${o.taxId}` : o.engagement,
      }))}
    />
  );
}
