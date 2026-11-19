"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  CircleAlert,
  Info,
  Link2,
  PanelRightOpen,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { Suspense, useEffect, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityTable, useUrlListState } from "@/components/entity";
import { BosDurum, TacRozeti } from "@/components/hakan";
import { createActionsColumn, createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useEngagementScope, useInvoiceHref } from "../engagement-scope";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import {
  useAcknowledgeWarningGroup,
  useWarningGroups,
  useWarningRules,
  useWarnings,
} from "../hooks/use-warnings";
import { warningRecordHref } from "../lib/related-href";
import { formatAmount } from "../lib/decimal";
import type {
  ApiDataWarning,
  ApiWarningGroup,
  ApiWarningSeverity,
  ApiWarningStatus,
  WarningListQuery,
} from "../services/warnings.service";
import {
  FilterSelect,
  Figure,
  SeverityBadge,
  WarningStatusLabel,
} from "./office-ui";
import {
  ApiWarningActions,
  DecisionNoteForm,
  WARNINGS_WRITE_PERMISSION,
} from "./warning-actions";
import { ENGAGEMENT_PARAM } from "./engagement-picker";
import { useFocusHref, useRelatedColumn } from "./related-column";
import { useOfficePeriod } from "../period";
import { useWarningSheet, WarningSheet } from "./warning-sheet";

const PAGE_SIZE = 25;
const ALL = "all";

function paramText(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") {
    return /^-?\d+\.\d{1,4}$/.test(value) ? formatAmount(value) : value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  // Ham JSON gösterilmez (#375): liste değerleri virgülle, nesneler boş.
  if (Array.isArray(value))
    return value.map(paramText).filter(Boolean).join(", ");
  return "";
}

/**
 * Message from the frontend catalog (`office.warning.<CODE>`, paket 05).
 * The API's rule catalog carries no titles, so an unknown code is shown raw.
 */
export function useApiWarningText() {
  const { t } = useLocale();
  return (w: Pick<ApiDataWarning, "code" | "params">) => {
    const key = `office.warning.${w.code}`;
    const params = Object.fromEntries(
      Object.entries(w.params ?? {}).map(([k, v]) => [k, paramText(v)]),
    );
    const msg = t(key, params);
    return msg === key ? w.code : msg;
  };
}

export function useEntityLabel() {
  const { t } = useLocale();
  return (type: string) => {
    const key = `office.warnings_api.entity_${type}`;
    const label = t(key);
    return label === key ? type : label;
  };
}

