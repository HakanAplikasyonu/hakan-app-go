"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowDownLeft, ArrowUpRight, Download, Link2Off } from "lucide-react";
import { Suspense, useEffect, useMemo, useRef } from "react";
import { toast } from "sonner";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createStatusColumn,
  MoneyCell,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import type { components } from "@/generated/api";
import { EngagementNextWork } from "@/features/copilot";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { downloadFile } from "../services/download";
import { useOfficePeriod } from "../period";
import { BankAliasPanel } from "./bank-alias-panel";
import { BankPostingPanel } from "./bank-posting-panel";
import { BankStatementUpload } from "./bank-statement-upload";
import { Money } from "./office-ui";
import { useRelatedColumn } from "./related-column";

type BankPage = components["schemas"]["BankTransactionPage"];
type BankRow = components["schemas"]["BankTransaction"];
type MatchFilter = "all" | "unmatched" | "suggested" | "matched";
// #383: hareketler, hareket → mizan hesabı eşleştirme, karşı taraf takma adları.
type BankView = "movements" | "posting" | "aliases";
const VIEWS: BankView[] = ["movements", "posting", "aliases"];
const MATCH_FILTERS: MatchFilter[] = [
  "all",
  "unmatched",
  "suggested",
  "matched",
];
const PAGE = 50;

// Eşleşme rozetleri: önceki tint sınıflarıyla aynı tonlar.
const matchVariants: Record<string, StatusVariant> = {
  unmatched: { labelKey: "office.bank.match_unmatched", tone: "warn" },
  suggested: { labelKey: "office.bank.match_suggested", tone: "info" },
  matched: { labelKey: "office.bank.match_matched", tone: "ok" },
  ignored: { labelKey: "office.bank.match_ignored", tone: "neutral" },
};

function useBankColumns() {
  const { t } = useLocale();
  return useMemo<ColumnDef<BankRow, unknown>[]>(
    () => [
      createDateColumn<BankRow>({
        id: "transaction_date",
        labelKey: "office.col_date",
        accessor: (x) => x.transaction_date,
        className: "text-sm",
        gridSecondary: true,
      }),
      createColumn<BankRow>({
        id: "description",
        labelKey: "office.bank.col_description",
        accessorFn: (x) => x.counterparty_name || x.description,
        gridPrimary: true,
        meta: { cellClassName: "min-w-64" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <>
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
            </>
          );
        },
      }),
      createColumn<BankRow>({
        id: "cash_box_name",
        labelKey: "office.bank.col_account",
        accessorKey: "cash_box_name",
        meta: { cellClassName: "text-text-2 text-sm whitespace-nowrap" },
      }),
      // İşaretli tutar (+ giriş / − çıkış); yön ekran okuyucuya ayrıca okunur.
      createColumn<BankRow>({
        id: "amount",
        labelKey: "office.bank.col_amount",
        accessorKey: "amount",
        meta: { align: "end" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <span
              className={cn(
                "font-semibold whitespace-nowrap",
                x.direction === "in" ? "text-ok" : "text-foreground",
              )}
            >
              <span className="sr-only">
                {t(`office.bank.direction_${x.direction}`)}:{" "}
              </span>
              {x.direction === "in" ? "+" : "−"}
              <MoneyCell value={x.amount} currency={x.currency} />
            </span>
          );
        },
      }),
      createStatusColumn<BankRow>({
        id: "match_status",
        labelKey: "office.bank.col_match",
        accessor: (x) => x.match_status,
        variants: matchVariants,
        fallback: matchVariants.ignored,
      }),
    ],
    [t],
  );
}

/**
 * `/t/{slug}/engagements/{uuid}/bank` (arayüz 6/9, #329): mükellefin banka
 * hareketleri. Dönem ofisin global seçicisinden; üstte giriş / çıkış ve
 * eşleşme özeti, eşleşme süzgeci ve sayfalama.
 */
export function EngagementBankPage({ engagement }: { engagement: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <EngagementBankInner engagement={engagement} />
    </Suspense>
  );
}

function EngagementBankInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { period } = useOfficePeriod();
  // Sayfa, sayfa boyutu ve eşleşme süzgeci URL'de (#381).
  const listState = useUrlListState({ initialPageSize: PAGE });
  const rawMatch = listState.getParam("match") as MatchFilter | null;
  const match: MatchFilter =
    rawMatch && MATCH_FILTERS.includes(rawMatch) ? rawMatch : "all";
  const setMatch = (m: MatchFilter) =>
    listState.setParams({ match: m === "all" ? null : m });
  const { limit, offset } = listState.params;
  const baseColumns = useBankColumns();
  // Satırın ilişkili kayıtları: mükellef detayında seçili açılır (#469).
  const relatedColumn = useRelatedColumn<BankRow>(
    "bank_transaction",
    (x) => x.uuid,
    (x) => x.description,
  );
  const columns = useMemo<ColumnDef<BankRow, unknown>[]>(
    () => [...baseColumns, ...relatedColumn],
    [baseColumns, relatedColumn],
  );
  const rawView = listState.getParam("view") as BankView | null;
  const view: BankView =
    rawView && VIEWS.includes(rawView) ? rawView : "movements";

  // Üst çubuktaki dönem URL dışında değişir; liste ilk sayfaya döner.
  const lastPeriod = useRef(period);
  const { onPaginationChange } = listState;
  useEffect(() => {
    if (lastPeriod.current === period) return;
    lastPeriod.current = period;
    onPaginationChange((p) => ({ ...p, pageIndex: 0 }));
  }, [onPaginationChange, period]);

  const list = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "bank-transactions",
      engagement,
      { period, match, limit, offset },
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
            limit,
            offset,
          },
        },
      ),
    placeholderData: keepPreviousData,
  });
  const items = list.data?.items ?? [];
  const sum = list.data?.summary;
  // Özet eşleşme süzgecinden bağımsız sayar; seçili süzgecin toplamı oradan.
  const total = sum ? (match === "all" ? sum.total : sum[match]) : 0;
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

  const header = (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display truncate text-xl font-bold">
            {t("office.bank.title")}
          </h2>
          <p className="text-text-2 text-sm">{t("office.bank.description")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {/* #395: süzgece (dönem, eşleşme) uyan hareketler .xlsx olarak. */}
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            onClick={() =>
              void downloadFile(
                slug,
                `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-transactions/export`,
                {
                  period,
                  match_status: match === "all" ? undefined : match,
                },
                "banka-hareketleri.xlsx",
              ).catch(() => toast.error(t("office.bank.export_failed")))
            }
          >
            <Download className="size-4" aria-hidden />
            {t("office.bank.export")}
          </Button>
          <BankStatementUpload engagement={engagement} />
        </div>
      </div>
      <EngagementNextWork engagement={engagement} />
      <div
        role="tablist"
        aria-label={t("office.bank.view_label")}
        className="border-line flex gap-1 overflow-x-auto border-b"
      >
        {VIEWS.map((v) => (
          <button
            key={v}
            type="button"
            role="tab"
            aria-selected={view === v}
            onClick={() =>
              listState.setParams({ view: v === "movements" ? null : v })
            }
            className={cn(
              "focus-visible:ring-ring -mb-px min-h-11 border-b-2 px-3 text-sm font-medium whitespace-nowrap focus-visible:ring-2 focus-visible:outline-none md:min-h-9",
              view === v
                ? "border-brand text-foreground"
                : "text-text-2 hover:text-foreground border-transparent",
            )}
          >
            {t(`office.bank.view_${v}`)}
          </button>
        ))}
      </div>
    </>
  );

  if (view !== "movements") {
    return (
      <div className="space-y-5">
        {header}
        {view === "posting" ? (
          <BankPostingPanel engagement={engagement} period={period} />
        ) : (
          <BankAliasPanel engagement={engagement} />
        )}
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {header}

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

      <EntityTable<BankRow>
        columns={columns}
        data={items}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.bank.empty_title")}
        emptyDescription={t("office.bank.empty")}
        state={listState.tableState}
        rowCount={total}
        pageSizeOptions={[25, 50, 100]}
        // Sunucu sıralama ve serbest arama sunmuyor; yalnızca sayfalama.
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          persistKey: "office.bank.table",
        }}
      />
    </div>
  );
}
