"use client";

import { FileText, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementInvoices } from "../hooks/use-office-invoices";
import { directionOf } from "../lib/from-api";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import { FilterSelect, Figure, Money, Panel, TableBox } from "./office-ui";
import { useOfficePeriod } from "../period";

type Dir = "all" | "outgoing" | "incoming";
const PAGE = 50;

const stateClass: Record<string, string> = {
  active: "bg-status-approved/12 text-status-approved",
  draft: "bg-secondary text-secondary-foreground",
  cancelled: "bg-status-blocking/12 text-status-blocking",
};

/** status_normalized of the invoice (30 §8): counts in the declaration or not. */
export function InvoiceStateBadge({ status }: { status: string }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        stateClass[status] ?? stateClass.draft,
      )}
    >
      {t(`office.api.invoice_state_${status}`)}
    </span>
  );
}

function InvoicesInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug);
  const engagement = selected?.engagement ?? "";

  const direction = (params.get("direction") ?? "all") as Dir;
  const offset = Math.max(0, Number(params.get("offset") ?? 0) || 0);
  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value === "all" || value === "0") next.delete(key);
    else next.set(key, value);
    if (key !== "offset") next.delete("offset");
    router.replace(`${pathname}?${next.toString()}`);
  };
  const { period } = useOfficePeriod();
  const list = useEngagementInvoices(slug, engagement, {
    direction: direction === "all" ? undefined : direction,
    period,
    limit: PAGE,
    offset,
  });
  const fmtDate = (d: string) =>
    new Date(`${d}T00:00:00`).toLocaleDateString(
      locale === "tr" ? "tr-TR" : "en-GB",
    );

  const header = (
    <PageHeader
      title={t("office.invoices_title")}
      description={t("office.invoices_description")}
      icon={<FileText className="size-6" />}
    />
  );
  if (isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (isError || options.length === 0) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={
            isError
              ? t("office.api.load_error")
              : t("office.api.no_engagements")
          }
        />
      </div>
    );
  }

  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  return (
    <div className="space-y-4">
      {header}
      <div className="flex flex-wrap gap-3">
        <EngagementPicker
          options={options}
          selected={selected}
          onSelect={select}
        />
        <FilterSelect<Dir>
          label={t("office.col_direction")}
          value={direction}
          onChange={(v) => setParam("direction", v)}
          options={[
            { value: "all", label: t("office.filter_all") },
            { value: "outgoing", label: t("office.direction_sales") },
            { value: "incoming", label: t("office.direction_purchase") },
          ]}
        />
      </div>
      <Panel>
        {list.isLoading ? (
          <Skeleton className="m-4 h-40" />
        ) : list.isError ? (
          <EmptyState className="m-4" title={t("office.api.load_error")} />
        ) : rows.length === 0 ? (
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
                const dir = directionOf(inv.direction);
                return (
                  <tr key={inv.uuid} className="hover:bg-muted/40">
                    <td className="whitespace-nowrap">
                      <Figure>{fmtDate(inv.issue_date)}</Figure>
                    </td>
                    <td>
                      <div className="flex items-center gap-1.5">
                        <Link
                          href={routes.tenant.invoice(
                            slug,
                            inv.uuid,
                            engagement,
                          )}
                          className="hover:underline"
                        >
                          <Figure className="font-medium">
                            {inv.invoice_number}
                          </Figure>
                        </Link>
                        {inv.open_warnings ? (
                          <span
                            className="text-status-warning inline-flex items-center gap-0.5 text-xs"
                            title={t("office.open_warnings", {
                              count: inv.open_warnings,
                            })}
                          >
                            <TriangleAlert className="size-3.5" aria-hidden />
                            <Figure>{inv.open_warnings}</Figure>
                          </span>
                        ) : null}
                      </div>
                    </td>
                    <td>
                      <div className="max-w-64 truncate">
                        {inv.counterparty_name ?? ""}
                      </div>
                      <Figure className="text-muted-foreground text-xs">
                        {inv.counterparty_tax_id ?? ""}
                      </Figure>
                    </td>
                    <td className="text-xs">
                      <span
                        className={
                          dir === "sales"
                            ? "text-status-approved"
                            : "text-brand"
                        }
                      >
                        {t(`office.direction_${dir}`)}
                      </span>
                      <div className="text-muted-foreground">
                        {inv.profile ? `${inv.profile} · ` : ""}
                        {inv.type}
                      </div>
                    </td>
                    <td className="text-right">
                      <Money
                        value={inv.tax_exclusive_amount}
                        currency={inv.currency}
                      />
                    </td>
                    <td className="text-right">
                      <Money value={inv.tax_amount} currency={inv.currency} />
                    </td>
                    <td className="text-right font-medium">
                      <Money
                        value={inv.payable_amount}
                        currency={inv.currency}
                      />
                    </td>
                    <td>
                      <InvoiceStateBadge status={inv.status} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </TableBox>
        )}
      </Panel>
      {total > PAGE ? (
        <div className="flex items-center justify-end gap-2 text-xs">
          <span className="text-muted-foreground">
            {t("office.api.page_of", {
              from: offset + 1,
              to: Math.min(offset + PAGE, total),
              total,
            })}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() =>
              setParam("offset", String(Math.max(0, offset - PAGE)))
            }
          >
            {t("office.api.prev")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={offset + PAGE >= total}
            onClick={() => setParam("offset", String(offset + PAGE))}
          >
            {t("office.api.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function InvoicesPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <InvoicesInner />
    </Suspense>
  );
}
