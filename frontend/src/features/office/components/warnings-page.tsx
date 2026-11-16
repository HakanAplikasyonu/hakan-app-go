"use client";

import { TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import {
  useEngagementDirectory,
  type EngagementEntry,
} from "../hooks/use-engagement-directory";
import { useWarningRules, useWarnings } from "../hooks/use-warnings";
import { formatAmount } from "../lib/decimal";
import type {
  ApiDataWarning,
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
import { ApiWarningActions } from "./warning-actions";
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
  return JSON.stringify(value);
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
 * `/t/{slug}/warnings`: data warnings from the warning engine. Filters and
 * pagination run on the server; a taxpayer filter switches to that
 * engagement's list (the office-wide list has no engagement filter).
 */
export function WarningsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const directory = useEngagementDirectory();
  const rules = useWarningRules(slug);
  const [engagement, setEngagement] = useState(ALL);
  const [severity, setSeverity] = useState<typeof ALL | ApiWarningSeverity>(
    ALL,
  );
  const [status, setStatus] = useState<typeof ALL | ApiWarningStatus>("open");
  const [code, setCode] = useState(ALL);
  // Dönem üst çubuktaki global seçiciden (#326); dönem değişince ilk sayfa.
  const { period } = useOfficePeriod();
  const [page, setPage] = useState({ period, offset: 0 });
  const offset = page.period === period ? page.offset : 0;
  const setOffset = (next: number) => setPage({ period, offset: next });

  const query: WarningListQuery = {
    limit: PAGE_SIZE,
    offset,
    severity: severity === ALL ? undefined : severity,
    status: status === ALL ? undefined : status,
    code: code === ALL ? undefined : code,
    period,
  };
  const list = useWarnings(
    slug,
    query,
    engagement === ALL ? undefined : engagement,
  );

  const filter =
    <T,>(set: (v: T) => void) =>
    (v: T) => {
      set(v);
      setOffset(0);
    };

  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + items.length, total);
  const codes = [
    ...new Set((rules.data?.items ?? []).map((r) => r.code)),
  ].sort();

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.warnings_title")}
        description={t("office.warnings_description")}
        icon={<TriangleAlert className="size-6" />}
      />
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label={t("office.col_taxpayer")}
          value={engagement}
          onChange={filter(setEngagement)}
          options={[
            { value: ALL, label: t("office.filter_all") },
            ...directory.list.map((x) => ({
              value: x.engagementUuid,
              label: x.title,
            })),
          ]}
        />
        <FilterSelect<typeof ALL | ApiWarningSeverity>
          label={t("office.col_severity")}
          value={severity}
          onChange={filter(setSeverity)}
          options={[
            { value: ALL, label: t("office.filter_all") },
            { value: "blocker", label: t("office.severity_blocker") },
            { value: "warning", label: t("office.severity_warning") },
            { value: "info", label: t("office.severity_info") },
          ]}
        />
        <FilterSelect<typeof ALL | ApiWarningStatus>
          label={t("office.col_status")}
          value={status}
          onChange={filter(setStatus)}
          options={[
            { value: "open", label: t("office.warning_status_open") },
            {
              value: "acknowledged",
              label: t("office.warning_status_acknowledged"),
            },
            { value: "resolved", label: t("office.warning_status_resolved") },
            {
              value: "auto_resolved",
              label: t("office.warning_status_auto_resolved"),
            },
            { value: ALL, label: t("office.filter_all") },
          ]}
        />
        {codes.length ? (
          <FilterSelect
            label={t("office.warnings_api.filter_code")}
            value={code}
            onChange={filter(setCode)}
            options={[
              { value: ALL, label: t("office.filter_all") },
              ...codes.map((c) => ({ value: c, label: c })),
            ]}
          />
        ) : null}
      </div>
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
              {t("office.warnings_api.range", {
                from,
                to,
                total,
              })}
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
