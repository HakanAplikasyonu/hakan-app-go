"use client";

import { Lock, Search, Waypoints } from "lucide-react";
import { useDeferredValue, useMemo, useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import {
  cellAt,
  cellStatuses,
  defaultFilter,
  filterRows,
  sortByProgress,
  statusCounts,
  statusOf,
  type CellStatus,
  type MatrixFilter,
} from "../lib/command";
import type { PeriodMatrixCell } from "../services/periods.service";
import { Figure, FilterSelect, Panel } from "./office-ui";

const PERIOD_COUNT = 6;

const statusClass: Record<CellStatus, string> = {
  collected: "bg-status-review/20 text-foreground",
  checked: "bg-sky-500/15 text-sky-800 dark:text-sky-200",
  approved: "bg-status-approved text-status-approved-foreground",
};

export function monthLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "short",
    year: "2-digit",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}

/** One period of one taxpayer: status, lock and open blockers / requests. */
export function CellBadge({ cell }: { cell: PeriodMatrixCell }) {
  const { t } = useLocale();
  const status = statusOf(cell);
  const parts = [t(`office.command.status_${status}`)];
  if (cell.open_blockers)
    parts.push(t("office.command.tip_blockers", { count: cell.open_blockers }));
  if (cell.open_change_requests)
    parts.push(
      t("office.command.tip_change_requests", {
        count: cell.open_change_requests,
      }),
    );
  return (
    <div
      className={cn(
        "relative flex h-8 min-w-24 items-center justify-center gap-1 rounded-md px-2 text-xs font-medium whitespace-nowrap",
        statusClass[status],
      )}
      title={parts.join(" · ")}
    >
      {cell.locked ? <Lock className="size-3" aria-hidden /> : null}
      {t(`office.command.status_${status}`)}
      {cell.open_blockers || cell.open_change_requests ? (
        <span
          className="bg-status-blocking ring-card absolute -top-1 -right-1 size-2.5 rounded-full ring-2"
          aria-label={parts.slice(1).join(", ")}
        />
      ) : null}
    </div>
  );
}

/**
 * Komuta merkezi (#34): mükellef × dönem durumu, dönem yönetiminin (#32)
 * matrisinden. Satırlar yetkiye göre gelir: tüm mükellefleri görme izni
 * olmayan kullanıcı yalnızca kendisine atananları görür.
 */
export function CommandCenterPage() {
  const { t, locale } = useLocale();
  const matrix = usePeriodMatrix(PERIOD_COUNT);
  const periods = useMemo(() => matrix.data?.periods ?? [], [matrix.data]);
  const [picked, setPicked] = useState<string | null>(null);
  // Varsayılan odak beyan dönemi: bir önceki ay.
  const focus = picked ?? periods[periods.length - 2] ?? periods[0] ?? "";
  const [filter, setFilter] = useState<MatrixFilter>(defaultFilter);
  const query = useDeferredValue(filter.query);
  const set = <K extends keyof MatrixFilter>(k: K, v: MatrixFilter[K]) =>
    setFilter((f) => ({ ...f, [k]: v }));

  const rows = useMemo(() => matrix.data?.items ?? [], [matrix.data]);
  const counts = useMemo(() => statusCounts(rows, focus), [rows, focus]);
  const visible = useMemo(
    () => sortByProgress(filterRows(rows, { ...filter, query }, focus), focus),
    [rows, filter, query, focus],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.command.title")}
        description={t("office.command.description")}
        icon={<Waypoints className="size-6" />}
      />

      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label={t("office.command.period")}
          value={focus}
          onChange={setPicked}
          options={periods.map((p) => ({
            value: p,
            label: monthLabel(p, locale),
          }))}
        />
        <FilterSelect<"all" | CellStatus>
          label={t("office.command.status")}
          value={filter.status}
          onChange={(v) => set("status", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...cellStatuses.map((s) => ({
              value: s,
              label: t(`office.command.status_${s}`),
            })),
          ]}
        />
        <label className="flex items-center gap-2 text-xs">
          <input
            type="checkbox"
            checked={filter.blockersOnly}
            onChange={(e) => set("blockersOnly", e.target.checked)}
          />
          {t("office.command.blockers_only")}
        </label>
        <label className="bg-background flex h-8 items-center gap-2 rounded-md border px-2 text-sm">
          <Search className="text-muted-foreground size-3.5" aria-hidden />
          <input
            className="w-44 bg-transparent outline-none"
            placeholder={t("office.command.search")}
            value={filter.query}
            onChange={(e) => set("query", e.target.value)}
          />
        </label>
      </div>

      <div className="grid grid-cols-3 gap-2">
        {cellStatuses.map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => set("status", filter.status === s ? "all" : s)}
            className={cn(
              "bg-card flex flex-col items-start gap-1 rounded-xl border p-3 text-left transition-colors",
              filter.status === s
                ? "border-foreground/40"
                : "hover:border-foreground/20",
            )}
            aria-pressed={filter.status === s}
          >
            <span className="flex items-center gap-1.5 text-xs">
              <span
                className={cn("size-2.5 rounded-sm", statusClass[s])}
                aria-hidden
              />
              <span className="text-muted-foreground">
                {t(`office.command.status_${s}`)}
              </span>
            </span>
            <Figure className="text-xl font-semibold">{counts[s]}</Figure>
          </button>
        ))}
      </div>

      <Panel
        title={t("office.command.matrix_title", {
          shown: visible.length,
          total: rows.length,
        })}
        description={t("office.command.matrix_description")}
      >
        {matrix.isLoading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : matrix.isError ? (
          <p className="text-destructive p-4 text-sm">
            {t("office.load_failed")}
          </p>
        ) : (
          <div className="max-h-[70vh] overflow-auto">
            <table className="w-full border-separate border-spacing-0 text-sm">
              <thead className="bg-card sticky top-0 z-10">
                <tr>
                  <th className="bg-card sticky left-0 z-20 border-b px-4 py-2 text-left text-xs font-medium">
                    {t("office.col_taxpayer")}
                  </th>
                  {periods.map((p) => (
                    <th
                      key={p}
                      className="border-b px-1 py-2 text-xs font-medium"
                    >
                      <button
                        type="button"
                        onClick={() => setPicked(p)}
                        className={cn(
                          "w-full rounded px-2 py-0.5",
                          p === focus
                            ? "bg-foreground text-background"
                            : "text-muted-foreground hover:text-foreground",
                        )}
                      >
                        {monthLabel(p, locale)}
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {visible.map((row) => (
                  <tr key={row.engagement_uuid}>
                    <td className="bg-card sticky left-0 z-[1] max-w-72 border-b px-4 py-1.5">
                      <div className="truncate font-medium">
                        {row.taxpayer_title}
                      </div>
                      <Figure className="text-muted-foreground text-xs">
                        {row.taxpayer_tax_id}
                      </Figure>
                    </td>
                    {periods.map((p) => {
                      const cell = cellAt(row, p);
                      return (
                        <td
                          key={p}
                          className={cn(
                            "border-b px-1 py-1.5",
                            p === focus && "bg-muted/30",
                          )}
                        >
                          {cell ? <CellBadge cell={cell} /> : null}
                        </td>
                      );
                    })}
                  </tr>
                ))}
                {visible.length === 0 ? (
                  <tr>
                    <td
                      colSpan={periods.length + 1}
                      className="text-muted-foreground px-4 py-8 text-center"
                    >
                      {t("office.command.no_rows")}
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.command.workflow_note_api")}
      </p>
    </div>
  );
}
