"use client";

import { useMutation } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { formatMoney, parseAmountInput, percentOf, sum } from "../lib/decimal";
import {
  type ManualExpenseInput,
  receiptsService,
} from "../services/receipts.service";

/** Türkiye'de geçerli KDV oranları. */
const RATES = ["0", "1", "10", "20"];
const PAYMENTS = [
  "cash",
  "credit_card",
  "bank_transfer",
  "check",
  "promissory_note",
  "credit",
];

type LineForm = { rate: string; base: string; tax: string; taxTyped: boolean };

const newLine = (rate = "20"): LineForm => ({
  rate,
  base: "",
  tax: "",
  taxTyped: false,
});

/**
 * Elle masraf girişi (#780): KDV her oran için ayrı satırda (matrah + KDV).
 * KDV matrahtan önerilir, elle düzeltilebilir. Oranı bilinmeyen masraf
 * "kırılım yok" seçilip yalnız toplamla girilir.
 */
export function ExpenseCreateDialog({
  open,
  onOpenChange,
  engagement,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  engagement: string;
  onCreated: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [date, setDate] = useState("");
  const [vendor, setVendor] = useState("");
  const [vkn, setVkn] = useState("");
  const [receiptNo, setReceiptNo] = useState("");
  const [description, setDescription] = useState("");
  const [payment, setPayment] = useState("");
  const [noBreakdown, setNoBreakdown] = useState(false);
  const [amount, setAmount] = useState("");
  const [lines, setLines] = useState<LineForm[]>([newLine()]);

  const reset = () => {
    setDate("");
    setVendor("");
    setVkn("");
    setReceiptNo("");
    setDescription("");
    setPayment("");
    setNoBreakdown(false);
    setAmount("");
    setLines([newLine()]);
  };

  const parsed = lines.map((l) => ({
    rate: l.rate,
    base: parseAmountInput(l.base),
    tax: parseAmountInput(l.tax),
  }));
  const linesValid = parsed.every((l) => l.base !== null && l.tax !== null);
  const filled = parsed.filter((l) => l.base || l.tax);
  const parsedAmount = parseAmountInput(amount);
  const total = noBreakdown
    ? (parsedAmount ?? "")
    : linesValid
      ? sum(filled.flatMap((l) => [l.base, l.tax]))
      : "";
  const ratesUnique = new Set(filled.map((l) => l.rate)).size === filled.length;
  const canSave =
    !!date &&
    (noBreakdown
      ? !!parsedAmount && Number(parsedAmount) > 0
      : linesValid && filled.length > 0 && ratesUnique && Number(total) > 0);

  const save = useMutation({
    mutationFn: () => {
      const input: ManualExpenseInput = {
        expense_date: date,
        vendor_name: vendor.trim() || undefined,
        vendor_tax_number: vkn.trim() || undefined,
        receipt_number: receiptNo.trim() || undefined,
        description: description.trim() || undefined,
        payment_method: (payment ||
          undefined) as ManualExpenseInput["payment_method"],
      };
      if (noBreakdown) input.amount = parsedAmount ?? undefined;
      else
        input.vat_lines = filled.map((l) => ({
          tax_rate: l.rate,
          tax_base: l.base || "0",
          tax_amount: l.tax || "0",
        }));
      return receiptsService.createExpense(slug, engagement, input);
    },
    onSuccess: () => {
      toast.success(t("office.expenses.create_saved"));
      reset();
      onOpenChange(false);
      onCreated();
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.expenses.create_failed"),
      ),
  });

  const setLine = (i: number, patch: Partial<LineForm>) =>
    setLines((ls) =>
      ls.map((l, j) => {
        if (j !== i) return l;
        const next = { ...l, ...patch };
        // KDV elle yazılmadıysa matrah × oran ile önerilir.
        if (!next.taxTyped) {
          const b = parseAmountInput(next.base);
          next.tax = b ? percentOf(b, next.rate).replace(".", ",") : "";
        }
        return next;
      }),
    );

  const field = "flex flex-col gap-1 text-xs leading-normal font-normal";
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("office.expenses.create_title")}</DialogTitle>
          <DialogDescription>
            {t("office.expenses.create_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className={field}>
            <Label
              htmlFor="expense-create-date"
              className="text-xs leading-normal font-normal"
            >
              {t("office.col_date")}
            </Label>
            <DatePicker
              id="expense-create-date"
              value={date}
              onChange={setDate}
            />
          </div>
          <Label className={field}>
            {t("office.expenses.col_vendor")}
            <Input value={vendor} onChange={(e) => setVendor(e.target.value)} />
          </Label>
          <Label className={field}>
            {t("office.expenses.create_vkn")}
            <Input
              inputMode="numeric"
              maxLength={11}
              value={vkn}
              onChange={(e) => setVkn(e.target.value)}
            />
          </Label>
          <Label className={field}>
            {t("office.expenses.create_receipt_no")}
            <Input
              value={receiptNo}
              onChange={(e) => setReceiptNo(e.target.value)}
            />
          </Label>
          <Label className={field}>
            {t("office.expenses.create_description")}
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Label>
          <Label className={field}>
            {t("office.expenses.create_payment")}
            <NativeSelect
              value={payment}
              onChange={(e) => setPayment(e.target.value)}
            >
              <NativeSelectOption value="">—</NativeSelectOption>
              {PAYMENTS.map((p) => (
                <NativeSelectOption key={p} value={p}>
                  {t(`sources.payment.${p}`)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Label>
        </div>

        <div className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 className="text-sm font-semibold">
              {t("office.expenses.vat_breakdown")}
            </h4>
            <Label className="flex items-center gap-2 text-xs leading-normal font-normal">
              <Checkbox
                checked={noBreakdown}
                onCheckedChange={(v) => setNoBreakdown(v === true)}
              />
              {t("office.expenses.create_no_breakdown")}
            </Label>
          </div>
          {noBreakdown ? (
            <Label className={field}>
              {t("office.expenses.create_amount")}
              <Input
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </Label>
          ) : (
            <div className="space-y-2" data-testid="expense-create-lines">
              {lines.map((l, i) => (
                <div
                  key={i}
                  className="grid grid-cols-[5.5rem_1fr_1fr_auto] items-end gap-2"
                >
                  <Label className={field}>
                    {t("office.expenses.vat_rate")}
                    <NativeSelect
                      value={l.rate}
                      onChange={(e) => setLine(i, { rate: e.target.value })}
                    >
                      {RATES.map((r) => (
                        <NativeSelectOption key={r} value={r}>
                          %{r}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </Label>
                  <Label className={field}>
                    {t("office.expenses.vat_base")}
                    <Input
                      inputMode="decimal"
                      value={l.base}
                      onChange={(e) => setLine(i, { base: e.target.value })}
                    />
                  </Label>
                  <Label className={field}>
                    {t("office.expenses.col_tax")}
                    <Input
                      inputMode="decimal"
                      value={l.tax}
                      onChange={(e) =>
                        setLine(i, { tax: e.target.value, taxTyped: true })
                      }
                    />
                  </Label>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("office.expenses.create_remove_line")}
                    disabled={lines.length === 1}
                    onClick={() =>
                      setLines((ls) => ls.filter((_, j) => j !== i))
                    }
                  >
                    <Trash2 className="size-4" aria-hidden />
                  </Button>
                </div>
              ))}
              {!ratesUnique ? (
                <p className="text-warn text-xs">
                  {t("office.expenses.create_rate_twice")}
                </p>
              ) : null}
              {lines.length < RATES.length ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setLines((ls) => [
                      ...ls,
                      newLine(
                        RATES.find((r) => !ls.some((l) => l.rate === r)) ??
                          "20",
                      ),
                    ])
                  }
                >
                  <Plus className="size-4" aria-hidden />
                  {t("office.expenses.create_add_line")}
                </Button>
              ) : null}
            </div>
          )}
          <p className="text-sm">
            {t("office.expenses.col_total")}:{" "}
            <span className="font-semibold tabular-nums">
              {total ? formatMoney(total, "TRY") : "—"}
            </span>
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("office.expenses.create_cancel")}
          </Button>
          <Button
            disabled={!canSave || save.isPending}
            onClick={() => save.mutate()}
            data-testid="expense-create-save"
          >
            {t("office.expenses.create_save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
