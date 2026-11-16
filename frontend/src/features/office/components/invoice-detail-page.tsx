"use client";

import { ArrowLeft, FileText } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useMemo } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useEngagementInvoice,
  useEngagementOptions,
  useInvoiceUBL,
} from "../hooks/use-office-invoices";
import { formatAmount } from "../lib/decimal";
import { toPanelInvoice } from "../lib/from-api";
import { AmountWaterfall } from "./amount-waterfall";
import { ENGAGEMENT_PARAM } from "./engagement-picker";
import { InvoicePreview } from "./invoice-preview";
import { InvoiceStateBadge } from "./invoices-page";
import {
  Figure,
  Money,
  Panel,
  SeverityBadge,
  TableBox,
  WarningStatusLabel,
  useWarningText,
} from "./office-ui";

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate text-sm">{children}</dd>
    </div>
  );
}

function InvoiceDetailInner({ id }: { id: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const engagement = useSearchParams().get(ENGAGEMENT_PARAM) ?? "";
  const detail = useEngagementInvoice(slug, engagement, id);
  const ubl = useInvoiceUBL(slug, engagement, id, !!detail.data?.has_ubl);
  const { options } = useEngagementOptions(slug);
  const taxpayer = options.find((o) => o.engagement === engagement);
  const warningText = useWarningText();
  const backHref = `${routes.tenant.invoices(slug)}${engagement ? `?${ENGAGEMENT_PARAM}=${encodeURIComponent(engagement)}` : ""}`;

  const invoice = useMemo(
    () =>
      detail.data ? toPanelInvoice(detail.data, engagement, ubl.data) : null,
    [detail.data, engagement, ubl.data],
  );

  if (detail.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (!engagement || !detail.data || !invoice) {
    return (
      <EmptyState
        title={t("office.invoice_not_found")}
        action={
          <Button asChild variant="outline">
            <Link href={backHref}>{t("office.back_to_invoices")}</Link>
          </Button>
        }
      />
    );
  }
  const d = detail.data;

  return (
    <div className="space-y-4">
      <PageHeader
        title={d.invoice_number}
        description={`${d.counterparty_name ?? ""} · ${taxpayer?.title ?? ""}`}
        icon={<FileText className="size-6" />}
        breadcrumbs={[
          { label: t("office.nav_invoices"), href: backHref },
          { label: d.invoice_number },
        ]}
        actions={
          <div className="flex items-center gap-2">
            <InvoiceStateBadge status={d.status} />
            <Button asChild size="sm" variant="ghost">
              <Link href={backHref}>
                <ArrowLeft className="size-4" />
                {t("office.back_to_invoices")}
              </Link>
            </Button>
          </div>
        }
      />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-4">
          <Panel title={t("office.invoice_info")}>
            <dl className="grid grid-cols-2 gap-x-4 gap-y-3 p-4 sm:grid-cols-3">
              <Field label="ETTN">
                <Figure className="text-xs">{d.ettn ?? "—"}</Figure>
              </Field>
              <Field label={t("office.col_date")}>
                <Figure>{d.issue_date}</Figure>
              </Field>
              <Field label={t("office.col_direction")}>
                {t(`office.direction_${invoice.direction}`)}
              </Field>
              <Field label={t("office.col_type")}>
                {d.profile ? `${d.profile} · ` : ""}
                {d.type}
              </Field>
              <Field label={t("office.col_counterparty")}>
                {d.counterparty_name ?? "—"}
              </Field>
              <Field
                label={
                  (d.counterparty_tax_id ?? "").length === 11 ? "TCKN" : "VKN"
                }
              >
                <Figure>{d.counterparty_tax_id || "—"}</Figure>
              </Field>
              <Field label={t("office.col_currency")}>
                <Figure>{d.currency}</Figure>
              </Field>
              <Field label={t("office.col_source")}>
                {d.sources.length
                  ? d.sources.map((s) => s.source_system).join(", ")
                  : d.source_system}
              </Field>
              <Field label={t("office.api.col_period")}>
                <Figure>{d.period}</Figure>
              </Field>
            </dl>
          </Panel>

          <Panel
            title={t("office.warnings_on_invoice")}
            description={
              d.warnings.length ? undefined : t("office.no_warnings")
            }
          >
            {d.warnings.length ? (
              <ul className="divide-y">
                {d.warnings.map((w) => (
                  <li
                    key={w.uuid}
                    className="flex flex-wrap items-center gap-3 px-4 py-2.5"
                  >
                    <SeverityBadge severity={w.severity} />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm">
                        {warningText({
                          code: w.code,
                          params: Object.fromEntries(
                            Object.entries(w.params ?? {}).map(([k, v]) => [
                              k,
                              String(v),
                            ]),
                          ),
                        })}
                      </div>
                      <div className="flex gap-2">
                        <Figure className="text-muted-foreground text-xs">
                          {w.code}
                        </Figure>
                        <WarningStatusLabel status={w.status} />
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            ) : null}
          </Panel>

          <Panel title={t("office.lines")}>
            <TableBox>
              <thead>
                <tr>
                  <th>{t("office.col_item")}</th>
                  <th className="text-right">{t("office.col_quantity")}</th>
                  <th className="text-right">{t("office.col_line_total")}</th>
                  <th className="text-right">KDV</th>
                </tr>
              </thead>
              <tbody>
                {invoice.lines.map((l) => (
                  <tr key={l.id}>
                    <td>{l.name}</td>
                    <td className="text-right whitespace-nowrap">
                      <Figure>
                        {formatAmount(l.quantity)} {l.unit}
                      </Figure>
                    </td>
                    <td className="text-right">
                      <Money value={l.lineTotal} currency={d.currency} />
                    </td>
                    <td className="text-right">
                      <Figure>%{l.vatPercent}</Figure>
                    </td>
                  </tr>
                ))}
              </tbody>
            </TableBox>
          </Panel>

          <Panel
            title={t("office.waterfall.title")}
            description={t("office.waterfall.description")}
          >
            <AmountWaterfall invoice={invoice} />
          </Panel>

          <Panel title={t("office.totals")}>
            <dl className="space-y-1.5 p-4 text-sm">
              {invoice.taxes.map((tx, i) => (
                <div key={i} className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">
                    {t(`office.tax_code.${tx.code}`) ===
                    `office.tax_code.${tx.code}`
                      ? tx.code
                      : t(`office.tax_code.${tx.code}`)}{" "}
                    <Figure>%{tx.percent}</Figure>
                  </dt>
                  <dd>
                    <Money value={tx.amount} currency={d.currency} />
                  </dd>
                </div>
              ))}
              <div className="flex justify-between gap-4 border-t pt-2">
                <dt className="text-muted-foreground">
                  {t("office.col_base")}
                </dt>
                <dd>
                  <Money value={d.tax_exclusive_amount} currency={d.currency} />
                </dd>
              </div>
              <div className="flex justify-between gap-4 font-semibold">
                <dt>{t("office.col_payable")}</dt>
                <dd>
                  <Money value={d.payable_amount} currency={d.currency} />
                </dd>
              </div>
            </dl>
          </Panel>
        </div>

        <Panel
          title={t("office.preview_title")}
          className="min-w-0 xl:sticky xl:top-4 xl:self-start"
        >
          {d.has_ubl && ubl.isLoading ? (
            <Skeleton className="m-4 h-96" />
          ) : (
            <InvoicePreview invoice={invoice} />
          )}
        </Panel>
      </div>
    </div>
  );
}

export function InvoiceDetailPage({ id }: { id: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <InvoiceDetailInner id={id} />
    </Suspense>
  );
}
