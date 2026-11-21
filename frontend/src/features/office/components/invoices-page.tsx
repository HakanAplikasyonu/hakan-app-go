"use client";

import { FileText, PanelRightOpen, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createMoneyColumn,
} from "@/components/tables";
import { TacRozeti } from "@/components/hakan";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementScope, useInvoiceHref } from "../engagement-scope";
import {
  useEngagementInvoices,
  useEngagementOptions,
  useFirstEngagementWithInvoices,
} from "../hooks/use-office-invoices";
import type { OfficeInvoice } from "../services/office-invoices.service";
import { directionOf } from "../lib/from-api";
import { invoiceKindText } from "../lib/ubl-codes";
import {
  ENGAGEMENT_PARAM,
  EngagementPicker,
  useSelectedEngagement,
} from "./engagement-picker";
import { InvoiceDrawer } from "./invoice-drawer";
import { InvoicePostings } from "./invoice-postings";
import { FilterSelect, Figure } from "./office-ui";
import { useUrlOfficePeriod } from "../period";
import { monthText } from "./vat-deduction-period";

type Dir = "all" | "outgoing" | "incoming";
const PAGE = 50;

// Tint token'ları (dört temada 4.5:1; saydam karışım seçili satırda yetmiyordu).
const stateClass: Record<string, string> = {
  active: "bg-ok-tint text-ok",
  draft: "bg-surface-3 text-text-2",
  cancelled: "bg-danger-tint text-danger",
};

/** status_normalized of the invoice (30 §8): counts in the declaration or not. */
export function InvoiceStateBadge({ status }: { status: string }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        stateClass[status] ?? stateClass.draft,
      )}
    >
      {t(`office.api.invoice_state_${status}`)}
    </span>
  );
}

type InvoiceRow = OfficeInvoice;

function useInvoiceColumns(engagement: string, period: string | null) {
  const { t, locale } = useLocale();
  const invoiceHref = useInvoiceHref();
  return useMemo<ColumnDef<InvoiceRow, unknown>[]>(
    () => [
      createColumn<InvoiceRow>({
        id: "signal",
        labelKey: "office.col_status",
        enableSorting: false,
        enableHiding: false,
        meta: { srOnlyHeader: true, cardHidden: true },
        size: 32,
        cell: ({ row }) => (
          <TacRozeti
            durum={row.original.open_warnings ? "sorun" : "yuklendi"}
            boyut={16}
          />
        ),
      }),
      createDateColumn<InvoiceRow>({
        id: "issue_date",
        labelKey: "office.col_date",
        accessor: (inv) => inv.issue_date,
        gridSecondary: true,
      }),
      createColumn<InvoiceRow>({
        id: "invoice_number",
        labelKey: "office.col_number",
        accessorKey: "invoice_number",
        gridPrimary: true,
        cell: ({ row }) => {
          const inv = row.original;
          return (
            <div className="flex items-center gap-1.5">
              <Link
                href={invoiceHref(inv.uuid, engagement)}
                className="hover:underline"
              >
                <Figure className="font-medium">{inv.invoice_number}</Figure>
              </Link>
              {inv.open_warnings ? (
                <span
                  className="text-status-warning inline-flex items-center gap-0.5 text-xs"
                  title={t("office.open_warnings", {
                    count: inv.open_warnings,
                  })}
                >
                  <TriangleAlert className="size-3.5" aria-hidden />
                  <Figure>{inv.open_warnings}</Figure>
                </span>
              ) : null}
              <VatMovedNote
                invoice={inv}
                period={period}
                text={(key, month) =>
                  t(key, { month: monthText(month, locale) })
                }
              />
            </div>
          );
        },
      }),
      createColumn<InvoiceRow>({
        id: "counterparty",
        labelKey: "office.col_counterparty",
        accessorFn: (inv) => inv.counterparty_name ?? "",
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="max-w-64 truncate">
              {row.original.counterparty_name ?? ""}
            </div>
            <Figure className="text-muted-foreground text-xs">
              {row.original.counterparty_tax_id ?? ""}
            </Figure>
          </div>
        ),
      }),
      createColumn<InvoiceRow>({
        id: "type",
        labelKey: "office.col_type",
        accessorFn: (inv) => inv.type,
        cell: ({ row }) => {
          const inv = row.original;
          const dir = directionOf(inv.direction);
          return (
            <div className="text-xs">
              <span
                className={
                  dir === "sales" ? "text-status-approved" : "text-brand"
                }
              >
                {t(`office.direction_${dir}`)}
              </span>
              <div className="text-muted-foreground">
                {invoiceKindText(inv.type, inv.profile, t)}
              </div>
            </div>
          );
        },
      }),
      createMoneyColumn<InvoiceRow>({
        id: "tax_exclusive_amount",
        labelKey: "office.col_base",
        accessor: (inv) => inv.tax_exclusive_amount,
        currency: (inv) => inv.currency,
      }),
      createMoneyColumn<InvoiceRow>({
        id: "tax_amount",
        labelKey: "office.col_vat",
        accessor: (inv) => inv.tax_amount,
        currency: (inv) => inv.currency,
      }),
      createMoneyColumn<InvoiceRow>({
        id: "payable_amount",
        labelKey: "office.col_payable",
        accessor: (inv) => inv.payable_amount,
        currency: (inv) => inv.currency,
        className: "font-medium",
      }),
      createColumn<InvoiceRow>({
        id: "status",
        labelKey: "office.col_status",
        accessorKey: "status",
        cell: ({ row }) => <InvoiceStateBadge status={row.original.status} />,
      }),
    ],
    [engagement, invoiceHref, locale, period, t],
  );
}

