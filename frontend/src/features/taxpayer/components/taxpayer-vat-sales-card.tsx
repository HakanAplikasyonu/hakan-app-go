"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";

import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from "@/components/ui/chart";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  monthLabel,
  thisMonth,
} from "@/features/reports/components/report-range-picker";
import { compact, num } from "@/features/reports/lib/format";
import {
  isVatPanelHidden,
  taxpayerVatSalesKeys,
  taxpayerVatSalesService,
  type TaxpayerVatSales,
} from "@/features/taxpayer/services/vat-sales.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

const MONTHS_BACK = 12;

function shift(period: string, months: number) {
  const [y, m] = period.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1 + months, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

function shortMonth(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { month: "short", timeZone: "UTC" },
  );
}

/**
 * Mükellef ana sayfasında anlık KDV durumu ve satış raporu (#780). Ofis
 * ayardan kapattıysa ya da mükellefin aktif ofisi yoksa hiç görünmez.
 */
export function TaxpayerVatSalesCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const current = thisMonth();
  const [period, setPeriod] = useState(current);
  const query = useQuery({
    queryKey: taxpayerVatSalesKeys.get(taxpayer, period),
    queryFn: () => taxpayerVatSalesService.get(taxpayer, period),
    retry: (count, error) => !isVatPanelHidden(error) && count < 2,
  });
  if (query.isError && isVatPanelHidden(query.error)) return null;

  const months = Array.from({ length: MONTHS_BACK }, (_, i) =>
    shift(current, -i),
  );
  const isCurrent = period === current;

  return (
    <section
      aria-labelledby="tp-vat-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
    >
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h2 id="tp-vat-title" className="font-display text-lg font-bold">
            {isCurrent
              ? t("taxpayer.vat_title")
              : t("taxpayer.vat_title_period", {
                  period: monthLabel(period, locale),
                })}
          </h2>
          <p className="text-text-2 text-sm">{t("taxpayer.vat_hint")}</p>
        </div>
        <Select value={period} onValueChange={setPeriod}>
          <SelectTrigger
            aria-label={t("taxpayer.vat_period_label")}
            className="min-h-11 w-48 md:min-h-9"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {months.map((m) => (
              <SelectItem key={m} value={m}>
                {monthLabel(m, locale)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {query.isLoading ? (
        <p className="text-text-3 text-sm">{t("common.loading")}</p>
      ) : query.isError || !query.data ? (
        <p className="text-danger text-sm">{t("taxpayer.vat_error")}</p>
      ) : (
        <VatSalesBody data={query.data} taxpayer={taxpayer} />
      )}
    </section>
  );
}

function VatSalesBody({
  data,
  taxpayer,
}: {
  data: TaxpayerVatSales;
  taxpayer: string;
}) {
  const { t, locale } = useLocale();
  const tot = data.totals;
  const cal = data.calendar;
  const payable = num(tot.payable) > 0;
  const money = (v: string) => formatMoney(v, "TRY", locale);
  let daysText: string;
  if (cal.days_left > 0)
    daysText = t("taxpayer.vat_days_left", { days: String(cal.days_left) });
  else if (cal.days_left === 0) daysText = t("taxpayer.vat_due_today");
  else daysText = t("taxpayer.vat_overdue");

  const trend = data.sales.trend.map((m) => ({
    label: shortMonth(m.period, locale),
    base: num(m.base),
  }));
  const chartConfig = {
    base: {
      label: t("taxpayer.sales_trend_label"),
      color: "var(--viz-revenue)",
    },
  } satisfies ChartConfig;

  return (
    <div className="space-y-6">
      <div>
        <dl className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Figure label={t("taxpayer.vat_output")} value={money(tot.output_vat)} />
          <Figure label={t("taxpayer.vat_input")} value={money(tot.input_vat)} />
          <Figure
            label={t("taxpayer.vat_carried_in")}
            value={money(tot.carried_in)}
          />
          <Figure
            label={
              payable ? t("taxpayer.vat_payable") : t("taxpayer.vat_carry_out")
            }
            value={money(payable ? tot.payable : tot.carry_out)}
            strong
          />
        </dl>
        <p className="text-text-2 mt-3 text-sm">
          {t("taxpayer.vat_due", { date: formatDate(cal.due_date, locale) })}
          {" · "}
          <span
            className={cn(
              cal.days_left <= 5 ? "text-danger font-semibold" : undefined,
            )}
          >
            {daysText}
          </span>
        </p>
      </div>

      <div aria-labelledby="tp-sales-title" role="region">
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-3">
          <div className="space-y-1">
            <h3 id="tp-sales-title" className="font-display text-base font-bold">
              {t("taxpayer.sales_title")}
            </h3>
            <p className="text-text-2 text-sm">{t("taxpayer.sales_hint")}</p>
          </div>
          <Link
            href={routes.taxpayer.invoices(taxpayer)}
            className="text-brand-text inline-flex min-h-6 items-center text-sm font-medium hover:underline max-md:min-h-11"
          >
            {t("taxpayer.sales_invoices_link")}
          </Link>
        </div>
        <div className="grid gap-6 lg:grid-cols-2">
          <div className="space-y-4">
            <div>
              <div className="text-text-2 text-sm">
                {t("taxpayer.sales_total")}
              </div>
              <div className="font-display text-2xl font-bold tabular-nums">
                {money(data.sales.base)}
              </div>
              <div className="text-text-3 text-xs">
                {t("taxpayer.sales_invoices", {
                  count: String(data.sales.invoices),
                })}
                {" · "}
                {t("taxpayer.sales_vat", { amount: money(data.sales.vat) })}
              </div>
            </div>
            <div>
              <h4 className="mb-2 text-sm font-semibold">
                {t("taxpayer.sales_top_buyers")}
              </h4>
              {data.sales.top_buyers.length === 0 ? (
                <p className="text-text-3 text-sm">
                  {t("taxpayer.sales_top_empty")}
                </p>
              ) : (
                <ol className="divide-line divide-y">
                  {data.sales.top_buyers.map((b) => (
                    <li
                      key={`${b.tax_id}:${b.name}`}
                      className="flex items-center gap-3 py-2 first:pt-0 last:pb-0"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-medium">
                          {b.name || b.tax_id || "—"}
                        </div>
                        <div className="text-text-3 text-xs">
                          {t("taxpayer.sales_invoices", {
                            count: String(b.invoices),
                          })}
                        </div>
                      </div>
                      <span className="text-sm font-semibold tabular-nums">
                        {money(b.base)}
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
          <div>
            <h4 className="mb-2 text-sm font-semibold">
              {t("taxpayer.sales_trend")}
            </h4>
            <ChartContainer
              config={chartConfig}
              className="aspect-auto h-56 w-full"
            >
              <BarChart data={trend} margin={{ left: 4, right: 12, top: 8 }}>
                <CartesianGrid vertical={false} strokeOpacity={0.5} />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tickMargin={8}
                />
                <YAxis
                  tickLine={false}
                  axisLine={false}
                  width={52}
                  tickFormatter={(v: number) => compact(v, locale)}
                />
                <ChartTooltip
                  cursor={{ fillOpacity: 0.4 }}
                  content={<SalesTooltip locale={locale} />}
                />
                <Bar
                  dataKey="base"
                  fill="var(--color-base)"
                  radius={[4, 4, 0, 0]}
                  maxBarSize={28}
                  isAnimationActive={false}
                />
              </BarChart>
            </ChartContainer>
          </div>
        </div>
      </div>
    </div>
  );
}

function Figure({
  label,
  value,
  strong,
}: {
  label: string;
  value: string;
  strong?: boolean;
}) {
  return (
    <div
      className={cn(
        "border-line rounded-lg border p-3",
        strong ? "bg-brand-tint" : "bg-surface-2",
      )}
    >
      <dt className="text-text-2 text-xs">{label}</dt>
      <dd className="mt-1 text-base font-bold tabular-nums">{value}</dd>
    </div>
  );
}

type TooltipRow = { value?: number | string };

function SalesTooltip({
  active,
  payload,
  label,
  locale,
}: {
  active?: boolean;
  payload?: TooltipRow[];
  label?: string;
  locale: string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="border-line bg-surface grid min-w-32 gap-1 rounded-md border px-2.5 py-1.5 text-xs shadow-md">
      {label ? <div className="font-medium">{label}</div> : null}
      <div className="font-mono tabular-nums">
        {formatMoney(String(payload[0]?.value ?? 0), "TRY", locale)}
      </div>
    </div>
  );
}
