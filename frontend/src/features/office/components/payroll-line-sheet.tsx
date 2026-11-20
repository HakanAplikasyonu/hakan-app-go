"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { relatedHref } from "../lib/related-href";
import { usePeriodText } from "../period";
import type { PayrollLine, PayrollRun } from "../services/payroll.service";
import { relatedKeys, relatedService } from "../services/related.service";
import { Money } from "./office-ui";

/**
 * Bordro işlem detayı (#710): bordro dönemi → personel satırı → detay.
 * Brütten nete hesap (işçi SGK ve işsizlik, gelir ve damga vergisi ile
 * istisnaları, öteki kesintiler), işveren payları ve maliyet; bordronun
 * ödeme durumu ve bağlı banka ödemesi (toplu maaş ödemesi; kişinin payı
 * net ücreti kadar).
 */
export function PayrollLineSheet({
  engagement,
  run,
  line,
  onClose,
}: {
  engagement: string;
  run: PayrollRun;
  line: PayrollLine | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const periodText = usePeriodText();
  const related = useQuery({
    queryKey: relatedKeys.one(slug, engagement, "payroll_run", run.uuid),
    queryFn: () =>
      relatedService.list(slug, engagement, "payroll_run", run.uuid),
    enabled: !!line,
  });
  const payments = (related.data?.items ?? []).filter(
    (r) => r.kind === "bank_transaction" && r.relation === "payment",
  );
  const neg = (v: string) => (Number(v) ? `-${v}` : v);

  return (
    <Sheet open={!!line} onOpenChange={(o) => (o ? null : onClose())}>
      <SheetContent
        className="w-full overflow-y-auto sm:max-w-lg"
        data-testid="payroll-line-sheet"
      >
        {line ? (
          <>
            <SheetHeader>
              <SheetTitle>{line.employee_name}</SheetTitle>
              <SheetDescription>
                {t("office.payroll.line_detail_of", {
                  period: periodText(run.period),
                })}
                {line.job_title ? ` · ${line.job_title}` : ""}
                {line.days_worked != null
                  ? ` · ${t("office.payroll.line_days", { count: Number(line.days_worked) })}`
                  : ""}
              </SheetDescription>
            </SheetHeader>
            <div className="space-y-5 px-4 pb-6 text-sm">
              <Section
                title={t("office.payroll.line_gross_to_net")}
                testId="payroll-line-calc"
                rows={[
                  [t("office.payroll.col_gross"), line.gross],
                  [
                    t("office.payroll.line_sgk_employee"),
                    neg(line.sgk_employee),
                  ],
                  [
                    t("office.payroll.line_unemployment_employee"),
                    neg(line.unemployment_employee),
                  ],
                  [t("office.payroll.line_income_tax"), neg(line.income_tax)],
                  [t("office.payroll.line_stamp_tax"), neg(line.stamp_tax)],
                  ...(Number(line.other_deductions ?? 0)
                    ? ([
                        [
                          t("office.payroll.line_other"),
                          neg(line.other_deductions ?? "0"),
                        ],
                      ] as [string, string][])
                    : []),
                ]}
                total={[t("office.payroll.col_net"), line.net]}
              />
              {Number(line.income_tax_exemption ?? 0) ||
              Number(line.stamp_tax_exemption ?? 0) ? (
                <p className="text-text-2 text-xs">
                  {t("office.payroll.line_exemptions", {
                    income: line.income_tax_exemption ?? "0",
                    stamp: line.stamp_tax_exemption ?? "0",
                  })}
                </p>
              ) : null}
              <Section
                title={t("office.payroll.line_employer")}
                testId="payroll-line-employer"
                rows={[
                  [t("office.payroll.col_gross"), line.gross],
                  [t("office.payroll.line_sgk_employer"), line.sgk_employer],
                  [
                    t("office.payroll.line_unemployment_employer"),
                    line.unemployment_employer,
                  ],
                  ...(Number(line.sgk_incentive ?? 0)
                    ? ([
                        [
                          t("office.payroll.col_incentive"),
                          neg(line.sgk_incentive ?? "0"),
                        ],
                      ] as [string, string][])
                    : []),
                ]}
                total={[t("office.payroll.col_cost"), line.employer_cost]}
              />
              <section
                aria-label={t("office.payroll.line_payment")}
                className="space-y-2"
                data-testid="payroll-line-payment"
              >
                <h3 className="font-medium">
                  {t("office.payroll.line_payment")}
                </h3>
                <p>
                  {t(`office.payroll.pay_${run.payment_status}`)}
                  {" · "}
                  {t("office.payroll.line_payment_share")}{" "}
                  <Money value={line.net} />
                </p>
                {related.isLoading ? (
                  <Skeleton className="h-10 w-full" />
                ) : related.isError ? (
                  <p className="text-danger text-xs" role="alert">
                    {t("office.payroll.line_payment_error")}
                  </p>
                ) : payments.length === 0 ? (
                  <p className="text-text-2 text-xs">
                    {t("office.payroll.line_payment_none")}
                  </p>
                ) : (
                  <ul className="divide-y rounded-md border">
                    {payments.map((p) => {
                      const href = relatedHref(slug, engagement, p);
                      return (
                        <li
                          key={p.uuid ?? p.label}
                          className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                        >
                          {href ? (
                            <Link
                              href={href}
                              className="font-medium underline-offset-4 hover:underline"
                            >
                              {t("office.payroll.line_payment_link", {
                                date: p.date ?? "",
                              })}
                            </Link>
                          ) : (
                            <span>{p.label}</span>
                          )}
                          {p.amount ? (
                            <Money
                              value={p.amount}
                              currency={p.currency ?? undefined}
                            />
                          ) : null}
                        </li>
                      );
                    })}
                  </ul>
                )}
              </section>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

function Section({
  title,
  rows,
  total,
  testId,
}: {
  title: string;
  rows: [string, string][];
  total: [string, string];
  testId: string;
}) {
  return (
    <section aria-label={title} className="space-y-2" data-testid={testId}>
      <h3 className="font-medium">{title}</h3>
      <dl className="divide-y rounded-md border">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-3 px-3 py-1.5">
            <dt className="text-text-2">{k}</dt>
            <dd className="tabular-nums">
              <Money value={v} />
            </dd>
          </div>
        ))}
        <div className="bg-surface-2 flex justify-between gap-3 px-3 py-1.5 font-semibold">
          <dt>{total[0]}</dt>
          <dd className="tabular-nums">
            <Money value={total[1]} />
          </dd>
        </div>
      </dl>
    </section>
  );
}
