"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { usePermission } from "@/providers/permission-provider";

import { FOCUS_PARAM } from "../lib/related-href";
import { warningKeys } from "../services/warnings.service";
import { errText } from "./engagement-expenses-page";
import { Money } from "./office-ui";

export type BankBalanceCheck = components["schemas"]["BankBalanceCheck"];
/** Bakiye tutmazken bakılacak hareket (#710): olası mükerrer ya da çıkarılmış. */
export type BalanceSuspect = {
  uuid: string;
  cash_box_uuid: string;
  transaction_date: string;
  debit_credit: "A" | "B";
  amount: string;
  currency: string;
  description: string;
  possible_duplicate: boolean;
  excluded: boolean;
};

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
      tenantRequest<{
        period: string;
        items: BankBalanceCheck[];
        suspects: BalanceSuspect[];
      }>(
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
  const suspects = q.data?.suspects ?? [];
  // Sayfa uyarının dönemiyle açılır (#710): dönemsiz bağlantı başka ayı
  // açıp boş görünüyordu.
  const bankHref = (tx?: string) =>
    `${routes.tenant.engagementBank(slug, engagement)}?period=${period}${
      tx
        ? `&${FOCUS_PARAM}=${encodeURIComponent(`bank_transaction:${tx}`)}`
        : ""
    }`;
  return (
    <section className="space-y-2" data-testid="bank-balance-check">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h4 className="text-sm font-semibold">
          {t("office.balance_check.title")}
        </h4>
        <Link
          href={bankHref()}
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
      {items.some((c) => c.difference !== null && !c.balanced) ? (
        <Guide items={items} />
      ) : null}
      {suspects.length > 0 ? (
        <Suspects
          engagement={engagement}
          period={period}
          items={suspects}
          href={bankHref}
        />
      ) : null}
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

/**
 * Ne demek, neden önemli, ne yapılır, "çözüldü" ne demek (#710). Farkın
 * yönü söylenir: ekstre fazla diyorsa eksik giriş ya da mükerrer çıkış,
 * az diyorsa eksik çıkış ya da mükerrer giriş aranır.
 */
function Guide({ items }: { items: BankBalanceCheck[] }) {
  const { t } = useLocale();
  const off = items.filter((c) => c.difference !== null && !c.balanced);
  return (
    <div
      className="border-line bg-surface-2 space-y-2 rounded-lg border p-3 text-sm"
      data-testid="bank-balance-guide"
    >
      {off.map((c) => {
        const d = Number(c.difference);
        return (
          <p key={c.cash_box_uuid} className="font-medium">
            {t(
              d > 0
                ? "office.balance_check.diff_more"
                : "office.balance_check.diff_less",
              { account: c.cash_box_name || c.iban || "—" },
            )}{" "}
            <Money value={String(Math.abs(d))} currency={c.currency} />
          </p>
        );
      })}
      <p className="text-text-2">{t("office.balance_check.why")}</p>
      <ol className="text-text-2 list-decimal space-y-1 ps-5">
        <li>{t("office.balance_check.step_duplicate")}</li>
        <li>{t("office.balance_check.step_missing")}</li>
        <li>{t("office.balance_check.step_link")}</li>
      </ol>
      <p className="text-text-2">{t("office.balance_check.resolved_means")}</p>
    </div>
  );
}

/**
 * Bakılacak hareketler (#710): olası mükerrerler "hesaptan çıkar" ile
 * toplamdan, eşleşmeden ve uyarılardan düşer; çıkarılan "yeniden dahil
 * et" ile geri gelir. "Hareketi aç" eşleştirme paneline götürür.
 */
function Suspects({
  engagement,
  period,
  items,
  href,
}: {
  engagement: string;
  period: string;
  items: BalanceSuspect[];
  href: (tx?: string) => string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const set = useMutation({
    mutationFn: (x: { uuid: string; excluded: boolean }) =>
      tenantRequest<{ uuid: string; excluded: boolean }>(
        slug,
        "PUT",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-transactions/${encodeURIComponent(x.uuid)}/exclusion`,
        { body: { excluded: x.excluded } },
      ),
    onSuccess: (r) => {
      toast.success(
        t(
          r.excluded
            ? "office.balance_check.excluded_done"
            : "office.balance_check.included_done",
        ),
      );
      for (const k of ["bank-balance-check", "bank-transactions"])
        void qc.invalidateQueries({
          queryKey: ["tenant", slug.toLowerCase(), k],
        });
      void qc.invalidateQueries({ queryKey: warningKeys.all(slug) });
    },
    onError: (e) =>
      toast.error(errText(e, t("office.balance_check.exclude_failed"))),
  });
  return (
    <section className="space-y-2" data-testid="bank-balance-suspects">
      <h5 className="text-sm font-semibold">
        {t("office.balance_check.suspects_title")}
      </h5>
      <ul className="divide-line divide-y rounded-lg border text-sm">
        {items.map((x) => (
          <li
            key={x.uuid}
            data-testid="bank-balance-suspect"
            className={cn(
              "flex flex-wrap items-center gap-2 px-3 py-2",
              x.excluded && "text-text-3",
            )}
          >
            <span className="tabular-nums">
              {x.transaction_date.split("-").reverse().join(".")}
            </span>
            <span className="min-w-0 flex-1 truncate">
              {x.description || "—"}
            </span>
            <span className="tabular-nums">
              {x.debit_credit === "A" ? "+" : "−"}
              <Money value={x.amount} currency={x.currency} />
            </span>
            {x.excluded ? (
              <Badge variant="outline">
                {t("office.balance_check.excluded")}
              </Badge>
            ) : x.possible_duplicate ? (
              <Badge variant="outline">
                {t("office.balance_check.possible_duplicate")}
              </Badge>
            ) : (
              <Badge variant="outline">{t("office.balance_check.twin")}</Badge>
            )}
            <Link
              href={href(x.uuid)}
              className="text-brand-text text-xs font-medium hover:underline"
            >
              {t("office.balance_check.open_movement")}
            </Link>
            {canWrite ? (
              <Button
                size="sm"
                variant="outline"
                disabled={set.isPending}
                onClick={() =>
                  set.mutate({ uuid: x.uuid, excluded: !x.excluded })
                }
              >
                {x.excluded
                  ? t("office.balance_check.include")
                  : t("office.balance_check.exclude")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      <p className="text-text-3 text-xs">
        {t("office.balance_check.suspects_hint", { period })}
      </p>
    </section>
  );
}
