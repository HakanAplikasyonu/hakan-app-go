"use client";

import { Search, TriangleAlert, Waypoints } from "lucide-react";
import { useDeferredValue, useMemo, useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { DEMO_PERIOD } from "../data/demo";
import { buildDemoOffice } from "../data/demo-office";
import { useOffice } from "../hooks/use-office";
import {
  buildMatrix,
  cellStatuses,
  defaultFilter,
  factsFromDocuments,
  filterRows,
  periodsEndingAt,
  sortByProgress,
  statusCounts,
  type Cell,
  type CellStatus,
  type MatrixFilter,
  type PeriodWorkflow,
} from "../lib/command";
import type { Severity } from "../types";
import {
  DemoNotice,
  Figure,
  FilterSelect,
  OfficeDataGate,
  Panel,
} from "./office-ui";

const PERIOD_COUNT = 6;

/**
 * #32 (dönem yönetimi) birleşene kadar ofis verisinin iş akışı aşaması
 * yoktur: hücreler en çok "kontrol edildi"ye çıkar. Uç geldiğinde bu boş
 * kaynak onun yanıtıyla değişir.
 */
const NO_WORKFLOW: PeriodWorkflow = {};

const statusClass: Record<CellStatus, string> = {
  empty: "bg-muted/40 text-muted-foreground",
  collected: "bg-status-review/20 text-foreground",
  checked: "bg-sky-500/15 text-sky-800 dark:text-sky-200",
  approved: "bg-status-approved/20 text-foreground",
  exported: "bg-status-approved/45 text-foreground",
  declared: "bg-status-approved text-status-approved-foreground",
};

const levelDot: Record<Severity, string> = {
  info: "bg-sky-500",
  warning: "bg-status-warning",
  blocker: "bg-status-blocking",
};

function monthLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "short",
    year: "2-digit",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}

function StatusCell({ cell }: { cell: Cell }) {
  const { t } = useLocale();
  const parts = [
    t(`office.command.status_${cell.status}`),
    t("office.command.tip_invoices", { count: cell.invoices }),
  ];
  if (cell.pendingSuggestions)
    parts.push(
      t("office.command.tip_pending", { count: cell.pendingSuggestions }),
    );
  if (cell.openBlockers)
    parts.push(t("office.command.tip_blockers", { count: cell.openBlockers }));
  if (cell.openWarnings)
    parts.push(t("office.command.tip_warnings", { count: cell.openWarnings }));
  if (cell.regressed) parts.push(t("office.command.tip_regressed"));
  return (
    <div
      className={cn(
        "relative flex h-8 min-w-24 items-center justify-center rounded-md px-2 text-xs font-medium whitespace-nowrap",
        statusClass[cell.status],
        cell.regressed && "ring-status-blocking ring-2",
      )}
      title={parts.join(" · ")}
    >
      {t(`office.command.status_${cell.status}`)}
      {cell.level !== "none" ? (
        <span
          className={cn(
            "ring-card absolute -top-1 -right-1 size-2.5 rounded-full ring-2",
            levelDot[cell.level],
          )}
          aria-label={t(`office.severity_${cell.level}`)}
        />
      ) : null}
    </div>
  );
}