function useWarningColumns() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const invoiceHref = useInvoiceHref({ carry: false });
  const directory = useEngagementDirectory();
  const warningText = useApiWarningText();
  const entityLabel = useEntityLabel();
  const { entries, isLoading: directoryLoading } = directory;
  return useMemo<ColumnDef<ApiDataWarning, unknown>[]>(
    () => [
      createColumn<ApiDataWarning>({
        id: "severity",
        labelKey: "office.col_severity",
        accessorKey: "severity",
        cell: ({ row }) => <SeverityBadge severity={row.original.severity} />,
      }),
      createColumn<ApiDataWarning>({
        id: "warning",
        labelKey: "office.col_warning",
        accessorKey: "code",
        gridPrimary: true,
        meta: { cellClassName: "min-w-72" },
        cell: ({ row }) => {
          const w = row.original;
          return (
            <>
              <div>{warningText(w)}</div>
              <Figure className="text-muted-foreground text-xs">
                {[w.code, w.producer, w.period].filter(Boolean).join(" · ")}
              </Figure>
            </>
          );
        },
      }),
      createColumn<ApiDataWarning>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "engagement_uuid",
        cell: ({ row }) => {
          const taxpayer = entries.get(row.original.engagement_uuid);
          return (
            <>
              <div>
                {taxpayer ? (
                  // Mükellef adı mükellefin Uyarılar sekmesine (#469).
                  <Link
                    href={routes.tenant.engagementWarnings(
                      slug,
                      row.original.engagement_uuid,
                    )}
                    className="hover:underline"
                  >
                    {taxpayer.title}
                  </Link>
                ) : directoryLoading ? (
                  "…"
                ) : (
                  t("office.warnings_api.unknown_taxpayer")
                )}
              </div>
              {taxpayer?.taxId ? (
                <Figure className="text-muted-foreground text-xs">
                  {taxpayer.taxId}
                </Figure>
              ) : null}
            </>
          );
        },
      }),
      createColumn<ApiDataWarning>({
        id: "document",
        labelKey: "office.col_document",
        accessorKey: "entity_type",
        cell: ({ row }) => {
          const w = row.original;
          const href =
            w.entity_type === "invoice" && w.entity_uuid
              ? invoiceHref(w.entity_uuid, w.engagement_uuid)
              : warningRecordHref(slug, w);
          return href ? (
            <Link href={href} className="text-brand hover:underline">
              {t("office.warnings_api.open_record", {
                record: entityLabel(w.entity_type),
              })}
            </Link>
          ) : (
            <span className="text-muted-foreground">
              {entityLabel(w.entity_type)}
            </span>
          );
        },
      }),
      createColumn<ApiDataWarning>({
        id: "status",
        labelKey: "office.col_status",
        accessorKey: "status",
        meta: { cellClassName: "whitespace-nowrap" },
        cell: ({ row }) => (
          <>
            <WarningStatusLabel status={row.original.status} />
            <div className="text-muted-foreground text-xs">
              {date(row.original.updated_at, "dd.MM.yyyy", locale)}
            </div>
          </>
        ),
      }),
      createActionsColumn<ApiDataWarning>({
        cell: (w) => <ApiWarningActions warning={w} />,
      }),
    ],
    // warningText / entityLabel her çizimde yeni; yalnızca t'ye bağlı.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [directoryLoading, entries, invoiceHref, locale, t],
  );
}

/**
 * Düz liste (eski görünüm): sunucuda süzme ve sayfalama. Mükellef seçiliyse
 * o engagement'ın listesi kullanılır (ofis listesinde engagement süzgeci yok).
 * Sayfa, sayfa boyutu ve kod süzgeci URL'de (#381).
 */
function WarningsTable({
  listState,
  engagement,
  severity,
  status,
  period,
}: {
  listState: ReturnType<typeof useUrlListState>;
  engagement?: string;
  severity?: ApiWarningSeverity;
  status?: ApiWarningStatus;
  period?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const rules = useWarningRules(slug);
  const baseColumns = useWarningColumns();
  // Uyarının ilişkili kayıtları: kaynak belge ve kural (#469).
  const relatedColumn = useRelatedColumn<ApiDataWarning>(
    "warning",
    (w) => w.uuid,
    (w) => w.code,
  );
  const columns = useMemo<ColumnDef<ApiDataWarning, unknown>[]>(
    () => [...baseColumns, ...relatedColumn],
    [baseColumns, relatedColumn],
  );
  const code = listState.getParam("code") ?? ALL;

  // Üst çubuktaki dönem URL dışında değişir; liste ilk sayfaya döner.
  const lastPeriod = useRef(period);
  const { onPaginationChange } = listState;
  useEffect(() => {
    if (lastPeriod.current === period) return;
    lastPeriod.current = period;
    onPaginationChange((p) => ({ ...p, pageIndex: 0 }));
  }, [onPaginationChange, period]);

  const query: WarningListQuery = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    severity,
    status,
    code: code === ALL ? undefined : code,
    period,
  };
  const list = useWarnings(slug, query, engagement);
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const sheet = useWarningSheet();
  const codes = [
    ...new Set((rules.data?.items ?? []).map((r) => r.code)),
  ].sort();

  return (
    <div className="space-y-3">
      {codes.length ? (
        <FilterSelect
          label={t("office.warnings_api.filter_code")}
          value={code}
          onChange={(v) => listState.setParams({ code: v === ALL ? null : v })}
          options={[
            { value: ALL, label: t("office.filter_all") },
            ...codes.map((c) => ({ value: c, label: c })),
          ]}
        />
      ) : null}
      <EntityTable<ApiDataWarning>
        columns={columns}
        data={items}
        getRowId={(w) => w.uuid}
        // Satır uyarıyı yan panelde açar: kayıt, karar, sıradaki (#529).
        onRowClick={(w) => sheet.open(w, items.indexOf(w))}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.warnings_api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.warnings_empty")}
        emptyDescription=""
        state={listState.tableState}
        rowCount={total}
        pageSizeOptions={[25, 50, 100]}
        // Sunucu sıralama ve serbest arama sunmuyor; yalnızca sayfalama.
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          persistKey: "office.warnings.table",
        }}
      />
      <WarningSheet items={items} sheet={sheet} />
    </div>
  );
}

