"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { Mail } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { components } from "@/generated/api";
import { usePeriodMatrix } from "@/features/office/hooks/use-office-data";
import { useOfficePeriod } from "@/features/office/period";
import { useBulkSendReports } from "@/features/reports/hooks/use-deliveries";
import type { ReportRange } from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { OfficeDeliveryHistory } from "./delivery-history";
import { ReportRangePicker } from "./report-range-picker";

type Row = components["schemas"]["PeriodMatrixRow"];

/**
 * `/t/{slug}/reports` (#386): ofisin mükellefleri; seçilenlere seçili
 * dönemin raporu tek seferde e-postalanır. Altta bütün gönderimler.
 */
export function OfficeReportsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.engagements.write);
  const { focus } = useOfficePeriod();
  const matrix = usePeriodMatrix(1);
  const [range, setRange] = useState<ReportRange>({
    kind: "month",
    period: focus,
  });
  const [selection, setSelection] = useState<RowSelectionState>({});
  const bulk = useBulkSendReports(slug);
  const rows = useMemo(() => matrix.data?.items ?? [], [matrix.data]);
  const selected = Object.keys(selection).filter((k) => selection[k]);

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      ...(canWrite ? [createSelectColumnDef<Row>()] : []),
      createColumn<Row>({
        accessorKey: "taxpayer_title",
        labelKey: "reports.office.col_taxpayer",
        cell: ({ row }) => (
          <Link
            className="font-medium hover:underline"
            href={routes.tenant.engagementReport(
              slug,
              row.original.engagement_uuid,
            )}
          >
            {row.original.taxpayer_title}
          </Link>
        ),
      }),
      createColumn<Row>({
        accessorKey: "taxpayer_tax_id",
        labelKey: "reports.office.col_tax_id",
        cell: ({ row }) => (
          <span className="font-mono tabular-nums">
            {row.original.taxpayer_tax_id}
          </span>
        ),
      }),
    ],
    [canWrite, slug],
  );

  const onSend = () => {
    bulk.mutate(
      { engagements: selected, range },
      {
        onSuccess: (r) => {
          if (r.skipped > 0) {
            toast.warning(
              t("reports.office.sent_partial", {
                queued: r.queued,
                skipped: r.skipped,
              }),
            );
          } else {
            toast.success(t("reports.office.sent", { queued: r.queued }));
          }
          setSelection({});
        },
        onError: () => toast.error(t("reports.send_error")),
      },
    );
  };

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-2xl font-bold">
          {t("reports.office.title")}
        </h1>
        <p className="text-text-2 text-sm">{t("reports.office.description")}</p>
      </div>
      <Card>
        <CardHeader className="space-y-3">
          <CardTitle className="text-base">
            {t("reports.office.bulk_title")}
          </CardTitle>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <ReportRangePicker value={range} onChange={setRange} />
            {canWrite ? (
              <Button
                onClick={onSend}
                disabled={selected.length === 0 || bulk.isPending}
              >
                <Mail aria-hidden className="size-4" />
                {bulk.isPending
                  ? t("reports.send_busy")
                  : t("reports.office.send_selected", {
                      count: selected.length,
                    })}
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent>
          <EntityTable
            columns={columns}
            data={rows}
            getRowId={(row) => row.engagement_uuid}
            isLoading={matrix.isLoading}
            isError={matrix.isError}
            onRetry={() => void matrix.refetch()}
            emptyTitle={t("reports.office.empty_title")}
            emptyDescription={t("reports.office.empty_description")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            state={{
              rowSelection: selection,
              onRowSelectionChange: setSelection,
            }}
            features={{
              persistKey: "office-reports-v1",
              rowSelection: canWrite,
            }}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("reports.history.office_title")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <OfficeDeliveryHistory />
        </CardContent>
      </Card>
    </div>
  );
}