function CommandInner() {
  const { t, locale } = useLocale();
  const storeTaxpayers = useOffice((s) => s.taxpayers);
  const storeMembers = useOffice((s) => s.members);
  const invoices = useOffice((s) => s.invoices);
  const suggestions = useOffice((s) => s.suggestions);
  const warnings = useOffice((s) => s.warnings);

  const [source, setSource] = useState<"office" | "demo100">("office");
  const periods = useMemo(() => periodsEndingAt(DEMO_PERIOD, PERIOD_COUNT), []);
  const [focus, setFocus] = useState(periods[periods.length - 1]);
  const [filter, setFilter] = useState<MatrixFilter>(defaultFilter);
  const query = useDeferredValue(filter.query);
  const set = <K extends keyof MatrixFilter>(k: K, v: MatrixFilter[K]) =>
    setFilter((f) => ({ ...f, [k]: v }));

  const demo = useMemo(
    () => (source === "demo100" ? buildDemoOffice(DEMO_PERIOD) : null),
    [source],
  );

  const { rows, members } = useMemo(() => {
    const built = demo
      ? buildMatrix({
          taxpayers: demo.taxpayers,
          periods,
          facts: demo.facts,
          workflow: demo.workflow,
        })
      : buildMatrix({
          taxpayers: storeTaxpayers,
          periods,
          facts: factsFromDocuments({ invoices, suggestions, warnings }),
          workflow: NO_WORKFLOW,
        });
    return {
      rows: built,
      members: demo ? demo.members : storeMembers,
    };
  }, [
    demo,
    periods,
    storeTaxpayers,
    storeMembers,
    invoices,
    suggestions,
    warnings,
  ]);

  const counts = useMemo(() => statusCounts(rows, focus), [rows, focus]);
  const visible = useMemo(
    () => sortByProgress(filterRows(rows, { ...filter, query }, focus), focus),
    [rows, filter, query, focus],
  );
  const regressed = useMemo(
    () => rows.filter((r) => r.cells[focus]?.regressed).length,
    [rows, focus],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.command.title")}
        description={t("office.command.description")}
        icon={<Waypoints className="size-6" />}
      />
      <DemoNotice />

      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect<"office" | "demo100">
          label={t("office.command.source")}
          value={source}
          onChange={setSource}
          options={[
            { value: "office", label: t("office.command.source_office") },
            { value: "demo100", label: t("office.command.source_demo100") },
          ]}
        />
        <FilterSelect
          label={t("office.command.period")}
          value={focus}
          onChange={setFocus}
          options={periods.map((p) => ({
            value: p,
            label: monthLabel(p, locale),
          }))}
        />
        <FilterSelect
          label={t("office.command.member")}
          value={filter.member}
          onChange={(v) => set("member", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...members.map((m) => ({ value: m.id, label: m.name })),
            { value: "unassigned", label: t("office.command.unassigned") },
          ]}
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
        <FilterSelect<"all" | Severity>
          label={t("office.command.level")}
          value={filter.level}
          onChange={(v) => set("level", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            { value: "info", label: t("office.command.level_info") },
            { value: "warning", label: t("office.command.level_warning") },
            { value: "blocker", label: t("office.command.level_blocker") },
          ]}
        />
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

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
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

      {regressed ? (
        <p className="text-status-blocking flex items-center gap-2 text-sm">
          <TriangleAlert className="size-4" aria-hidden />
          {t("office.command.regressed", { count: regressed })}
        </p>
      ) : null}

      <Panel
        title={t("office.command.matrix_title", {
          shown: visible.length,
          total: rows.length,
        })}
        description={t("office.command.matrix_description")}
        actions={
          <span
            className="text-muted-foreground text-xs tabular-nums"
            data-testid="command-cell-count"
          >
            {t("office.command.computed", {
              cells: rows.length * periods.length,
            })}
          </span>
        }
      >
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
                      onClick={() => setFocus(p)}
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
              {visible.map(({ taxpayer, cells }) => (
                <tr key={taxpayer.id}>
                  <td className="bg-card sticky left-0 z-[1] max-w-72 border-b px-4 py-1.5">
                    <div className="truncate font-medium">{taxpayer.title}</div>
                    <div className="text-muted-foreground flex gap-2 text-xs">
                      <Figure>{taxpayer.taxId}</Figure>
                      <span className="truncate">
                        {taxpayer.assigneeIds
                          .map((id) => members.find((m) => m.id === id)?.name)
                          .filter(Boolean)
                          .join(", ") || t("office.command.unassigned")}
                      </span>
                    </div>
                  </td>
                  {periods.map((p) => (
                    <td
                      key={p}
                      className={cn(
                        "border-b px-1 py-1.5",
                        p === focus && "bg-muted/30",
                      )}
                    >
                      <StatusCell cell={cells[p]} />
                    </td>
                  ))}
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
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.command.workflow_note")}
      </p>
    </div>
  );
}

export function CommandCenterPage() {
  return (
    <OfficeDataGate>
      <CommandInner />
    </OfficeDataGate>
  );
}
