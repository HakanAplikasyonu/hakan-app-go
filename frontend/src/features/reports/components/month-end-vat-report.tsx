"use client";

import { Landmark } from "lucide-react";
import Link from "next/link";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { routes } from "@/config/routes";
import { useOfficeVATForecast } from "@/features/office/hooks/use-office-data";
import {
  useSaveVATOpening,
  useVATTrackingSettings,
} from "@/features/office/hooks/use-vat-tracking";
import type { VATForecastPage } from "@/features/office/services/office-summary.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { formatMoney, parseAmountInput } from "@/lib/decimal";
import { useLocale } from "@/providers/locale-provider";

import { monthLabel } from "./report-range-picker";

type Item = VATForecastPage["items"][number];

function dayLabel(iso: string, locale: string) {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { day: "numeric", month: "long", timeZone: "UTC" },
  );
}

function sum(items: Item[], pick: (i: Item) => string) {
  // Kuruş cinsinden topla: kayan nokta hatası olmasın.
  let minor = BigInt(0);
  for (const i of items) {
    const [int, frac = ""] = pick(i).split(".");
    const neg = int.startsWith("-");
    const v =
      BigInt(int.replace("-", "")) * BigInt(100) +
      BigInt((frac + "00").slice(0, 2));
    minor += neg ? -v : v;
  }
  const neg = minor < BigInt(0);
  const abs = neg ? -minor : minor;
  const s = `${abs / BigInt(100)}.${String(abs % BigInt(100)).padStart(2, "0")}`;
  return neg ? `-${s}` : s;
}

/**
 * Ay sonu KDV raporu (#780 B): bütün mükelleflerin bu ayki satış ve alış
 * KDV'si, devreden KDV ve ödenecek / devredecek tahmini. Ofis ayarında
 * kapatılabilir; ayın seçilen gününden (varsayılan 29) itibaren "hazır".
 * Devreden KDV'yi ofis satırdan tek tıkla girer.
 */
