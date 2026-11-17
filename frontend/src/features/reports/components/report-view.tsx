"use client";

import { Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  LabelList,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from "recharts";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  type ChartConfig,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import type {
  ReportCategory,
  TaxpayerReport,
} from "@/features/reports/services/reports.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { compact, money, num, percent } from "../lib/format";

/** Kategori grafiğinde gösterilen en çok kalem; kalanı "Diğer"e katlanır. */
const MAX_CATEGORIES = 6;

type Translate = ReturnType<typeof useLocale>["t"];

type TooltipRow = {
  dataKey?: string | number;
  name?: string | number;
  value?: number | string;
  color?: string;
  payload?: Record<string, unknown>;
};

/** Tutar ipucu: seri rengi kimliği taşır, metin token renginde. */
function MoneyTooltip({
  active,
  payload,
  label,
  config,
  locale,
}: {
  active?: boolean;
  payload?: TooltipRow[];
  label?: ReactNode;
  config: ChartConfig;
  locale: string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="border-line bg-surface grid min-w-40 gap-1.5 rounded-md border px-2.5 py-1.5 text-xs shadow-md">
      {label ? (
        <div className="text-foreground font-medium">{label}</div>
      ) : null}
      {payload.map((row) => {
        const key = String(row.dataKey ?? row.name ?? "");
        return (
          <div key={key} className="flex items-center gap-2">
            <span
              aria-hidden
              className="size-2.5 shrink-0 rounded-[2px]"
              style={{ background: row.color }}
            />
            <span className="text-text-2">{config[key]?.label ?? key}</span>
            <span className="text-foreground ml-auto font-mono tabular-nums">
              {money(String(row.value ?? 0), locale)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function ChartCard({
  title,
  description,
  children,
  empty,
  emptyText,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  empty?: boolean;
  emptyText: string;
}) {
  return (
    <Card className="min-w-0">
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
        {description ? (
          <p className="text-text-2 text-xs">{description}</p>
        ) : null}
      </CardHeader>
      <CardContent>
        {empty ? (
          <div className="text-text-2 flex h-56 items-center justify-center text-sm">
            {emptyText}
          </div>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  );
}

function Kpi({
  label,
  value,
  change,
  hint,
}: {
  label: string;
  value: string;
  change?: string | null;
  hint?: string;
}) {
  return (
    <Card className="min-w-0">
      <CardContent className="space-y-1 p-4">
        <div className="text-text-2 text-xs font-medium">{label}</div>
        <div className="font-display truncate text-xl font-bold tabular-nums">
          {value}
        </div>
        <div className="text-text-2 min-h-4 text-xs tabular-nums">
          {change ?? hint ?? ""}
        </div>
      </CardContent>
    </Card>
  );
}

function foldCategories(
  items: ReportCategory[],
  t: Translate,
): { label: string; amount: number; share: string }[] {
  const rows = items.map((c) => ({
    label: c.label,
    amount: num(c.amount),
    share: c.share,
  }));
  if (rows.length <= MAX_CATEGORIES) return rows;
  const head = rows.slice(0, MAX_CATEGORIES - 1);
  const rest = rows.slice(MAX_CATEGORIES - 1);
  return [
    ...head,
    {
      label: t("reports.category_other"),
      amount: rest.reduce((s, r) => s + r.amount, 0),
      share: rest.reduce((s, r) => s + num(r.share), 0).toFixed(1),
    },
  ];
}

/**
 * Mükellef raporunun gövdesi (#386): dört özet kutusu, dörtlü grafik
 * (gelir-gider trendi, aylık ciro, gider kategorileri, KDV), AI yorumu,
 * notlar ve tablo görünümü. Ofis ve mükellef paneli aynı bileşeni kullanır.
 */
export function ReportView({
  report,
  showDraftCommentary = false,
}: {
  report: TaxpayerReport;
  showDraftCommentary?: boolean;
}) {
  const { t, locale } = useLocale();
  const tot = report.totals;
  const months = report.months.map((m) => ({
    label: m.label,
    revenue: num(m.revenue),
    expenses: num(m.expenses),
    net: num(m.net),
    vat_calculated: num(m.vat_calculated),
    vat_deductible: num(m.vat_deductible),
  }));
  const hasTrend = months.some((m) => m.revenue || m.expenses);
  const hasVat = months.some((m) => m.vat_calculated || m.vat_deductible);
  const categories = foldCategories(report.categories, t);
  const previousLabel =
    report.kind === "year"
      ? t("reports.vs_previous_year")
      : t("reports.vs_previous_month");
  const change = (v: string | null | undefined) => {
    const p = percent(v, locale);
    return p ? `${p} ${previousLabel}` : null;
  };
  const vatPayable = num(tot.vat_payable);

  const trendConfig = {
    revenue: {
      label: t("reports.series_revenue"),
      color: "var(--viz-revenue)",
    },
    expenses: {
      label: t("reports.series_expenses"),
      color: "var(--viz-expense)",
    },
    net: { label: t("reports.series_net"), color: "var(--viz-net)" },
  } satisfies ChartConfig;
  const revenueConfig = {
    revenue: {
      label: t("reports.series_revenue"),
      color: "var(--viz-revenue)",
    },
  } satisfies ChartConfig;
  const categoryConfig = {
    amount: { label: t("reports.series_amount"), color: "var(--viz-expense)" },
  } satisfies ChartConfig;
  const vatConfig = {
    vat_calculated: {
      label: t("reports.series_vat_calculated"),
      color: "var(--viz-revenue)",
    },
    vat_deductible: {
      label: t("reports.series_vat_deductible"),
      color: "var(--viz-expense)",
    },
  } satisfies ChartConfig;
  const yTick = (v: number) => compact(v, locale);
  const commentary =
    report.commentary &&
    (showDraftCommentary || report.commentary.status === "approved")
      ? report.commentary
      : null;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Kpi
          label={t("reports.kpi_revenue")}
          value={money(tot.revenue, locale)}
          change={change(report.change.revenue)}
        />
        <Kpi
          label={t("reports.kpi_expenses")}
          value={money(tot.expenses, locale)}
          change={change(report.change.expenses)}
        />
        <Kpi
          label={t("reports.kpi_net")}
          value={money(tot.net, locale)}
          change={change(report.change.net)}
        />
        <Kpi
          label={
            vatPayable < 0
              ? t("reports.kpi_vat_carry")
              : t("reports.kpi_vat_payable")
          }
          value={money(String(Math.abs(vatPayable)), locale)}
          hint={t("reports.kpi_vat_hint")}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <ChartCard
          title={t("reports.chart_trend")}
          description={t("reports.chart_trend_hint")}
          empty={!hasTrend}
          emptyText={t("reports.empty_chart")}
        >
          <ChartContainer
            config={trendConfig}
            className="aspect-auto h-64 w-full"
          >
            <LineChart data={months} margin={{ left: 4, right: 12, top: 8 }}>
              <CartesianGrid vertical={false} strokeOpacity={0.5} />
              <XAxis
                dataKey="label"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={12}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={52}
                tickFormatter={yTick}
              />
              <ChartTooltip
                content={<MoneyTooltip config={trendConfig} locale={locale} />}
              />
              <ChartLegend itemSorter={null} content={<ChartLegendContent />} />
              {(["revenue", "expenses", "net"] as const).map((key) => (
                <Line
                  key={key}
                  dataKey={key}
                  type="monotone"
                  stroke={`var(--color-${key})`}
                  strokeWidth={2}
                  strokeDasharray={key === "net" ? "5 4" : undefined}
                  dot={false}
                  activeDot={{ r: 4 }}
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ChartContainer>
        </ChartCard>

        <ChartCard
          title={t("reports.chart_revenue")}
          description={t("reports.chart_revenue_hint")}
          empty={!months.some((m) => m.revenue)}
          emptyText={t("reports.empty_chart")}
        >
          <ChartContainer
            config={revenueConfig}
            className="aspect-auto h-64 w-full"
          >
            <BarChart data={months} margin={{ left: 4, right: 12, top: 8 }}>
              <CartesianGrid vertical={false} strokeOpacity={0.5} />
              <XAxis
                dataKey="label"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={12}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={52}
                tickFormatter={yTick}
              />
              <ChartTooltip
                cursor={{ fillOpacity: 0.4 }}
                content={
                  <MoneyTooltip config={revenueConfig} locale={locale} />
                }
              />
              <Bar
                dataKey="revenue"
                fill="var(--color-revenue)"
                radius={[4, 4, 0, 0]}
                maxBarSize={28}
                isAnimationActive={false}
              />
            </BarChart>
          </ChartContainer>
        </ChartCard>

        <ChartCard
          title={t("reports.chart_categories")}
          description={t("reports.chart_categories_hint", {
            period: report.label,
          })}
          empty={categories.length === 0}
          emptyText={t("reports.empty_categories")}
        >
          <ChartContainer
            config={categoryConfig}
            className="aspect-auto h-64 w-full"
          >
            <BarChart
              data={categories}
              layout="vertical"
              margin={{ left: 4, right: 48, top: 4 }}
            >
              <XAxis type="number" hide />
              <YAxis
                type="category"
                dataKey="label"
                tickLine={false}
                axisLine={false}
                width={132}
              />
              <ChartTooltip
                cursor={{ fillOpacity: 0.4 }}
                content={
                  <MoneyTooltip config={categoryConfig} locale={locale} />
                }
              />
              <Bar
                dataKey="amount"
                fill="var(--color-amount)"
                radius={[0, 4, 4, 0]}
                maxBarSize={22}
                isAnimationActive={false}
              >
                <LabelList
                  dataKey="share"
                  position="right"
                  className="fill-text-2"
                  fontSize={11}
                  formatter={(v: unknown) =>
                    percent(String(v), locale)?.replace("+", "") ?? ""
                  }
                />
              </Bar>
            </BarChart>
          </ChartContainer>
        </ChartCard>

        <ChartCard
          title={t("reports.chart_vat")}
          description={t("reports.chart_vat_hint")}
          empty={!hasVat}
          emptyText={t("reports.empty_chart")}
        >
          <ChartContainer
            config={vatConfig}
            className="aspect-auto h-64 w-full"
          >
            <BarChart
              data={months}
              margin={{ left: 4, right: 12, top: 8 }}
              barGap={2}
            >
              <CartesianGrid vertical={false} strokeOpacity={0.5} />
              <XAxis
                dataKey="label"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={12}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={52}
                tickFormatter={yTick}
              />
              <ChartTooltip
                cursor={{ fillOpacity: 0.4 }}
                content={<MoneyTooltip config={vatConfig} locale={locale} />}
              />
              <ChartLegend itemSorter={null} content={<ChartLegendContent />} />
              <Bar
                dataKey="vat_calculated"
                fill="var(--color-vat_calculated)"
                radius={[4, 4, 0, 0]}
                maxBarSize={16}
                isAnimationActive={false}
              />
              <Bar
                dataKey="vat_deductible"
                fill="var(--color-vat_deductible)"
                radius={[4, 4, 0, 0]}
                maxBarSize={16}
                isAnimationActive={false}
              />
            </BarChart>
          </ChartContainer>
        </ChartCard>
      </div>

      {commentary ? (
        <Card>
          <CardHeader className="flex flex-row items-center gap-2 pb-2">
            <Sparkles aria-hidden className="text-brand-text size-4" />
            <CardTitle className="text-base">
              {t("reports.commentary_title")}
            </CardTitle>
            {commentary.status === "draft" ? (
              <Badge variant="warning">{t("reports.commentary_draft")}</Badge>
            ) : null}
          </CardHeader>
          <CardContent className="space-y-2 text-sm leading-relaxed">
            {commentary.body
              .split(/\n{2,}/)
              .filter((p) => p.trim())
              .map((p, i) => (
                <p key={i}>{p.trim()}</p>
              ))}
          </CardContent>
        </Card>
      ) : null}

      <details className="border-line rounded-lg border">
        <summary className="text-text-2 hover:text-foreground cursor-pointer px-4 py-3 text-sm font-medium">
          {t("reports.table_toggle")}
        </summary>
        <div className="overflow-x-auto px-4 pb-4">
          <table className="w-full text-sm">
            <caption className="sr-only">{t("reports.table_caption")}</caption>
            <thead>
              <tr className="text-text-2 border-line border-b text-left text-xs">
                <th scope="col" className="py-2 pr-3 font-medium">
                  {t("reports.col_month")}
                </th>
                {(
                  [
                    "series_revenue",
                    "series_expenses",
                    "series_net",
                    "series_vat_calculated",
                    "series_vat_deductible",
                  ] as const
                ).map((k) => (
                  <th
                    key={k}
                    scope="col"
                    className="py-2 pl-3 text-right font-medium"
                  >
                    {t(`reports.${k}`)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {report.months.map((m) => (
                <tr
                  key={m.period}
                  className="border-line border-b last:border-0"
                >
                  <th scope="row" className="py-1.5 pr-3 text-left font-normal">
                    {m.label}
                  </th>
                  {[
                    m.revenue,
                    m.expenses,
                    m.net,
                    m.vat_calculated,
                    m.vat_deductible,
                  ].map((v, i) => (
                    <td
                      key={i}
                      className={cn(
                        "py-1.5 pl-3 text-right font-mono tabular-nums",
                        num(v) < 0 && "text-danger",
                      )}
                    >
                      {money(v, locale)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>

      <ul className="text-text-2 list-disc space-y-0.5 pl-5 text-xs">
        {report.notes.map((n) => (
          <li key={n}>{n}</li>
        ))}
      </ul>
    </div>
  );
}

/** Rapor yüklenirken: özet kutuları ve dört grafik iskeleti. */
export function ReportSkeleton() {
  return (
    <div className="space-y-4" aria-busy>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-24 rounded-xl" />
        ))}
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-80 rounded-xl" />
        ))}
      </div>
    </div>
  );
}
