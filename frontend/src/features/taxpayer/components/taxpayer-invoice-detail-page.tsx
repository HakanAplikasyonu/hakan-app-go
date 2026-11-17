"use client";

import { quantity } from "@/lib/utils/format";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import {
  DataTable,
  createColumn,
  createMoneyColumn,
  type DataTableFeatures,
} from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
  type TaxpayerInvoiceDetail,
} from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

type LineRow = TaxpayerInvoiceDetail["items"][number];
type SummaryRow = TaxpayerInvoiceDetail["tax_summary"][number];

// Gömülü küçük tablolar: araç çubuğu, sayfalama ve seçim yok. Telefonda
// satırlar karta döner (yatay taşma yok, #371).
const EMBEDDED_FEATURES: DataTableFeatures = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  globalFilter: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
  facetedFilters: false,
};

function useLineColumns(currency: string | undefined) {
  const { locale } = useLocale();
  return useMemo<ColumnDef<LineRow, unknown>[]>(
    () => [
      createColumn<LineRow>({
        id: "line_no",
        labelKey: "taxpayer.col_line_no",
        accessorKey: "line_no",
        gridSecondary: true,
      }),
      createColumn<LineRow>({
        id: "name",
        labelKey: "taxpayer.col_item",
        accessorKey: "name",
        gridPrimary: true,
      }),
      createColumn<LineRow>({
        id: "quantity",
        labelKey: "taxpayer.col_quantity",
        accessorFn: (it) => Number(it.quantity),
        meta: { align: "end", cellClassName: "figure" },
        cell: ({ row }) => (
          <>
            {quantity(row.original.quantity, locale === "en" ? "en" : "tr")}{" "}
            {row.original.unit_name || row.original.unit_code}
          </>
        ),
      }),
      createColumn<LineRow>({
        id: "tax_rate",
        labelKey: "taxpayer.col_rate",
        accessorFn: (it) => Number(it.tax_rate),
        meta: { align: "end", cellClassName: "figure" },
        cell: ({ row }) => <>%{row.original.tax_rate}</>,
      }),
      createMoneyColumn<LineRow>({
        id: "tax_exclusive_amount",
        labelKey: "taxpayer.amount_base",
        accessor: (it) => it.tax_exclusive_amount,
        currency,
      }),
      createMoneyColumn<LineRow>({
        id: "tax_amount",
        labelKey: "taxpayer.col_vat",
        accessor: (it) => it.tax_amount,
        currency,
      }),
    ],
    [currency, locale],
  );
}

function useSummaryColumns(currency: string | undefined) {
  return useMemo<ColumnDef<SummaryRow, unknown>[]>(
    () => [
      createColumn<SummaryRow>({
        id: "tax_rate",
        labelKey: "taxpayer.col_rate",
        accessorKey: "tax_rate",
        gridPrimary: true,
        meta: { cellClassName: "figure" },
        cell: ({ row }) => <>%{row.original.tax_rate}</>,
      }),
      createMoneyColumn<SummaryRow>({
        id: "base",
        labelKey: "taxpayer.amount_base",
        accessor: (s) => s.base,
        currency,
      }),
      createMoneyColumn<SummaryRow>({
        id: "vat",
        labelKey: "taxpayer.col_vat",
        accessor: (s) => s.vat,
        currency,
      }),
    ],
    [currency],
  );
}

/** `/m/{uuid}/invoices/{invoice}`: lines, KDV summary and sources. */
export function TaxpayerInvoiceDetailPage({
  taxpayer,
  invoice,
}: {
  taxpayer: string;
  invoice: string;
}) {
  const { t, locale } = useLocale();
  const q = useQuery({
    queryKey: taxpayerPanelKeys.invoice(taxpayer, invoice),
    queryFn: () => taxpayerPanelService.invoice(taxpayer, invoice),
    retry: false,
  });
  const lineColumns = useLineColumns(q.data?.currency);
  const summaryColumns = useSummaryColumns(q.data?.currency);
  const back = (
    <Link
      href={routes.taxpayer.invoices(taxpayer)}
      className="text-muted-foreground text-sm hover:underline"
    >
      ← {t("taxpayer.back_to_invoices")}
    </Link>
  );
  if (q.isLoading)
    return (
      <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
    );
  if (q.isError || !q.data) {
    return (
      <div className="space-y-4">
        {back}
        <p className="text-destructive text-sm">
          {t("taxpayer.invoice_not_found")}
        </p>
      </div>
    );
  }
  const inv = q.data;
  const money = (v: string) => formatMoney(v, inv.currency, locale);
  return (
    <div className="space-y-6">
      {back}
      <Card>
        <CardHeader>
          <CardTitle className="figure">{inv.invoice_number}</CardTitle>
          <CardDescription className="flex flex-wrap items-center gap-2">
            <span>{formatDate(inv.issue_date, locale)}</span>
            <Badge
              variant={inv.direction === "outgoing" ? "secondary" : "outline"}
            >
              {t(
                inv.direction === "outgoing"
                  ? "taxpayer.direction_outgoing"
                  : "taxpayer.direction_incoming",
              )}
            </Badge>
            <span>{inv.type}</span>
            {inv.profile ? <span>· {inv.profile}</span> : null}
            {inv.ettn ? (
              <span className="figure text-xs">ETTN {inv.ettn}</span>
            ) : null}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <div>
            <div className="text-muted-foreground text-xs">
              {t("taxpayer.col_counterparty")}
            </div>
            <div>{inv.counterparty_name || "—"}</div>
            {inv.counterparty_tax_id ? (
              <div className="figure text-muted-foreground text-xs">
                {inv.counterparty_tax_id}
              </div>
            ) : null}
          </div>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
            <dt className="text-muted-foreground">
              {t("taxpayer.amount_base")}
            </dt>
            <dd className="figure text-right">
              {money(inv.tax_exclusive_amount)}
            </dd>
            <dt className="text-muted-foreground">{t("taxpayer.col_vat")}</dt>
            <dd className="figure text-right">{money(inv.tax_amount)}</dd>
            <dt className="text-muted-foreground">
              {t("taxpayer.amount_withholding")}
            </dt>
            <dd className="figure text-right">
              {money(inv.withholding_amount)}
            </dd>
            <dt className="font-medium">{t("taxpayer.col_payable")}</dt>
            <dd className="figure text-right font-medium">
              {money(inv.payable_amount)}
            </dd>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("taxpayer.lines_title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <DataTable<LineRow>
            columns={lineColumns}
            data={inv.items}
            getRowId={(it) => String(it.line_no)}
            bordered={false}
            features={EMBEDDED_FEATURES}
          />
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("taxpayer.vat_summary_title")}</CardTitle>
          </CardHeader>
          <CardContent>
            <DataTable<SummaryRow>
              columns={summaryColumns}
              data={inv.tax_summary}
              bordered={false}
              features={{ ...EMBEDDED_FEATURES, sorting: false }}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("taxpayer.sources_title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            {inv.sources.map((s, i) => (
              <div key={i} className="flex justify-between gap-2">
                <span>
                  {s.source_system} · {s.capture_method}
                </span>
                <span className="text-muted-foreground">
                  {formatDate(s.first_seen_at, locale)}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
