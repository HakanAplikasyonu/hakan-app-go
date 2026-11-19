"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";

import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import type { components } from "@/generated/api";
import { useLocale } from "@/providers/locale-provider";

import { Money } from "./office-ui";

export type BankBalanceCheck = components["schemas"]["BankBalanceCheck"];

/** Uyarıda bu kod varsa panel ekstre bakiye kontrolünü gösterir. */
export const BALANCE_WARNING_CODE = "BNK_BALANCE_MISMATCH";

export function useBankBalanceCheck(engagement: string, period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "bank-balance-check",
      engagement,
      period,
    ],
    queryFn: () =>
      tenantRequest<{ period: string; items: BankBalanceCheck[] }>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-balance-check`,
        { query: { period } },
      ),
    enabled: !!engagement && !!period,
  });
}

/**
 * Ekstre bakiye kontrolü (#570): her hesapta açılış + giren − çıkan =
 * beklenen kapanış; bankanın verdiği kapanışla farkı. "Banka bakiyesi
 * tutmuyor" uyarısı artık neyin, ne kadar tutmadığını söyler.
 */
export function BankBalanceCheckPanel({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const q = useBankBalanceCheck(engagement, period);
  const items = q.data?.items ?? [];
  return (
    <section className="space-y-2" data-testid="bank-balance-check">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h4 className="text-sm font-semibold">
          {t("office.balance_check.title")}
        </h4>
        <Link
          href={routes.tenant.engagementBank(slug, engagement)}
          className="text-brand-text text-sm font-medium hover:underline"
        >
          {t("office.balance_check.open_bank")}
        </Link>
      </div>
      {q.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : q.isError ? (
        <p className="text-danger text-sm">{t("office.load_failed")}</p>
      ) : items.length === 0 ? (
        <p className="text-text-2 text-sm">{t("office.balance_check.empty")}</p>
      ) : (
        <div className="border-line rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("office.balance_check.col_account")}</TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_opening")}
                </TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_in")}
                </TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_out")}
                </TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_expected")}
                </TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_closing")}
                </TableHead>
                <TableHead className="text-right">
                  {t("office.balance_check.col_diff")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((c) => (
                <TableRow key={c.cash_box_uuid}>
                  <TableCell className="max-w-48">
                    <div className="truncate font-medium">
                      {c.cash_box_name || c.iban || "—"}
                    </div>
                    <div className="text-text-3 text-xs">
                      {t("office.balance_check.movements", {
                        count: c.movements,
                      })}
                    </div>
                  </TableCell>
                  <Amount value={c.opening} currency={c.currency} />
                  <Amount value={c.in_total} currency={c.currency} />
                  <Amount value={c.out_total} currency={c.currency} />
                  <Amount value={c.expected_closing} currency={c.currency} />
                  <Amount value={c.closing} currency={c.currency} />
                  <TableCell
                    className={cn(
                      "text-right font-semibold",
                      c.difference === null
                        ? "text-text-3"
                        : c.balanced
                          ? "text-ok"
                          : "text-danger",
                    )}
                  >
                    {c.difference === null ? (
                      "—"
                    ) : (
                      <Money value={c.difference} currency={c.currency} />
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {items.length > 0 ? <Verdict items={items} /> : null}
    </section>
  );
}

function Amount({
  value,
  currency,
}: {
  value: string | null;
  currency: string;
}) {
  return (
    <TableCell className="text-right">
      {value === null ? (
        <span className="text-text-3">—</span>
      ) : (
        <Money value={value} currency={currency} />
      )}
    </TableCell>
  );
}

/** Sonucun düz cümlesi ve ne yapılacağı. */
function Verdict({ items }: { items: BankBalanceCheck[] }) {
  const { t } = useLocale();
  const off = items.filter((c) => c.difference !== null && !c.balanced);
  const unknown = items.filter((c) => c.difference === null);
  if (off.length > 0)
    return (
      <p className="text-danger text-sm">{t("office.balance_check.off")}</p>
    );
  if (unknown.length > 0)
    return (
      <p className="text-warn text-sm">
        {t("office.balance_check.no_balance")}
      </p>
    );
  return <p className="text-ok text-sm">{t("office.balance_check.ok")}</p>;
}
