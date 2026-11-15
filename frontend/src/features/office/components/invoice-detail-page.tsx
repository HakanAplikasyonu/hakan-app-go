"use client";

import { ArrowLeft, FileText } from "lucide-react";
import Link from "next/link";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { formatAmount } from "../lib/decimal";
import { useOffice } from "../hooks/use-office";
import { AmountWaterfall } from "./amount-waterfall";
import { InvoicePreview } from "./invoice-preview";
import {
  DemoNotice,
  Figure,
  InvoiceStatusBadge,
  Money,
  OfficeDataGate,
  Panel,
  SeverityBadge,
  TableBox,
  WarningStatusLabel,
  useWarningText,
} from "./office-ui";
import { WarningActions } from "./warning-actions";

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
  const invoice = useOffice((s) => s.invoices.find((i) => i.id === id));
  const taxpayer = useOffice((s) =>
    s.taxpayers.find((x) => x.id === invoice?.taxpayerId),
  );
  const allWarnings = useOffice((s) => s.warnings);
  const allSuggestions = useOffice((s) => s.suggestions);
  const warningText = useWarningText();

  if (!invoice) {
    return (
      <EmptyState
        title={t("office.invoice_not_found")}
        action={
          <Button asChild variant="outline">
            <Link href={routes.tenant.invoices(slug)}>
              {t("office.back_to_invoices")}
            </Link>
          </Button>
        }
      />
    );
  }
  const warnings = allWarnings.filter((w) => w.entityId === invoice.id);
  const suggestions = allSuggestions.filter((s) => s.invoiceId === invoice.id);
  const pendingCount = suggestions.filter((s) => s.status === "pending").length;

  return (
    <div className="space-y-4">
      <PageHeader
        title={invoice.number}
        description={`${invoice.counterparty.title} · ${taxpayer?.title ?? ""}`}
        icon={<FileText className="size-6" />}
        breadcrumbs={[
          {
            label: t("office.nav_invoices"),
            href: routes.tenant.invoices(slug),
          },
          { label: invoice.number },
        ]}
        actions={
          <div className="flex items-center gap-2">
            <InvoiceStatusBadge status={invoice.status} />
            {pendingCount ? (
              <Button asChild size="sm">
                <Link
                  href={`${routes.tenant.review(slug)}?invoice=${invoice.id}`}
                >
                  {t("office.review_this_invoice", { count: pendingCount })}
                </Link>
              </Button>
            ) : null}
            <Button asChild size="sm" variant="ghost">
              <Link href={routes.tenant.invoices(slug)}>
                <ArrowLeft className="size-4" />
                {t("office.back_to_invoices")}
              </Link>
            </Button>
          </div>
        }
      />
      <DemoNotice />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-4">
          <Panel title={t("office.invoice_info")}>
            <dl className="grid grid-cols-2 gap-x-4 gap-y-3 p-4 sm:grid-cols-3">
              <Field label="ETTN">
                <Figure className="text-xs">{invoice.ettn}</Figure>
              </Field>
              <Field label={t("office.col_date")}>
                <Figure>{invoice.issueDate}</Figure>
              </Field>
              <Field label={t("office.col_direction")}>
                {t(`office.direction_${invoice.direction}`)}
              </Field>
              <Field label={t("office.col_type")}>
                {invoice.profile} · {invoice.typeCode}
              </Field>
              <Field label={t("office.col_counterparty")}>
                {invoice.counterparty.title}
              </Field>
              <Field
                label={
                  invoice.counterparty.taxId.length === 11 ? "TCKN" : "VKN"
                }
              >
                <Figure>{invoice.counterparty.taxId || "—"}</Figure>
              </Field>
              <Field label={t("office.col_currency")}>
                <Figure>
                  {invoice.currency}
                  {invoice.exchangeRate ? ` · ${invoice.exchangeRate}` : ""}
                </Figure>
              </Field>
              <Field label={t("office.col_source")}>
                {t(`office.source_${invoice.source}`)}
              </Field>
            </dl>
          </Panel>

          <Panel
            title={t("office.warnings_on_invoice")}
            description={warnings.length ? undefined : t("office.no_warnings")}
          >
            {warnings.length ? (
              <ul className="divide-y">
                {warnings.map((w) => (
                  <li
                    key={w.id}
                    className="flex flex-wrap items-center gap-3 px-4 py-2.5"
                  >
                    <SeverityBadge severity={w.severity} />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm">{warningText(w)}</div>
                      <div className="flex gap-2">
                        <Figure className="text-muted-foreground text-xs">
                          {w.code}
                        </Figure>
                        <WarningStatusLabel status={w.status} />
                      </div>
                    </div>
                    <WarningActions warning={w} />
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
                  <th>{t("office.col_account")}</th>
                </tr>
              </thead>
              <tbody>
                {invoice.lines.map((l) => {
                  const s = suggestions.find((x) => x.lineId === l.id);
                  return (
                    <tr key={l.id}>
                      <td>{l.name}</td>
                      <td className="text-right whitespace-nowrap">
                        <Figure>
                          {formatAmount(l.quantity)} {l.unit}
                        </Figure>
                      </td>
                      <td className="text-right">
                        <Money
                          value={l.lineTotal}
                          currency={invoice.currency}
                        />
                      </td>
                      <td className="text-right">
                        <Figure>%{l.vatPercent}</Figure>
                      </td>
                      <td>
                        {l.accountCode ? (
                          <Figure className="text-status-approved font-medium">
                            {l.accountCode}
                          </Figure>
                        ) : s?.status === "pending" ? (
                          <span className="dark:text-status-review text-xs text-amber-800">
                            <Figure>{s.accountCode}</Figure> ·{" "}
                            {t("office.suggestion_pending")}
                          </span>
                        ) : (
                          <span className="text-muted-foreground text-xs">
                            —
                          </span>
                        )}
                      </td>
                    </tr>
                  );
                })}
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
                    <Money value={tx.amount} currency={invoice.currency} />
                  </dd>
                </div>
              ))}
              <div className="flex justify-between gap-4 border-t pt-2">
                <dt className="text-muted-foreground">
                  {t("office.col_base")}
                </dt>
                <dd>
                  <Money
                    value={invoice.totals.taxExclusive}
                    currency={invoice.currency}
                  />
                </dd>
              </div>
              <div className="flex justify-between gap-4 font-semibold">
                <dt>{t("office.col_payable")}</dt>
                <dd>
                  <Money
                    value={invoice.totals.payable}
                    currency={invoice.currency}
                  />
                </dd>
              </div>
            </dl>
          </Panel>
        </div>

        <Panel
          title={t("office.preview_title")}
          className="min-w-0 xl:sticky xl:top-4 xl:self-start"
        >
          <InvoicePreview invoice={invoice} />
        </Panel>
      </div>
    </div>
  );
}

export function InvoiceDetailPage({ id }: { id: string }) {
  return (
    <OfficeDataGate>
      <InvoiceDetailInner id={id} />
    </OfficeDataGate>
  );
}
