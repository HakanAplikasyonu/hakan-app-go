"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { Suspense, useMemo, useState } from "react";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createMoneyColumn,
} from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
  type TaxpayerInvoice,
} from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

const PAGE = 20;
const PERIOD_RE = /^\d{4}-\d{2}$/;

type InvoiceRow = TaxpayerInvoice;

function useInvoiceColumns(taxpayer: string) {
  const { t } = useLocale();
  return useMemo<ColumnDef<InvoiceRow, unknown>[]>(
    () => [
      createDateColumn<InvoiceRow>({
        id: "issue_date",
        labelKey: "taxpayer.col_date",
        accessor: (inv) => inv.issue_date,
        style: "medium",
        gridSecondary: true,
      }),
      createColumn<InvoiceRow>({
        id: "invoice_number",
        labelKey: "taxpayer.col_number",
        accessorKey: "invoice_number",
        gridPrimary: true,
        cell: ({ row }) => {
          const inv = row.original;
          return (
            <>
              {/* Dokunma hedefi ≥24 px, telefonda 44 px (#371). */}
              <Link
                href={routes.taxpayer.invoice(taxpayer, inv.uuid)}
                className="figure inline-flex min-h-6 items-center underline-offset-4 hover:underline max-md:min-h-11"
              >
                {inv.invoice_number}
              </Link>
              {inv.status === "cancelled" ? (
                <Badge variant="danger" className="ml-2">
                  {t("taxpayer.status_cancelled")}
                </Badge>
              ) : null}
            </>
          );
        },
      }),
      createColumn<InvoiceRow>({
        id: "direction",
        labelKey: "taxpayer.col_direction",
        accessorKey: "direction",
        cell: ({ row }) => (
          <Badge
            variant={
              row.original.direction === "outgoing" ? "secondary" : "outline"
            }
          >
            {t(
              row.original.direction === "outgoing"
                ? "taxpayer.direction_outgoing"
                : "taxpayer.direction_incoming",
            )}
          </Badge>
        ),
      }),
      createColumn<InvoiceRow>({
        id: "counterparty",
        labelKey: "taxpayer.col_counterparty",
        accessorFn: (inv) => inv.counterparty_name ?? "",
        cell: ({ row }) => (
          <>
            <div>{row.original.counterparty_name || "—"}</div>
            {row.original.counterparty_tax_id ? (
              <div className="figure text-muted-foreground text-xs">
                {row.original.counterparty_tax_id}
              </div>
            ) : null}
          </>
        ),
      }),
      createMoneyColumn<InvoiceRow>({
        id: "tax_amount",
        labelKey: "taxpayer.col_vat",
        accessor: (inv) => inv.tax_amount,
        currency: (inv) => inv.currency,
      }),
      createMoneyColumn<InvoiceRow>({
        id: "payable_amount",
        labelKey: "taxpayer.col_payable",
        accessor: (inv) => inv.payable_amount,
        currency: (inv) => inv.currency,
      }),
    ],
    [taxpayer, t],
  );
}

function TaxpayerInvoicesInner({ taxpayer }: { taxpayer: string }) {
  const { t } = useLocale();
  // Sayfa, dönem ve yön URL'de (#381): geri tuşu ve paylaşılan bağlantı
  // aynı listeyi açar.
  const listState = useUrlListState({ initialPageSize: PAGE });
  const urlPeriod = listState.getParam("period") ?? "";
  const direction = listState.getParam("direction") ?? "all";
  // Ay alanı yazılırken yarım değer taşıyabilir; URL'ye yalnızca geçerli
  // (ya da boş) dönem yazılır.
  const [period, setPeriod] = useState(urlPeriod);

  const filters = {
    period: PERIOD_RE.test(urlPeriod) ? urlPeriod : "",
    direction: direction === "all" ? "" : direction,
    offset: listState.params.offset,
  };
  const limit = listState.params.limit;
  const list = useQuery({
    queryKey: taxpayerPanelKeys.invoices(taxpayer, { ...filters, limit }),
    queryFn: () =>
      taxpayerPanelService.invoices(taxpayer, { ...filters, limit }),
  });
  const columns = useInvoiceColumns(taxpayer);

  return (
    <Card>
      <CardHeader className="gap-4 sm:flex-row sm:items-end sm:justify-between">
        <CardTitle>{t("taxpayer.invoices_title")}</CardTitle>
        <div className="flex flex-wrap gap-2">
          <Input
            type="month"
            aria-label={t("taxpayer.invoices_period")}
            value={period}
            onChange={(e) => {
              const v = e.target.value;
              setPeriod(v);
              if (v === "" || PERIOD_RE.test(v)) {
                listState.setParams({ period: v || null });
              }
            }}
            className="w-44"
          />
          <Select
            value={direction}
            onValueChange={(v) => {
              const next = String(v);
              listState.setParams({ direction: next === "all" ? null : next });
            }}
          >
            <SelectTrigger
              className="w-40"
              aria-label={t("taxpayer.invoices_direction")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("taxpayer.direction_all")}</SelectItem>
              <SelectItem value="outgoing">
                {t("taxpayer.direction_outgoing")}
              </SelectItem>
              <SelectItem value="incoming">
                {t("taxpayer.direction_incoming")}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        <EntityTable<InvoiceRow>
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(inv) => inv.uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          errorTitle={t("taxpayer.load_error")}
          onRetry={() => void list.refetch()}
          emptyTitle={t("taxpayer.invoices_empty")}
          emptyDescription=""
          state={listState.tableState}
          rowCount={list.data?.total ?? 0}
          pageSizeOptions={[20, 50, 100]}
          bordered={false}
          // Sunucu sıralama ve serbest arama sunmuyor; süzgeçler kart
          // başlığında. Telefonda satırlar karta döner (yatay taşma yok).
          features={{
            toolbar: false,
            sorting: false,
            globalFilter: false,
            columnFilters: false,
            columnVisibility: false,
            columnResizing: false,
            columnPinning: false,
            columnOrdering: false,
            rowSelection: false,
            density: false,
            viewMode: false,
            facetedFilters: false,
          }}
        />
      </CardContent>
    </Card>
  );
}

/** `/m/{uuid}/invoices`: the company's invoices (issue #39). */
export function TaxpayerInvoicesPage({ taxpayer }: { taxpayer: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <TaxpayerInvoicesInner taxpayer={taxpayer} />
    </Suspense>
  );
}