/**
 * Geç gelen alış faturası (#640): KDV'si alındığı ayın listesinde "Eylül
 * 2026'dan gelen", belge ayında "Kasım 2026'ya alındı" notu.
 */
function VatMovedNote({
  invoice,
  period,
  text,
}: {
  invoice: InvoiceRow;
  period: string | null;
  text: (key: string, month: string) => string;
}) {
  const moved = invoice.vat_deduction_period;
  if (!moved) return null;
  const docMonth = invoice.issue_date.slice(0, 7);
  const label =
    period === moved
      ? text("office.invoices.vat_moved_from", docMonth)
      : text("office.invoices.vat_moved_to", moved);
  return (
    <span
      className="bg-surface-3 text-text-2 rounded-md px-1.5 py-0.5 text-xs whitespace-nowrap"
      data-testid="vat-moved-note"
    >
      {label}
    </span>
  );
}

function InvoicesInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  // Mükellef detayının Faturalar sekmesinde mükellef sabit, seçici yok (#468).
  const scope = useEngagementScope();
  const { period, focus } = useUrlOfficePeriod();
  // İlk açılış boş ekranla açılmasın (#780): adreste mükellef yokken
  // seçili dönemde faturası olan ilk mükellef.
  const search = useSearchParams();
  const all = useEngagementOptions(slug);
  const preferred = useFirstEngagementWithInvoices(
    slug,
    all.options,
    period ?? undefined,
    !scope && !search.get(ENGAGEMENT_PARAM) && !all.isLoading,
  );
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug, scope, preferred);
  const engagement = scope ?? selected?.engagement ?? "";

  // Sayfa, sayfa boyutu ve süzgeçler URL'de (#381); çekmecedeki fatura da
  // URL'de (#329): geri tuşu ve paylaşılan bağlantı çalışır.
  const listState = useUrlListState({ initialPageSize: PAGE });
  const direction = (listState.getParam("direction") ?? "all") as Dir;
  const open = listState.getParam("invoice");
  const openInvoice = (uuid: string | null) =>
    listState.setParams({ invoice: uuid }, { resetPage: false });

  const list = useEngagementInvoices(slug, engagement, {
    direction: direction === "all" ? undefined : direction,
    period,
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const baseColumns = useInvoiceColumns(engagement, period ?? null);
  const columns = useMemo<ColumnDef<InvoiceRow, unknown>[]>(
    () => [
      ...baseColumns,
      createActionsColumn<InvoiceRow>({
        labelKey: "office.drawer.preview",
        cell: (inv) => (
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="size-11 md:size-8"
            aria-haspopup="dialog"
            aria-label={t("office.drawer.preview_of", {
              number: inv.invoice_number,
            })}
            onClick={() => openInvoice(inv.uuid)}
          >
            <PanelRightOpen className="size-4" aria-hidden />
          </Button>
        ),
      }),
    ],
    // openInvoice yalnızca listState'e bağlı.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [baseColumns, t, listState.setParams],
  );

  const header = (
    <PageHeader
      title={t("office.invoices_title")}
      description={t("office.invoices_description")}
      icon={<FileText className="size-6" />}
      headingLevel={scope ? 2 : 1}
    />
  );
  if (!scope && (isLoading || preferred.waiting)) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (!scope && (isError || options.length === 0)) {
    return (
      <div className="space-y-4">
        {header}
        {/* Hata ile boşluk ayrı görünür (#710): hata tehlike tonunda. */}
        {isError ? (
          <ErrorState title={t("office.api.load_error")} />
        ) : (
          <EmptyState title={t("office.api.no_engagements")} />
        )}
      </div>
    );
  }

  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  return (
    <div className="space-y-4">
      {header}
      <div className="flex flex-wrap gap-3">
        {scope ? null : (
          <EngagementPicker
            options={options}
            selected={selected}
            onSelect={select}
          />
        )}
        <FilterSelect<Dir>
          label={t("office.col_direction")}
          value={direction}
          onChange={(v) =>
            listState.setParams({ direction: v === "all" ? null : v })
          }
          options={[
            { value: "all", label: t("office.filter_all") },
            { value: "outgoing", label: t("office.direction_sales") },
            { value: "incoming", label: t("office.direction_purchase") },
          ]}
        />
      </div>
      <InvoicePostings slug={slug} engagement={engagement} period={focus} />
      <EntityTable<InvoiceRow>
        columns={columns}
        data={rows}
        getRowId={(inv) => inv.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.invoices_empty")}
        emptyDescription={t("office.invoices_empty_hint")}
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
          persistKey: "office.invoices.table",
        }}
        // Bağlantı ve düğmeler kendi işini yapar; satırın geri kalanı
        // çekmeceyi açar.
        onRowClick={(inv) => openInvoice(inv.uuid)}
        isRowActive={(inv) => inv.uuid === open}
      />
      <InvoiceDrawer
        engagement={engagement}
        invoice={open}
        taxpayerTitle={selected?.title}
        onClose={() => openInvoice(null)}
      />
    </div>
  );
}

export function InvoicesPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <InvoicesInner />
    </Suspense>
  );
}
