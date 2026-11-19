"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowRight } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";
import { useMemo } from "react";

import { DonemTaci, TacRozeti } from "@/components/hakan";
import type { TacDurumu } from "@/components/hakan";
import { DataTable, createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useOfficeDashboard,
  useOfficeVATForecast,
  usePeriodMatrixAt,
  useReviewPeriodTotals,
} from "../hooks/use-office-data";
import { cellAt, sortByProgress } from "../lib/command";
import {
  daysUntil,
  monthName,
  useOfficePeriod,
  usePeriodText,
  vatDueDate,
} from "../period";
import type { OfficeDashboardUpload } from "../services/dashboard.service";
import type {
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";
import { OfflineAgentsCard } from "./agents-page";
import { Figure, Money, Panel } from "./office-ui";

const LIST_LIMIT = 8;

function Kpi({
  label,
  value,
  sub,
  href,
  danger,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  href?: string;
  danger?: boolean;
}) {
  const body = (
    <>
      <span className="text-text-2 flex items-center justify-between text-sm">
        {label}
        {href ? (
          <ArrowRight
            className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100"
            aria-hidden
          />
        ) : null}
      </span>
      <Figure
        className={`font-display text-3xl font-bold ${danger ? "text-danger" : ""}`}
      >
        {value}
      </Figure>
      {sub ? <span className="text-text-2 text-xs">{sub}</span> : null}
    </>
  );
  const cls =
    "bg-card group flex flex-col gap-1 rounded-[14px] border p-4 transition-colors";
  return href ? (
    <Link href={href} className={`${cls} hover:border-line-strong`}>
      {body}
    </Link>
  ) : (
    <div className={cls}>{body}</div>
  );
}

const dash = <span className="text-muted-foreground">—</span>;

/** Mükellefin dönem hücresi → taç durumu (README "Üç taraf, bir taç"). */
function cellCrown(c: PeriodMatrixCell | undefined): TacDurumu {
  if (!c) return "bekleniyor";
  if (c.open_blockers > 0) return "sorun";
  if (c.locked) return "aktarildi";
  if (c.ready) return "onaylandi";
  return "yuklendi";
}

function greetingKey(now = new Date()) {
  const h = Number(
    new Intl.DateTimeFormat("en-GB", {
      timeZone: "Europe/Istanbul",
      hour: "2-digit",
      hourCycle: "h23",
    }).format(now),
  );
  if (h >= 5 && h < 12) return "office.home.greeting_morning";
  if (h >= 12 && h < 18) return "office.home.greeting_day";
  return "office.home.greeting_evening";
}

const feedKey: Record<OfficeDashboardUpload["kind"], string> = {
  receipt: "office.home.feed_receipt",
  invoice: "office.home.feed_invoice",
  document: "office.home.feed_document",
};

const cellKey: Record<TacDurumu, string> = {
  bekleniyor: "office.home.cell_bekleniyor",
  yuklendi: "office.home.cell_yuklendi",
  onaylandi: "office.home.cell_onaylandi",
  aktarildi: "office.home.cell_aktarildi",
  sorun: "office.home.cell_sorun",
};

/**
 * "İşi geride olan mükellefler" kolonları (#381). Sıra ilerlemeye göre
 * (sortByProgress) anlamlı olduğundan tablo sıralaması kapalı.
 */
function useBehindColumns(focus: string) {
  const { t } = useLocale();
  // Mükellef adı mükellef detayına (#469).
  const { slug } = useTenant();
  return useMemo<ColumnDef<PeriodMatrixRow, unknown>[]>(
    () => [
      createColumn<PeriodMatrixRow>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <>
            <Link
              href={routes.tenant.engagement(
                slug,
                row.original.engagement_uuid,
              )}
              className="block font-medium hover:underline"
            >
              {row.original.taxpayer_title}
            </Link>
            <Figure className="text-text-2 text-xs">
              {row.original.taxpayer_tax_id}
            </Figure>
          </>
        ),
      }),
      createColumn<PeriodMatrixRow>({
        id: "status",
        labelKey: "office.col_status",
        accessorFn: (r) => cellCrown(cellAt(r, focus)),
        enableHiding: false,
        cell: ({ row }) => {
          const crown = cellCrown(cellAt(row.original, focus));
          return (
            <TacRozeti
              durum={crown}
              etiket
              metin={t(cellKey[crown])}
              className="text-xs"
            />
          );
        },
      }),
      // Engelleyici ve değişiklik talebi masaüstünde; mobil kartta yok (önceki
      // `hidden md:table-cell` davranışı).
      createColumn<PeriodMatrixRow>({
        id: "blockers",
        labelKey: "office.col_blockers",
        accessorFn: (r) => cellAt(r, focus)?.open_blockers ?? 0,
        enableHiding: false,
        meta: { align: "end", cardHidden: true },
        cell: ({ getValue }) => <Figure>{getValue() as number}</Figure>,
      }),
      // Açık uyarıların tamamı (engelleyenler dâhil, #529).
      createColumn<PeriodMatrixRow>({
        id: "open_warnings",
        labelKey: "office.col_open_warnings",
        accessorFn: (r) => cellAt(r, focus)?.open_warnings ?? 0,
        meta: { align: "end", cardHidden: true },
        cell: ({ getValue }) => <Figure>{getValue() as number}</Figure>,
      }),
      createColumn<PeriodMatrixRow>({
        id: "change_requests",
        labelKey: "office.col_change_requests",
        accessorFn: (r) => cellAt(r, focus)?.open_change_requests ?? 0,
        enableHiding: false,
        meta: { align: "end", cardHidden: true },
        cell: ({ getValue }) => <Figure>{getValue() as number}</Figure>,
      }),
    ],
    [focus, slug, t],
  );
}

