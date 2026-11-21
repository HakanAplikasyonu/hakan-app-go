"use client";

import { ArrowRightLeft, CircleCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import {
  createColumn,
  createDateColumn,
  createStatusColumn,
} from "@/components/tables";
import { EXPORT_ANCHOR, routes, TRANSFERS_ANCHOR } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useOfficePendingTransfers,
  useOfficeTransfers,
} from "../hooks/use-journals";
import { useOfficePeriod, usePeriodText } from "../period";
import type {
  OfficeJournalTransfer,
  PendingJournalTransfer,
} from "../services/journals.service";
import { TRANSFER_STATUS } from "./journal-transfers-panel";
import { Panel } from "./office-ui";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

/** Program adı (Luca, Zirve); bilinmeyen sürücü adı aynen. */
function program(t: Translate, name: string) {
  const key = `office.journals.driver_${name}`;
  const msg = t(key);
  return msg === key ? name : msg;
}

// Unvanla aranır; çok mükellefli ofiste satır sayfalar arasında kaybolmaz.
const tableFeatures = (persistKey: string) => ({
  globalFilter: true,
  columnFilters: false,
  facetedFilters: false,
  rowSelection: false,
  viewMode: false,
  persistKey,
});

/** Bekleyen satırın özeti: onaylı, aktarımda, hata verdi (fiş sayısıyla). */
function PendingCell({ row }: { row: PendingJournalTransfer }) {
  const { t } = useLocale();
  return (
    <span className="flex flex-col text-xs">
      {row.approved ? (
        <span className="text-status-warning">
          {t("office.transfers.pending_approved", { count: row.approved })}
        </span>
      ) : null}
      {row.in_transfer ? (
        <span>
          {t("office.transfers.pending_in_transfer", {
            count: row.in_transfer,
          })}
        </span>
      ) : null}
      {row.failed ? (
        <span className="text-status-blocking">
          {t("office.transfers.pending_failed", { count: row.failed })}
        </span>
      ) : null}
    </span>
  );
}

/**
 * Programa alınan dönemin mizan durumu (#574); yalnız aktarılanlarda.
 * Fark, eski ya da hiç karşılaştırma yoksa yazı mükellefin Mutabakat
 * ekranını o dönemle açar (#669 UX geçişi): farkın nedeni ve
 * "Yeniden karşılaştır" orada; satır tıklaması Fişler'e gider.
 */
function CheckCell({ tr }: { tr: OfficeJournalTransfer }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { setPeriod } = useOfficePeriod();
  const c = tr.check;
  if (!c) return <span className="text-muted-foreground text-xs">—</span>;
  if (c.status === "ok")
    return (
      <span className="text-status-approved inline-flex items-center gap-1 text-xs">
        <CircleCheck className="size-3.5" aria-hidden />
        {t("office.transfers.check_ok")}
      </span>
    );
  const [label, tone] =
    c.status === "differences"
      ? [
          t("office.transfers.check_differences", { count: c.issues }),
          "text-status-blocking",
        ]
      : c.status === "stale"
        ? [t("office.transfers.check_stale"), "text-status-warning"]
        : [t("office.transfers.check_none"), "text-status-warning"];
  return (
    <Link
      href={routes.tenant.engagementReconciliation(slug, tr.engagement_uuid)}
      className={`${tone} text-xs font-medium underline-offset-2 hover:underline`}
      aria-label={`${label}: ${t("office.transfers.open_reconciliation")}`}
      onClick={(e) => {
        // Satır tıklaması Fişler'i açar; bu bağlantı Mutabakat'ı.
        e.stopPropagation();
        if (tr.period) setPeriod(tr.period);
      }}
    >
      {label}
    </Link>
  );
}

/**
 * `/t/{slug}/transfers` (#574): ofis genelinde muhasebe programına
 * aktarım. Üstte onaylanmış ama programa ulaşmamış fişi olan mükellef ve
 * dönemler, altta her çıktının son aktarımı ve aktarılan dönemin mizan
 * durumu. Satır o mükellefin Fişler ekranını, dönemi seçili açar.
 */
