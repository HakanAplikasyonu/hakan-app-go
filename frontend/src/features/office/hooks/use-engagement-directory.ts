"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { useTenant } from "@/features/tenant/providers/tenant-provider";

import { periodKeys, periodService } from "../services/periods.service";

export type EngagementEntry = {
  engagementUuid: string;
  taxpayerUuid: string;
  title: string;
  taxId: string;
};

/**
 * Engagement → mükellef lookup for office screens. Office-wide lists
 * (suggestions, warnings) name only the engagement; the period matrix is the
 * office-wide list that carries the taxpayer title, so one query (all pages,
 * a single period) feeds every screen.
 */
export function useEngagementDirectory(enabled = true) {
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: periodKeys.matrixAll(slug, { periods: 1 }),
    queryFn: () => periodService.matrixAll(slug, { periods: 1 }),
    staleTime: 5 * 60_000,
    enabled,
  });
  const entries = useMemo(() => {
    const map = new Map<string, EngagementEntry>();
    for (const row of query.data?.items ?? []) {
      map.set(row.engagement_uuid, {
        engagementUuid: row.engagement_uuid,
        taxpayerUuid: row.taxpayer_uuid,
        title: row.taxpayer_title,
        taxId: row.taxpayer_tax_id,
      });
    }
    return map;
  }, [query.data]);
  const list = useMemo(
    () =>
      [...entries.values()].sort((a, b) =>
        a.title.localeCompare(b.title, "tr"),
      ),
    [entries],
  );
  return { ...query, entries, list };
}
