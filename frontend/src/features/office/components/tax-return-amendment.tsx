"use client";

import { useMutation } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { useMemo } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  type TaxReturnAmendment,
  type TaxReturnKind,
  type TaxReturnPreview,
  taxReturnsService,
} from "../services/tax-returns.service";
import { Money } from "./office-ui";

/** Ba/Bs kişi sayıları tutar değil. */
const COUNT_KEYS = new Set(["ba_count", "bs_count"]);

/**
 * Düzeltme beyannamesi (#575). Verilmiş beyanın güncel verisi verildiği
 * halinden farklıysa (ofis kararı: satış değişti → düzeltme) uyarı ve
 * farklar; "Düzeltme beyannamesi başlat" verilmiş hali arşivleyip beyanı
 * taslağa döndürür. Düzeltme sürümünde önceki sürümle fark ve önceki
 * sürümler listelenir.
 */
export function TaxReturnAmendmentCard({
  p,
  amendment: a,
  canWrite,
  onSaved,
  engagement,
}: {
  p: TaxReturnPreview;
  amendment: TaxReturnAmendment;
  canWrite: boolean;
  onSaved: (p: TaxReturnPreview) => void;
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const day = useMemo(() => {
    const tag = locale === "tr" ? "tr-TR" : "en-GB";
    return (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(tag);
  }, [locale]);
  const amend = useMutation({
    mutationFn: () =>
      taxReturnsService.amend(
        slug,
        engagement,
        p.kind as TaxReturnKind,
        p.period,
      ),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.amend_started"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.message
          ? e.message
          : t("office.tax_returns.amend_failed"),
      ),
  });
  const filed = p.status === "filed";
  const showDiff = a.differences.length > 0 && (a.needed || a.version > 1);
  const value = (key: string, v: string) =>
    COUNT_KEYS.has(key) ? (
      <span className="tabular-nums">{Number(v).toLocaleString(locale)}</span>
    ) : (
      <Money value={v} />
    );
  return (
    <section
      aria-label={t("office.tax_returns.amend_title")}
      className={cn(
        "bg-surface flex flex-col gap-3 rounded-xl border p-4 text-sm",
        a.needed ? "border-warn" : "border-line",
      )}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className="font-semibold">
          {a.version > 1
            ? t("office.tax_returns.amend_version", { version: a.version })
            : t("office.tax_returns.amend_title")}
        </span>
        {a.previous ? (
          <span className="text-text-2">
            {t("office.tax_returns.amend_previous", {
              version: a.previous.version,
              date: day(a.previous.filed_on),
            })}
          </span>
        ) : null}
        {filed && canWrite ? (
          <Button
            size="sm"
            variant={a.needed ? "default" : "outline"}
            className="ms-auto max-md:min-h-11"
            disabled={amend.isPending}
            onClick={() => amend.mutate()}
          >
            {t("office.tax_returns.amend_start")}
          </Button>
        ) : null}
      </div>
      {a.needed ? (
        <p className="text-warn flex items-start gap-2">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
          {t("office.tax_returns.amend_needed")}
        </p>
      ) : null}
      {showDiff ? (
        <Table aria-label={t("office.tax_returns.amend_diff")}>
          <TableHeader>
            <TableRow>
              <TableHead>{t("office.tax_returns.amend_col_figure")}</TableHead>
              <TableHead className="text-end">
                {a.needed
                  ? t("office.tax_returns.amend_col_filed")
                  : t("office.tax_returns.amend_col_previous")}
              </TableHead>
              <TableHead className="text-end">
                {a.needed
                  ? t("office.tax_returns.amend_col_live")
                  : t("office.tax_returns.amend_col_current")}
              </TableHead>
              <TableHead className="text-end">
                {t("office.tax_returns.amend_col_diff")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {a.differences.map((d) => (
              <TableRow key={d.key}>
                <TableCell>{d.label}</TableCell>
                <TableCell className="text-end">
                  {value(d.key, d.previous)}
                </TableCell>
                <TableCell className="text-end">
                  {value(d.key, d.current)}
                </TableCell>
                <TableCell
                  className={cn(
                    "text-end",
                    Number(d.difference) !== 0 && "font-semibold",
                  )}
                >
                  {value(d.key, d.difference)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
      {a.versions.length > 0 ? (
        <div className="flex flex-col gap-1">
          <span className="text-text-2">
            {t("office.tax_returns.amend_versions")}
          </span>
          <ul className="flex flex-col gap-1">
            {a.versions.map((v) => (
              <li key={v.version} className="flex flex-wrap gap-2">
                <span className="font-medium">
                  {t("office.tax_returns.amend_version_short", {
                    version: v.version,
                  })}
                </span>
                <span className="text-text-2">
                  {t("office.tax_returns.amend_filed_on", {
                    date: day(v.filed_on),
                  })}
                  {v.accrual_number ? ` · ${v.accrual_number}` : ""}
                </span>
                {v.payable_amount ? <Money value={v.payable_amount} /> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}
