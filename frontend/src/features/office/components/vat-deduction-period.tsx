"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useEngagementInvoice } from "../hooks/use-office-invoices";
import {
  officeInvoiceKeys,
  officeInvoiceService,
  type OfficeInvoiceDetail,
} from "../services/office-invoices.service";

const INVOICES_WRITE = "tenant.invoices.write";

/** "2026-03" → bir sonraki ay ("2026-04"). */
export function nextPeriod(period: string) {
  const [y, m] = period.split("-").map(Number);
  return m === 12 ? `${y + 1}-01` : `${y}-${String(m + 1).padStart(2, "0")}`;
}

/**
 * KDV indirim döneminin seçilebileceği aralık (#640, KDVK 29/3): belge
 * ayından sonraki ay ile izleyen yılın Aralık'ı arası.
 */
export function vatDeductionRange(docPeriod: string) {
  return {
    min: nextPeriod(docPeriod),
    max: `${Number(docPeriod.slice(0, 4)) + 1}-12`,
  };
}

function monthText(period: string, locale: string) {
  const d = new Date(`${period}-01T00:00:00`);
  if (Number.isNaN(d.getTime())) return period;
  return d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "long",
    year: "numeric",
  });
}

/**
 * Alış faturasının KDV indirim dönemi (#640): geç gelen faturanın KDV'si
 * belge tarihi değişmeden sonraki bir ayın beyanına alınır. Fatura kendi
 * ayında kalır, "şu aya alındı" notu düşer. Değişiklik geri alınabilir;
 * kilitli ayda sunucu reddeder.
 */
export function VatDeductionPeriod({
  engagement,
  invoice: d,
}: {
  engagement: string;
  invoice: OfficeInvoiceDetail;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const range = vatDeductionRange(d.period);
  const current = d.vat_deduction_period ?? null;
  const [value, setValue] = useState(current ?? range.min);

  // Son değişikliğin geri alınması kutunun içinde (#640): çekmece açıkken
  // bildirimdeki düğmeye tıklanamıyor, çekmece önünü kapatıyor.
  const [undo, setUndo] = useState<{ previous: string | null } | null>(null);

  const save = useMutation({
    mutationFn: (v: { period: string | null; undoing?: boolean }) =>
      officeInvoiceService.setVatDeductionPeriod(
        slug,
        engagement,
        d.uuid,
        v.period,
      ),
    onSuccess: (next, v) => {
      qc.setQueryData(
        officeInvoiceKeys.invoice(slug, engagement, d.uuid),
        next,
      );
      // Liste, KDV tahmini ve beyan özetleri bu mükellefin anahtarları altında.
      void qc.invalidateQueries({
        queryKey: [
          "tenant",
          slug.toLowerCase(),
          "engagements",
          engagement.toLowerCase(),
        ],
      });
      setEditing(false);
      setUndo(v.undoing ? null : { previous: current });
      toast.success(
        v.period
          ? t("office.vat_deduction.saved", {
              month: monthText(v.period, locale),
            })
          : t("office.vat_deduction.cleared"),
      );
    },
    onError: (err) =>
      toast.error(err.message || t("office.vat_deduction.error")),
  });

  if (d.direction !== "incoming") return null;
  const writable = can(INVOICES_WRITE);
  if (!current && !writable) return null;

  return (
    <section
      aria-labelledby={`vat-deduction-${d.uuid}`}
      className="border-line space-y-2 rounded-lg border p-3 text-sm"
    >
      <h3 id={`vat-deduction-${d.uuid}`} className="font-semibold">
        {t("office.vat_deduction.title")}
      </h3>
      {current ? (
        <p data-testid="vat-deduction-note">
          {t("office.vat_deduction.moved", {
            month: monthText(current, locale),
            doc: monthText(d.period, locale),
          })}
          {d.vat_deduction_entry ? (
            <span className="text-text-2">
              {" "}
              {t("office.vat_deduction.by", {
                name:
                  d.vat_deduction_entry.set_by_name ||
                  t("office.drawer.fx_entered_unknown"),
                date: new Date(d.vat_deduction_entry.set_at).toLocaleDateString(
                  locale === "tr" ? "tr-TR" : "en-GB",
                  { day: "numeric", month: "short", year: "numeric" },
                ),
              })}
            </span>
          ) : null}
        </p>
      ) : (
        <p className="text-text-2 text-xs">
          {t("office.vat_deduction.default", {
            doc: monthText(d.period, locale),
          })}
        </p>
      )}
      {writable ? (
        editing ? (
          <form
            className="flex flex-wrap items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (value) save.mutate({ period: value });
            }}
          >
            <Input
              type="month"
              className="h-9 w-44"
              min={range.min}
              max={range.max}
              aria-label={t("office.vat_deduction.label")}
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
            <Button
              type="submit"
              size="sm"
              disabled={!value || value === current || save.isPending}
            >
              {t("office.vat_deduction.save")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setEditing(false)}
            >
              {t("office.vat_deduction.cancel")}
            </Button>
            <p className="text-text-3 w-full text-xs">
              {t("office.vat_deduction.hint", {
                max: monthText(range.max, locale),
              })}
            </p>
          </form>
        ) : (
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setValue(current ?? range.min);
                setEditing(true);
              }}
            >
              {t("office.vat_deduction.change")}
            </Button>
            {undo ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={save.isPending}
                onClick={() =>
                  save.mutate({ period: undo.previous, undoing: true })
                }
              >
                {t("office.vat_deduction.undo")}
              </Button>
            ) : null}
            {current ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={save.isPending}
                onClick={() => save.mutate({ period: null })}
              >
                {t("office.vat_deduction.reset")}
              </Button>
            ) : null}
          </div>
        )
      ) : null}
    </section>
  );
}

/**
 * Uyarı panelinde (#640): fatura ya da kalem uyarısı bir alış faturasına
 * aitse indirim dönemi oradan da değiştirilir.
 */
export function WarningVatDeduction({
  engagement,
  invoice,
}: {
  engagement: string;
  invoice: string;
}) {
  const { slug } = useTenant();
  const detail = useEngagementInvoice(slug, engagement, invoice);
  if (!detail.data || detail.data.direction !== "incoming") return null;
  return <VatDeductionPeriod engagement={engagement} invoice={detail.data} />;
}
