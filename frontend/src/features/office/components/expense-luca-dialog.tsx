"use client";

import { useMutation } from "@tanstack/react-query";
import { FileSpreadsheet } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { receiptsService } from "../services/receipts.service";

/**
 * "Luca'ya aktar (Excel)" (#780): seçilen dönemin masrafları Luca fiş
 * dosyası olarak iner (fiş dosyasıyla aynı başlık satırı). Her KDV oranı
 * kendi 191 satırında; kırılımı olmayan masraf tek 191 satırıyla ve
 * açıklamada işaretli yazılır.
 */
export function ExpenseLucaDialog({
  open,
  onOpenChange,
  engagement,
  period,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [month, setMonth] = useState(period);
  const run = useMutation({
    mutationFn: () =>
      receiptsService.exportExpensesLuca(slug, engagement, month),
    onSuccess: (h) => {
      const n = (k: string) => Number(h.get(k) ?? 0);
      // Sayılar başlıkta gelir; okunamazsa yalnız "indirildi" yazılır.
      const parts = [
        n("X-Expenses-Exported")
          ? t("office.expenses.luca_done_count", {
              count: n("X-Expenses-Exported"),
            })
          : t("office.expenses.luca_done"),
      ];
      if (n("X-Expenses-No-Breakdown"))
        parts.push(
          t("office.expenses.luca_no_breakdown", {
            count: n("X-Expenses-No-Breakdown"),
          }),
        );
      if (n("X-Expenses-Skipped"))
        parts.push(
          t("office.expenses.luca_skipped", {
            count: n("X-Expenses-Skipped"),
          }),
        );
      toast.success(parts.join(" "));
      onOpenChange(false);
    },
    onError: (e) =>
      toast.error(
        (e as { status?: number }).status === 422
          ? t("office.expenses.luca_empty")
          : t("office.expenses.luca_failed"),
      ),
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) setMonth(period);
        onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("office.expenses.luca_title")}</DialogTitle>
          <DialogDescription>
            {t("office.expenses.luca_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1">
          <Label
            htmlFor="expense-luca-period"
            className="text-xs leading-normal font-normal"
          >
            {t("office.expenses.luca_period")}
          </Label>
          <MonthPicker
            id="expense-luca-period"
            className="w-48"
            value={month}
            onChange={setMonth}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("office.expenses.create_cancel")}
          </Button>
          <Button
            disabled={!month || run.isPending}
            onClick={() => run.mutate()}
            data-testid="expense-luca-download"
          >
            <FileSpreadsheet className="size-4" aria-hidden />
            {t("office.expenses.luca_download")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
