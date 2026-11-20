"use client";

import { useQuery } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import { reviewKeys } from "../services/review.service";

export type ReviewTaxpayerCount = {
  engagement_uuid: string;
  total: number;
  oldest_created_at: string;
};

/**
 * Kuyruk mükellef bazında (#710): kuyruğun süzgeciyle (tür, dönem) her
 * mükellefin bekleyen öneri sayısı, çok olan önce. `total` kuyruğun
 * toplamıyla aynıdır. Karar verilince `reviewKeys.all` ile tazelenir.
 */
export function useReviewTaxpayers(filter: { kind?: string; period?: string }) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...reviewKeys.all(slug), "taxpayers", filter],
    queryFn: () =>
      tenantRequest<{ items: ReviewTaxpayerCount[]; total: number }>(
        slug,
        "GET",
        "/v1/tenant/review/taxpayers",
        {
          query: {
            ...(filter.kind ? { kind: filter.kind } : {}),
            ...(filter.period ? { period: filter.period } : {}),
          },
        },
      ),
  });
}

/**
 * Mükellef seçimi (Uyarılar'daki gibi): "Tümü" ve bekleyeni olan her
 * mükellef sayısıyla. Seçilen mükellefin işleri sırayla bitirilir.
 */
export function ReviewTaxpayers({
  kind,
  period,
  selected,
  onSelect,
}: {
  kind?: string;
  period?: string;
  selected: string;
  onSelect: (engagement: string) => void;
}) {
  const { t } = useLocale();
  const directory = useEngagementDirectory();
  const q = useReviewTaxpayers({ kind, period });
  const items = q.data?.items ?? [];
  if (!q.data) return null;
  const chosenMissing =
    selected !== "all" && !items.some((x) => x.engagement_uuid === selected);
  return (
    <div
      role="group"
      aria-label={t("office.review.by_taxpayer")}
      className="flex gap-1 overflow-x-auto pb-1"
      data-testid="review-taxpayers"
    >
      <Button
        size="sm"
        variant={selected === "all" ? "secondary" : "ghost"}
        aria-pressed={selected === "all"}
        className="shrink-0 pointer-coarse:h-11"
        onClick={() => onSelect("all")}
      >
        {t("office.review.taxpayer_all", { count: q.data.total })}
      </Button>
      {chosenMissing ? (
        <Button
          size="sm"
          variant="secondary"
          aria-pressed
          className="shrink-0 pointer-coarse:h-11"
          onClick={() => onSelect(selected)}
        >
          {t("office.review.taxpayer_count", {
            name: directory.entries.get(selected)?.title ?? "—",
            count: 0,
          })}
        </Button>
      ) : null}
      {items.map((x) => (
        <Button
          key={x.engagement_uuid}
          size="sm"
          variant={selected === x.engagement_uuid ? "secondary" : "ghost"}
          aria-pressed={selected === x.engagement_uuid}
          className="shrink-0 pointer-coarse:h-11"
          onClick={() => onSelect(x.engagement_uuid)}
        >
          {t("office.review.taxpayer_count", {
            name: directory.entries.get(x.engagement_uuid)?.title ?? "—",
            count: x.total,
          })}
        </Button>
      ))}
    </div>
  );
}
