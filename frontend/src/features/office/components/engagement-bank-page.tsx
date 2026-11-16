"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowDownLeft, ArrowUpRight, Link2Off } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useOfficePeriod } from "../period";
import { EngagementTabs } from "./engagement-overview";
import { Money, TableBox } from "./office-ui";

type BankPage = components["schemas"]["BankTransactionPage"];
type MatchFilter = "all" | "unmatched" | "suggested" | "matched";
const PAGE = 50;

const matchClass: Record<string, string> = {
  unmatched: "bg-warn-tint text-warn",
  suggested: "bg-brand-tint text-brand-text",
  matched: "bg-ok-tint text-ok",
  ignored: "bg-surface-3 text-text-2",
};

/**
 * `/t/{slug}/engagements/{uuid}/bank` (arayüz 6/9, #329): mükellefin banka
 * hareketleri. Dönem ofisin global seçicisinden; üstte giriş / çıkış ve
 * eşleşme özeti, eşleşme süzgeci ve sayfalama.
 */
export function EngagementBankPage({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { period } = useOfficePeriod();
  const matrix = usePeriodMatrix(1);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  const [match, setMatch] = useState<MatchFilter>("all");
  const key = `${period ?? "all"}|${match}`;
  const [page, setPage] = useState({ key, offset: 0 });
  const offset = page.key === key ? page.offset : 0;

  const list = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "bank-transactions",
      engagement,
      { period, match, offset },
    ],
    queryFn: () =>
      tenantRequest<BankPage>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-transactions`,
        {
          query: {
            period,
            match_status: match === "all" ? undefined : match,
            limit: PAGE,
            offset,
          },
        },
      ),
    placeholderData: keepPreviousData,
  });
  const items = list.data?.items ?? [];
  const sum = list.data?.summary;
  const fmtDate = (d: string) =>
    new Date(`${d}T00:00:00`).toLocaleDateString(
      locale === "tr" ? "tr-TR" : "en-GB",
    );
  const filters: { key: MatchFilter; label: string; count?: number }[] = [
    { key: "all", label: t("office.bank.filter_all"), count: sum?.total },
    {
      key: "unmatched",
      label: t("office.bank.match_unmatched"),
      count: sum?.unmatched,
    },
    {
      key: "suggested",
      label: t("office.bank.match_suggested"),
      count: sum?.suggested,
    },
    {
      key: "matched",
      label: t("office.bank.match_matched"),
      count: sum?.matched,
    },
  ];

  return (
    <div className="space-y-5">
      <div className="min-w-0">
        <h1 className="font-display truncate text-2xl font-bold">
          {row?.taxpayer_title ?? t("office.bank.title")}
        </h1>
        <p className="text-text-2 text-sm">{t("office.bank.description")}</p>
      </div>
      <EngagementTabs engagement={engagement} />

      <div className="grid gap-3 sm:grid-cols-3">
        {[
          {
            label: t("office.bank.in_total"),
            value: sum ? <Money value={sum.in_total} currency="TRY" /> : "–",
            icon: ArrowDownLeft,
            box: "bg-ok-tint text-ok",
          },
          {
            label: t("office.bank.out_total"),
            value: sum ? <Money value={sum.out_total} currency="TRY" /> : "–",
            icon: ArrowUpRight,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.bank.unmatched_count"),
            value: sum ? sum.unmatched : "–",
            icon: Link2Off,
            box: "bg-warn-tint text-warn",
          },
        ].map((c) => (
          <div
            key={c.label}
            className="bg-surface border-line flex items-center gap-3.5 rounded-xl border p-4"
          >
            <span
              aria-hidden
              className={cn(
                "flex size-10 shrink-0 items-center justify-center rounded-lg",
                c.box,
              )}
            >
              <c.icon className="size-5" />
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="font-display truncate text-xl font-bold tabular-nums">
                {c.value}
              </span>
              <span className="text-text-2 text-sm">{c.label}</span>
            </span>
          </div>
        ))}
      </div>

      <div
        role="group"
        aria-label={t("office.bank.filter_label")}
        className="flex flex-wrap gap-2"
      >
        {filters.map((f) => (
          <button
            key={f.key}
            type="button"
            aria-pressed={match === f.key}
            onClick={() => setMatch(f.key)}
            className={cn(
              "focus-visible:ring-ring min-h-11 rounded-full border px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none md:min-h-9",
              match === f.key
                ? "border-brand bg-brand text-on-brand"
                : "border-line bg-surface text-text-2 hover:text-foreground",
            )}
          >
            {f.label}
            {f.count !== undefined ? (
              <span className="ms-1.5 tabular-nums">{f.count}</span>
            ) : null}
          </button>
        ))}
      </div>

      <section className="bg-surface border-line overflow-hidden rounded-xl border">
        {list.isLoading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : list.isError ? (
          <ErrorState
            className="m-4"
            title={t("office.api.load_error")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : items.length === 0 ? (
          <BosDurum
            className="border-0"
            baslik={t("office.bank.empty_title")}
            aciklama={t("office.bank.empty")}
          />
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_date")}</th>
                <th>{t("office.bank.col_description")}</th>
                <th>{t("office.bank.col_account")}</th>
                <th className="text-right">{t("office.bank.col_amount")}</th>
                <th>{t("office.bank.col_match")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((x) => (
                <tr key={x.uuid}>
                  <td className="font-mono text-sm whitespace-nowrap">
                    {fmtDate(x.transaction_date)}
                  </td>
                  <td className="min-w-64">
                    <div className="max-w-md truncate">
                      {x.counterparty_name || x.description}
                    </div>
                    <div className="text-text-3 max-w-md truncate text-xs">
                      {x.counterparty_name ? x.description : null}
                      {x.possible_duplicate ? (
                        <span className="text-warn ms-1 font-semibold">
                          · {t("office.bank.possible_duplicate")}
                        </span>
                      ) : null}
                    </div>
                  </td>
                  <td className="text-text-2 text-sm whitespace-nowrap">
                    {x.cash_box_name}
                  </td>
                  <td
                    className={cn(
                      "text-right font-semibold whitespace-nowrap",
                      x.direction === "in" ? "text-ok" : "text-foreground",
                    )}
                  >
                    <span className="sr-only">
                      {t(`office.bank.direction_${x.direction}`)}:{" "}
                    </span>
                    {x.direction === "in" ? "+" : "−"}
                    <Money value={x.amount} currency={x.currency} />
                  </td>
                  <td>
                    <span
                      className={cn(
                        "rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
                        matchClass[x.match_status] ?? matchClass.ignored,
                      )}
                    >
                      {t(`office.bank.match_${x.match_status}`)}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </TableBox>
        )}
      </section>

      {sum && (match === "all" ? sum.total : items.length) > PAGE ? (
        <div className="flex items-center justify-end gap-2 text-xs">
          <span className="text-text-3">
            {t("office.api.page_of", {
              from: offset + 1,
              to: offset + items.length,
              total: sum.total,
            })}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() => setPage({ key, offset: Math.max(0, offset - PAGE) })}
          >
            {t("office.api.prev")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={items.length < PAGE}
            onClick={() => setPage({ key, offset: offset + PAGE })}
          >
            {t("office.api.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