export function MonthEndVATReport({ canWrite }: { canWrite: boolean }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const settings = useVATTrackingSettings(slug);
  const readyOn = settings.data?.report_ready_on;
  const period = readyOn?.slice(0, 7);
  const enabled = settings.data?.report_enabled === true && !!period;
  const forecast = useOfficeVATForecast(period, enabled);
  const [opening, setOpening] = useState<Item | null>(null);

  if (settings.data && !settings.data.report_enabled) return null;

  const items = forecast.data?.items ?? [];
  const today = new Date().toLocaleDateString("en-CA", {
    timeZone: "Europe/Istanbul",
  });
  const ready = !!readyOn && today >= readyOn;
  const payers = items.filter((i) => Number(i.totals.payable) > 0).length;
  const money = (v: string) => (
    <span className="whitespace-nowrap tabular-nums">{formatMoney(v)}</span>
  );

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-2 text-base">
          <Landmark aria-hidden className="size-4" />
          {period
            ? t("reports.vat.title", { month: monthLabel(period, locale) })
            : t("reports.vat.title_plain")}
        </CardTitle>
        <p className="text-text-2 text-sm">{t("reports.vat.why")}</p>
        {readyOn ? (
          <p role="status" className="text-sm font-medium">
            {ready
              ? t("reports.vat.ready", {
                  date: dayLabel(readyOn, locale),
                  count: payers,
                })
              : t("reports.vat.not_ready", {
                  date: dayLabel(readyOn, locale),
                })}
          </p>
        ) : null}
      </CardHeader>
      <CardContent>
        {!forecast.data && (forecast.isLoading || settings.isLoading) ? (
          <Skeleton className="h-32 w-full" />
        ) : forecast.isError ? (
          <p role="alert" className="text-danger text-sm">
            {t("reports.vat.error")}
          </p>
        ) : items.length === 0 ? (
          <p className="text-text-2 text-sm">{t("reports.vat.empty")}</p>
        ) : (
          <Table aria-label={t("reports.vat.table")}>
            <TableHeader>
              <TableRow>
                <TableHead>{t("reports.vat.col_taxpayer")}</TableHead>
                <TableHead className="text-right">
                  {t("reports.vat.col_output")}
                </TableHead>
                <TableHead className="text-right">
                  {t("reports.vat.col_input")}
                </TableHead>
                <TableHead className="text-right">
                  {t("reports.vat.col_carried_in")}
                </TableHead>
                <TableHead className="text-right">
                  {t("reports.vat.col_payable")}
                </TableHead>
                <TableHead className="text-right">
                  {t("reports.vat.col_carry_out")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((i) => (
                <TableRow key={i.engagement_uuid}>
                  <TableCell>
                    <Link
                      className="font-medium hover:underline"
                      href={routes.tenant.engagementReport(
                        slug,
                        i.engagement_uuid,
                      )}
                    >
                      {i.taxpayer_title}
                    </Link>
                  </TableCell>
                  <TableCell className="text-right">
                    {money(i.totals.output_vat)}
                  </TableCell>
                  <TableCell className="text-right">
                    {money(i.totals.input_vat)}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      {money(i.totals.carried_in)}
                      {canWrite ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={t("reports.vat.opening_for", {
                            name: i.taxpayer_title,
                          })}
                          onClick={() => setOpening(i)}
                        >
                          {t("reports.vat.opening")}
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      {i.checks.payable_limit_reached ? (
                        <Badge variant="warning">
                          {t("reports.vat.limit_reached")}
                        </Badge>
                      ) : null}
                      <span className="font-semibold">
                        {money(i.totals.payable)}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    {money(i.totals.carry_out)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell className="font-semibold">
                  {t("reports.vat.total")}
                </TableCell>
                <TableCell className="text-right">
                  {money(sum(items, (i) => i.totals.output_vat))}
                </TableCell>
                <TableCell className="text-right">
                  {money(sum(items, (i) => i.totals.input_vat))}
                </TableCell>
                <TableCell className="text-right">
                  {money(sum(items, (i) => i.totals.carried_in))}
                </TableCell>
                <TableCell className="text-right font-semibold">
                  {money(sum(items, (i) => i.totals.payable))}
                </TableCell>
                <TableCell className="text-right">
                  {money(sum(items, (i) => i.totals.carry_out))}
                </TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        )}
      </CardContent>
      {opening && period ? (
        <OpeningDialog
          item={opening}
          period={period}
          onClose={() => setOpening(null)}
        />
      ) : null}
    </Card>
  );
}

function OpeningDialog({
  item,
  period,
  onClose,
}: {
  item: Item;
  period: string;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const save = useSaveVATOpening(slug);
  const id = useId();
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  return (
    <Dialog open onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            const v = parseAmountInput(amount);
            if (!v) {
              setError(t("reports.vat.opening_invalid"));
              return;
            }
            save.mutate(
              {
                engagement: item.engagement_uuid,
                period,
                amount: v,
                note: note.trim(),
              },
              {
                onSuccess: () => {
                  toast.success(t("reports.vat.opening_saved"));
                  onClose();
                },
                onError: () => setError(t("reports.vat.opening_error")),
              },
            );
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {t("reports.vat.opening_title", { name: item.taxpayer_title })}
            </DialogTitle>
            <DialogDescription>
              {t("reports.vat.opening_hint", {
                month: monthLabel(period, locale),
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor={`${id}-amount`}>
              {t("reports.vat.opening_amount")}
            </Label>
            <Input
              id={`${id}-amount`}
              inputMode="decimal"
              placeholder="0,00"
              className="text-right tabular-nums"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`${id}-note`}>
              {t("reports.vat.opening_note")}
            </Label>
            <Input
              id={`${id}-note`}
              maxLength={500}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {error ? (
            <p role="alert" className="text-danger text-sm">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              {t("reports.vat.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("reports.vat.opening_save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
