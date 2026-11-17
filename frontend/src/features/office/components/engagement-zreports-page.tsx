"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Banknote,
  Check,
  Clock,
  CreditCard,
  Download,
  ListChecks,
  Store,
  Upload,
  X,
} from "lucide-react";
import Link from "next/link";
import { Suspense, useEffect, useMemo, useRef } from "react";
import { toast } from "sonner";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
  formatCellMoney,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useOfficePeriod } from "../period";
import {
  type RetailSale,
  receiptKeys,
  receiptsService,
} from "../services/receipts.service";
import { approvalVariants, errText } from "./engagement-expenses-page";
import { EngagementTabs } from "./engagement-overview";
import { Money } from "./office-ui";
import { ImportResultNote, SummaryCards, useImportFile } from "./receipts-ui";

type StatusFilter = "all" | "pending" | "approved" | "rejected";
const STATUS_FILTERS: StatusFilter[] = [
  "all",
  "pending",
  "approved",
  "rejected",
];
const PAGE = 50;

const warningClass: Record<string, string> = {
  Z_NUMBER_GAP: "bg-warn-tint text-warn",
  Z_NUMBER_DUPLICATE: "bg-warn-tint text-warn",
  Z_DAY_GAP: "bg-surface-3 text-text-2",
};

/**
 * `/t/{slug}/engagements/{uuid}/z-reports` (#384): ÖKC seri no bazında
 * günlük Z raporları, KDV oranı ve ödeme tipi kırılımı, süreklilik
 * uyarıları (atlanan numara / gün), onaydan satış fişi taslağı, Excel.
 */
export function EngagementZReportsPage({ engagement }: { engagement: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <ZReportsInner engagement={engagement} />
    </Suspense>
  );
}

function ZReportsInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.receipts.write);
  const canApprove = can(permissions.tenant.receipts.approve);
  const qc = useQueryClient();
  const { period } = useOfficePeriod();
  const matrix = usePeriodMatrix(1);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  const listState = useUrlListState({ initialPageSize: PAGE });
  const rawStatus = listState.getParam("status") as StatusFilter | null;
  const status: StatusFilter =
    rawStatus && STATUS_FILTERS.includes(rawStatus) ? rawStatus : "all";
  const okc = listState.getParam("okc") ?? "";
  const { limit, offset } = listState.params;

  const lastPeriod = useRef(period);
  const { onPaginationChange } = listState;
  useEffect(() => {
    if (lastPeriod.current === period) return;
    lastPeriod.current = period;
    onPaginationChange((p) => ({ ...p, pageIndex: 0 }));
  }, [onPaginationChange, period]);

  const filter = {
    period,
    status: status === "all" ? undefined : status,
    okc: okc || undefined,
  };
  const list = useQuery({
    queryKey: [
      ...receiptKeys.retail(slug, engagement),
      { ...filter, limit, offset },
    ],
    queryFn: () =>
      receiptsService.retailSales(slug, engagement, {
        ...filter,
        limit,
        offset,
      }),
    placeholderData: keepPreviousData,
  });
  const refresh = () =>
    void qc.invalidateQueries({
      queryKey: receiptKeys.retail(slug, engagement),
    });
  const decide = useMutation({
    mutationFn: (v: { uuid: string; approve: boolean }) =>
      receiptsService.decideRetailSale(slug, engagement, v.uuid, v.approve),
    onSuccess: (x, v) => {
      refresh();
      toast.success(
        v.approve
          ? x.journal
            ? t("office.zreports.approved_with_journal", {
                no: x.journal.entry_no,
              })
            : t("office.receipts.approved")
          : t("office.receipts.rejected"),
      );
    },
    onError: (e) => toast.error(errText(e, t("office.receipts.decide_failed"))),
  });
  const continuity = useMutation({
    mutationFn: () => receiptsService.runContinuity(slug, engagement),
    onSuccess: (r) => {
      refresh();
      toast.success(
        t("office.zreports.continuity_done", {
          count: r.reports_with_findings,
        }),
      );
    },
    onError: (e) =>
      toast.error(errText(e, t("office.zreports.continuity_failed"))),
  });
  const imp = useImportFile(
    (file) => receiptsService.importRetailSales(slug, engagement, file),
    refresh,
  );
  const money = (v: string) => formatCellMoney(v, "TRY");

  const columns = useMemo<ColumnDef<RetailSale, unknown>[]>(() => {
    const money = (v: string) => formatCellMoney(v, "TRY");
    return [
      createDateColumn<RetailSale>({
        id: "sale_date",
        labelKey: "office.col_date",
        accessor: (x) => x.sale_date,
        className: "text-sm",
        gridSecondary: true,
      }),
      createColumn<RetailSale>({
        id: "okc",
        labelKey: "office.zreports.col_device",
        accessorFn: (x) => x.okc_serial_number ?? "",
        gridPrimary: true,
        cell: ({ row }) => {
          const x = row.original;
          return (
            <>
              <div className="font-mono text-sm whitespace-nowrap">
                Z {x.z_report_number ?? "—"}
              </div>
              <div className="text-text-3 font-mono text-xs whitespace-nowrap">
                {x.okc_serial_number ?? t("office.zreports.no_device")}
                {x.receipt_count != null ? (
                  <span className="ms-1 font-sans">
                    ·{" "}
                    {t("office.zreports.receipts", { count: x.receipt_count })}
                  </span>
                ) : null}
              </div>
            </>
          );
        },
      }),
      createColumn<RetailSale>({
        id: "vat_lines",
        labelKey: "office.zreports.col_vat",
        accessorFn: (x) => x.vat_lines.length,
        meta: { cellClassName: "min-w-48" },
        cell: ({ row }) => {
          const x = row.original;
          if (!x.vat_lines.length)
            return (
              <span className="text-text-3 text-xs">
                {t("office.zreports.no_breakdown")}
              </span>
            );
          return (
            <ul className="space-y-0.5 text-xs tabular-nums">
              {x.vat_lines.map((l) => (
                <li key={l.tax_rate} className="whitespace-nowrap">
                  <span className="text-text-2 inline-block w-10">
                    %{l.tax_rate}
                  </span>
                  {money(l.tax_base)}{" "}
                  <span className="text-text-3">+ {money(l.tax_amount)}</span>
                </li>
              ))}
              {x.breakdown_mismatch ? (
                <li className="text-warn font-medium">
                  {t("office.zreports.breakdown_mismatch")}
                </li>
              ) : null}
            </ul>
          );
        },
      }),
      createColumn<RetailSale>({
        id: "payments",
        labelKey: "office.zreports.col_payments",
        accessorFn: (x) => x.card_amount ?? "",
        cell: ({ row }) => {
          const x = row.original;
          return (
            <div className="space-y-0.5 text-xs whitespace-nowrap tabular-nums">
              <div>
                <span className="text-text-2">{t("office.zreports.cash")}</span>{" "}
                {x.cash_amount != null ? money(x.cash_amount) : "—"}
              </div>
              <div>
                <span className="text-text-2">{t("office.zreports.card")}</span>{" "}
                {x.card_amount != null ? money(x.card_amount) : "—"}
              </div>
            </div>
          );
        },
      }),
      createMoneyColumn<RetailSale>({
        id: "total_amount",
        labelKey: "office.zreports.col_total",
        accessor: (x) => x.total_amount,
        currency: (x) => x.currency,
        className: "font-semibold",
      }),
      createColumn<RetailSale>({
        id: "warnings",
        labelKey: "office.zreports.col_warnings",
        accessorFn: (x) => x.warnings.join(","),
        cell: ({ row }) =>
          row.original.warnings.length ? (
            <div className="flex flex-wrap gap-1">
              {row.original.warnings.map((w) => (
                <span
                  key={w}
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[11px]",
                    warningClass[w] ?? "bg-surface-3",
                  )}
                >
                  {t(`office.warning.${w}`)}
                </span>
              ))}
            </div>
          ) : (
            <span className="text-text-3">—</span>
          ),
      }),
      createStatusColumn<RetailSale>({
        id: "approval_status",
        labelKey: "office.receipts.col_status",
        accessor: (x) => x.approval_status,
        variants: approvalVariants,
      }),
      createColumn<RetailSale>({
        id: "journal",
        labelKey: "office.receipts.col_journal",
        accessorFn: (x) => x.journal?.entry_no ?? "",
        cell: ({ row }) =>
          row.original.journal ? (
            <Link
              href={routes.tenant.journals(slug)}
              className="text-brand-text font-mono text-sm hover:underline"
            >
              #{row.original.journal.entry_no}
            </Link>
          ) : (
            <span className="text-text-3">—</span>
          ),
      }),
      createActionsColumn<RetailSale>({
        cell: (x) =>
          canApprove && x.approval_status === "pending" ? (
            <div className="flex justify-end gap-1.5">
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ uuid: x.uuid, approve: true })}
              >
                <Check className="size-4" aria-hidden />
                {t("office.receipts.approve")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                aria-label={t("office.receipts.reject")}
                onClick={() => decide.mutate({ uuid: x.uuid, approve: false })}
              >
                <X className="size-4" aria-hidden />
              </Button>
            </div>
          ) : null,
      }),
    ];
  }, [t, slug, canApprove, decide]);

  const data = list.data;
  const filters = STATUS_FILTERS.map((k) => ({
    key: k,
    label:
      k === "all"
        ? t("office.receipts.filter_all")
        : t(`office.receipts.status_${k}`),
  }));

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="font-display truncate text-2xl font-bold">
            {row?.taxpayer_title ?? t("office.zreports.title")}
          </h1>
          <p className="text-text-2 text-sm">
            {t("office.zreports.description")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {canWrite ? (
            <Button
              variant="outline"
              size="sm"
              className="max-md:min-h-11"
              disabled={continuity.isPending}
              onClick={() => continuity.mutate()}
            >
              <ListChecks className="size-4" aria-hidden />
              {t("office.zreports.continuity")}
            </Button>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            onClick={() =>
              void receiptsService
                .exportRetailSales(slug, engagement, filter)
                .catch(() => toast.error(t("office.receipts.export_failed")))
            }
          >
            <Download className="size-4" aria-hidden />
            {t("office.receipts.export")}
          </Button>
          {canWrite ? (
            <Button
              variant="outline"
              size="sm"
              className="max-md:min-h-11"
              onClick={imp.open}
              disabled={imp.pending}
            >
              <Upload className="size-4" aria-hidden />
              {t("office.receipts.import")}
            </Button>
          ) : null}
          {imp.input}
        </div>
      </div>
      <EngagementTabs engagement={engagement} />

      <SummaryCards
        cards={[
          {
            label: t("office.zreports.total"),
            value: data ? <Money value={data.total_amount} /> : "–",
            icon: Store,
            box: "bg-brand-tint text-brand-text",
          },
          {
            label: t("office.zreports.cash_total"),
            value: data ? <Money value={data.cash_amount} /> : "–",
            icon: Banknote,
            box: "bg-ok-tint text-ok",
          },
          {
            label: t("office.zreports.card_total"),
            value: data ? <Money value={data.card_amount} /> : "–",
            icon: CreditCard,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.receipts.pending_count"),
            value: data ? data.pending : "–",
            icon: Clock,
            box: "bg-warn-tint text-warn",
          },
        ]}
      />

      {data && data.vat_rates.length ? (
        <section
          aria-label={t("office.zreports.vat_summary")}
          className="bg-surface border-line rounded-xl border p-4"
        >
          <h2 className="mb-2 text-sm font-semibold">
            {t("office.zreports.vat_summary")}
          </h2>
          <dl className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
            {data.vat_rates.map((r) => (
              <div key={r.tax_rate} className="flex flex-col">
                <dt className="text-text-2 text-xs">%{r.tax_rate}</dt>
                <dd className="tabular-nums">
                  {money(r.tax_base)}{" "}
                  <span className="text-text-3 text-xs">
                    KDV {money(r.tax_amount)}
                  </span>
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}

      <ImportResultNote result={imp.result} onClose={imp.clear} />

      <div className="flex flex-wrap items-center gap-2">
        <div
          role="group"
          aria-label={t("office.receipts.filter_label")}
          className="flex flex-wrap gap-2"
        >
          {filters.map((f) => (
            <button
              key={f.key}
              type="button"
              aria-pressed={status === f.key}
              onClick={() =>
                listState.setParams({ status: f.key === "all" ? null : f.key })
              }
              className={cn(
                "focus-visible:ring-ring min-h-11 rounded-full border px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none md:min-h-9",
                status === f.key
                  ? "border-brand bg-brand text-on-brand"
                  : "border-line bg-surface text-text-2 hover:text-foreground",
              )}
            >
              {f.label}
            </button>
          ))}
        </div>
        <label className="ms-auto flex items-center gap-2 text-xs">
          <span className="text-muted-foreground">
            {t("office.zreports.col_device")}
          </span>
          <select
            className="bg-background focus-visible:ring-ring h-9 rounded-md border px-2 font-mono text-sm focus-visible:ring-2 focus-visible:outline-none max-md:h-11"
            value={okc}
            onChange={(e) =>
              listState.setParams({ okc: e.target.value || null })
            }
          >
            <option value="">{t("office.receipts.filter_all")}</option>
            {(data?.devices ?? []).map((d) => (
              <option
                key={d.okc_serial_number || "-"}
                value={d.okc_serial_number || "-"}
              >
                {d.okc_serial_number || t("office.zreports.no_device")} (
                {d.reports})
              </option>
            ))}
          </select>
        </label>
      </div>

      <EntityTable<RetailSale>
        columns={columns}
        data={data?.items ?? []}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.zreports.empty_title")}
        emptyDescription={t("office.zreports.empty")}
        state={listState.tableState}
        rowCount={data?.total ?? 0}
        pageSizeOptions={[25, 50, 100]}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          persistKey: "office.zreports.table",
        }}
      />
    </div>
  );
}