const SEVERITIES: ApiWarningSeverity[] = ["blocker", "warning", "info"];
const SEVERITY_RANK: Record<string, number> = {
  blocker: 0,
  warning: 1,
  info: 2,
};
const GROUP_PREVIEW = 5;

type StatusTab = "open" | "acknowledged" | "resolved";
type ViewMode = "grouped" | "list";

const severityCard: Record<
  ApiWarningSeverity,
  { icon: typeof TriangleAlert; box: string; ring: string }
> = {
  blocker: {
    icon: TriangleAlert,
    box: "bg-danger-tint text-danger",
    ring: "border-danger",
  },
  warning: {
    icon: CircleAlert,
    box: "bg-warn-tint text-warn",
    ring: "border-warn",
  },
  info: {
    icon: Info,
    box: "bg-surface-3 text-text-2",
    ring: "border-line-strong",
  },
};

/** Grup başlığı: parametreli mesajların kısa adı, yoksa mesajın kendisi. */
function useGroupTitle() {
  const { t } = useLocale();
  return (code: string) => {
    const short = t(`office.warning_group.${code}`);
    if (short !== `office.warning_group.${code}`) return short;
    const msg = t(`office.warning.${code}`);
    return msg === `office.warning.${code}` ? code : msg;
  };
}

/**
 * Bir kodun kartı (tuval OfisUyarilar): başlıkta seviye, ad, kod, sayı ve
 * "Tümünü gördüm"; altında ilk satırlar, gerekirse "tümünü göster".
 */
