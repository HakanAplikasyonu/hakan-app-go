"use client";

import { FileText, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { sum } from "../lib/decimal";
import { useOffice } from "../hooks/use-office";
import type { Invoice } from "../types";
import {
  DemoNotice,
  FilterSelect,
  Figure,
  InvoiceStatusBadge,
  Money,
  OfficeDataGate,
  Panel,
  TableBox,
} from "./office-ui";

type Dir = "all" | Invoice["direction"];
type St = "all" | Invoice["status"];

export function vatOf(inv: Invoice) {
  return sum(inv.taxes.filter((t) => t.code === "0015").map((t) => t.amount));
}

function InvoicesInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const taxpayers = useOffice((s) => s.taxpayers);
  const invoices = useOffice((s) => s.invoices);
  const warnings = useOffice((s) => s.warnings);

  const taxpayer = params.get("taxpayer") ?? "all";
  const direction = (params.get("direction") ?? "all") as Dir;
  const status = (params.get("status") ?? "all") as St;
  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value === "all") next.delete(key);
    else next.set(key, value);
    router.replace(`${pathname}?${next.toString()}`);
  };

  const rows = invoices
    .filter((i) => taxpayer === "all" || i.taxpayerId === taxpayer)
    .filter((i) => direction === "all" || i.direction === direction)
    .filter((i) => status === "all" || i.status === status)
    .sort(
      (a, b) =>
        b.issueDate.localeCompare(a.issueDate) ||
        b.number.localeCompare(a.number),
    );

  const tpName = (id: string) =>
    taxpayers.find((x) => x.id === id)?.title ?? "";
  const openWarnings = (id: string) =>
    warnings.filter((w) => w.entityId === id && w.status === "open");
  const fmtDate = (d: string) =>
    new Date(`${d}T00:00:00`).toLocaleDateString(
      locale === "tr" ? "tr-TR" : "en-GB",
    );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.invoices_title")}
        description={t("office.invoices_description")}
        icon={<FileText className="size-6" />}
      />
      <DemoNotice />
      <div className="flex flex-wrap gap-3">
        <FilterSelect
          label={t("office.col_taxpayer")}
          value={taxpayer}
          onChange={(v) => setParam("taxpayer", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...taxpayers.map((x) => ({ value: x.id, label: x.title })),
          ]}
        />
        <FilterSelect<Dir>
          label={t("office.col_direction")}
          value={direction}
          onChange={(v) => setParam("direction", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            { value: "sales", label: t("office.direction_sales") },
            { value: "purchase", label: t("office.direction_purchase") },
          ]}
        />
        <FilterSelect<St>
          label={t("office.col_status")}
          value={status}
          onChange={(v) => setParam("status", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            { value: "in_review", label: t("office.invoice_status_in_review") },
            { value: "approved", label: t("office.invoice_status_approved") },
          ]}
        />
      </div>
      <Panel>
        {rows.length === 0 ? (
          <EmptyState className="m-4" title={t("office.invoices_empty")} />
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_date")}</th>
                <th>{t("office.col_number")}</th>
                <th>{t("office.col_counterparty")}</th>
                <th>{t("office.col_type")}</th>
                <th className="text-right">{t("office.col_base")}</th>
                <th className="text-right">{t("office.col_vat")}</th>
                <th className="text-right">{t("office.col_payable")}</th>
                <th>{t("office.col_status")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((inv) => {
                const ws = openWarnings(inv.id);
                const blocker = ws.some((w) => w.severity === "blocker");
                return (
                  <tr key={inv.id} className="hover:bg-muted/40">
                    <td className="whitespace-nowrap">
                      <Figure>{fmtDate(inv.issueDate)}</Figure>
                    </td>
                    <td>
                      <div className="flex items-center gap-1.5">
                        <Link
                          href={routes.tenant.invoice(slug, inv.id)}
                          className="hover:underline"
                        >
                          <Figure className="font-medium">{inv.number}</Figure>
                        </Link>
                        {ws.length ? (
                          <span
                            className={`inline-flex items-center gap-0.5 text-xs ${blocker ? "text-status-blocking" : "text-status-warning"}`}
                            title={t("office.open_warnings", {
                              count: ws.length,
                            })}
                          >
                            <TriangleAlert className="size-3.5" aria-hidden />
                            <Figure>{ws.length}</Figure>
                          </span>
                        ) : null}
                      </div>
                      {taxpayer === "all" ? (
                        <div className="text-muted-foreground text-xs">
                          {tpName(inv.taxpayerId)}
                        </div>
                      ) : null}
                    </td>
                    <td>
                      <div className="max-w-64 truncate">
                        {inv.counterparty.title}
                      </div>
                      <Figure className="text-muted-foreground text-xs">
                        {inv.counterparty.taxId}
                      </Figure>
                    </td>
                    <td className="text-xs">
                      <span
                        className={
                          inv.direction === "sales"
                            ? "text-status-approved"
                            : "text-brand"
                        }
                      >
                        {t(`office.direction_${inv.direction}`)}
                      </span>
                      <div className="text-muted-foreground">
                        {inv.profile} · {inv.typeCode}
                      </div>
                    </td>
                    <td className="text-right">
                      <Money
                        value={inv.totals.taxExclusive}
                        currency={inv.currency}
                      />
                    </td>
                    <td className="text-right">
                      <Money value={vatOf(inv)} currency={inv.currency} />
                    </td>
                    <td className="text-right font-medium">
                      <Money
                        value={inv.totals.payable}
                        currency={inv.currency}
                      />
                    </td>
                    <td>
                      <InvoiceStatusBadge status={inv.status} />
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

export function InvoicesPage() {
  return (
    <OfficeDataGate>
      <Suspense>
        <InvoicesInner />
      </Suspense>
    </OfficeDataGate>
  );
}