export function OfficeTransfersPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const periodText = usePeriodText();
  const { setPeriod } = useOfficePeriod();
  const pending = useOfficePendingTransfers(slug);
  const transfers = useOfficeTransfers(slug);

  const open = (
    engagement: string,
    period: string | undefined,
    anchor: string,
  ) => {
    if (period) setPeriod(period);
    router.push(
      `${routes.tenant.engagementJournals(slug, engagement)}#${anchor}`,
    );
  };

  const pendingColumns = useMemo<ColumnDef<PendingJournalTransfer, unknown>[]>(
    () => [
      createColumn<PendingJournalTransfer>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.taxpayer_title}</span>
        ),
      }),
      createColumn<PendingJournalTransfer>({
        id: "period",
        labelKey: "office.api.col_period",
        accessorKey: "period",
        gridSecondary: true,
        cell: ({ row }) => periodText(row.original.period),
      }),
      createColumn<PendingJournalTransfer>({
        id: "pending",
        labelKey: "office.transfers.col_pending",
        enableSorting: false,
        cell: ({ row }) => <PendingCell row={row.original} />,
      }),
    ],
    [periodText],
  );

  const transferColumns = useMemo<ColumnDef<OfficeJournalTransfer, unknown>[]>(
    () => [
      createColumn<OfficeJournalTransfer>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.taxpayer_title}</span>
        ),
      }),
      createColumn<OfficeJournalTransfer>({
        id: "period",
        labelKey: "office.api.col_period",
        accessorFn: (r) => r.period ?? "",
        cell: ({ row }) => periodText(row.original.period),
      }),
      createColumn<OfficeJournalTransfer>({
        id: "driver",
        labelKey: "office.journals.driver",
        accessorFn: (r) => program(t, r.driver),
      }),
      createStatusColumn<OfficeJournalTransfer>({
        id: "status",
        labelKey: "office.col_status",
        accessor: (r) => r.status,
        variants: TRANSFER_STATUS,
        gridSecondary: true,
      }),
      createColumn<OfficeJournalTransfer>({
        id: "check",
        labelKey: "office.transfers.col_check",
        enableSorting: false,
        cell: ({ row }) => <CheckCell tr={row.original} />,
      }),
      createDateColumn<OfficeJournalTransfer>({
        id: "created_at",
        labelKey: "office.journals.transfer_col_requested",
        accessor: (r) => r.created_at,
        style: "datetime",
      }),
    ],
    [t, periodText],
  );

  const pendingRows = pending.data?.items ?? [];
  const pendingEntries = pendingRows.reduce(
    (n, r) => n + r.approved + r.in_transfer + r.failed,
    0,
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.transfers.title")}
        description={t("office.transfers.description")}
        icon={<ArrowRightLeft className="size-6" />}
      />
      <Panel
        title={t("office.transfers.pending_title")}
        description={
          pendingRows.length
            ? t("office.transfers.pending_summary", {
                periods: pendingRows.length,
                entries: pendingEntries,
              })
            : t("office.transfers.pending_description")
        }
      >
        <div className="p-4">
          <EntityTable<PendingJournalTransfer>
            columns={pendingColumns}
            data={pendingRows}
            getRowId={(r) => `${r.engagement_uuid}|${r.period}`}
            onRowClick={(r) => open(r.engagement_uuid, r.period, EXPORT_ANCHOR)}
            isLoading={pending.isLoading}
            isError={pending.isError}
            errorTitle={t("office.load_failed")}
            onRetry={() => void pending.refetch()}
            emptyTitle={t("office.transfers.pending_empty")}
            emptyDescription=""
            manual={{ sorting: false, filtering: false, pagination: false }}
            initialState={{ pagination: { pageIndex: 0, pageSize: 25 } }}
            pageSizeOptions={[25, 50, 100]}
            features={tableFeatures("office.transfers.pending")}
          />
        </div>
      </Panel>
      <Panel
        title={t("office.transfers.recent_title")}
        description={t("office.transfers.recent_description")}
      >
        <div className="p-4">
          <EntityTable<OfficeJournalTransfer>
            columns={transferColumns}
            data={transfers.data?.items ?? []}
            getRowId={(r) => r.uuid}
            onRowClick={(r) =>
              open(r.engagement_uuid, r.period, TRANSFERS_ANCHOR)
            }
            isLoading={transfers.isLoading}
            isError={transfers.isError}
            errorTitle={t("office.load_failed")}
            onRetry={() => void transfers.refetch()}
            emptyTitle={t("office.transfers.recent_empty")}
            emptyDescription=""
            manual={{ sorting: false, filtering: false, pagination: false }}
            initialState={{ pagination: { pageIndex: 0, pageSize: 25 } }}
            pageSizeOptions={[25, 50, 100]}
            features={tableFeatures("office.transfers.recent")}
          />
        </div>
      </Panel>
    </div>
  );
}
