"use client";

import { useMutation } from "@tanstack/react-query";
import { useMemo } from "react";
import { toast } from "sonner";

import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  type TaxReturnPreview,
  type TaxReturnRentPayment,
  taxReturnsService,
} from "../services/tax-returns.service";
import { Money } from "./office-ui";

/**
 * Faturasız kira stopajı (#575): şahıstan kiralanan işyerinin kirası
 * bankadan net ödenir. Kira işaretlenen çıkışın brütü (net / 0,80) ve %20
 * stopajı muhtasarın "Kira ödemeleri" satırına girer. Açıklamasında kira
 * geçen, faturayla eşleşmemiş çıkışlar aday olarak listelenir.
 */
export function TaxReturnRentPayments({
  p,
  rows,
  editable,
  onSaved,
  engagement,
}: {
  p: TaxReturnPreview;
  rows: TaxReturnRentPayment[];
  editable: boolean;
  onSaved: (p: TaxReturnPreview) => void;
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const day = useMemo(() => {
    const tag = locale === "tr" ? "tr-TR" : "en-GB";
    return (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(tag);
  }, [locale]);
  const save = useMutation({
    mutationFn: (v: { uuid: string; mark: boolean }) =>
      v.mark
        ? taxReturnsService.setRent(slug, engagement, p.period, v.uuid)
        : taxReturnsService.deleteRent(slug, engagement, p.period, v.uuid),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.rent_saved"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.message
          ? e.message
          : t("office.tax_returns.rent_failed"),
      ),
  });
  return (
    <section
      aria-label={t("office.tax_returns.rent_title")}
      className="bg-surface border-line flex flex-col gap-3 rounded-xl border p-4 text-sm"
    >
      <div className="flex flex-col gap-1">
        <span className="font-semibold">
          {t("office.tax_returns.rent_title")}
        </span>
        <span className="text-text-2">{t("office.tax_returns.rent_hint")}</span>
      </div>
      <ul className="flex flex-col gap-2">
        {rows.map((r) => (
          <li
            key={r.uuid}
            className="border-line flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2"
          >
            <span className="tabular-nums">{day(r.transaction_date)}</span>
            <span className="min-w-0 flex-1 truncate">
              {r.counterparty_name || r.description}
            </span>
            <span className="text-text-2">
              {t("office.tax_returns.rent_net")} <Money value={r.net} />
            </span>
            {r.marked ? (
              <>
                <span className="text-text-2">
                  {t("office.tax_returns.rent_gross")} <Money value={r.gross} />
                </span>
                <StatusPill tone="info">
                  {t("office.tax_returns.rent_tax", { rate: r.rate })}{" "}
                  <Money value={r.tax} />
                </StatusPill>
              </>
            ) : (
              <StatusPill tone="warn">
                {t("office.tax_returns.rent_unmarked")}
              </StatusPill>
            )}
            {editable ? (
              <Button
                size="sm"
                variant={r.marked ? "outline" : "default"}
                className="max-md:min-h-11"
                disabled={save.isPending}
                onClick={() => save.mutate({ uuid: r.uuid, mark: !r.marked })}
              >
                {r.marked
                  ? t("office.tax_returns.rent_remove")
                  : t("office.tax_returns.rent_add")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
