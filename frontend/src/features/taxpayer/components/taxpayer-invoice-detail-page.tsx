"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";

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
} from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

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
        <CardContent className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-muted-foreground text-left">
              <tr className="border-b">
                <th className="py-2 pr-3 font-medium">#</th>
                <th className="py-2 pr-3 font-medium">
                  {t("taxpayer.col_item")}
                </th>
                <th className="py-2 pr-3 text-right font-medium">
                  {t("taxpayer.col_quantity")}
                </th>
                <th className="py-2 pr-3 text-right font-medium">
                  {t("taxpayer.col_rate")}
                </th>
                <th className="py-2 pr-3 text-right font-medium">
                  {t("taxpayer.amount_base")}
                </th>
                <th className="py-2 text-right font-medium">
                  {t("taxpayer.col_vat")}
                </th>
              </tr>
            </thead>
            <tbody>
              {inv.items.map((it) => (
                <tr key={it.line_no} className="border-b last:border-0">
                  <td className="py-2 pr-3">{it.line_no}</td>
                  <td className="py-2 pr-3">{it.name}</td>
                  <td className="figure py-2 pr-3 text-right">
                    {it.quantity} {it.unit_code}
                  </td>
                  <td className="figure py-2 pr-3 text-right">
                    %{it.tax_rate}
                  </td>
                  <td className="figure py-2 pr-3 text-right">
                    {money(it.tax_exclusive_amount)}
                  </td>
                  <td className="figure py-2 text-right">
                    {money(it.tax_amount)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("taxpayer.vat_summary_title")}</CardTitle>
          </CardHeader>
          <CardContent>
            <table className="w-full text-sm">
              <tbody>
                {inv.tax_summary.map((s, i) => (
                  <tr key={i} className="border-b last:border-0">
                    <td className="figure py-1.5">%{s.tax_rate}</td>
                    <td className="figure py-1.5 text-right">
                      {money(s.base)}
                    </td>
                    <td className="figure py-1.5 text-right">{money(s.vat)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
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
