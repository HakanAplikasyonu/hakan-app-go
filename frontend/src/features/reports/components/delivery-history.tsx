"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import {
  useOfficeReportDeliveries,
  useReportDeliveries,
} from "@/features/reports/hooks/use-deliveries";
import type { ReportDelivery } from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const statusVariant: Record<
  ReportDelivery["status"],
  "success" | "warning" | "danger" | "secondary"
> = {
  queued: "secondary",
  sending: "warning",
  sent: "success",
  failed: "danger",
};

function useColumns(withTaxpayer: boolean) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  return useMemo<ColumnDef<ReportDelivery>[]>(() => {
    const cols: ColumnDef<ReportDelivery>[] = [
      createColumn<ReportDelivery>({
        accessorKey: "created_at",
        labelKey: "reports.history.col_created",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {datetime(row.original.created_at, "dd.MM.yyyy HH:mm", locale)}
          </span>
        ),
      }),
    ];
    if (withTaxpayer) {
      cols.push(
        createColumn<ReportDelivery>({
          accessorKey: "taxpayer_title",
          labelKey: "reports.history.col_taxpayer",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.engagement_uuid ? (
              <Link
                className="hover:underline"
                href={routes.tenant.engagementReport(
                  slug,
                  row.original.engagement_uuid,
                )}
              >
                {row.original.taxpayer_title}
              </Link>
            ) : (
              row.original.taxpayer_title
            ),
        }),
      );
    }
    cols.push(
      createColumn<ReportDelivery>({
        accessorKey: "label",
        labelKey: "reports.history.col_period",
        enableSorting: false,
      }),
      createColumn<ReportDelivery>({
        accessorKey: "recipient",
        labelKey: "reports.history.col_recipient",
        enableSorting: false,
      }),
      createColumn<ReportDelivery>({
        accessorKey: "origin",
        labelKey: "reports.history.col_origin",
        enableSorting: false,
        cell: ({ row }) => t(`reports.origin.${row.original.origin}`),
      }),
      createColumn<ReportDelivery>({
        accessorKey: "status",
        labelKey: "reports.history.col_status",
        enableSorting: false,
        cell: ({ row }) => {
          const d = row.original;
          return (
            <div className="space-y-0.5">
              <Badge variant={statusVariant[d.status]}>
                {t(`reports.status.${d.status}`)}
              </Badge>
              {d.status === "sent" && d.sent_at ? (
                <div className="text-text-2 text-xs tabular-nums">
                  {datetime(d.sent_at, "dd.MM.yyyy HH:mm", locale)}
                </div>
              ) : null}
              {d.status !== "sent" && d.last_error ? (
                <div
                  className="text-text-2 max-w-64 truncate text-xs"
                  title={d.last_error}
                >
                  {d.last_error}
                </div>
              ) : null}
            </div>
          );
        },
      }),
    );
    return cols;
  }, [locale, slug, t, withTaxpayer]);
}

function pageCountOf(total: number | undefined, size: number) {
  return Math.max(1, Math.ceil((total ?? 0) / (size || 20)));
}

/** Bir mükellefin rapor gönderim geçmişi (#386). */
export function EngagementDeliveryHistory({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const list = useServerListState({ initialPageSize: 10 });
  const q = useReportDeliveries(slug, engagement, {
    limit: list.params.limit,
    offset: list.params.offset,
  });
  const columns = useColumns(false);
  return (
    <EntityTable
      columns={columns}
      data={q.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={q.isLoading}
      isError={q.isError}
      onRetry={() => void q.refetch()}
      emptyTitle={t("reports.history.empty_title")}
      emptyDescription={t("reports.history.empty_description")}
      pageCount={pageCountOf(q.data?.total, list.pagination.pageSize)}
      state={list.tableState}
      features={{ persistKey: "report-deliveries-v1", rowSelection: false }}
    />
  );
}

/** Ofisin bütün rapor gönderimleri (#386). */
export function OfficeDeliveryHistory() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const list = useServerListState({ initialPageSize: 20 });
  const q = useOfficeReportDeliveries(slug, {
    limit: list.params.limit,
    offset: list.params.offset,
  });
  const columns = useColumns(true);
  return (
    <EntityTable
      columns={columns}
      data={q.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={q.isLoading}
      isError={q.isError}
      onRetry={() => void q.refetch()}
      emptyTitle={t("reports.history.empty_title")}
      emptyDescription={t("reports.history.empty_description")}
      pageCount={pageCountOf(q.data?.total, list.pagination.pageSize)}
      state={list.tableState}
      features={{
        persistKey: "office-report-deliveries-v1",
        rowSelection: false,
      }}
    />
  );
}
