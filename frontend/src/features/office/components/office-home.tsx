"use client";

import { ArrowRight, Building2 } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";
import { useMemo } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useOfficeVATForecast,
  useOpenBlockersTotal,
  usePendingSuggestionsTotal,
  usePeriodMatrix,
} from "../hooks/use-office-data";
import { cellAt, sortByProgress, statusOf } from "../lib/command";
import { OfflineAgentsCard } from "./agents-page";
import { CellBadge, monthLabel } from "./command-center";
import { Figure, Money, Panel, TableBox } from "./office-ui";

const LIST_LIMIT = 10;

function Stat({
  label,
  value,
  href,
  tone,
}: {
  label: string;
  value: ReactNode;
  href: string;
  tone?: string;
}) {
  return (
    <Link
      href={href}
      className="bg-card hover:border-foreground/20 group flex flex-col gap-1 rounded-xl border p-4 transition-colors"
    >
      <span className="text-muted-foreground flex items-center justify-between text-xs">
        {label}
        <ArrowRight className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
      </span>
      <Figure className={`text-2xl font-semibold ${tone ?? ""}`}>
        {value}
      </Figure>
    </Link>
  );
}

const dash = <span className="text-muted-foreground">—</span>;

/**
 * Ofis ana ekranı: sayılar ilgili listelerin uçlarından, mükellef × dönem
 * durumu dönem matrisinden (#32), KDV tahmini KDV erken uyarıdan (#33).
 */
export function OfficeHome() {
  const { t, locale } = useLocale();
  const { slug, organization, role } = useTenant();
  // Beyan dönemi: bir önceki ay (beyannamesi bu ay veriliyor).
  const matrix = usePeriodMatrix(2);
  const pending = usePendingSuggestionsTotal();
  const blockers = useOpenBlockersTotal();
  const period = matrix.data?.periods[0];
  const vat = useOfficeVATForecast(period, Boolean(period));

  const rows = useMemo(() => matrix.data?.items ?? [], [matrix.data]);
  const ready = rows.filter((r) => {
    const c = period ? cellAt(r, period) : undefined;
    return c && statusOf(c) !== "collected";
  }).length;
  const titles = useMemo(
    () => new Map(rows.map((r) => [r.engagement_uuid, r.taxpayer_title])),
    [rows],
  );
  const behind = useMemo(
    () => (period ? sortByProgress(rows, period).slice(0, LIST_LIMIT) : []),
    [rows, period],
  );
  const vatTop = useMemo(
    () =>
      [...(vat.data?.items ?? [])]
        .sort((a, b) => Number(b.totals.payable) - Number(a.totals.payable))
        .slice(0, LIST_LIMIT),
    [vat.data],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={organization.name}
        description={t("office.home_description", {
          role: role.name,
          period: period ? monthLabel(period, locale) : "…",
        })}
        icon={<Building2 className="size-6" />}
      />
      <OfflineAgentsCard />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat
          label={t("office.stat_taxpayers")}
          value={matrix.data ? matrix.data.total : dash}
          href={routes.tenant.taxpayers(slug)}
        />
        <Stat
          label={t("office.stat_ready")}
          value={matrix.data ? `${ready} / ${rows.length}` : dash}
          href={routes.tenant.command(slug)}
        />
        <Stat
          label={t("office.stat_review")}
          value={pending.data ?? dash}
          href={routes.tenant.review(slug)}
          tone={pending.data ? "text-amber-700 dark:text-status-review" : ""}
        />
        <Stat
          label={t("office.stat_blockers")}
          value={blockers.data ?? dash}
          href={routes.tenant.warnings(slug)}
          tone={blockers.data ? "text-status-blocking" : ""}
        />
      </div>

      <Panel
        title={t("office.home_behind_title", {
          period: period ? monthLabel(period, locale) : "",
        })}
        description={t("office.home_behind_description")}
        actions={
          <Link
            href={routes.tenant.command(slug)}
            className="text-brand text-xs font-medium hover:underline"
          >
            {t("office.home_open_command")}
          </Link>
        }
      >
        {matrix.isLoading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : matrix.isError ? (
          <p className="text-destructive p-4 text-sm">
            {t("office.load_failed")}
          </p>
        ) : behind.length === 0 ? (
          <p className="text-muted-foreground p-6 text-center text-sm">
            {t("office.taxpayers_empty")}
          </p>
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_taxpayer")}</th>
                <th className="text-right">{t("office.col_blockers")}</th>
                <th className="text-right">
                  {t("office.col_change_requests")}
                </th>
                <th>{t("office.col_readiness")}</th>
              </tr>
            </thead>
            <tbody>
              {behind.map((r) => {
                const c = period ? cellAt(r, period) : undefined;
                return (
                  <tr key={r.engagement_uuid}>
                    <td>
                      <div className="font-medium">{r.taxpayer_title}</div>
                      <Figure className="text-muted-foreground text-xs">
                        {r.taxpayer_tax_id}
                      </Figure>
                    </td>
                    <td className="text-right">
                      <Figure>{c?.open_blockers ?? 0}</Figure>
                    </td>
                    <td className="text-right">
                      <Figure>{c?.open_change_requests ?? 0}</Figure>
                    </td>
                    <td>{c ? <CellBadge cell={c} /> : null}</td>
                  </tr>
                );
              })}
            </tbody>
          </TableBox>
        )}
      </Panel>

      <Panel
        title={t("office.home_vat_title", {
          period: period ? monthLabel(period, locale) : "",
        })}
        description={t("office.home_vat_description")}
      >
        {vat.isLoading || !period ? (
          <div className="p-4">
            <Skeleton className="h-8 w-full" />
          </div>
        ) : vat.isError ? (
          <p className="text-muted-foreground p-4 text-sm">
            {t("office.home_vat_unavailable")}
          </p>
        ) : vatTop.length === 0 ? (
          <p className="text-muted-foreground p-6 text-center text-sm">
            {t("office.home_vat_empty")}
          </p>
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_taxpayer")}</th>
                <th className="text-right">{t("office.col_vat_payable")}</th>
                <th className="text-right">{t("office.col_vat_carry")}</th>
                <th>{t("office.col_vat_due")}</th>
              </tr>
            </thead>
            <tbody>
              {vatTop.map((f) => (
                <tr key={f.engagement_uuid}>
                  <td className="font-medium">
                    {titles.get(f.engagement_uuid) ?? "—"}
                  </td>
                  <td className="text-right">
                    <Money value={f.totals.payable} />
                  </td>
                  <td className="text-right">
                    <Money value={f.totals.carry_out} />
                  </td>
                  <td>
                    {f.calendar.due_date}
                    <span
                      className={
                        f.checks.deadline_near
                          ? "text-status-warning ml-1.5 text-xs"
                          : "text-muted-foreground ml-1.5 text-xs"
                      }
                    >
                      {t("office.days_left", { count: f.calendar.days_left })}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </TableBox>
        )}
      </Panel>
    </div>
  );
}
