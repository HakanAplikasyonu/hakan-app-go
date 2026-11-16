"use client";

import { CircleAlert, Info, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { BosDurum, TacRozeti } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useEngagementDirectory,
  type EngagementEntry,
} from "../hooks/use-engagement-directory";
import {
  useAcknowledgeWarningGroup,
  useWarningGroups,
  useWarningRules,
  useWarnings,
} from "../hooks/use-warnings";
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
  Panel,
  SeverityBadge,
  TableBox,
  WarningStatusLabel,
} from "./office-ui";
import {
  ApiWarningActions,
  WARNINGS_WRITE_PERMISSION,
} from "./warning-actions";
import { useOfficePeriod } from "../period";

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
function useApiWarningText() {
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

function useEntityLabel() {
  const { t } = useLocale();
  return (type: string) => {
    const key = `office.warnings_api.entity_${type}`;
    const label = t(key);
    return label === key ? type : label;
  };
}

function WarningRow({
  warning: w,
  taxpayer,
  directoryLoading,
}: {
  warning: ApiDataWarning;
  taxpayer: EngagementEntry | undefined;
  directoryLoading: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const warningText = useApiWarningText();
  const entityLabel = useEntityLabel();
  return (
    <tr>
      <td>
        <SeverityBadge severity={w.severity} />
      </td>
      <td className="min-w-72">
        <div>{warningText(w)}</div>
        <Figure className="text-muted-foreground text-xs">
          {[w.code, w.producer, w.period].filter(Boolean).join(" · ")}
        </Figure>
      </td>
      <td>
        <div>
          {taxpayer?.title ??
            (directoryLoading
              ? "…"
              : t("office.warnings_api.unknown_taxpayer"))}
        </div>
        {taxpayer?.taxId ? (
          <Figure className="text-muted-foreground text-xs">
            {taxpayer.taxId}
          </Figure>
        ) : null}
      </td>
      <td>
        {w.entity_type === "invoice" && w.entity_uuid ? (
          <Link
            href={routes.tenant.invoice(slug, w.entity_uuid, w.engagement_uuid)}
            className="text-brand hover:underline"
          >
            {t("office.warnings_api.open_invoice")}
          </Link>
        ) : (
          <span className="text-muted-foreground">
            {entityLabel(w.entity_type)}
          </span>
        )}
      </td>
      <td className="whitespace-nowrap">
        <WarningStatusLabel status={w.status} />
        <div className="text-muted-foreground text-xs">
          {date(w.updated_at, "dd.MM.yyyy", locale)}
        </div>
      </td>
      <td className="text-right">
        <ApiWarningActions warning={w} />
      </td>
    </tr>
  );
}

/**
 * Düz liste (eski görünüm): sunucuda süzme ve sayfalama. Mükellef seçiliyse
 * o engagement'ın listesi kullanılır (ofis listesinde engagement süzgeci yok).
 */
function WarningsTable({
  engagement,
  severity,
  status,
  period,
}: {
  engagement?: string;
  severity?: ApiWarningSeverity;
  status?: ApiWarningStatus;
  period?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const directory = useEngagementDirectory();
  const rules = useWarningRules(slug);
  const [code, setCode] = useState(ALL);
  const filterKey = [engagement, severity, status, period, code].join("|");
  const [page, setPage] = useState({ key: filterKey, offset: 0 });
  const offset = page.key === filterKey ? page.offset : 0;
  const setOffset = (next: number) => setPage({ key: filterKey, offset: next });

  const query: WarningListQuery = {
    limit: PAGE_SIZE,
    offset,
    severity,
    status,
    code: code === ALL ? undefined : code,
    period,
  };
  const list = useWarnings(slug, query, engagement);
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + items.length, total);
  const codes = [
    ...new Set((rules.data?.items ?? []).map((r) => r.code)),
  ].sort();

  return (
    <div className="space-y-3">
      {codes.length ? (
        <FilterSelect
          label={t("office.warnings_api.filter_code")}
          value={code}
          onChange={setCode}
          options={[
            { value: ALL, label: t("office.filter_all") },
            ...codes.map((c) => ({ value: c, label: c })),
          ]}
        />
      ) : null}
      <Panel>
        {list.isLoading ? (
          <div className="p-4">
            <Loading label={t("common.loading")} />
          </div>
        ) : list.isError ? (
          <ErrorState
            className="m-4"
            title={t("office.warnings_api.load_error")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : items.length === 0 ? (
          <EmptyState className="m-4" title={t("office.warnings_empty")} />
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_severity")}</th>
                <th>{t("office.col_warning")}</th>
                <th>{t("office.col_taxpayer")}</th>
                <th>{t("office.col_document")}</th>
                <th>{t("office.col_status")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((w) => (
                <WarningRow
                  key={w.uuid}
                  warning={w}
                  taxpayer={directory.entries.get(w.engagement_uuid)}
                  directoryLoading={directory.isLoading}
                />
              ))}
            </tbody>
          </TableBox>
        )}
        {total > 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-3 text-sm">
            <span className="text-muted-foreground text-xs">
              {t("office.warnings_api.range", { from, to, total })}
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={offset === 0 || list.isFetching}
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
              >
                {t("office.warnings_api.prev")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={offset + PAGE_SIZE >= total || list.isFetching}
                onClick={() => setOffset(offset + PAGE_SIZE)}
              >
                {t("office.warnings_api.next")}
              </Button>
            </div>
          </div>
        ) : null}
      </Panel>
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
  const { can } = usePermission();
  const directory = useEngagementDirectory();
  const warningText = useApiWarningText();
  const entityLabel = useEntityLabel();
  const groupTitle = useGroupTitle();
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
        {status === "open" && can(WARNINGS_WRITE_PERMISSION) ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11 md:min-h-8"
            disabled={ack.isPending}
            onClick={() => ack.mutate({ code: group.code, period })}
          >
            {t("office.warnings_ack_all")}
          </Button>
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
          {items.map((w) => {
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
                    <span className="font-semibold">
                      {tp?.title ??
                        (directory.isLoading
                          ? "…"
                          : t("office.warnings_api.unknown_taxpayer"))}
                    </span>
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
                  {w.entity_type === "invoice" && w.entity_uuid ? (
                    <Button
                      asChild
                      size="sm"
                      variant="outline"
                      className="min-h-11 md:min-h-8"
                    >
                      <Link
                        href={routes.tenant.invoice(
                          slug,
                          w.entity_uuid,
                          w.engagement_uuid,
                        )}
                      >
                        {t("office.warnings_api.open_invoice")}
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
  const { t } = useLocale();
  const { slug } = useTenant();
  const directory = useEngagementDirectory();
  const { period } = useOfficePeriod();
  const [engagement, setEngagement] = useState(ALL);
  const [status, setStatus] = useState<StatusTab>("open");
  const [severity, setSeverity] = useState<ApiWarningSeverity | null>(null);
  const [view, setView] = useState<ViewMode>("grouped");
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
        <h1 className="font-display text-xl font-bold">
          {t("office.warnings_title")}
        </h1>
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
