"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { Skeleton } from "@/components/ui/skeleton";
import { EngagementNextWork } from "@/features/copilot";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useEngagementInvoices } from "../hooks/use-office-invoices";
import { useWarningGroups } from "../hooks/use-warnings";
import { useOfficePeriod, usePeriodText } from "../period";
import { ENGAGEMENT_MONTHS, periodCrown } from "./engagement-layout";

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
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  // Başlık, sekmeler ve dönem şeridi düzende (#468); aynı sorgu önbellekten.
  const matrix = usePeriodMatrix(ENGAGEMENT_MONTHS);
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
            {t("office.engagement.readiness_title", {
              period: periodText(period),
            })}
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
              href={routes.tenant.engagementInvoices(slug, engagement)}
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
