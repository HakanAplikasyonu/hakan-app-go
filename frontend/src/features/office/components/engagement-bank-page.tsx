"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowDownLeft, ArrowUpRight, Download, Link2Off } from "lucide-react";
import { useRouter } from "next/navigation";
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
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
import { Skeleton } from "@/components/ui/skeleton";
import type { components } from "@/generated/api";
import { EngagementNextWork } from "@/features/copilot";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementScope } from "../engagement-scope";
import { useLedgerType } from "./ledger-type";
import { LucaListExport } from "./luca-list-export";
import { downloadFile } from "../services/download";
import { useUrlOfficePeriod } from "../period";
import { BankAliasPanel } from "./bank-alias-panel";
import { BankLedgerDifferencePanel } from "./bank-ledger-difference";
import { BankPostingPanel } from "./bank-posting-panel";
import { BankStatementUpload } from "./bank-statement-upload";
import { LucaBankExportPanel } from "./luca-bank-export-panel";
import { Money } from "./office-ui";
import { useFocusHref, useRelatedColumn } from "./related-column";

type BankPage = components["schemas"]["BankTransactionPage"];
type BankRow = components["schemas"]["BankTransaction"];
type MatchFilter = "all" | "unmatched" | "suggested" | "matched";
// #383: hareketler, hareket → mizan hesabı eşleştirme, karşı taraf takma adları.
// #465: Luca banka ekstresi CSV'si ve eşleme rehberi.
// #780 C: ekstre bakiyesi ile 102 bakiyesi farkı ve nedenleri.
type BankView = "movements" | "posting" | "difference" | "aliases" | "luca";
const VIEWS: BankView[] = [
  "movements",
  "posting",
  "difference",
  "aliases",
  "luca",
];
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
  // Faturası olmayan ama hesabı onaylanmış hareket (vergi, SGK, maaş,
  // masraf): iş bitmiştir, "Eşleşmedi" uyarısı gibi görünmez (#710).
  booked: { labelKey: "office.bank.match_booked", tone: "ok" },
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
              {/* Açıklama kısalır; "olası mükerrer" ve "Luca'ya N kez
                  aktarıldı" işaretleri hiç kesilmez (dar ekranda da). */}
              <div className="text-text-3 flex max-w-md min-w-0 flex-wrap items-baseline text-xs">
                {x.counterparty_name ? (
                  <span className="max-w-full min-w-0 truncate">
                    {x.description}
                  </span>
                ) : null}
                {x.possible_duplicate ? (
                  <span className="text-warn ms-1 font-semibold whitespace-nowrap">
                    · {t("office.bank.possible_duplicate")}
                  </span>
                ) : null}
                {x.luca_exports ? (
                  <span
                    className={cn(
                      "ms-1 whitespace-nowrap",
                      x.luca_exports > 1
                        ? "text-warn font-semibold"
                        : "text-text-3",
                    )}
                  >
                    ·{" "}
                    {x.luca_exports > 1
                      ? t("office.bank.luca.exported_again", {
                          count: x.luca_exports,
                        })
                      : t("office.bank.luca.exported_once")}
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
      // Karşı hesabı onaylanmış hareket (#565): faturayla eşleşmese de
      // hangi hesaba işlendiği listede görünür.
      createColumn<BankRow>({
        id: "booked_account",
        labelKey: "office.bank.col_booked",
        accessorFn: (x) => x.booked_account_code ?? "",
        meta: { cellClassName: "whitespace-nowrap" },
        cell: ({ row }) => {
          const x = row.original;
          if (!x.booked_account_code)
            return <span className="text-text-3">—</span>;
          // Uzun hesap adı (dar ekranda kart) taşmaz: kısalır, tamamı ipucunda.
          return (
            <span className="inline-flex max-w-full min-w-0 items-baseline">
              <span className="font-mono text-sm font-semibold">
                {x.booked_account_code}
              </span>
              {x.booked_account_name ? (
                <span
                  className="text-text-2 ms-1.5 max-w-48 min-w-0 truncate text-xs"
                  title={x.booked_account_name}
                >
                  {x.booked_account_name}
                </span>
              ) : null}
            </span>
          );
        },
      }),
      createStatusColumn<BankRow>({
        id: "match_status",
        labelKey: "office.bank.col_match",
        accessor: (x) =>
          x.match_status === "unmatched" && x.booked_account_code
            ? "booked"
            : x.match_status,
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
  const { singleEntry } = useLedgerType(engagement);
  const { t } = useLocale();
  const { slug } = useTenant();
  // Dönem adreste (`?period=`): uyarıdan gelen bağlantı o ayı açar (#710).
  const { period, focus } = useUrlOfficePeriod();
  // Sayfa, sayfa boyutu ve eşleşme süzgeci URL'de (#381).
  const listState = useUrlListState({ initialPageSize: PAGE });
  const rawMatch = listState.getParam("match") as MatchFilter | null;
  const match: MatchFilter =
    rawMatch && MATCH_FILTERS.includes(rawMatch) ? rawMatch : "all";
  const setMatch = (m: MatchFilter) =>
    listState.setParams({ match: m === "all" ? null : m });
  const { limit, offset } = listState.params;
  const baseColumns = useBankColumns();
  // Satıra tıklayınca hareket seçili açılır: ayrıntı ve fatura eşleştirme (#528).
  const scope = useEngagementScope();
  const focusHref = useFocusHref();
  const router = useRouter();
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
      {/* Ortak sekme başlığı (#710): başlık, kısa açıklama; eylemlerde tek
          birincil (ekstre yükle), dışa aktarma ikincil. */}
      <PageHeader
        headingLevel={2}
        title={t("office.bank.title")}
        description={t("office.bank.description")}
        actions={
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
            {/* Luca'ya aktar (Excel): dönemin hareketleri Luca fiş dosyası. */}
            <LucaListExport
              engagement={engagement}
              period={focus}
              lists={["bank-transactions"]}
            />
            <BankStatementUpload engagement={engagement} variant="default" />
          </div>
        }
      />
      <EngagementNextWork engagement={engagement} />
      <TabStrip aria-label={t("office.bank.view_label")}>
        {VIEWS.filter(
          // İşletme defteri / serbest meslek (#780): hesap kodu ve Luca yok.
          (v) => !singleEntry || (v !== "posting" && v !== "luca"),
        ).map((v) => (
          <TabStripItem
            key={v}
            selected={view === v}
            onClick={() =>
              listState.setParams({ view: v === "movements" ? null : v })
            }
          >
            {t(`office.bank.view_${v}`)}
          </TabStripItem>
        ))}
      </TabStrip>
    </>
  );

  if (view !== "movements") {
    return (
      <div className="space-y-5">
        {header}
        {singleEntry && (view === "posting" || view === "luca") ? (
          <p
            className="text-text-2 bg-surface border-line rounded-xl border p-4 text-sm"
            data-testid="bank-single-entry-note"
          >
            {t("office.bookkeeping.bank_note")}
          </p>
        ) : view === "posting" ? (
          <BankPostingPanel engagement={engagement} period={period} />
        ) : view === "difference" ? (
          <BankLedgerDifferencePanel
            engagement={engagement}
            period={period ?? ""}
          />
        ) : view === "luca" ? (
          <LucaBankExportPanel engagement={engagement} period={period} />
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
            value: sum ? sum.unmatched - sum.unmatched_booked : "–",
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
          <FilterChip
            key={f.key}
            pressed={match === f.key}
            onPressedChange={() => setMatch(f.key)}
          >
            {f.label}
            {f.count !== undefined ? (
              <span className="ms-1.5 tabular-nums">{f.count}</span>
            ) : null}
          </FilterChip>
        ))}
      </div>

      <EntityTable<BankRow>
        columns={columns}
        data={items}
        getRowId={(x) => x.uuid}
        onRowClick={
          scope
            ? (x) =>
                router.push(focusHref("bank_transaction", x.uuid), {
                  scroll: false,
                })
            : undefined
        }
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
