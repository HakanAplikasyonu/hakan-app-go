"use client";

import { CalendarClock } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";

import { PageHeader } from "@/components/layout/page-header";
import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { formatMoney } from "@/lib/decimal";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useDecideLateDocument,
  useLateDocumentSummary,
  useLateDocuments,
  type LateDocument,
  type LateDocumentDecision,
} from "../hooks/use-late-documents";
import { periodText } from "../lib/change-request-text";
import {
  LATE_STATUSES,
  lateDocumentKind,
  lateSentence,
  lateStatusTone,
} from "../lib/late-document-text";
import { relatedHref } from "../lib/related-href";
import { useOfficePeriod } from "../period";
import { ChangeRequestDecisions } from "./change-requests-panel";
import { Panel } from "./office-ui";

/**
 * Geç gelen bir belgenin kartı (#780 C): ait olduğu dönem kapandıktan sonra
 * geldiği cümleyle söylenir; ofis düzeltme beyannamesine alır, cari döneme
 * alır ya da yok sayar. Karar kaydedilir; "Geri al" bekliyor durumuna
 * döndürür.
 */
export function LateDocumentCard({
  item,
  engagement,
  showDocumentLink = true,
}: {
  item: LateDocument;
  engagement: string;
  showDocumentLink?: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canDecide = can(Permission.TenantChangeRequestsDecide);
  const decide = useDecideLateDocument(engagement);
  const router = useRouter();
  const { setPeriod } = useOfficePeriod();
  const month = periodText(item.period, locale);
  const purchase =
    item.document_type === "invoice" && item.invoice_direction === "incoming";
  // Alış faturasının KDV'si ileri bir aya alındıysa (#640) cari dönem o ay.
  const target = item.vat_deduction_period ?? undefined;
  const href = showDocumentLink
    ? relatedHref(slug, engagement, {
        kind: item.document_type,
        uuid: item.document_uuid,
        label: item.label,
      })
    : null;

  const run = (decision: LateDocumentDecision, done: string) =>
    decide.mutate(
      { uuid: item.uuid, decision },
      {
        onSuccess: () => toast.success(t(done)),
        onError: (err) =>
          toast.error(
            isApiError(err) ? err.message : t("office.late_docs.failed"),
          ),
      },
    );

  return (
    <article
      data-testid="late-document"
      data-status={item.status}
      className="bg-surface border-line space-y-3 rounded-lg border p-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill tone={lateStatusTone(item.status)}>
          {t(`office.late_docs.status_${item.status}`)}
        </StatusPill>
        <span className="text-sm font-medium">
          {lateDocumentKind(item.document_type, t)} · {item.label || "—"}
        </span>
        {item.counterparty ? (
          <span className="text-text-2 text-sm">{item.counterparty}</span>
        ) : null}
        <span className="ml-auto text-sm font-medium tabular-nums">
          {formatMoney(item.amount, item.currency)}
        </span>
      </div>
      <p className="text-sm" data-testid="late-document-sentence">
        {lateSentence(item, t, locale)}
      </p>
      <p className="text-text-2 text-sm">
        {t(`office.late_docs.reason_${item.close_reason}`, { month })}
      </p>
      {item.vat_deduction_period ? (
        <p className="text-text-2 text-sm">
          {t("office.late_docs.vat_moved", {
            month: periodText(item.vat_deduction_period, locale),
          })}
        </p>
      ) : null}
      {item.status === "pending" ? (
        <ul className="text-text-2 list-disc space-y-1 pl-5 text-sm">
          <li>{t("office.late_docs.effect_correction", { month })}</li>
          <li>
            {purchase
              ? t("office.late_docs.effect_current_purchase")
              : t("office.late_docs.effect_current")}
          </li>
          <li>{t("office.late_docs.effect_ignored")}</li>
        </ul>
      ) : (
        <p className="text-text-2 text-sm" data-testid="late-document-decision">
          {t(`office.late_docs.decided_${item.status}`, {
            month: item.target_period
              ? periodText(item.target_period, locale)
              : month,
          })}
          {item.decided_by_name
            ? ` · ${t("office.late_docs.decided_by", { name: item.decided_by_name })}`
            : ""}
          {item.note ? ` · ${item.note}` : ""}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {href ? (
          <Button asChild variant="ghost" size="sm">
            <Link href={href}>{t("office.late_docs.open_document")}</Link>
          </Button>
        ) : null}
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            setPeriod(item.period);
            router.push(routes.tenant.engagementTaxReturns(slug, engagement));
          }}
        >
          {t("office.late_docs.open_returns", { month })}
        </Button>
        {canDecide ? (
          <span className="ml-auto flex flex-wrap gap-2">
            {item.status === "pending" ? (
              <>
                <Button
                  size="sm"
                  disabled={decide.isPending}
                  onClick={() =>
                    run(
                      { status: "correction" },
                      "office.late_docs.done_correction",
                    )
                  }
                >
                  {t("office.late_docs.action_correction")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={decide.isPending}
                  onClick={() =>
                    run(
                      target
                        ? { status: "current_period", target_period: target }
                        : { status: "current_period" },
                      "office.late_docs.done_current_period",
                    )
                  }
                >
                  {t("office.late_docs.action_current_period")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={decide.isPending}
                  onClick={() =>
                    run({ status: "ignored" }, "office.late_docs.done_ignored")
                  }
                >
                  {t("office.late_docs.action_ignored")}
                </Button>
              </>
            ) : (
              <Button
                size="sm"
                variant="ghost"
                disabled={decide.isPending}
                onClick={() =>
                  run({ status: "pending" }, "office.late_docs.done_pending")
                }
              >
                {t("office.late_docs.action_undo")}
              </Button>
            )}
          </span>
        ) : null}
      </div>
    </article>
  );
}

/**
 * Belgenin kendi ekranındaki uyarı (fatura ayrıntısı, seçili masraf / Z
 * raporu): belge kapanıştan sonra geldiyse kart görünür, gelmediyse hiçbir
 * şey çizilmez.
 */
export function LateDocumentNote({
  engagement,
  documentUuid,
}: {
  engagement: string;
  documentUuid: string;
}) {
  const { can } = usePermission();
  const q = useLateDocuments(
    engagement,
    { document_uuid: documentUuid },
    can(Permission.TenantPeriodsRead) && !!documentUuid,
  );
  const item = q.data?.items[0];
  if (!item) return null;
  return (
    <section
      data-testid="late-document-note"
      className="border-warn bg-warn-tint rounded-xl border p-3"
    >
      <LateDocumentCard
        item={item}
        engagement={engagement}
        showDocumentLink={false}
      />
    </section>
  );
}

/**
 * Dönem şeridinin altında (#780 C): seçili kapanmış döneme kapanıştan sonra
 * gelen ve karar bekleyen belge sayısı; tek tıkla geçmiş dönem takibine.
 */
export function LatePeriodNote({
  engagement,
  period,
  closed,
}: {
  engagement: string;
  period: string;
  /** Dönem kilitli ya da beyanı verilmiş; değilse sorgu atılmaz. */
  closed: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const q = useLateDocuments(
    engagement,
    { period, status: "pending" },
    closed && can(Permission.TenantPeriodsRead),
  );
  const count = q.data?.items.length ?? 0;
  if (!closed || count === 0) return null;
  const month = periodText(period, locale);
  return (
    <section
      data-testid="late-period-note"
      aria-labelledby="late-period-title"
      className="border-warn bg-warn-tint flex flex-wrap items-center gap-3 rounded-xl border p-3"
    >
      <CalendarClock aria-hidden className="text-warn size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <h2 id="late-period-title" className="text-sm font-semibold">
          {t("office.late_docs.period_title", { month, count })}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.late_docs.period_hint")}
        </p>
      </div>
      <Button asChild variant="outline" size="sm">
        <Link href={routes.tenant.engagementLateDocuments(slug, engagement)}>
          {t("office.late_docs.open_tracking")}
        </Link>
      </Button>
    </section>
  );
}

/** Mükellef sekmesi: Geçmiş dönem takibi (#780 C). */
export function EngagementLateDocumentsPage({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const [status, setStatus] = useState<string>("pending");
  const q = useLateDocuments(engagement, status === "all" ? {} : { status });
  const counts = q.data?.counts;
  const items = q.data?.items ?? [];
  const total = counts
    ? counts.pending +
      counts.correction +
      counts.current_period +
      counts.ignored
    : 0;
  return (
    <div className="space-y-6">
      <Panel
        title={t("office.late_docs.title")}
        description={t("office.late_docs.description")}
      >
        <div
          role="group"
          aria-label={t("office.late_docs.filter")}
          className="mb-4 flex flex-wrap gap-2"
        >
          {[...LATE_STATUSES, "all" as const].map((s) => (
            <FilterChip
              key={s}
              pressed={status === s}
              onPressedChange={() => setStatus(s)}
              data-testid={`late-filter-${s}`}
            >
              {t(`office.late_docs.filter_${s}`)}
              {counts ? ` (${s === "all" ? total : counts[s]})` : ""}
            </FilterChip>
          ))}
        </div>
        {q.isLoading ? (
          <p className="text-text-3 text-sm">{t("common.loading")}</p>
        ) : q.isError ? (
          <p role="alert" className="text-danger text-sm">
            {q.error.message}
          </p>
        ) : items.length === 0 ? (
          <p className="text-text-3 text-sm" data-testid="late-documents-empty">
            {t(
              status === "pending"
                ? "office.late_docs.none_pending"
                : "office.late_docs.none",
            )}
          </p>
        ) : (
          <div className="space-y-3">
            {items.map((it) => (
              <LateDocumentCard
                key={it.uuid}
                item={it}
                engagement={engagement}
              />
            ))}
          </div>
        )}
      </Panel>
      <ChangeRequestDecisions engagement={engagement} />
    </div>
  );
}

/** Ofis geneli geçmiş dönem takibi: mükellef başına bekleyen geç belge. */
export function OfficeLateDocumentsPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const q = useLateDocumentSummary();
  const data = q.data;
  return (
    <div className="space-y-5">
      <PageHeader
        title={t("office.late_docs.office_title")}
        description={t("office.late_docs.office_description")}
      />
      {q.isLoading ? (
        <p className="text-text-3 text-sm">{t("common.loading")}</p>
      ) : q.isError ? (
        <p role="alert" className="text-danger text-sm">
          {q.error.message}
        </p>
      ) : (
        <Panel
          title={t("office.late_docs.office_count", {
            count: data?.pending ?? 0,
          })}
        >
          {!data || data.items.length === 0 ? (
            <p className="text-text-3 text-sm">
              {t("office.late_docs.office_none")}
            </p>
          ) : (
            <Table data-testid="late-documents-summary">
              <TableCaption className="sr-only">
                {t("office.late_docs.office_title")}
              </TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead scope="col">
                    {t("office.late_docs.col_taxpayer")}
                  </TableHead>
                  <TableHead scope="col" className="text-right">
                    {t("office.late_docs.col_pending")}
                  </TableHead>
                  <TableHead scope="col" className="text-right">
                    {t("office.late_docs.col_total")}
                  </TableHead>
                  <TableHead scope="col">
                    {t("office.late_docs.col_oldest")}
                  </TableHead>
                  <TableHead scope="col">
                    <span className="sr-only">
                      {t("office.late_docs.col_action")}
                    </span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((r) => (
                  <TableRow key={r.engagement_uuid}>
                    <TableCell className="font-medium">
                      {r.taxpayer_title}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {r.pending}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {r.total}
                    </TableCell>
                    <TableCell>
                      {r.oldest_pending_period
                        ? periodText(r.oldest_pending_period, locale)
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button asChild size="sm" variant="outline">
                        <Link
                          href={routes.tenant.engagementLateDocuments(
                            slug,
                            r.engagement_uuid,
                          )}
                        >
                          {r.pending > 0
                            ? t("office.late_docs.action_decide")
                            : t("office.late_docs.action_view")}
                        </Link>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Panel>
      )}
    </div>
  );
}

/**
 * Ofis beyannameler ekranında (#780 C): kapanmış dönemlere sonradan gelen ve
 * karar bekleyen belge varsa sayısı ve geçmiş dönem takibine bağlantı.
 */
export function LateDocumentsOfficeBanner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const q = useLateDocumentSummary(can(Permission.TenantPeriodsRead));
  const pending = q.data?.pending ?? 0;
  if (pending === 0) return null;
  return (
    <section
      data-testid="late-documents-banner"
      aria-labelledby="late-documents-banner-title"
      className="border-warn bg-warn-tint flex flex-wrap items-center gap-3 rounded-xl border p-3"
    >
      <CalendarClock aria-hidden className="text-warn size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <h2 id="late-documents-banner-title" className="text-sm font-semibold">
          {t("office.late_docs.office_count", { count: pending })}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.late_docs.period_hint")}
        </p>
      </div>
      <Button asChild variant="outline" size="sm">
        <Link href={routes.tenant.lateDocuments(slug)}>
          {t("office.late_docs.open_tracking")}
        </Link>
      </Button>
    </section>
  );
}
