"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum, TacRozeti, type TacDurumu } from "@/components/hakan";
import { Skeleton } from "@/components/ui/skeleton";
import { EngagementNextWork } from "@/features/copilot";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useEngagementInvoices } from "../hooks/use-office-invoices";
import { useWarningGroups } from "../hooks/use-warnings";
import { useOfficePeriod } from "../period";
import type { PeriodMatrixRow } from "../services/periods.service";

type Cell = PeriodMatrixRow["cells"][number];
const MONTHS = 6;

/** Dönem hücresi → taç ve etiket (veri · onay · aktarım). */
export function periodCrown(cell: Cell): {
  durum: TacDurumu;
  key: string;
} {
  if (cell.locked) return { durum: "aktarildi", key: "locked" };
  if (cell.open_blockers > 0) return { durum: "sorun", key: "blocked" };
  if (cell.ready) return { durum: "onaylandi", key: "ready" };
  if (cell.open_change_requests > 0)
    return { durum: "yuklendi", key: "changes" };
  return { durum: "bekleniyor", key: "open" };
}

function initials(title: string) {
  const parts = title
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toLocaleUpperCase(
    "tr-TR",
  );
}

/** Mükellef ayrıntısının sekmeleri (tuval OfisMukellefler). */
export function EngagementTabs({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const pathname = usePathname();
  const { can } = usePermission();
  const tabs = [
    {
      href: routes.tenant.engagement(slug, engagement),
      label: t("office.engagement.tab_overview"),
    },
    {
      href: routes.tenant.invoicesFor(slug, engagement),
      label: t("office.nav_invoices"),
    },
    {
      href: routes.tenant.engagementBank(slug, engagement),
      label: t("office.engagement.tab_bank"),
    },
    // Maaş verisi hassas: izni olmayan (stajyer) sekmeyi görmez (#382).
    ...(can(Permission.TenantPayrollRead)
      ? [
          {
            href: routes.tenant.engagementPayroll(slug, engagement),
            label: t("office.engagement.tab_payroll"),
          },
        ]
      : []),
    {
      href: routes.tenant.engagementEasytrade(slug, engagement),
      label: t("sources.easytrade.tab"),
    },
    {
      href: routes.tenant.engagementExpenses(slug, engagement),
      label: t("office.engagement.tab_expenses"),
    },
    {
      href: routes.tenant.engagementZReports(slug, engagement),
      label: t("office.engagement.tab_zreports"),
    },
    {
      href: routes.tenant.engagementReconciliation(slug, engagement),
      label: t("office.engagement.tab_reconciliation"),
    },
    {
      href: routes.tenant.engagementDocuments(slug, engagement),
      label: t("office.engagement.tab_documents"),
    },
    // MUHSGK bordrodan beslenir: izni olmayan (stajyer) sekmeyi görmez (#390).
    ...(can(Permission.TenantTaxReturnsRead)
      ? [
          {
            href: routes.tenant.engagementTaxReturns(slug, engagement),
            label: t("office.engagement.tab_tax_returns"),
          },
        ]
      : []),
    {
      href: routes.tenant.engagementPetitions(slug, engagement),
      label: t("office.engagement.tab_petitions"),
    },
    {
      href: routes.tenant.engagementReport(slug, engagement),
      label: t("office.engagement.tab_report"),
    },
  ];
  return (
    <nav
      aria-label={t("office.engagement.tabs")}
      className="border-line flex gap-1 overflow-x-auto border-b"
    >
      {tabs.map((tab) => {
        const href = tab.href.split("?")[0];
        const active = pathname === href;
        return (
          <Link
            key={tab.label}
            href={tab.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "focus-visible:ring-ring relative flex min-h-11 items-center px-3 text-sm whitespace-nowrap focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset",
              "after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full",
              active
                ? "text-foreground after:bg-brand font-semibold"
                : "text-text-2 hover:text-foreground",
            )}
          >
            {tab.label}
          </Link>
        );
      })}
    </nav>
  );
}

/**
 * Dönem şeridi: son altı dönem, her biri bir taç (veri · onay · aktarım).
 * Bir döneme basınca ofisin global dönemi o olur; seçili dönem vurgulu.
 */
