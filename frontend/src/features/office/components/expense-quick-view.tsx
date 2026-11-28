"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, X } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { relatedKeys } from "../services/related.service";
import { receiptKeys, receiptsService } from "../services/receipts.service";
import { warningKeys } from "../services/warnings.service";
import { Figure, Money } from "./office-ui";
import { ExpenseVatBreakdown } from "./expense-vat-breakdown";
import { useRejectReason } from "./reject-reason-dialog";
import type { ReceiptRejection } from "../lib/reject-reason";

const tone: Record<string, "warning" | "success" | "secondary"> = {
  pending: "warning",
  approved: "success",
  rejected: "secondary",
};

/**
 * Masrafın yan paneldeki özeti (#529): tarih, satıcı, tutar, kategori ve
 * hesap; bekleyen masraf buradan onaylanır ya da reddedilir. Karar
 * "onaylanmamış masraf" uyarısını kendiliğinden kapatır.
 */
export function ExpenseQuickView({
  engagement,
  expense,
  onChanged,
}: {
  engagement: string;
  expense: string;
  onChanged?: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const key = receiptKeys.expense(slug, engagement, expense);
  const q = useQuery({
    queryKey: key,
    queryFn: () => receiptsService.expense(slug, engagement, expense),
  });
  const decide = useMutation({
    mutationFn: ({
      approve,
      reason,
    }: {
      approve: boolean;
      reason?: ReceiptRejection;
    }) =>
      receiptsService.decideExpense(slug, engagement, expense, approve, reason),
    onSuccess: (x, { approve }) => {
      qc.setQueryData(key, x);
      void qc.invalidateQueries({
        queryKey: receiptKeys.expenses(slug, engagement),
      });
      void qc.invalidateQueries({ queryKey: warningKeys.all(slug) });
      void qc.invalidateQueries({
        queryKey: relatedKeys.one(slug, engagement, "expense", expense),
      });
      toast.success(
        approve
          ? x.journal
            ? t("office.expenses.approved_with_journal", {
                no: x.journal.entry_no,
              })
            : t("office.receipts.approved")
          : t("office.receipts.rejected"),
      );
      onChanged?.();
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.receipts.decide_failed"),
      ),
  });

  const reject = useRejectReason(decide.isPending);

  if (q.isLoading) return <Skeleton className="h-40 w-full" />;
  if (q.isError || !q.data)
    return (
      <p className="text-text-3 text-sm">
        {t("office.expense_view.load_failed")}
      </p>
    );
  const x = q.data;
  const rows: [string, React.ReactNode][] = [
    [
      t("office.col_date"),
      x.expense_date ? date(x.expense_date, "dd.MM.yyyy", locale) : "—",
    ],
    [
      t("office.expenses.col_vendor"),
      [x.vendor_name, x.vendor_tax_number].filter(Boolean).join(" · ") || "—",
    ],
    [
      t("office.expenses.col_total"),
      x.amount ? <Money value={x.amount} currency={x.currency} /> : "—",
    ],
    [
      t("office.expenses.col_tax"),
      x.tax_amount ? <Money value={x.tax_amount} currency={x.currency} /> : "—",
    ],
    [
      t("office.expenses.vat_breakdown"),
      <ExpenseVatBreakdown
        key="vat"
        lines={x.vat_lines}
        receiptRead={x.receipt_read}
        currency={x.currency}
      />,
    ],
    [
      t("office.expenses.col_posting"),
      x.account_code ? (
        <Figure>
          {[x.category_name ?? x.category_code, x.account_code]
            .filter(Boolean)
            .join(" · ")}
        </Figure>
      ) : (
        "—"
      ),
    ],
  ];
  const pending = x.approval_status === "pending";
  return (
    <section
      className="border-line space-y-3 rounded-xl border p-3"
      data-testid="expense-quick-view"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="text-sm font-semibold">
          {x.description || t("office.expense_view.title")}
        </h4>
        <Badge variant={tone[x.approval_status] ?? "outline"}>
          {t(`office.receipts.status_${x.approval_status}`)}
        </Badge>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-text-3">{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      {pending && can(permissions.tenant.receipts.approve) ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            className="max-md:min-h-11"
            disabled={decide.isPending}
            onClick={() => decide.mutate({ approve: true })}
          >
            <Check className="size-4" aria-hidden />
            {t("office.receipts.approve")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="max-md:min-h-11"
            disabled={decide.isPending}
            onClick={() =>
              reject.ask((reason) => decide.mutate({ approve: false, reason }))
            }
          >
            <X className="size-4" aria-hidden />
            {t("office.receipts.reject")}
          </Button>
          <span className="text-text-3 text-xs">
            {t("office.expense_view.decide_hint")}
          </span>
          {reject.dialog}
        </div>
      ) : null}
    </section>
  );
}
