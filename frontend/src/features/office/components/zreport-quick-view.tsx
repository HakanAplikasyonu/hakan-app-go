"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, X } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { relatedKeys } from "../services/related.service";
import {
  receiptKeys,
  receiptsService,
  type RetailSale,
} from "../services/receipts.service";
import { warningKeys } from "../services/warnings.service";
import { Figure, Money } from "./office-ui";
import { ReviewReceiptImage } from "./review-receipt-image";

const tone: Record<string, "warning" | "success" | "secondary"> = {
  pending: "warning",
  approved: "success",
  rejected: "secondary",
};

/**
 * Z raporunun yan paneldeki özeti (#529): ÖKC, toplam, KDV. Bekleyen
 * raporda Z numarası ve tarih burada düzeltilir (kayıt sürekliliği yeniden
 * denetler; "Z numarası atlıyor" / "gün atlıyor" uyarısı kendiliğinden
 * kapanır) ve rapor onaylanır ya da reddedilir.
 */
export function ZReportQuickView({
  engagement,
  sale,
  onChanged,
}: {
  engagement: string;
  sale: string;
  onChanged?: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const key = receiptKeys.retailSale(slug, engagement, sale);
  const q = useQuery({
    queryKey: key,
    queryFn: () => receiptsService.retailSale(slug, engagement, sale),
  });
  if (q.isLoading) return <Skeleton className="h-48 w-full" />;
  if (q.isError || !q.data)
    return (
      <p className="text-text-3 text-sm">
        {t("office.zreport_view.load_failed")}
      </p>
    );
  // Alanlar sunucudaki değerle başlar; kayıt değişince form yeniden kurulur.
  return (
    <ZReportBody
      key={`${q.data.z_report_number}|${q.data.sale_date}|${q.data.approval_status}`}
      engagement={engagement}
      sale={q.data}
      onChanged={onChanged}
    />
  );
}

function ZReportBody({
  engagement,
  sale: x,
  onChanged,
}: {
  engagement: string;
  sale: RetailSale;
  onChanged?: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const id = useId();
  const [zNumber, setZNumber] = useState(x.z_report_number ?? "");
  const [saleDate, setSaleDate] = useState(x.sale_date ?? "");
  const settle = (v: RetailSale) => {
    qc.setQueryData(receiptKeys.retailSale(slug, engagement, x.uuid), v);
    void qc.invalidateQueries({
      queryKey: receiptKeys.retail(slug, engagement),
    });
    void qc.invalidateQueries({ queryKey: warningKeys.all(slug) });
    void qc.invalidateQueries({
      queryKey: relatedKeys.one(slug, engagement, "retail_sale", x.uuid),
    });
    onChanged?.();
  };
  const fail = (fallback: string) => (e: unknown) =>
    toast.error(isApiError(e) ? e.message : fallback);
  const save = useMutation({
    mutationFn: () =>
      receiptsService.updateRetailSale(slug, engagement, x.uuid, {
        z_report_number: zNumber.trim(),
        ...(saleDate ? { sale_date: saleDate } : {}),
      }),
    onSuccess: (v) => {
      settle(v);
      toast.success(t("office.zreport_view.saved"));
    },
    onError: fail(t("office.zreport_view.save_failed")),
  });
  const decide = useMutation({
    mutationFn: (approve: boolean) =>
      receiptsService.decideRetailSale(slug, engagement, x.uuid, approve),
    onSuccess: (v, approve) => {
      settle(v);
      toast.success(
        approve
          ? v.journal
            ? t("office.zreports.approved_with_journal", {
                no: v.journal.entry_no,
              })
            : t("office.receipts.approved")
          : t("office.receipts.rejected"),
      );
    },
    onError: fail(t("office.receipts.decide_failed")),
  });

  const pending = x.approval_status === "pending";
  const canWrite = pending && can(permissions.tenant.receipts.write);
  const dirty =
    zNumber.trim() !== (x.z_report_number ?? "") ||
    saleDate !== (x.sale_date ?? "");
  const busy = save.isPending || decide.isPending;
  const capture = x.receipt_uuid;
  return (
    <section
      className="border-line space-y-3 rounded-xl border p-3"
      data-testid="zreport-quick-view"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="text-sm font-semibold">
          {t("office.zreport_view.title")}
          {x.okc_serial_number ? (
            <Figure className="text-text-3 ms-2 text-xs font-normal">
              {x.okc_serial_number}
            </Figure>
          ) : null}
        </h4>
        <Badge variant={tone[x.approval_status] ?? "secondary"}>
          {t(`office.receipts.status_${x.approval_status}`)}
        </Badge>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
        <dt className="text-text-3">{t("office.zreports.col_total")}</dt>
        <dd>
          {x.total_amount ? (
            <Money value={x.total_amount} currency={x.currency} />
          ) : (
            "—"
          )}
        </dd>
        <dt className="text-text-3">{t("office.expenses.col_tax")}</dt>
        <dd>
          {x.tax_amount ? (
            <Money value={x.tax_amount} currency={x.currency} />
          ) : (
            "—"
          )}
        </dd>
      </dl>
      {capture && x.part_count > 1 ? (
        <section className="space-y-2" data-testid="zreport-parts">
          <h5 className="text-text-2 text-xs font-semibold">
            {t("office.zreport_view.parts", { count: x.part_count })}
          </h5>
          <ol className="grid gap-2 sm:grid-cols-2">
            {Array.from({ length: x.part_count }, (_, i) => (
              <li
                key={i}
                className="space-y-1"
                data-testid={`zreport-part-${i + 1}`}
              >
                <span className="text-text-3 text-xs">
                  {t("office.zreport_view.part", { no: i + 1 })}
                </span>
                <ReviewReceiptImage
                  engagement={engagement}
                  receipt={capture}
                  part={i + 1}
                />
              </li>
            ))}
          </ol>
        </section>
      ) : null}
      <form
        className="grid gap-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (canWrite && dirty) save.mutate();
        }}
      >
        <div className="space-y-1">
          <Label htmlFor={`${id}-z`}>{t("office.zreport_view.z_number")}</Label>
          <Input
            id={`${id}-z`}
            inputMode="numeric"
            value={zNumber}
            disabled={!canWrite}
            onChange={(e) => setZNumber(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-d`}>{t("office.zreport_view.date")}</Label>
          <DatePicker
            id={`${id}-d`}
            className="w-full"
            value={saleDate}
            disabled={!canWrite}
            onChange={setSaleDate}
          />
        </div>
        <p className="text-text-3 text-xs sm:col-span-2">
          {pending
            ? t("office.zreport_view.edit_hint")
            : t("office.zreport_view.locked_hint")}
        </p>
        {canWrite ? (
          <div className="sm:col-span-2">
            <Button
              type="submit"
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              disabled={!dirty || busy}
            >
              {t("office.zreport_view.save")}
            </Button>
          </div>
        ) : null}
      </form>
      {pending && can(permissions.tenant.receipts.approve) ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            className="max-md:min-h-11"
            disabled={busy || dirty}
            onClick={() => decide.mutate(true)}
          >
            <Check className="size-4" aria-hidden />
            {t("office.receipts.approve")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="max-md:min-h-11"
            disabled={busy}
            onClick={() => decide.mutate(false)}
          >
            <X className="size-4" aria-hidden />
            {t("office.receipts.reject")}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
