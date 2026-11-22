"use client";

import { Download, FileText, Plus } from "lucide-react";
import Link from "next/link";
import { Suspense, useState } from "react";
import { toast } from "sonner";

import { StatusPill } from "@/components/tables";
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
import { MonthPicker } from "@/components/ui/month-picker";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { formatMoney } from "@/lib/decimal";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  foreignInvoicesPath,
  useForeignInvoiceActions,
  useForeignInvoices,
  useForeignServiceKinds,
  type ForeignInvoice,
  type ForeignInvoiceTreatment,
  type ForeignServiceKind,
  type ForeignServiceKindSetting,
} from "../hooks/use-foreign-invoices";
import { periodText } from "../lib/change-request-text";
import { FOCUS_PARAM } from "../lib/related-href";
import { useUrlOfficePeriod } from "../period";
import { downloadFile } from "../services/download";
import { Panel } from "./office-ui";

const CURRENCIES = ["USD", "EUR", "GBP", "CHF", "TRY"] as const;

/** Bir TL tutarı; kur yoksa tire. */
function tl(value: string | null | undefined) {
  return value ? formatMoney(value, "TRY") : "—";
}

/**
 * Mükellef sekmesi: Yabancı faturalar / KDV2 (#780 A1). Yurt dışından alınan
 * hizmetlerin (Google Ads, AWS, yazılım lisansı) faturaları ay ay toplanır;
 * sorumlu sıfatıyla KDV (KDV2) ve gerekiyorsa stopaj burada hesaplanır, ofis
 * onaylar. Beyan Luca'da verilir.
 */
export function EngagementForeignInvoicesPage({
  engagement,
}: {
  engagement: string;
}) {
  // Dönem adreste de durur (?period=YYYY-MM); useSearchParams Suspense ister.
  return (
    <Suspense fallback={null}>
      <ForeignInvoicesPanel engagement={engagement} />
    </Suspense>
  );
}

