"use client";

import { ArrowRight, Building2 } from "lucide-react";
import Link from "next/link";

import { PageHeader } from "@/components/layout/page-header";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { DEMO_PERIOD } from "../data/demo";
import { useOffice } from "../hooks/use-office";
import {
  DemoNotice,
  Figure,
  OfficeDataGate,
  Panel,
  TableBox,
} from "./office-ui";

function Stat({
  label,
  value,
  href,
  tone,
}: {
  label: string;
  value: number;
  href: string;
  tone?: string;
}) {
  return (
    <Link
      href={href}
      className="bg-card hover:border-foreground/20 group flex flex-col gap-1 rounded-xl border p-4 transition-colors"
    >
      <span className="text-muted-foreground flex items-center justify-between text-xs">
        {label}
        <ArrowRight className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
      </span>
      <Figure className={`text-2xl font-semibold ${tone ?? ""}`}>
        {value}
      </Figure>
    </Link>
  );
}

/**
 * Ofis ana ekranı: mükellef × dönem durumu (docs/01 §3 "command center"
 * fikrinin ilk hali).
 */
function OfficeHomeInner() {
  const { t } = useLocale();
  const { slug, organization, role } = useTenant();
  const taxpayers = useOffice((s) => s.taxpayers);
  const invoices = useOffice((s) => s.invoices);
  const warnings = useOffice((s) => s.warnings);
  const suggestions = useOffice((s) => s.suggestions);

  const open = warnings.filter((w) => w.status === "open");
  const pending = suggestions.filter((s) => s.status === "pending");

  return (
    <div className="space-y-4">
      <PageHeader
        title={organization.name}
        description={t("office.home_description", {
          role: role.name,
          period: DEMO_PERIOD,
        })}
        icon={<Building2 className="size-6" />}
      />
      <DemoNotice />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat
          label={t("office.stat_taxpayers")}
          value={taxpayers.length}
          href={routes.tenant.taxpayers(slug)}
        />
        <Stat
          label={t("office.stat_invoices")}
          value={invoices.length}
          href={routes.tenant.invoices(slug)}
        />
        <Stat
          label={t("office.stat_review")}
          value={pending.length}
          href={routes.tenant.review(slug)}
          tone={pending.length ? "text-amber-700 dark:text-status-review" : ""}
        />
        <Stat
          label={t("office.stat_blockers")}
          value={open.filter((w) => w.severity === "blocker").length}
          href={routes.tenant.warnings(slug)}
          tone={
            open.some((w) => w.severity === "blocker")
              ? "text-status-blocking"
              : ""
          }
        />
      </div>
      <Panel
        title={t("office.matrix_title", { period: DEMO_PERIOD })}
        description={t("office.matrix_description")}
      >
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.col_taxpayer")}</th>
              <th className="text-right">{t("office.col_invoices")}</th>
              <th className="text-right">{t("office.col_review")}</th>
              <th className="text-right">{t("office.col_warnings")}</th>
              <th>{t("office.col_readiness")}</th>
            </tr>
          </thead>
          <tbody>
            {taxpayers.map((tp) => {
              const inv = invoices.filter((i) => i.taxpayerId === tp.id);
              const pend = pending.filter((s) => s.taxpayerId === tp.id).length;
              const ws = open.filter((w) => w.taxpayerId === tp.id);
              const blockers = ws.filter(
                (w) => w.severity === "blocker",
              ).length;
              const ready = blockers === 0 && pend === 0;
              return (
                <tr key={tp.id}>
                  <td>
                    <div className="font-medium">{tp.title}</div>
                    <Figure className="text-muted-foreground text-xs">
                      {tp.taxId}
                    </Figure>
                  </td>
                  <td className="text-right">
                    <Figure>{inv.length}</Figure>
                  </td>
                  <td className="text-right">
                    <Figure>{pend}</Figure>
                  </td>
                  <td className="text-right">
                    <Figure>{ws.length}</Figure>
                    {blockers ? (
                      <span className="text-status-blocking ml-1 text-xs">
                        ({t("office.blockers_count", { count: blockers })})
                      </span>
                    ) : null}
                  </td>
                  <td>
                    <span
                      className={
                        ready
                          ? "bg-status-approved/12 text-status-approved rounded-md px-2 py-0.5 text-xs font-medium"
                          : blockers
                            ? "bg-status-blocking/12 text-status-blocking rounded-md px-2 py-0.5 text-xs font-medium"
                            : "bg-status-review/20 dark:text-status-review rounded-md px-2 py-0.5 text-xs font-medium text-amber-800"
                      }
                    >
                      {ready
                        ? t("office.readiness_ready")
                        : blockers
                          ? t("office.readiness_blocked")
                          : t("office.readiness_review")}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </TableBox>
      </Panel>
    </div>
  );
}

export function OfficeHome() {
  return (
    <OfficeDataGate>
      <OfficeHomeInner />
    </OfficeDataGate>
  );
}
