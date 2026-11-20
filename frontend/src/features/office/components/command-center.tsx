"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Lock, Search, Waypoints } from "lucide-react";
import Link from "next/link";
import { useDeferredValue, useId, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { DataTable, createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { StepMarker, type StepState } from "@/components/ui/step-marker";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useOfficePeriod } from "../period";
import {
  cellAt,
  cellStatuses,
  cellHref,
  cellTarget,
  defaultFilter,
  filterRows,
  sortByProgress,
  statusCounts,
  statusOf,
  type CellStatus,
  type MatrixFilter,
} from "../lib/command";
import type {
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";
import { Figure, FilterSelect, Panel } from "./office-ui";

const PERIOD_COUNT = 6;

/** Dönem durumu → akış işareti ve hücre zemini (sade; anlamı metin taşır). */
const statusStep: Record<CellStatus, StepState> = {
  open: "pending",
  collected: "current",
  checked: "done",
  approved: "done",
};
const statusClass: Record<CellStatus, string> = {
  open: "text-text-3",
  collected: "text-foreground",
  checked: "text-foreground",
  approved: "bg-brand-tint/60 text-foreground",
};

export function monthLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "short",
    year: "2-digit",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}

/** One period of one taxpayer: status, lock, open warnings and requests. */
export function CellBadge({
  cell,
  hint,
}: {
  cell: PeriodMatrixCell;
  /** Tıklayınca ne açılacağı (ipucu metninin sonuna eklenir, #567). */
  hint?: string;
}) {
  const { t } = useLocale();
  const status = statusOf(cell);
  const parts = [t(`office.command.status_${status}`)];
  if (cell.open_blockers)
    parts.push(t("office.command.tip_blockers", { count: cell.open_blockers }));
  // Engellemeyen açık uyarılar da iş: sayısı hücrede görünür (#529).
  const others = cell.open_warnings - cell.open_blockers;
  if (others > 0)
    parts.push(t("office.command.tip_warnings", { count: others }));
  if (cell.open_change_requests)
    parts.push(
      t("office.command.tip_change_requests", {
        count: cell.open_change_requests,
      }),
    );
  const blocked = !!(cell.open_blockers || cell.open_change_requests);
  return (
    <div
      className={cn(
        "border-line bg-surface flex h-8 min-w-24 items-center justify-center gap-1.5 rounded-md border px-2 text-xs font-medium whitespace-nowrap",
        statusClass[status],
      )}
      title={[...parts, hint].filter(Boolean).join(" · ")}
    >
      {/* #310: işaret süsleyici; durum ve engel metni sr-only olarak okunur. */}
      <StepMarker state={blocked ? "error" : statusStep[status]} />
      {cell.locked ? <Lock className="text-text-3 size-3" aria-hidden /> : null}
      {t(`office.command.status_${status}`)}
      {cell.open_warnings > 0 ? (
        <span
          className={cn(
            "tabular-nums",
            blocked ? "text-status-blocking" : "text-text-3",
          )}
          aria-hidden
        >
          · {cell.open_warnings}
        </span>
      ) : null}
      {parts.length > 1 ? (
        <span className="sr-only">{parts.slice(1).join(", ")}</span>
      ) : null}
    </div>
  );
}

/**
 * Matris kolonları (#381): mükellef sabit (sol), her dönem bir kolon. Odak
 * dönem başlıkta ve hücrede vurgulanır; sıra ilerlemeye göre olduğundan
 * tablo sıralaması kapalı.
 */
function useMatrixColumns(
  periods: string[],
  focus: string,
  locale: string,
  onPick: (period: string) => void,
) {
  // Mükellef adı mükellef detayına (#469), seçili dönemle (#567).
  const { slug } = useTenant();
  const { setPeriod } = useOfficePeriod();
  const { t } = useLocale();
  const goes = useMemo(
    () => ({
      blockers: t("office.command.goes_blockers"),
      warnings: t("office.command.goes_warnings"),
      change_requests: t("office.command.goes_change_requests"),
      overview: t("office.command.goes_overview"),
    }),
    [t],
  );
  return useMemo<ColumnDef<PeriodMatrixRow, unknown>[]>(
    () => [
      createColumn<PeriodMatrixRow>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        enableHiding: false,
        gridPrimary: true,
        meta: { cellClassName: "bg-card max-w-72" },
        cell: ({ row }) => (
          <>
            <Link
              href={routes.tenant.engagement(
                slug,
                row.original.engagement_uuid,
              )}
              onClick={() => focus && setPeriod(focus)}
              className="block truncate font-medium hover:underline"
            >
              {row.original.taxpayer_title}
            </Link>
            <Figure className="text-muted-foreground text-xs">
              {row.original.taxpayer_tax_id}
            </Figure>
          </>
        ),
      }),
      ...periods.map((p) =>
        createColumn<PeriodMatrixRow>({
          id: p,
          // Başlık ay adı (çeviri anahtarı değil): labelKey boş, label dolu.
          labelKey: "",
          label: monthLabel(p, locale),
          accessorFn: (r) => {
            const c = cellAt(r, p);
            return c ? statusOf(c) : "";
          },
          enableHiding: false,
          // Başlık dönemi seçen düğme (sıralama menüsü yerine).
          header: () => (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onPick(p)}
              aria-pressed={p === focus}
              className={cn(
                "h-auto min-h-6 w-full rounded px-2 py-0.5 text-xs font-medium max-md:min-h-11",
                p === focus
                  ? "bg-foreground text-background hover:bg-foreground hover:text-background"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {monthLabel(p, locale)}
            </Button>
          ),
          meta: {
            align: "center",
            customHeader: true,
            headerClassName: "px-1",
            cellClassName: cn("px-1", p === focus && "bg-muted/30"),
          },
          // Hücre o dönemin işine götürür (#567): dönem seçilir; engel varsa
          // engelleyici uyarılar, yalnız uyarı varsa uyarı listesi, yoksa
          // mükellef özeti o dönemle açılır.
          cell: ({ row }) => {
            const cell = cellAt(row.original, p);
            if (!cell) return null;
            const target = cellTarget(cell);
            const href = cellHref(slug, row.original.engagement_uuid, target);
            return (
              <Link
                href={href}
                data-target={target}
                onClick={() => setPeriod(p)}
                className="focus-visible:ring-ring block rounded-md transition-opacity hover:opacity-85 focus-visible:ring-2 focus-visible:outline-none"
              >
                {/* Ad görünen metni içerir (axe label-content-name-mismatch). */}
                <span className="sr-only">
                  {`${row.original.taxpayer_title} · ${monthLabel(p, locale)}: `}
                </span>
                <CellBadge cell={cell} hint={goes[target]} />
              </Link>
            );
          },
        }),
      ),
    ],
    [periods, focus, locale, onPick, slug, setPeriod, goes],
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
  const blockersId = useId();

  const rows = useMemo(() => matrix.data?.items ?? [], [matrix.data]);
  const counts = useMemo(() => statusCounts(rows, focus), [rows, focus]);
  const visible = useMemo(
    () => sortByProgress(filterRows(rows, { ...filter, query }, focus), focus),
    [rows, filter, query, focus],
  );
  const columns = useMatrixColumns(periods, focus, locale, setPicked);

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
        <div className="flex min-h-6 items-center gap-2 max-md:min-h-11">
          <Checkbox
            id={blockersId}
            checked={filter.blockersOnly}
            onCheckedChange={(v) => set("blockersOnly", v === true)}
          />
          <Label htmlFor={blockersId} className="text-xs font-normal">
            {t("office.command.blockers_only")}
          </Label>
        </div>
        <InputGroup className="h-8 w-auto max-md:h-11">
          <InputGroupAddon>
            <Search className="size-3.5" aria-hidden />
          </InputGroupAddon>
          <InputGroupInput
            className="w-44 min-w-0"
            aria-label={t("office.command.search")}
            placeholder={t("office.command.search")}
            value={filter.query}
            onChange={(e) => set("query", e.target.value)}
          />
        </InputGroup>
      </div>

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {cellStatuses.map((s) => (
          <Button
            key={s}
            variant="outline"
            onClick={() => set("status", filter.status === s ? "all" : s)}
            className={cn(
              "bg-card hover:bg-card h-auto flex-col items-start gap-1 rounded-xl p-3 text-left font-normal shadow-none transition-colors",
              filter.status === s
                ? "border-foreground/40"
                : "hover:border-foreground/20",
            )}
            aria-pressed={filter.status === s}
          >
            <span className="flex items-center gap-1.5 text-xs">
              <StepMarker state={statusStep[s]} />
              <span className="text-muted-foreground">
                {t(`office.command.status_${s}`)}
              </span>
            </span>
            <Figure className="text-xl font-semibold">{counts[s]}</Figure>
          </Button>
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
          <ErrorState
            className="m-4 border-0 py-8"
            title={t("office.load_failed")}
            onRetry={() => void matrix.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <DataTable<PeriodMatrixRow>
            columns={columns}
            data={visible}
            getRowId={(r) => r.engagement_uuid}
            bordered={false}
            maxHeight="70vh"
            emptyTitle={t("office.command.no_rows")}
            emptyDescription=""
            // Mükellef kolonu yatay kaydırmada solda sabit.
            initialState={{ columnPinning: { left: ["taxpayer"] } }}
            features={{
              toolbar: false,
              sorting: false,
              pagination: false,
              rowSelection: false,
              columnFilters: false,
              globalFilter: false,
              facetedFilters: false,
              viewMode: false,
              columnVisibility: false,
              columnResizing: false,
              columnPinning: false,
              columnOrdering: false,
              density: false,
              stickyHeader: true,
            }}
          />
        )}
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.command.workflow_note_api")}
      </p>
    </div>
  );
}