function PeriodStrip({
  cells,
  focus,
  onPick,
}: {
  cells: Cell[];
  focus: string;
  onPick: (period: string) => void;
}) {
  const { t, locale } = useLocale();
  const month = (p: string) =>
    new Intl.DateTimeFormat(locale, { month: "short", year: "2-digit" }).format(
      new Date(`${p}-01T00:00:00`),
    );
  return (
    <section
      aria-labelledby="period-strip-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
    >
      <div className="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2
          id="period-strip-title"
          className="font-display text-base font-bold"
        >
          {t("office.engagement.strip_title")}
        </h2>
        <span className="text-text-3 text-xs">
          {t("office.engagement.strip_hint")}
        </span>
      </div>
      <ol className="grid grid-cols-3 gap-2 sm:grid-cols-6">
        {cells.map((cell) => {
          const { durum, key } = periodCrown(cell);
          const on = cell.period === focus;
          const label = t(`office.engagement.state_${key}`);
          return (
            <li key={cell.period}>
              <button
                type="button"
                aria-pressed={on}
                aria-label={`${cell.period}: ${label}${cell.open_blockers ? ` · ${cell.open_blockers}` : ""}`}
                onClick={() => onPick(cell.period)}
                className={cn(
                  "focus-visible:ring-ring flex min-h-11 w-full flex-col items-center gap-2 rounded-xl px-1.5 py-3 transition-colors focus-visible:ring-2 focus-visible:outline-none",
                  on
                    ? "bg-brand-tint outline-brand outline-[1.5px]"
                    : "hover:bg-surface-2",
                )}
              >
                <span className="font-mono text-xs tabular-nums">
                  {month(cell.period)}
                </span>
                <TacRozeti durum={durum} boyut={30} />
                <span
                  className={cn(
                    "text-center text-xs leading-tight",
                    key === "blocked"
                      ? "text-danger font-semibold"
                      : "text-text-2",
                  )}
                >
                  {label}
                  {cell.open_blockers ? ` · ${cell.open_blockers}` : ""}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

function Stat({
  label,
  value,
  tone,
  href,
}: {
  label: string;
  value: ReactNode;
  tone?: "danger" | "ok";
  href?: string;
}) {
  const body = (
    <>
      <span className="text-text-2 text-sm">{label}</span>
      <span
        className={cn(
          "font-mono text-sm font-semibold tabular-nums",
          tone === "danger" && "text-danger",
          tone === "ok" && "text-ok",
        )}
      >
        {value}
      </span>
    </>
  );
  return (
    <li>
      {href ? (
        <Link
          href={href}
          className="hover:bg-surface-2 flex min-h-11 items-center justify-between gap-3 rounded-md px-2 py-1.5"
        >
          {body}
        </Link>
      ) : (
        <div className="flex min-h-11 items-center justify-between gap-3 px-2 py-1.5">
          {body}
        </div>
      )}
    </li>
  );
}

type DocTask = {
  uuid: string;
  kind: string;
  title: string;
  status: string;
  overdue: boolean;
  due_at: string;
};

/**
 * `/t/{slug}/engagements/{uuid}` (arayüz 6/9, #329; tuval OfisMukellefler):
 * başlık, sekmeler, dönem şeridi, seçili dönemin hazırlığı ve açık belge
 * istekleri.
 */
export function EngagementOverviewPage({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { focus, setPeriod } = useOfficePeriod();
  const matrix = usePeriodMatrix(MONTHS);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  // Matris yeniden eskiye gelir; şerit soldan sağa eskiden yeniye.
  const cells = [...(row?.cells ?? [])].sort((a, b) =>
    a.period.localeCompare(b.period),
  );
  const cell = cells.find((c) => c.period === focus) ?? cells.at(-1);
  const period = cell?.period ?? focus;

  const invoices = useEngagementInvoices(slug, engagement, {
    period,
    limit: 1,
    offset: 0,
  });
  const warnings = useWarningGroups(
    slug,
    { period, status: "open" },
    engagement,
  );
  const tasks = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement, "open"],
    queryFn: () =>
      tenantRequest<{ items: DocTask[]; total: number }>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks`,
        { query: { status: "open", limit: 20 } },
      ),
  });
  const openWarnings = (warnings.data?.items ?? []).reduce(
    (n, g) => n + g.count,
    0,
  );

  if (matrix.isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-16 w-80" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  if (matrix.isError || !row) {
    return (
      <ErrorState
        title={t("office.engagement.not_found")}
        description={t("office.engagement.not_found_hint")}
      />
    );
  }

  const taskTitle = (x: DocTask) =>
    x.kind === "undocumented_payment" || x.kind === "uninvoiced_collection"
      ? t(`office.engagement.task_${x.kind}`)
      : x.title || t("office.engagement.task_generic");

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-4">
        <span
          aria-hidden
          className="bg-brand text-on-brand flex size-12 items-center justify-center rounded-xl text-base font-bold"
        >
          {initials(row.taxpayer_title)}
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="font-display truncate text-2xl font-bold">
            {row.taxpayer_title}
          </h1>
          <p className="text-text-2 text-sm">
            {row.taxpayer_tax_id.length === 11 ? "TCKN" : "VKN"}{" "}
            <span className="font-mono">{row.taxpayer_tax_id}</span>
          </p>
        </div>
      </div>

      <EngagementTabs engagement={engagement} />

      <PeriodStrip cells={cells} focus={period} onPick={setPeriod} />

      <EngagementNextWork engagement={engagement} variant="card" />

      <div className="grid gap-5 lg:grid-cols-2">
        <section
          aria-labelledby="readiness-title"
          className="bg-surface border-line rounded-xl border p-4 md:p-5"
        >
          <h2
            id="readiness-title"
            className="font-display mb-2 text-base font-bold"
          >
            {t("office.engagement.readiness_title", { period })}
          </h2>
          {cell ? (
            <p
              className={cn(
                "mb-2 text-sm font-semibold",
                cell.open_blockers ? "text-danger" : "text-text-2",
              )}
            >
              {t(`office.engagement.state_${periodCrown(cell).key}`)}
            </p>
          ) : null}
          <ul className="divide-line divide-y">
            <Stat
              label={t("office.engagement.stat_invoices")}
              value={invoices.isLoading ? "–" : (invoices.data?.total ?? 0)}
              href={routes.tenant.invoicesFor(slug, engagement)}
            />
            <Stat
              label={t("office.engagement.stat_blockers")}
              value={cell?.open_blockers ?? 0}
              tone={cell?.open_blockers ? "danger" : "ok"}
              href={routes.tenant.warnings(slug)}
            />
            <Stat
              label={t("office.engagement.stat_warnings")}
              value={warnings.isLoading ? "–" : openWarnings}
              href={routes.tenant.warnings(slug)}
            />
            <Stat
              label={t("office.engagement.stat_changes")}
              value={cell?.open_change_requests ?? 0}
            />
            <Stat
              label={t("office.engagement.stat_tasks")}
              value={tasks.isLoading ? "–" : (tasks.data?.total ?? 0)}
            />
          </ul>
        </section>

        <section
          aria-labelledby="tasks-title"
          className="bg-surface border-line rounded-xl border p-4 md:p-5"
        >
          <h2
            id="tasks-title"
            className="font-display mb-3 text-base font-bold"
          >
            {t("office.engagement.tasks_title")}
          </h2>
          {tasks.isLoading ? (
            <Skeleton className="h-24 w-full" />
          ) : (tasks.data?.items ?? []).length === 0 ? (
            <BosDurum
              className="border-0 py-4"
              baslik={t("office.engagement.tasks_empty")}
            />
          ) : (
            <ul className="divide-line divide-y">
              {tasks.data!.items.map((x) => (
                <li key={x.uuid} className="flex items-center gap-3 py-2.5">
                  <span
                    className={cn(
                      "rounded-full px-2 py-0.5 text-xs font-semibold whitespace-nowrap",
                      x.overdue
                        ? "bg-danger-tint text-danger"
                        : "bg-warn-tint text-warn",
                    )}
                  >
                    {x.overdue
                      ? t("office.engagement.task_overdue")
                      : t("office.engagement.task_waiting")}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {taskTitle(x)}
                  </span>
                  {x.due_at ? (
                    <time
                      dateTime={x.due_at}
                      className="text-text-3 text-xs whitespace-nowrap"
                    >
                      {new Date(x.due_at).toLocaleDateString(
                        locale === "tr" ? "tr-TR" : "en-GB",
                      )}
                    </time>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}
