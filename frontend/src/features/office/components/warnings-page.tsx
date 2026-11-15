"use client";

import { TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { useOffice } from "../hooks/use-office";
import type { Severity, WarningStatus } from "../types";
import {
  DemoNotice,
  FilterSelect,
  Figure,
  OfficeDataGate,
  Panel,
  SeverityBadge,
  TableBox,
  WarningStatusLabel,
  useWarningText,
} from "./office-ui";
import { WarningActions } from "./warning-actions";

const severityOrder: Record<Severity, number> = {
  blocker: 0,
  warning: 1,
  info: 2,
};

function WarningsInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const taxpayers = useOffice((s) => s.taxpayers);
  const invoices = useOffice((s) => s.invoices);
  const warnings = useOffice((s) => s.warnings);
  const warningText = useWarningText();
  const [taxpayer, setTaxpayer] = useState("all");
  const [severity, setSeverity] = useState<"all" | Severity>("all");
  const [status, setStatus] = useState<"all" | WarningStatus>("open");

  const rows = warnings
    .filter((w) => taxpayer === "all" || w.taxpayerId === taxpayer)
    .filter((w) => severity === "all" || w.severity === severity)
    .filter((w) => status === "all" || w.status === status)
    .sort(
      (a, b) =>
        severityOrder[a.severity] - severityOrder[b.severity] ||
        b.createdAt.localeCompare(a.createdAt),
    );

  const counts = (s: Severity) =>
    warnings.filter((w) => w.status === "open" && w.severity === s).length;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.warnings_title")}
        description={t("office.warnings_description")}
        icon={<TriangleAlert className="size-6" />}
      />
      <DemoNotice />
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label={t("office.col_taxpayer")}
          value={taxpayer}
          onChange={setTaxpayer}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...taxpayers.map((x) => ({ value: x.id, label: x.title })),
          ]}
        />
        <FilterSelect<"all" | Severity>
          label={t("office.col_severity")}
          value={severity}
          onChange={setSeverity}
          options={[
            { value: "all", label: t("office.filter_all") },
            {
              value: "blocker",
              label: `${t("office.severity_blocker")} (${counts("blocker")})`,
            },
            {
              value: "warning",
              label: `${t("office.severity_warning")} (${counts("warning")})`,
            },
            {
              value: "info",
              label: `${t("office.severity_info")} (${counts("info")})`,
            },
          ]}
        />
        <FilterSelect<"all" | WarningStatus>
          label={t("office.col_status")}
          value={status}
          onChange={setStatus}
          options={[
            { value: "open", label: t("office.warning_status_open") },
            {
              value: "acknowledged",
              label: t("office.warning_status_acknowledged"),
            },
            { value: "resolved", label: t("office.warning_status_resolved") },
            { value: "all", label: t("office.filter_all") },
          ]}
        />
      </div>
      <Panel>
        {rows.length === 0 ? (
          <EmptyState className="m-4" title={t("office.warnings_empty")} />
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_severity")}</th>
                <th>{t("office.col_warning")}</th>
                <th>{t("office.col_document")}</th>
                <th>{t("office.col_status")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((w) => {
                const inv = invoices.find((i) => i.id === w.entityId);
                const tp = taxpayers.find((x) => x.id === w.taxpayerId);
                return (
                  <tr key={w.id}>
                    <td>
                      <SeverityBadge severity={w.severity} />
                    </td>
                    <td className="min-w-72">
                      <div>{warningText(w)}</div>
                      <Figure className="text-muted-foreground text-xs">
                        {w.code} · {w.producer} · {w.period}
                      </Figure>
                    </td>
                    <td>
                      {inv ? (
                        <Link
                          href={routes.tenant.invoice(slug, inv.id)}
                          className="hover:underline"
                        >
                          <Figure>{inv.number}</Figure>
                        </Link>
                      ) : (
                        "—"
                      )}
                      <div className="text-muted-foreground text-xs">
                        {tp?.title}
                      </div>
                    </td>
                    <td>
                      <WarningStatusLabel status={w.status} />
                    </td>
                    <td className="text-right">
                      <WarningActions warning={w} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </TableBox>
        )}
      </Panel>
    </div>
  );
}

export function WarningsPage() {
  return (
    <OfficeDataGate>
      <WarningsInner />
    </OfficeDataGate>
  );
}
