"use client";

import { quantity } from "@/lib/utils/format";
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
        <CardContent>
          <DataTable<LineRow>
            columns={lineColumns}
            data={inv.items}
            getRowId={(it) => String(it.line_no)}
            emptyTitle={t("taxpayer.lines_title")}
            emptyDescription=""
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
              emptyTitle={t("taxpayer.vat_summary_title")}
              emptyDescription=""
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