function WarningGroupCard({
  group,
  status,
  period,
  engagement,
}: {
  group: ApiWarningGroup;
  status: StatusTab;
  period?: string;
  engagement?: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const invoiceHref = useInvoiceHref({ carry: false });
  const scope = useEngagementScope();
  const focusHref = useFocusHref();
  const { can } = usePermission();
  const directory = useEngagementDirectory();
  const warningText = useApiWarningText();
  const entityLabel = useEntityLabel();
  const groupTitle = useGroupTitle();
  const recordHref = (w: ApiDataWarning) =>
    w.entity_type === "invoice" && w.entity_uuid
      ? invoiceHref(w.entity_uuid, w.engagement_uuid)
      : warningRecordHref(slug, w);
  const [expanded, setExpanded] = useState(false);
  const rows = useWarnings(
    slug,
    {
      limit: expanded ? 100 : GROUP_PREVIEW,
      offset: 0,
      code: group.code,
      severity: group.severity,
      status,
      period,
    },
    engagement,
  );
  const ack = useAcknowledgeWarningGroup(slug, engagement);
  const [bulk, setBulk] = useState(false);
  const sheet = useWarningSheet();
  const titleId = `warning-group-${group.code}-${group.severity}`;
  const items = rows.data?.items ?? [];
  return (
    <section
      aria-labelledby={titleId}
      className="bg-surface border-line overflow-hidden rounded-xl border"
    >
      <div className="bg-surface-2 border-line flex flex-wrap items-center gap-3 border-b px-4 py-3 md:px-5">
        <SeverityBadge severity={group.severity} />
        <h2 id={titleId} className="min-w-48 flex-1 text-sm font-semibold">
          {groupTitle(group.code)}
        </h2>
        <span className="text-text-3 font-mono text-xs">{group.code}</span>
        <span
          className="bg-surface-3 text-text-2 rounded-full px-2 py-0.5 text-xs font-semibold tabular-nums"
          aria-label={t("office.warnings_group_count", { count: group.count })}
        >
          {group.count}
        </span>
        {status === "open" && can(WARNINGS_WRITE_PERMISSION) && !bulk ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11 md:min-h-8"
            disabled={ack.isPending}
            onClick={() => setBulk(true)}
          >
            {t("office.warnings_ack_all")}
          </Button>
        ) : null}
        {bulk ? (
          <div className="w-full">
            <DecisionNoteForm
              label={t("office.warning_decision.reason_bulk", {
                count: group.count,
              })}
              confirmLabel={t("office.warnings_ack_all")}
              busy={ack.isPending}
              onConfirm={(note) =>
                ack.mutate(
                  { code: group.code, period, note },
                  { onSuccess: () => setBulk(false) },
                )
              }
              onCancel={() => setBulk(false)}
            />
          </div>
        ) : null}
      </div>
      {ack.error ? (
        <p role="alert" className="text-danger px-5 pt-2 text-xs">
          {ack.error.message || t("office.warnings_api.action_error")}
        </p>
      ) : null}
      {rows.isLoading ? (
        <div className="px-5 py-4">
          <Loading label={t("common.loading")} />
        </div>
      ) : rows.isError ? (
        <p className="text-danger px-5 py-4 text-sm">
          {t("office.warnings_api.load_error")}
        </p>
      ) : (
        <ul className="divide-line divide-y">
          {items.map((w, i) => {
            const tp = directory.entries.get(w.engagement_uuid);
            return (
              <li
                key={w.uuid}
                className="flex flex-wrap items-center gap-3 px-4 py-3 md:px-5"
              >
                <TacRozeti
                  durum={w.status === "open" ? "sorun" : "onaylandi"}
                  boyut={16}
                />
                <div className="min-w-56 flex-1">
                  <div className="text-sm">
                    {tp && !scope ? (
                      // Genel listede mükellef adı mükellefin Uyarılar sekmesine (#469).
                      <Link
                        href={routes.tenant.engagementWarnings(
                          slug,
                          w.engagement_uuid,
                        )}
                        className="font-semibold hover:underline"
                      >
                        {tp.title}
                      </Link>
                    ) : (
                      <span className="font-semibold">
                        {tp?.title ??
                          (directory.isLoading
                            ? "…"
                            : t("office.warnings_api.unknown_taxpayer"))}
                      </span>
                    )}
                    {" · "}
                    <span className="text-text-2">
                      {entityLabel(w.entity_type)}
                    </span>
                  </div>
                  <div className="text-text-3 text-xs">{warningText(w)}</div>
                </div>
                <time
                  dateTime={w.updated_at}
                  className="text-text-3 text-xs whitespace-nowrap"
                >
                  {date(w.updated_at, "dd.MM.yyyy", locale)}
                </time>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    size="sm"
                    className="min-h-11 md:min-h-8"
                    onClick={() => {
                      // Gezinme grubun tamamında olsun.
                      setExpanded(true);
                      sheet.open(w, i);
                    }}
                  >
                    <PanelRightOpen className="size-4" aria-hidden />
                    {t("office.warning_sheet.open")}
                  </Button>
                  {recordHref(w) ? (
                    <Button
                      asChild
                      size="sm"
                      variant="outline"
                      className="min-h-11 md:min-h-8"
                    >
                      <Link href={recordHref(w)!}>
                        {t("office.warnings_api.open_record", {
                          record: entityLabel(w.entity_type),
                        })}
                      </Link>
                    </Button>
                  ) : null}
                  {scope ? (
                    <Button
                      asChild
                      size="sm"
                      variant="ghost"
                      className="min-h-11 md:min-h-8"
                    >
                      <Link
                        href={focusHref("warning", w.uuid)}
                        scroll={false}
                        aria-label={`${t("office.related.title")}: ${w.code}`}
                      >
                        <Link2 className="size-4" aria-hidden />
                        {t("office.related.title")}
                      </Link>
                    </Button>
                  ) : null}
                  <ApiWarningActions warning={w} />
                </div>
              </li>
            );
          })}
        </ul>
      )}
      <WarningSheet items={items} sheet={sheet} />
      {group.count > GROUP_PREVIEW ? (
        <div className="border-line border-t px-4 py-2 md:px-5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-brand-text min-h-11 md:min-h-8"
            aria-expanded={expanded}
            onClick={() => setExpanded((x) => !x)}
          >
            {expanded
              ? t("office.warnings_show_less")
              : t("office.warnings_show_all", { count: group.count })}
          </Button>
        </div>
      ) : null}
    </section>
  );
}

