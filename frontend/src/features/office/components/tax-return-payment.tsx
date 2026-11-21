"use client";

import { useMutation } from "@tanstack/react-query";
import { useId, useMemo, useState } from "react";
import { toast } from "sonner";

import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  type TaxReturnKind,
  type TaxReturnPayment,
  type TaxReturnPreview,
  taxReturnsService,
} from "../services/tax-returns.service";
import { Money } from "./office-ui";

const statusTone: Record<TaxReturnPayment["status"], StatusTone> = {
  paid: "ok",
  unpaid: "warn",
  overdue: "danger",
};

/**
 * Tahakkuk ödemesi (#575): verilmiş KDV1 / MUHSGK beyanının ödenecek
 * tutarı vade gününe kadar vergi dairesine ödenir. Ödeyen banka hareketi
 * seçilirse ödeme günü onun tarihidir ve hareket 360 Ödenecek Vergi ve
 * Fonlar'a işlenir; banka dışı ödemede gün elle girilir. Vadesi geçmiş
 * ödenmemiş tahakkuk kırmızı görünür.
 */
export function TaxReturnPaymentCard({
  p,
  payment,
  canWrite,
  onSaved,
  engagement,
}: {
  p: TaxReturnPreview;
  payment: TaxReturnPayment;
  canWrite: boolean;
  onSaved: (p: TaxReturnPreview) => void;
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const dateId = useId();
  const [paidOn, setPaidOn] = useState("");
  const kind = p.kind as TaxReturnKind;
  const day = useMemo(() => {
    const tag = locale === "tr" ? "tr-TR" : "en-GB";
    return (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(tag);
  }, [locale]);
  const save = useMutation({
    mutationFn: (body: {
      paid_on: string | null;
      bank_transaction_uuid?: string | null;
    }) => taxReturnsService.setPayment(slug, engagement, kind, p.period, body),
    onSuccess: (x) => {
      onSaved(x);
      setPaidOn("");
      toast.success(t("office.tax_returns.payment_saved"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.message
          ? e.message
          : t("office.tax_returns.payment_failed"),
      ),
  });
  const paid = payment.status === "paid";
  return (
    <section
      aria-label={t("office.tax_returns.payment_title")}
      data-testid="tax-return-payment"
      className="bg-surface border-line flex flex-col gap-3 rounded-xl border p-4 text-sm"
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className="font-semibold">
          {t("office.tax_returns.payment_title")}
        </span>
        <StatusPill tone={statusTone[payment.status]}>
          {t(`office.tax_returns.payment_status_${payment.status}`)}
        </StatusPill>
        <span className="text-text-2">
          <Money value={payment.amount} /> ·{" "}
          {t("office.tax_returns.payment_due", { date: day(payment.due_date) })}
        </span>
        {paid && payment.paid_on ? (
          <span className="text-text-2">
            {t("office.tax_returns.payment_paid_on", {
              date: day(payment.paid_on),
            })}
          </span>
        ) : null}
        {paid && canWrite ? (
          <Button
            size="sm"
            variant="outline"
            className="ms-auto max-md:min-h-11"
            disabled={save.isPending}
            onClick={() => save.mutate({ paid_on: null })}
          >
            {t("office.tax_returns.payment_clear")}
          </Button>
        ) : null}
      </div>
      {payment.bank_transaction ? (
        <p className="text-text-2">
          {t("office.tax_returns.payment_bank", {
            date: day(payment.bank_transaction.transaction_date),
            description: payment.bank_transaction.description,
          })}{" "}
          <Money value={payment.bank_transaction.amount} />
        </p>
      ) : null}
      {!paid && canWrite ? (
        <>
          {payment.candidates.length > 0 ? (
            <div className="flex flex-col gap-2">
              <span className="text-text-2">
                {t("office.tax_returns.payment_candidates")}
              </span>
              <ul className="flex flex-col gap-2">
                {payment.candidates.map((c) => (
                  <li
                    key={c.uuid}
                    className="border-line flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2"
                  >
                    <span className="tabular-nums">
                      {day(c.transaction_date)}
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {c.description}
                    </span>
                    <Money value={c.amount} />
                    {c.same_amount ? (
                      <StatusPill tone="ok">
                        {t("office.tax_returns.payment_same_amount")}
                      </StatusPill>
                    ) : null}
                    <Button
                      size="sm"
                      className="max-md:min-h-11"
                      disabled={save.isPending}
                      onClick={() =>
                        save.mutate({
                          paid_on: null,
                          bank_transaction_uuid: c.uuid,
                        })
                      }
                    >
                      {t("office.tax_returns.payment_link")}
                    </Button>
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="text-text-2">
              {t("office.tax_returns.payment_no_candidates")}
            </p>
          )}
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex flex-col gap-1">
              <Label htmlFor={dateId} className="text-text-2 font-normal">
                {t("office.tax_returns.payment_manual")}
              </Label>
              <DatePicker id={dateId} value={paidOn} onChange={setPaidOn} />
            </div>
            <Button
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              disabled={!paidOn || save.isPending}
              onClick={() => save.mutate({ paid_on: paidOn })}
            >
              {t("office.tax_returns.payment_mark_paid")}
            </Button>
          </div>
        </>
      ) : null}
    </section>
  );
}