function ForeignInvoicesPanel({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantTaxReturnsWrite);
  const canJournal = can(Permission.TenantJournalsWrite);
  const { focus: period, setPeriod } = useUrlOfficePeriod();
  const { createPeriodJournals } = useForeignInvoiceActions(engagement);
  const kinds = useForeignServiceKinds();
  const q = useForeignInvoices(engagement, period);
  const [adding, setAdding] = useState(false);
  const [downloading, setDownloading] = useState<string | null>(null);

  if (kinds.data && !kinds.data.enabled) {
    return (
      <Panel title={t("office.foreign.title")}>
        <p className="text-text-2 text-sm" data-testid="foreign-disabled">
          {t("office.foreign.disabled")}
        </p>
      </Panel>
    );
  }

  const download = async (format: "xlsx" | "pdf") => {
    setDownloading(format);
    try {
      await downloadFile(
        slug,
        `${foreignInvoicesPath(engagement)}/export`,
        { period, format },
        `yabanci-faturalar-${period}.${format}`,
      );
    } catch {
      toast.error(t("office.foreign.download_failed"));
    } finally {
      setDownloading(null);
    }
  };

  const items = q.data?.items ?? [];
  const totals = q.data?.totals;
  const month = periodText(period, locale);

  return (
    <div className="space-y-5">
      <Panel
        title={t("office.foreign.title")}
        description={t("office.foreign.description")}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <MonthPicker
              id="foreign-month"
              aria-label={t("office.foreign.month")}
              value={period}
              onChange={(v) => {
                if (v) setPeriod(v);
              }}
            />
            <Button
              variant="outline"
              size="sm"
              disabled={downloading !== null}
              onClick={() => void download("xlsx")}
            >
              <Download aria-hidden className="size-4" />
              {t("office.foreign.download_xlsx")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={downloading !== null}
              onClick={() => void download("pdf")}
            >
              <Download aria-hidden className="size-4" />
              {t("office.foreign.download_pdf")}
            </Button>
            {canWrite ? (
              <Button
                size="sm"
                disabled={!kinds.data}
                onClick={() => setAdding(true)}
              >
                <Plus aria-hidden className="size-4" />
                {t("office.foreign.add")}
              </Button>
            ) : null}
          </div>
        }
      >
        <div className="space-y-4 p-4">
          <p className="text-text-2 text-sm">{t("office.foreign.why")}</p>
          {totals ? (
            <section
              aria-labelledby="foreign-totals-title"
              data-testid="foreign-totals"
              className="bg-surface-2 rounded-lg border p-3"
            >
              <h3
                id="foreign-totals-title"
                className="mb-2 text-sm font-semibold"
              >
                {t("office.foreign.totals_title", { month })}
              </h3>
              <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
                <div>
                  <dt className="text-text-2">
                    {t("office.foreign.total_gross")}
                  </dt>
                  <dd
                    className="font-semibold tabular-nums"
                    data-testid="foreign-total-gross"
                  >
                    {tl(totals.gross_try)}
                  </dd>
                </div>
                <div>
                  <dt className="text-text-2">
                    {t("office.foreign.total_kdv2")}
                  </dt>
                  <dd
                    className="font-semibold tabular-nums"
                    data-testid="foreign-total-kdv2"
                  >
                    {tl(totals.kdv2_try)}
                  </dd>
                </div>
                <div>
                  <dt className="text-text-2">
                    {t("office.foreign.total_stopaj")}
                  </dt>
                  <dd
                    className="font-semibold tabular-nums"
                    data-testid="foreign-total-stopaj"
                  >
                    {tl(totals.stopaj_try)}
                  </dd>
                </div>
                <div>
                  <dt className="text-text-2">
                    {t("office.foreign.total_status")}
                  </dt>
                  <dd className="tabular-nums">
                    {t("office.foreign.total_counts", {
                      count: totals.count,
                      confirmed: totals.confirmed,
                    })}
                  </dd>
                </div>
              </dl>
              {totals.missing_rate > 0 ? (
                <p
                  className="text-warn mt-2 text-sm font-medium"
                  data-testid="foreign-missing-rate"
                >
                  {t("office.foreign.missing_rate_total", {
                    count: totals.missing_rate,
                  })}
                </p>
              ) : null}
              {totals.confirmed > 0 ? (
                <div
                  className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t pt-3"
                  data-testid="foreign-journal-summary"
                >
                  <p className="text-text-2 text-sm">
                    {totals.journal_pending > 0
                      ? t("office.foreign.journal_pending", {
                          count: totals.journal_pending,
                        })
                      : t("office.foreign.journal_all_done")}
                  </p>
                  {canJournal && totals.journal_pending > 0 ? (
                    <Button
                      size="sm"
                      disabled={createPeriodJournals.isPending}
                      onClick={() =>
                        createPeriodJournals.mutate(period, {
                          onSuccess: (res) => {
                            if (res.failed.length > 0) {
                              toast.error(
                                t("office.foreign.journal_bulk_failed", {
                                  created: res.created.length,
                                  failed: res.failed.length,
                                }),
                              );
                            } else {
                              toast.success(
                                t("office.foreign.journal_bulk_done", {
                                  count: res.created.length,
                                }),
                              );
                            }
                          },
                          onError: (err) =>
                            toast.error(
                              errorText(err, t("office.foreign.failed")),
                            ),
                        })
                      }
                    >
                      <FileText aria-hidden className="size-4" />
                      {t("office.foreign.journal_bulk")}
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </section>
          ) : null}
          {q.isLoading ? (
            <p className="text-text-3 text-sm">{t("common.loading")}</p>
          ) : q.isError ? (
            <p role="alert" className="text-danger text-sm">
              {q.error.message}
            </p>
          ) : items.length === 0 ? (
            <p className="text-text-3 text-sm" data-testid="foreign-empty">
              {t("office.foreign.empty", { month })}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table data-testid="foreign-invoices">
                <TableCaption className="sr-only">
                  {t("office.foreign.title")}
                </TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">
                      {t("office.foreign.col_vendor")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("office.foreign.col_invoice")}
                    </TableHead>
                    <TableHead scope="col" className="text-right">
                      {t("office.foreign.col_amount")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("office.foreign.col_kind")}
                    </TableHead>
                    <TableHead scope="col" className="text-right">
                      {t("office.foreign.col_gross")}
                    </TableHead>
                    <TableHead scope="col" className="text-right">
                      {t("office.foreign.col_kdv2")}
                    </TableHead>
                    <TableHead scope="col" className="text-right">
                      {t("office.foreign.col_stopaj")}
                    </TableHead>
                    <TableHead scope="col" className="text-right">
                      {t("office.foreign.col_try")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("office.foreign.col_status")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("office.foreign.col_journal")}
                    </TableHead>
                    <TableHead scope="col">
                      <span className="sr-only">
                        {t("office.foreign.col_action")}
                      </span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((it) => (
                    <InvoiceRow
                      key={it.uuid}
                      item={it}
                      engagement={engagement}
                      canWrite={canWrite}
                      canJournal={canJournal}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </Panel>
      {adding && kinds.data ? (
        <AddDialog
          engagement={engagement}
          period={period}
          kinds={kinds.data.items}
          onClose={() => setAdding(false)}
        />
      ) : null}
    </div>
  );
}

function errorText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/** Bir fatura satırı: kur eksikse satırda giriş; onayla / geri al / sil. */
function InvoiceRow({
  item,
  engagement,
  canWrite,
  canJournal,
}: {
  item: ForeignInvoice;
  engagement: string;
  canWrite: boolean;
  canJournal: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const {
    update,
    remove,
    confirm,
    unconfirm,
    createJournal,
    removeJournal,
  } = useForeignInvoiceActions(engagement);
  const [rate, setRate] = useState("");
  const busy =
    update.isPending ||
    remove.isPending ||
    confirm.isPending ||
    unconfirm.isPending ||
    createJournal.isPending ||
    removeJournal.isPending;
  const journal = item.journal;
  const confirmed = item.status === "confirmed";
  const fail = (err: unknown) =>
    toast.error(errorText(err, t("office.foreign.failed")));
  const date = item.invoice_date
    ? new Date(`${item.invoice_date}T00:00:00`).toLocaleDateString(
        locale === "tr" ? "tr-TR" : "en-GB",
      )
    : "—";
  return (
    <TableRow
      data-testid="foreign-invoice"
      data-status={item.status}
      data-missing-rate={item.missing_rate ? "true" : undefined}
    >
      <TableCell>
        <span className="font-medium">{item.vendor}</span>
        <span className="text-text-2 block text-xs">{item.country}</span>
      </TableCell>
      <TableCell>
        <span className="block">{item.invoice_no || "—"}</span>
        <span className="text-text-2 block text-xs">{date}</span>
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatMoney(item.amount, item.currency)}
        {item.exchange_rate && item.currency !== "TRY" ? (
          <span className="text-text-2 block text-xs">
            {t("office.foreign.rate_value", {
              rate: item.exchange_rate.replace(".", ","),
            })}
          </span>
        ) : null}
      </TableCell>
      <TableCell>
        <span className="block">{item.service_kind_label}</span>
        <span className="text-text-2 block text-xs">
          {item.treatment === "kdv2_stopaj"
            ? t(
                item.gross_up
                  ? "office.foreign.treatment_stopaj_grossup"
                  : "office.foreign.treatment_stopaj",
                { rate: item.stopaj_rate.replace(".", ",") },
              )
            : t("office.foreign.treatment_kdv2")}
        </span>
      </TableCell>
      <TableCell
        className="text-right tabular-nums"
        data-testid="foreign-gross"
      >
        {formatMoney(item.gross, item.currency)}
      </TableCell>
      <TableCell className="text-right tabular-nums" data-testid="foreign-kdv2">
        {formatMoney(item.kdv2, item.currency)}
      </TableCell>
      <TableCell
        className="text-right tabular-nums"
        data-testid="foreign-stopaj"
      >
        {item.treatment === "kdv2_stopaj"
          ? formatMoney(item.stopaj, item.currency)
          : "—"}
      </TableCell>
      <TableCell className="text-right tabular-nums" data-testid="foreign-try">
        {item.missing_rate ? (
          canWrite && !confirmed ? (
            <form
              className="flex items-center justify-end gap-1"
              onSubmit={(e) => {
                e.preventDefault();
                if (!rate.trim()) return;
                update.mutate(
                  { uuid: item.uuid, body: { exchange_rate: rate.trim() } },
                  {
                    onSuccess: () =>
                      toast.success(t("office.foreign.rate_saved")),
                    onError: fail,
                  },
                );
              }}
            >
              <Input
                aria-label={t("office.foreign.rate_for", {
                  vendor: item.vendor,
                })}
                inputMode="decimal"
                placeholder={t("office.foreign.rate_placeholder")}
                className="h-8 w-24"
                value={rate}
                onChange={(e) => setRate(e.target.value)}
              />
              <Button size="sm" type="submit" disabled={busy || !rate.trim()}>
                {t("office.foreign.rate_save")}
              </Button>
            </form>
          ) : (
            <StatusPill tone="warn">
              {t("office.foreign.missing_rate")}
            </StatusPill>
          )
        ) : (
          <span className="block">
            <span className="block">
              {t("office.foreign.try_gross", { value: tl(item.gross_try) })}
            </span>
            <span className="text-text-2 block text-xs">
              {t("office.foreign.try_kdv2", { value: tl(item.kdv2_try) })}
            </span>
            {item.treatment === "kdv2_stopaj" ? (
              <span className="text-text-2 block text-xs">
                {t("office.foreign.try_stopaj", {
                  value: tl(item.stopaj_try),
                })}
              </span>
            ) : null}
          </span>
        )}
      </TableCell>
      <TableCell>
        <StatusPill tone={confirmed ? "ok" : "neutral"}>
          {confirmed
            ? t("office.foreign.status_confirmed")
            : t("office.foreign.status_draft")}
        </StatusPill>
        {item.missing_rate ? (
          <span className="text-warn block text-xs">
            {t("office.foreign.confirm_needs_rate")}
          </span>
        ) : null}
      </TableCell>
      <TableCell data-testid="foreign-journal" data-journal={journal?.status}>
        {journal ? (
          <span className="flex flex-col items-start gap-1">
            <Link
              href={`${routes.tenant.engagementJournals(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`journal_draft:${journal.uuid}`)}`}
              className="text-brand-text font-mono text-sm hover:underline"
            >
              {t("office.foreign.journal_no", { no: journal.entry_no })}
            </Link>
            <StatusPill tone={journal.status === "draft" ? "neutral" : "ok"}>
              {t(`office.foreign.journal_status_${journal.status}`)}
            </StatusPill>
            {canJournal && journal.status === "draft" ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                aria-label={t("office.foreign.journal_remove_for", {
                  vendor: item.vendor,
                })}
                onClick={() =>
                  removeJournal.mutate(item.uuid, {
                    onSuccess: () =>
                      toast.success(t("office.foreign.journal_removed")),
                    onError: fail,
                  })
                }
              >
                {t("office.foreign.journal_remove")}
              </Button>
            ) : null}
          </span>
        ) : confirmed && canJournal ? (
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            aria-label={t("office.foreign.journal_create_for", {
              vendor: item.vendor,
            })}
            onClick={() =>
              createJournal.mutate(item.uuid, {
                onSuccess: (res) =>
                  toast.success(
                    res.rounding !== "0.00"
                      ? t("office.foreign.journal_created_rounding", {
                          no: res.journal.entry_no,
                          value: tl(res.rounding.replace("-", "")),
                        })
                      : t("office.foreign.journal_created", {
                          no: res.journal.entry_no,
                        }),
                  ),
                onError: fail,
              })
            }
          >
            {t("office.foreign.journal_create")}
          </Button>
        ) : (
          <span className="text-text-3 text-xs">
            {confirmed ? "—" : t("office.foreign.journal_after_confirm")}
          </span>
        )}
      </TableCell>
      <TableCell className="text-right">
        {canWrite ? (
          <span className="flex flex-wrap justify-end gap-1">
            {confirmed && journal ? (
              <span className="text-text-2 max-w-40 text-xs">
                {t("office.foreign.unconfirm_needs_journal_removed")}
              </span>
            ) : confirmed ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                aria-label={t("office.foreign.unconfirm_for", {
                  vendor: item.vendor,
                })}
                onClick={() =>
                  unconfirm.mutate(item.uuid, {
                    onSuccess: () =>
                      toast.success(t("office.foreign.unconfirmed")),
                    onError: fail,
                  })
                }
              >
                {t("office.foreign.unconfirm")}
              </Button>
            ) : (
              <>
                <Button
                  size="sm"
                  disabled={busy || item.missing_rate}
                  aria-label={t("office.foreign.confirm_for", {
                    vendor: item.vendor,
                  })}
                  onClick={() =>
                    confirm.mutate(item.uuid, {
                      onSuccess: () =>
                        toast.success(t("office.foreign.confirmed")),
                      onError: fail,
                    })
                  }
                >
                  {t("office.foreign.confirm")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  aria-label={t("office.foreign.delete_for", {
                    vendor: item.vendor,
                  })}
                  onClick={() =>
                    remove.mutate(item.uuid, {
                      onSuccess: () =>
                        toast.success(t("office.foreign.deleted")),
                      onError: fail,
                    })
                  }
                >
                  {t("office.foreign.delete")}
                </Button>
              </>
            )}
          </span>
        ) : null}
      </TableCell>
    </TableRow>
  );
}

/** "Fatura ekle": işlem türü seçilince ofis varsayılanları dolar. */
function AddDialog({
  engagement,
  period,
  kinds,
  onClose,
}: {
  engagement: string;
  period: string;
  kinds: ForeignServiceKindSetting[];
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { create } = useForeignInvoiceActions(engagement);
  const usable = kinds.filter((k) => k.enabled);
  const first = usable[0] ?? kinds[0];
  const [vendor, setVendor] = useState("");
  const [country, setCountry] = useState("");
  const [invoiceNo, setInvoiceNo] = useState("");
  const [invoiceDate, setInvoiceDate] = useState("");
  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState<string>("USD");
  const [rate, setRate] = useState("");
  const [kind, setKind] = useState<ForeignServiceKind>(first.service_kind);
  const [treatment, setTreatment] = useState<ForeignInvoiceTreatment>(
    first.treatment,
  );
  const [stopajRate, setStopajRate] = useState(first.stopaj_rate);
  const [grossUp, setGrossUp] = useState(first.gross_up);
  const [vatRate, setVatRate] = useState(first.vat_rate);
  const [note, setNote] = useState("");

  const pickKind = (value: ForeignServiceKind) => {
    setKind(value);
    const k = kinds.find((x) => x.service_kind === value);
    if (!k) return;
    setTreatment(k.treatment);
    setStopajRate(k.stopaj_rate);
    setGrossUp(k.gross_up);
    setVatRate(k.vat_rate);
  };

  const submit = () =>
    create.mutate(
      {
        period,
        vendor,
        country: country.trim().toUpperCase(),
        invoice_no: invoiceNo,
        invoice_date: invoiceDate,
        amount: amount.trim(),
        currency,
        exchange_rate: currency === "TRY" || !rate.trim() ? null : rate.trim(),
        service_kind: kind,
        treatment,
        stopaj_rate: treatment === "kdv2_stopaj" ? stopajRate : "0",
        gross_up: treatment === "kdv2_stopaj" && grossUp,
        vat_rate: vatRate,
        note,
      },
      {
        onSuccess: () => {
          toast.success(t("office.foreign.added"));
          onClose();
        },
        onError: (err) =>
          toast.error(errorText(err, t("office.foreign.failed"))),
      },
    );

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && !create.isPending && onClose()}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office.foreign.add_title")}</DialogTitle>
          <DialogDescription>{t("office.foreign.add_hint")}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="fi-vendor">
                {t("office.foreign.field_vendor")}
              </Label>
              <Input
                id="fi-vendor"
                required
                value={vendor}
                onChange={(e) => setVendor(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-country">
                {t("office.foreign.field_country")}
              </Label>
              <Input
                id="fi-country"
                required
                maxLength={2}
                placeholder="IE"
                value={country}
                onChange={(e) => setCountry(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-no">
                {t("office.foreign.field_invoice_no")}
              </Label>
              <Input
                id="fi-no"
                value={invoiceNo}
                onChange={(e) => setInvoiceNo(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-date">
                {t("office.foreign.field_invoice_date")}
              </Label>
              <DatePicker
                id="fi-date"
                value={invoiceDate}
                onChange={setInvoiceDate}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-amount">
                {t("office.foreign.field_amount")}
              </Label>
              <Input
                id="fi-amount"
                required
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-currency">
                {t("office.foreign.field_currency")}
              </Label>
              <NativeSelect
                id="fi-currency"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              >
                {CURRENCIES.map((c) => (
                  <NativeSelectOption key={c} value={c}>
                    {c}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="fi-rate">{t("office.foreign.field_rate")}</Label>
              <Input
                id="fi-rate"
                inputMode="decimal"
                disabled={currency === "TRY"}
                placeholder={
                  currency === "TRY"
                    ? "1"
                    : t("office.foreign.field_rate_placeholder")
                }
                value={currency === "TRY" ? "" : rate}
                onChange={(e) => setRate(e.target.value)}
              />
              <p className="text-text-3 text-xs">
                {t("office.foreign.field_rate_hint")}
              </p>
            </div>
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="fi-kind">{t("office.foreign.field_kind")}</Label>
              <NativeSelect
                id="fi-kind"
                value={kind}
                onChange={(e) => pickKind(e.target.value as ForeignServiceKind)}
              >
                {usable.map((k) => (
                  <NativeSelectOption
                    key={k.service_kind}
                    value={k.service_kind}
                  >
                    {k.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-treatment">
                {t("office.foreign.field_treatment")}
              </Label>
              <NativeSelect
                id="fi-treatment"
                value={treatment}
                onChange={(e) =>
                  setTreatment(e.target.value as ForeignInvoiceTreatment)
                }
              >
                <NativeSelectOption value="kdv2">
                  {t("office.foreign.treatment_kdv2")}
                </NativeSelectOption>
                <NativeSelectOption value="kdv2_stopaj">
                  {t("office.foreign.treatment_kdv2_stopaj")}
                </NativeSelectOption>
              </NativeSelect>
            </div>
            <div className="space-y-1">
              <Label htmlFor="fi-vat">
                {t("office.foreign.field_vat_rate")}
              </Label>
              <Input
                id="fi-vat"
                inputMode="decimal"
                value={vatRate}
                onChange={(e) => setVatRate(e.target.value)}
              />
            </div>
            {treatment === "kdv2_stopaj" ? (
              <>
                <div className="space-y-1">
                  <Label htmlFor="fi-stopaj">
                    {t("office.foreign.field_stopaj_rate")}
                  </Label>
                  <Input
                    id="fi-stopaj"
                    inputMode="decimal"
                    value={stopajRate}
                    onChange={(e) => setStopajRate(e.target.value)}
                  />
                </div>
                <div className="flex items-center gap-2 self-end pb-2">
                  <Checkbox
                    id="fi-grossup"
                    checked={grossUp}
                    onCheckedChange={(v) => setGrossUp(v === true)}
                  />
                  <Label htmlFor="fi-grossup">
                    {t("office.foreign.field_gross_up")}
                  </Label>
                </div>
              </>
            ) : null}
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="fi-note">{t("office.foreign.field_note")}</Label>
              <Input
                id="fi-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
          </div>
          <p className="text-text-3 text-xs">
            {t("office.foreign.gross_up_hint")}
          </p>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              disabled={create.isPending}
              onClick={onClose}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {t("office.foreign.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