/**
 * `/t/{slug}/warnings` (arayüz 6/9, #329; tuval OfisUyarilar): seviye
 * kartları (süzgeç olarak da çalışır), Açık / Görüldü / Çözüldü, koda göre
 * gruplu kartlar ya da düz liste. Dönem üst çubuktaki global seçiciden.
 */
export function WarningsPage() {
  return (
    <Suspense fallback={<Loading />}>
      <WarningsInner />
    </Suspense>
  );
}

const STATUS_TABS: StatusTab[] = ["open", "acknowledged", "resolved"];

function WarningsInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const directory = useEngagementDirectory();
  const { period } = useOfficePeriod();
  // Süzgeçler URL'de (#381): geri tuşu ve paylaşılan bağlantı aynı görünümü
  // açar; süzgeç değişince liste görünümü ilk sayfaya döner.
  const listState = useUrlListState({ initialPageSize: PAGE_SIZE });
  // Mükellef detayının Uyarılar sekmesinde mükellef sabit, seçici yok (#468).
  const scope = useEngagementScope();
  const engagement = scope ?? listState.getParam(ENGAGEMENT_PARAM) ?? ALL;
  const rawStatus = listState.getParam("status") as StatusTab | null;
  const status: StatusTab =
    rawStatus && STATUS_TABS.includes(rawStatus) ? rawStatus : "open";
  const rawSeverity = listState.getParam("severity") as ApiWarningSeverity;
  const severity = SEVERITIES.includes(rawSeverity) ? rawSeverity : null;
  const view: ViewMode =
    listState.getParam("view") === "list" ? "list" : "grouped";
  const setEngagement = (v: string) =>
    listState.setParams({ [ENGAGEMENT_PARAM]: v === ALL ? null : v });
  const setStatus = (v: StatusTab) =>
    listState.setParams({ status: v === "open" ? null : v });
  const setSeverity = (v: ApiWarningSeverity | null) =>
    listState.setParams({ severity: v });
  const setView = (v: ViewMode) =>
    listState.setParams({ view: v === "grouped" ? null : v });
  const eng = engagement === ALL ? undefined : engagement;

  const open = useWarningGroups(slug, { period, status: "open" }, eng);
  const seen = useWarningGroups(slug, { period, status: "acknowledged" }, eng);
  const done = useWarningGroups(slug, { period, status: "resolved" }, eng);
  const byStatus = { open, acknowledged: seen, resolved: done };
  const current = byStatus[status];
  const sum = (items?: ApiWarningGroup[]) =>
    (items ?? []).reduce((n, g) => n + g.count, 0);
  const severityCount = (s: ApiWarningSeverity) =>
    sum(current.data?.items.filter((g) => g.severity === s));
  const groups = [...(current.data?.items ?? [])]
    .filter((g) => !severity || g.severity === severity)
    .sort(
      (a, b) =>
        (SEVERITY_RANK[a.severity] ?? 9) - (SEVERITY_RANK[b.severity] ?? 9) ||
        b.count - a.count,
    );

  const statusTabs: { key: StatusTab; label: string; count?: number }[] = [
    {
      key: "open",
      label: t("office.warning_status_open"),
      count: sum(open.data?.items),
    },
    {
      key: "acknowledged",
      label: t("office.warning_status_acknowledged"),
      count: sum(seen.data?.items),
    },
    {
      key: "resolved",
      label: t("office.warning_status_resolved"),
      count: sum(done.data?.items),
    },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-3">
        {scope ? (
          <h2 className="font-display text-xl font-bold">
            {t("office.warnings_title")}
          </h2>
        ) : (
          <h1 className="font-display text-xl font-bold">
            {t("office.warnings_title")}
          </h1>
        )}
        <div
          role="group"
          aria-label={t("office.col_status")}
          className="bg-surface-3 flex gap-0.5 rounded-lg p-0.5"
        >
          {statusTabs.map((s) => (
            <button
              key={s.key}
              type="button"
              aria-pressed={status === s.key}
              onClick={() => setStatus(s.key)}
              className={cn(
                "focus-visible:ring-ring min-h-11 rounded-md px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none md:min-h-8",
                status === s.key
                  ? "bg-surface text-foreground shadow-sm"
                  : "text-text-2 hover:text-foreground",
              )}
            >
              {s.label}
              {s.count ? (
                <span className="ms-1.5 tabular-nums">{s.count}</span>
              ) : null}
            </button>
          ))}
        </div>
        <div className="flex-1" />
        {scope ? null : (
          <FilterSelect
            label={t("office.col_taxpayer")}
            value={engagement}
            onChange={setEngagement}
            options={[
              { value: ALL, label: t("office.filter_all") },
              ...directory.list.map((x) => ({
                value: x.engagementUuid,
                label: x.title,
              })),
            ]}
          />
        )}
        <FilterSelect<ViewMode>
          label={t("office.warnings_view")}
          value={view}
          onChange={setView}
          options={[
            { value: "grouped", label: t("office.warnings_view_grouped") },
            { value: "list", label: t("office.warnings_view_list") },
          ]}
        />
      </div>
      <p className="text-text-2 -mt-2 text-sm">
        {t("office.warnings_description")}
      </p>

      <div
        role="group"
        aria-label={t("office.col_severity")}
        className="grid gap-3 sm:grid-cols-3"
      >
        {SEVERITIES.map((s) => {
          const { icon: Icon, box, ring } = severityCard[s];
          const on = severity === s;
          return (
            <button
              key={s}
              type="button"
              aria-pressed={on}
              onClick={() => setSeverity(on ? null : s)}
              className={cn(
                "bg-surface focus-visible:ring-ring flex items-center gap-3.5 rounded-xl border p-4 text-start transition-colors focus-visible:ring-2 focus-visible:outline-none",
                on || (s === "blocker" && severityCount(s) > 0)
                  ? ring
                  : "border-line",
                on && "ring-1 ring-current",
              )}
            >
              <span
                aria-hidden
                className={cn(
                  "flex size-10 shrink-0 items-center justify-center rounded-lg",
                  box,
                )}
              >
                <Icon className="size-5" />
              </span>
              <span className="flex flex-col">
                <span className="font-display text-2xl font-bold tabular-nums">
                  {current.isLoading ? "–" : severityCount(s)}
                </span>
                <span className="text-text-2 text-sm">
                  {t(`office.warnings_card_${s}`)}
                </span>
              </span>
            </button>
          );
        })}
      </div>

      {view === "list" ? (
        <WarningsTable
          listState={listState}
          engagement={eng}
          severity={severity ?? undefined}
          status={status}
          period={period}
        />
      ) : current.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : current.isError ? (
        <ErrorState
          title={t("office.warnings_api.load_error")}
          description={current.error.message}
          onRetry={() => void current.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : groups.length === 0 ? (
        <BosDurum
          baslik={t("office.warnings_empty")}
          aciklama={t(`office.warnings_empty_${status}`)}
        />
      ) : (
        <div className="space-y-4">
          {groups.map((g) => (
            <WarningGroupCard
              key={`${g.code}:${g.severity}`}
              group={g}
              status={status}
              period={period}
              engagement={eng}
            />
          ))}
        </div>
      )}
    </div>
  );
}
