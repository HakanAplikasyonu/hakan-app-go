"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
} from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

const PAGE = 20;

/** `/m/{uuid}/invoices`: the company's invoices (issue #39). */
export function TaxpayerInvoicesPage({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const [period, setPeriod] = useState("");
  const [direction, setDirection] = useState("all");
  const [offset, setOffset] = useState(0);
  const filters = {
    period: /^\d{4}-\d{2}$/.test(period) ? period : "",
    direction: direction === "all" ? "" : direction,
    offset,
  };
  const list = useQuery({
    queryKey: taxpayerPanelKeys.invoices(taxpayer, filters),
    queryFn: () =>
      taxpayerPanelService.invoices(taxpayer, { ...filters, limit: PAGE }),
  });
  const data = list.data;

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
              setPeriod(e.target.value);
              setOffset(0);
            }}
            className="w-44"
          />
          <Select
            value={direction}
            onValueChange={(v) => {
              setDirection(String(v));
              setOffset(0);
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
        {list.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : list.isError ? (
          <p className="text-destructive text-sm">{t("taxpayer.load_error")}</p>
        ) : !data || data.items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("taxpayer.invoices_empty")}
          </p>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-muted-foreground text-left">
                  <tr className="border-b">
                    <th className="py-2 pr-3 font-medium">
                      {t("taxpayer.col_date")}
                    </th>
                    <th className="py-2 pr-3 font-medium">
                      {t("taxpayer.col_number")}
                    </th>
                    <th className="py-2 pr-3 font-medium">
                      {t("taxpayer.col_direction")}
                    </th>
                    <th className="py-2 pr-3 font-medium">
                      {t("taxpayer.col_counterparty")}
                    </th>
                    <th className="py-2 pr-3 text-right font-medium">
                      {t("taxpayer.col_vat")}
                    </th>
                    <th className="py-2 text-right font-medium">
                      {t("taxpayer.col_payable")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((inv) => (
                    <tr
                      key={inv.uuid}
                      className="hover:bg-muted/50 border-b last:border-0"
                    >
                      <td className="py-2 pr-3 whitespace-nowrap">
                        {formatDate(inv.issue_date, locale)}
                      </td>
                      <td className="py-2 pr-3">
                        <Link
                          href={routes.taxpayer.invoice(taxpayer, inv.uuid)}
                          className="figure underline-offset-4 hover:underline"
                        >
                          {inv.invoice_number}
                        </Link>
                        {inv.status === "cancelled" ? (
                          <Badge variant="danger" className="ml-2">
                            {t("taxpayer.status_cancelled")}
                          </Badge>
                        ) : null}
                      </td>
                      <td className="py-2 pr-3">
                        <Badge
                          variant={
                            inv.direction === "outgoing"
                              ? "secondary"
                              : "outline"
                          }
                        >
                          {t(
                            inv.direction === "outgoing"
                              ? "taxpayer.direction_outgoing"
                              : "taxpayer.direction_incoming",
                          )}
                        </Badge>
                      </td>
                      <td className="py-2 pr-3">
                        <div>{inv.counterparty_name || "—"}</div>
                        {inv.counterparty_tax_id ? (
                          <div className="figure text-muted-foreground text-xs">
                            {inv.counterparty_tax_id}
                          </div>
                        ) : null}
                      </td>
                      <td className="figure py-2 pr-3 text-right">
                        {formatMoney(inv.tax_amount, inv.currency, locale)}
                      </td>
                      <td className="figure py-2 text-right">
                        {formatMoney(inv.payable_amount, inv.currency, locale)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="text-muted-foreground mt-4 flex items-center justify-between text-sm">
              <span>{t("taxpayer.total_count", { count: data.total })}</span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - PAGE))}
                >
                  {t("taxpayer.prev")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + PAGE >= data.total}
                  onClick={() => setOffset(offset + PAGE)}
                >
                  {t("taxpayer.next")}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