function FeedItem({ u }: { u: OfficeDashboardUpload }) {
  const { t, locale } = useLocale();
  const failed = /fail|error|rejected/i.test(u.status);
  const time = new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    timeZone: "Europe/Istanbul",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(u.created_at));
  return (
    <li className="flex items-start gap-3">
      <TacRozeti durum={failed ? "sorun" : "yuklendi"} className="mt-1" />
      <div className="min-w-0">
        <div className="truncate text-sm">
          {t(feedKey[u.kind], { name: u.taxpayer_title })}
        </div>
        <time className="text-text-2 text-xs" dateTime={u.created_at}>
          {time}
        </time>
      </div>
    </li>
  );
}

/**
 * Genel bakış (#326, OfisPano): selamlama ve KDV son günü, dönem tacı, KPI'lar,
 * işi geride olan mükellefler, KDV tahmini ve "Bugün olanlar". Dönem üst
 * çubuktaki seçiciden; "Tüm dönemler"de beyan dönemi (önceki ay).
 */
export function OfficeHome() {
  const { t, locale } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { user } = useAuth();
  const { focus } = useOfficePeriod();
  const matrix = usePeriodMatrixAt(focus);
  const dashboard = useOfficeDashboard();
  const totals = useReviewPeriodTotals(focus);
  const vat = useOfficeVATForecast(focus);

  const rows = useMemo(() => matrix.data?.items ?? [], [matrix.data]);
  const cells = rows.map((r) => cellAt(r, focus));
  const ready = cells.filter((c) => c && (c.ready || c.locked)).length;
  const blocked = cells.filter((c) => c && c.open_blockers > 0).length;
  const behind = useMemo(
    () => sortByProgress(rows, focus).slice(0, LIST_LIMIT),
    [rows, focus],
  );
  const behindColumns = useBehindColumns(focus);
  const vatSum = useMemo(() => {
    const items = vat.data?.items ?? [];
    let payable = 0;
    let carry = 0;
    for (const f of items) {
      payable += Number(f.totals.payable) || 0;
      carry += Number(f.totals.carry_out) || 0;
    }
    return { payable, carry, count: items.length };
  }, [vat.data]);

  const d = dashboard.data;
  const approval =
    totals.data && totals.data.all > 0
      ? (totals.data.all - totals.data.pending) / totals.data.all
      : null;
  const tasks = d?.doc_tasks;
  const requested = tasks ? tasks.open + tasks.submitted : 0;
  const data = tasks && requested > 0 ? tasks.submitted / requested : null;

  const due = vatDueDate(focus);
  const left = daysUntil(due);
  const dueLabel = new Intl.DateTimeFormat(
    locale === "tr" ? "tr-TR" : "en-GB",
    {
      day: "numeric",
      month: "long",
      timeZone: "UTC",
    },
  ).format(new Date(`${due}T00:00:00Z`));
  const today = new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    timeZone: "Europe/Istanbul",
  }).format(new Date());
  const firstName = user?.name?.trim().split(/\s+/)[0] ?? "";
  const month = monthName(focus, locale);

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-text-2 text-sm capitalize" data-visual-mask>
            {today}
          </p>
          <h1
            className="font-display text-2xl font-semibold tracking-tight sm:text-3xl"
            data-visual-mask
          >
            {t(greetingKey(), { name: firstName })}{" "}
            {left >= 0
              ? t("office.home.days_to_declaration", {
                  month,
                  count: left,
                })
              : t("office.home.declaration_passed", { month })}
          </h1>
        </div>
        <p className="text-text-2 text-sm" data-visual-mask>
          {t("office.home.vat_due")}{" "}
          <b className="text-foreground">{dueLabel}</b>
        </p>
      </div>

      <OfflineAgentsCard />

      {matrix.isLoading ? (
        <Skeleton className="h-64 w-full rounded-[14px]" />
      ) : (
        <DonemTaci
          baslik={t("office.home.crown_title", { period: periodText(focus) })}
          hazir={ready}
          toplam={rows.length}
          veri={data}
          onay={approval}
          etiketler={{
            hazirSatiri: t("office.home.crown_ready_line", {
              total: rows.length,
            }),
            veri: t("office.home.crown_data"),
            hazir: t("office.home.crown_ready"),
            onay: t("office.home.crown_approval"),
          }}
          ozet={[
            approval !== null
              ? t("office.home.crown_summary_approval", {
                  pct: Math.round(approval * 100),
                })
              : undefined,
            rows.length - ready > 0
              ? t("office.home.crown_summary_left", {
                  left: rows.length - ready,
                  blocked,
                })
              : undefined,
          ]
            .filter(Boolean)
            .join(" ")}
          eylemler={
            <>
              <Button
                asChild
                className="bg-brand text-on-brand hover:bg-brand/90 h-11"
              >
                <Link href={routes.tenant.review(slug)}>
                  {t("office.home.start_queue", {
                    count: d?.review?.pending ?? 0,
                  })}
                </Link>
              </Button>
              <Button asChild variant="outline" className="h-11">
                <Link href={routes.tenant.warnings(slug)}>
                  {t("office.home.see_blockers")}
                </Link>
              </Button>
            </>
          }
        />
      )}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Kpi
          label={t("office.stat_review")}
          value={d?.review ? d.review.pending : dash}
          href={routes.tenant.review(slug)}
        />
        <Kpi
          label={t("office.stat_blockers")}
          value={d?.warnings ? d.warnings.blocker : dash}
          sub={
            d?.warnings
              ? t("office.home.kpi_warnings_sub", {
                  warning: d.warnings.warning,
                  info: d.warnings.info,
                })
              : undefined
          }
          href={routes.tenant.warnings(slug)}
          danger={!!d?.warnings?.blocker}
        />
        <Kpi
          label={t("office.home.kpi_tasks")}
          value={d?.doc_tasks ? d.doc_tasks.open : dash}
          sub={
            d?.doc_tasks?.overdue
              ? t("office.home.kpi_tasks_overdue", {
                  count: d.doc_tasks.overdue,
                })
              : undefined
          }
          href={routes.tenant.taxpayers(slug)}
        />
        <Kpi
          label={t("office.home.kpi_today")}
          value={d ? d.uploads.total : dash}
          sub={
            d
              ? t("office.home.kpi_today_sub", {
                  invoices: d.uploads.invoices,
                  receipts: d.uploads.receipts,
                })
              : undefined
          }
          href={routes.tenant.upload(slug)}
        />
      </div>

      <div className="grid grid-cols-[minmax(0,1fr)] gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <Panel
          title={t("office.home_behind_title", { period: periodText(focus) })}
          description={t("office.home_behind_description")}
          actions={
            <Link
              href={routes.tenant.taxpayers(slug)}
              className="text-brand-text inline-flex min-h-6 items-center justify-center text-sm font-semibold hover:underline max-md:min-h-11 max-md:min-w-11"
            >
              {t("office.home.all")}
            </Link>
          }
        >
          {matrix.isLoading ? (
            <div className="space-y-2 p-4">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          ) : matrix.isError ? (
            <p className="text-danger p-4 text-sm">{t("office.load_failed")}</p>
          ) : behind.length === 0 ? (
            <p className="text-text-2 p-6 text-center text-sm">
              {t("office.taxpayers_empty")}
            </p>
          ) : (
            <DataTable<PeriodMatrixRow>
              columns={behindColumns}
              data={behind}
              getRowId={(r) => r.engagement_uuid}
              bordered={false}
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
              }}
            />
          )}
        </Panel>

        <div className="flex flex-col gap-4">
          <section className="bg-card rounded-[14px] border p-4">
            <h2 className="mb-3 text-base font-semibold">
              {t("office.home_vat_title", { period: periodText(focus) })}
            </h2>
            {vat.isLoading ? (
              <Skeleton className="h-16 w-full" />
            ) : vat.isError ? (
              <p className="text-text-2 text-sm">
                {t("office.home_vat_unavailable")}
              </p>
            ) : vatSum.count === 0 ? (
              <p className="text-text-2 text-sm">
                {t("office.home_vat_empty")}
              </p>
            ) : (
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between gap-3">
                  <dt className="text-text-2">{t("office.col_vat_payable")}</dt>
                  <dd className="font-semibold">
                    <Money value={String(vatSum.payable)} />
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-text-2">{t("office.col_vat_carry")}</dt>
                  <dd className="font-semibold">
                    <Money value={String(vatSum.carry)} />
                  </dd>
                </div>
              </dl>
            )}
            {vatSum.count > 0 && !vat.isLoading && !vat.isError ? (
              <p className="text-text-2 mt-2 border-t pt-2 text-xs">
                {t("office.home.vat_basis", { count: vatSum.count })}
              </p>
            ) : null}
          </section>

          <section className="bg-card rounded-[14px] border p-4">
            <h2 className="mb-3 text-base font-semibold">
              {t("office.home.feed_title")}
            </h2>
            {dashboard.isLoading ? (
              <Skeleton className="h-24 w-full" />
            ) : d?.uploads.recent.length ? (
              <ul className="space-y-3">
                {d.uploads.recent.slice(0, 6).map((u) => (
                  <FeedItem key={`${u.kind}-${u.uuid}`} u={u} />
                ))}
              </ul>
            ) : (
              <p className="text-text-2 text-sm">
                {t("office.home.feed_empty")}
              </p>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
