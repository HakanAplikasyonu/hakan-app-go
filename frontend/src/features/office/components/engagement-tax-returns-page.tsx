"use client";

import Link from "next/link";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  Download,
  FileCheck2,
  FileText,
  FileUp,
  Info,
  Landmark,
  RotateCcw,
} from "lucide-react";
import {
  type ReactNode,
  Suspense,
  useCallback,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { toast } from "sonner";

import { Hint } from "@/components/ui/hint";
import { ErrorState } from "@/components/common/error-state";
import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
  StatusPill,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { FilterChip } from "@/components/ui/filter-chip";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useUrlOfficePeriod } from "../period";
import {
  TAX_RETURN_KINDS,
  type TaxReturnAdjustments,
  type TaxReturnBABS,
  type TaxReturnBABSRow,
  type TaxReturnCheckItem,
  type TaxReturnEmployee,
  type TaxReturnKDV1,
  type TaxReturnKind,
  type TaxReturnMUHSGK,
  type TaxReturnPreview,
  type TaxReturnSummary,
  taxReturnKeys,
  taxReturnsService,
} from "../services/tax-returns.service";
import { TaxReturnRentPayments } from "./tax-return-rent";
import { TaxReturnAmendmentCard } from "./tax-return-amendment";
import { TaxReturnPaymentCard } from "./tax-return-payment";
import { TaxReturnSections } from "./tax-return-sections";
import { TaxReturnWarnings } from "./tax-return-warnings";
import { Money } from "./office-ui";
import { SummaryCards } from "./receipts-ui";
import { useEngagementScope } from "../engagement-scope";
import { checklistHref } from "../lib/related-href";
import { useTaxReturnSettings } from "../hooks/use-tax-return-settings";

/** Form alanı etiketi: soluk, normal kalınlıkta (önceki görünümle aynı). */
const FIELD_LABEL = "text-text-2 leading-normal font-normal";

const statusVariants: Record<string, StatusVariant> = {
  draft: { labelKey: "office.tax_returns.status_draft", tone: "neutral" },
  prepared: { labelKey: "office.tax_returns.status_prepared", tone: "info" },
  filed: { labelKey: "office.tax_returns.status_filed", tone: "ok" },
};

const kindVariants: Record<string, StatusVariant> = Object.fromEntries(
  TAX_RETURN_KINDS.map((k) => [
    k,
    { labelKey: `office.tax_returns.kind_${k}`, tone: "neutral" as const },
  ]),
);

const tableFeatures = {
  sorting: false,
  globalFilter: false,
  columnFilters: false,
  rowSelection: false,
  facetedFilters: false,
  viewMode: false,
} as const;

function errText(e: unknown, fallback: string) {
  return isApiError(e) && e.message ? e.message : fallback;
}

function useDates() {
  const { locale } = useLocale();
  return useMemo(() => {
    const tag = locale === "tr" ? "tr-TR" : "en-GB";
    return {
      day: (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(tag),
      month: (p: string) =>
        new Date(`${p}-01T00:00:00`).toLocaleDateString(tag, {
          month: "long",
          year: "numeric",
        }),
    };
  }, [locale]);
}

function rateLabel(rate: string, unknown: string) {
  return rate === "" ? unknown : `%${rate}`;
}

/**
 * `/t/{slug}/engagements/{uuid}/tax-returns` (#390): dönemin KDV1, MUHSGK ve
 * Ba/Bs beyanı form düzeninde. Dönem üst çubuktan, beyan türü URL'de
 * (`?kind=`). Taslak güncel veriden hesaplanır; kontrol listesindeki uyarılar
 * onaylanınca hazırlanır (donmuş görüntü), sonra verildi olarak işaretlenir.
 * Excel çıktısı Luca / Zirve'ye aktarım için.
 */
export function EngagementTaxReturnsPage({
  engagement,
}: {
  engagement: string;
}) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <TaxReturnsInner engagement={engagement} />
    </Suspense>
  );
}

function TaxReturnsInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantTaxReturnsWrite);
  // Son kontrol (#669): kontrol edildi / verildi yalnız sorumlu (sahip,
  // yönetici); ofis ayarı kapalıysa hazırlayan da işaretler (#710).
  const settings = useTaxReturnSettings(slug, canWrite);
  const canApprove =
    can(Permission.TenantTaxReturnsApprove) ||
    (canWrite && settings.data?.final_check_required === false);
  const qc = useQueryClient();
  // Beyan tek döneme aittir: "Tüm dönemler"de beyan dönemi (önceki ay).
  // Dönem adreste de durur (#710): bağlantıyla gelen `?period=` geçerli.
  const { focus: period, setPeriod } = useUrlOfficePeriod();
  const dates = useDates();
  const listState = useUrlListState({ initialPageSize: 50 });
  const rawKind = listState.getParam("kind") as TaxReturnKind | null;
  const kind: TaxReturnKind =
    rawKind && TAX_RETURN_KINDS.includes(rawKind) ? rawKind : "kdv1";

  const key = taxReturnKeys.one(slug, engagement, kind, period);
  const preview = useQuery({
    queryKey: key,
    queryFn: () => taxReturnsService.get(slug, engagement, kind, period),
  });
  const history = useQuery({
    queryKey: [...taxReturnKeys.all(slug, engagement), "list"],
    queryFn: () => taxReturnsService.list(slug, engagement),
  });
  const onSaved = (p: TaxReturnPreview) => {
    qc.setQueryData(key, p);
    void qc.invalidateQueries({
      queryKey: taxReturnKeys.all(slug, engagement),
    });
  };

  // Bir sorun düzelince beyan yeniden hesaplanır (#528).
  const recompute = useCallback(() => {
    void qc.invalidateQueries({ queryKey: key });
  }, [qc, key]);

  const p = preview.data;
  const isDraft = !p || p.status === "draft";
  const editable = canWrite && isDraft;
  const acks = p?.record?.acknowledged ?? [];
  const adjustments: TaxReturnAdjustments = p?.record?.adjustments ?? {};

  const save = useMutation({
    mutationFn: (body: {
      acknowledged?: string[];
      adjustments?: TaxReturnAdjustments;
    }) =>
      taxReturnsService.saveDraft(slug, engagement, kind, period, {
        acknowledged: body.acknowledged ?? acks,
        adjustments: kind === "kdv1" ? (body.adjustments ?? adjustments) : {},
        note: p?.record?.note ?? null,
      }),
    onSuccess: onSaved,
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.save_failed"))),
  });
  const prepare = useMutation({
    mutationFn: () =>
      taxReturnsService.prepare(slug, engagement, kind, period, acks),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.prepared"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.prepare_failed"))),
  });
  const reopen = useMutation({
    mutationFn: () => taxReturnsService.reopen(slug, engagement, kind, period),
    onSuccess: onSaved,
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.save_failed"))),
  });
  const checkReturn = useMutation({
    mutationFn: () => taxReturnsService.check(slug, engagement, kind, period),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.checked"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.save_failed"))),
  });
  const file = useMutation({
    mutationFn: (v: { filed_on: string; accrual_number: string }) =>
      taxReturnsService.file(slug, engagement, kind, period, {
        filed_on: v.filed_on,
        accrual_number: v.accrual_number || null,
      }),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.filed"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.file_failed"))),
  });

  const toggleAck = (code: string, on: boolean) =>
    save.mutate({
      acknowledged: on ? [...acks, code] : acks.filter((a) => a !== code),
    });

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display truncate text-xl font-bold">
            {t("office.tax_returns.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.tax_returns.description")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            disabled={!p}
            onClick={() =>
              void taxReturnsService
                .exportFile(slug, engagement, kind, period)
                .catch(() => toast.error(t("office.tax_returns.export_failed")))
            }
          >
            <Download className="size-4" aria-hidden />
            {t("office.tax_returns.export")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            disabled={!p}
            onClick={() =>
              void taxReturnsService
                .exportFile(slug, engagement, kind, period, "pdf")
                .catch(() =>
                  toast.error(t("office.tax_returns.export_pdf_failed")),
                )
            }
          >
            <FileText className="size-4" aria-hidden />
            {t("office.tax_returns.export_pdf")}
          </Button>
        </div>
      </div>

      <div
        role="group"
        aria-label={t("office.tax_returns.kinds")}
        className="flex flex-wrap gap-2"
      >
        {TAX_RETURN_KINDS.map((k) => (
          <FilterChip
            key={k}
            pressed={kind === k}
            onPressedChange={() =>
              listState.setParams({ kind: k === "kdv1" ? null : k })
            }
          >
            {t(`office.tax_returns.kind_${k}`)}
          </FilterChip>
        ))}
      </div>

      {preview.isLoading ? (
        <div className="space-y-3" aria-busy>
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : preview.isError || !p ? (
        <ErrorState
          title={t("office.api.load_error")}
          description={preview.error?.message}
          onRetry={() => void preview.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <>
          <StatusBar
            p={p}
            periodLabel={dates.month(period)}
            dueLabel={dates.day(p.calendar.due_date)}
            filedLabel={
              p.record?.filed_on ? dates.day(p.record.filed_on) : undefined
            }
            canWrite={canWrite}
            canApprove={canApprove}
            busy={
              prepare.isPending ||
              reopen.isPending ||
              file.isPending ||
              checkReturn.isPending
            }
            onCheck={() => checkReturn.mutate()}
            onPrepare={() => prepare.mutate()}
            onReopen={() => reopen.mutate()}
            onFile={(v) => file.mutate(v)}
          />
          {p.status === "filed" ? (
            <AccrualDocument
              p={p}
              canWrite={canWrite}
              onSaved={onSaved}
              engagement={engagement}
            />
          ) : null}
          {p.payment ? (
            <TaxReturnPaymentCard
              p={p}
              payment={p.payment}
              canWrite={canWrite}
              onSaved={onSaved}
              engagement={engagement}
            />
          ) : null}
          {p.amendment ? (
            <TaxReturnAmendmentCard
              p={p}
              amendment={p.amendment}
              canWrite={canWrite}
              onSaved={onSaved}
              engagement={engagement}
            />
          ) : null}
          <TaxReturnSummary p={p} />
          <TaxReturnSections
            slug={slug}
            engagement={engagement}
            period={period}
            sections={p.sections}
          />
          {p.status === "draft" ? (
            <TaxReturnWarnings
              engagement={engagement}
              period={period}
              codes={p.open_warnings}
              onChanged={recompute}
            />
          ) : null}
          <Checklist
            items={p.checklist}
            editable={editable}
            busy={save.isPending}
            onToggle={toggleAck}
          />
          {p.kdv1 ? (
            <KDV1View
              k={p.kdv1}
              editable={editable}
              busy={save.isPending}
              onAdjust={(a) => save.mutate({ adjustments: a })}
            />
          ) : null}
          {p.muhsgk ? <MUHSGKView m={p.muhsgk} /> : null}
          {p.muhsgk?.rent_payments?.length ? (
            <TaxReturnRentPayments
              p={p}
              rows={p.muhsgk.rent_payments}
              editable={editable}
              onSaved={onSaved}
              engagement={engagement}
            />
          ) : null}
          {p.babs ? <BABSView b={p.babs} /> : null}
        </>
      )}

      <History
        items={history.data?.items ?? []}
        loading={history.isLoading}
        error={history.isError}
        onRetry={() => void history.refetch()}
        onPick={(x) => {
          setPeriod(x.period);
          listState.setParams({ kind: x.kind === "kdv1" ? null : x.kind });
        }}
      />
    </div>
  );
}

/**
 * Beyanın özeti (#528): sorunlar düzeltildikçe yeniden hesaplanan ana
 * tutarlar, sayfanın başında.
 */
function TaxReturnSummary({ p }: { p: TaxReturnPreview }) {
  const { t } = useLocale();
  const figures: { label: string; value: string; strong?: boolean }[] = [];
  if (p.kdv1) {
    const x = p.kdv1.totals;
    figures.push(
      { label: t("office.tax_returns.base_total"), value: x.base_total },
      { label: t("office.tax_returns.calculated_vat"), value: x.total_vat },
      {
        label: t("office.tax_returns.total_deductions"),
        value: x.total_deductions,
      },
      {
        label: t("office.tax_returns.payable"),
        value: x.payable,
        strong: true,
      },
      { label: t("office.tax_returns.carry_out"), value: x.carry_out },
    );
  } else if (p.muhsgk) {
    const x = p.muhsgk.totals;
    figures.push(
      {
        label: t("office.tax_returns.tax_payable"),
        value: x.tax_payable,
        strong: true,
      },
      {
        label: t("office.tax_returns.premium_payable"),
        value: x.premium_payable,
      },
    );
  } else if (p.babs) {
    figures.push(
      {
        label: t("office.tax_returns.summary_ba", {
          count: p.babs.ba.document_count,
        }),
        value: p.babs.ba.amount,
      },
      {
        label: t("office.tax_returns.summary_bs", {
          count: p.babs.bs.document_count,
        }),
        value: p.babs.bs.amount,
      },
    );
  }
  if (figures.length === 0) return null;
  return (
    <section
      aria-label={t("office.tax_returns.summary_label")}
      data-testid="tax-return-summary"
      className="bg-surface border-line grid grid-cols-2 gap-x-6 gap-y-3 rounded-xl border p-4 sm:grid-cols-3 lg:grid-cols-5"
    >
      {figures.map((f) => (
        <div key={f.label} className="flex min-w-0 flex-col">
          <span className="text-text-2 truncate text-xs">{f.label}</span>
          <Money
            value={f.value}
            className={cn("text-base", f.strong && "font-display font-bold")}
          />
        </div>
      ))}
    </section>
  );
}

function StatusBar({
  p,
  periodLabel,
  dueLabel,
  filedLabel,
  canWrite,
  canApprove,
  busy,
  onCheck,
  onPrepare,
  onReopen,
  onFile,
}: {
  p: TaxReturnPreview;
  periodLabel: string;
  dueLabel: string;
  filedLabel?: string;
  canWrite: boolean;
  canApprove: boolean;
  busy: boolean;
  onCheck: () => void;
  onPrepare: () => void;
  onReopen: () => void;
  onFile: (v: { filed_on: string; accrual_number: string }) => void;
}) {
  const { t } = useLocale();
  const [filing, setFiling] = useState(false);
  const [filedOn, setFiledOn] = useState(() =>
    new Date().toISOString().slice(0, 10),
  );
  const [accrual, setAccrual] = useState("");
  const filedOnId = useId();
  const open = p.open_warnings.length;
  const days = p.calendar.days_left;
  const tone = statusVariants[p.status]?.tone ?? "neutral";
  const dates = useDates();
  const rec = p.record;
  // Kim, ne zaman (#669): hazırlayan, kontrol eden, veren.
  const trail = [
    rec?.prepared_at && p.status !== "draft"
      ? t("office.tax_returns.trail_prepared", {
          who: rec.prepared_by_name || "—",
          date: dates.day(rec.prepared_at.slice(0, 10)),
        })
      : "",
    rec?.checked_at
      ? t("office.tax_returns.trail_checked", {
          who: rec.checked_by_name || "—",
          date: dates.day(rec.checked_at.slice(0, 10)),
        })
      : "",
    rec?.filed_at
      ? t("office.tax_returns.trail_filed", {
          who: rec.filed_by_name || "—",
          date: dates.day(rec.filed_at.slice(0, 10)),
        })
      : "",
  ].filter(Boolean);
  const approveHint = canApprove
    ? undefined
    : t("office.tax_returns.approve_only_responsible");
  return (
    <section
      aria-label={t("office.tax_returns.status_label")}
      className="bg-surface border-line flex flex-wrap items-center gap-x-6 gap-y-3 rounded-xl border p-4"
    >
      <div className="flex min-w-0 flex-col">
        <span className="text-text-2 text-xs">
          {t(`office.tax_returns.kind_${p.kind}_long`)}
        </span>
        <span className="font-display text-lg font-bold">{periodLabel}</span>
      </div>
      {/* Kaydı olmayan beyan ofis ekranındaki gibi "Başlanmadı" (#575). */}
      <StatusPill tone={tone}>
        {p.record
          ? t(`office.tax_returns.status_${p.status}`)
          : t("office.tax_returns.office.not_started")}
      </StatusPill>
      <span className="text-text-2 flex items-center gap-1.5 text-sm">
        <CalendarClock className="size-4" aria-hidden />
        {t("office.tax_returns.due", { date: dueLabel })}
        {p.status !== "filed" ? (
          <span
            className={cn(days < 0 ? "text-danger" : days <= 5 && "text-warn")}
          >
            ·{" "}
            {days < 0
              ? t("office.tax_returns.overdue", { days: -days })
              : t("office.tax_returns.days_left", { days })}
          </span>
        ) : null}
      </span>
      {p.status === "filed" ? (
        <span className="text-text-2 text-sm">
          {t("office.tax_returns.filed_on", { date: filedLabel ?? "—" })}
          {p.record?.accrual_number
            ? ` · ${t("office.tax_returns.accrual_number")}: ${p.record.accrual_number}`
            : ""}
        </span>
      ) : null}
      {p.frozen && p.record?.prepared_at ? (
        <span className="text-text-3 text-xs">
          {t("office.tax_returns.frozen_hint")}
        </span>
      ) : null}
      <div className="ms-auto flex flex-wrap gap-2">
        {canWrite && p.status === "draft" ? (
          <Hint
            label={
              open > 0 ? t("office.tax_returns.prepare_blocked") : undefined
            }
            wrap={busy || open > 0}
          >
            <Button
              size="sm"
              className="max-md:min-h-11"
              disabled={busy || open > 0}
              onClick={onPrepare}
            >
              <FileCheck2 className="size-4" aria-hidden />
              {t("office.tax_returns.prepare")}
            </Button>
          </Hint>
        ) : null}
        {(canWrite || canApprove) && p.status === "prepared" ? (
          <>
            {canWrite ? (
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                disabled={busy}
                onClick={onReopen}
              >
                <RotateCcw className="size-4" aria-hidden />
                {t("office.tax_returns.reopen")}
              </Button>
            ) : null}
            {/* Son kontrol (#669): yetkisi olmayana düğme pasif, nedeni ipucunda. */}
            {!rec?.checked_at ? (
              <Hint label={approveHint} wrap={!canApprove || busy}>
                <Button
                  size="sm"
                  variant="outline"
                  className="max-md:min-h-11"
                  disabled={busy || !canApprove}
                  onClick={onCheck}
                >
                  <FileCheck2 className="size-4" aria-hidden />
                  {t("office.tax_returns.mark_checked")}
                </Button>
              </Hint>
            ) : null}
            <Hint label={approveHint} wrap={!canApprove || busy}>
              <Button
                size="sm"
                className="max-md:min-h-11"
                disabled={busy || !canApprove}
                aria-expanded={filing}
                onClick={() => setFiling((v) => !v)}
              >
                <CheckCircle2 className="size-4" aria-hidden />
                {t("office.tax_returns.mark_filed")}
              </Button>
            </Hint>
          </>
        ) : null}
      </div>
      {trail.length ? (
        <p
          className="text-text-2 w-full text-xs"
          data-testid="tax-return-trail"
        >
          {trail.join(" · ")}
        </p>
      ) : null}
      {!canApprove && p.status === "prepared" ? (
        <p className="text-text-3 w-full text-xs">{approveHint}</p>
      ) : null}
      {canWrite && p.status === "draft" && open > 0 ? (
        <p className="text-warn flex w-full flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          {t("office.tax_returns.open_warnings", { count: open })}
          {/* Tek tık: onaylanacak maddelere götürür. */}
          <a
            href={`#${CHECKLIST_ID}`}
            className="text-brand-text font-medium hover:underline"
          >
            {t("office.tax_returns.go_checklist")}
          </a>
        </p>
      ) : null}
      {filing && p.status === "prepared" ? (
        <form
          className="border-line flex w-full flex-wrap items-end gap-3 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (filedOn)
              onFile({ filed_on: filedOn, accrual_number: accrual.trim() });
          }}
        >
          <div className="flex flex-col gap-1 text-sm">
            <Label htmlFor={filedOnId} className={FIELD_LABEL}>
              {t("office.tax_returns.filed_on_label")}
            </Label>
            <DatePicker id={filedOnId} value={filedOn} onChange={setFiledOn} />
          </div>
          <div className="flex flex-col gap-1 text-sm">
            <Label htmlFor={`${filedOnId}-accrual`} className={FIELD_LABEL}>
              {t("office.tax_returns.accrual_number")}
            </Label>
            <Input
              id={`${filedOnId}-accrual`}
              maxLength={64}
              className="w-56"
              value={accrual}
              onChange={(e) => setAccrual(e.target.value)}
            />
          </div>
          <Button
            type="submit"
            size="sm"
            className="max-md:min-h-11"
            disabled={busy}
          >
            {t("office.tax_returns.save_filed")}
          </Button>
        </form>
      ) : null}
    </section>
  );
}

/**
 * Tahakkuk fişi (#575): beyan GİB'e verildikten sonra alınan tahakkuk PDF'i
 * beyannamenin kaydına eklenir; ekip ve denetim için buradan indirilir.
 */
function AccrualDocument({
  p,
  canWrite,
  onSaved,
  engagement,
}: {
  p: TaxReturnPreview;
  canWrite: boolean;
  onSaved: (p: TaxReturnPreview) => void;
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const input = useRef<HTMLInputElement>(null);
  const kind = p.kind as TaxReturnKind;
  const doc = p.record?.accrual_document ?? null;
  const upload = useMutation({
    mutationFn: (file: File) =>
      taxReturnsService.uploadAccrual(slug, engagement, kind, p.period, file),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.tax_returns.accrual_saved"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.tax_returns.accrual_failed"))),
  });
  return (
    <section
      aria-label={t("office.tax_returns.accrual_title")}
      className="bg-surface border-line flex flex-wrap items-center gap-3 rounded-xl border p-4 text-sm"
    >
      <span className="font-semibold">
        {t("office.tax_returns.accrual_title")}
      </span>
      {doc ? (
        <span className="text-text-2 min-w-0 truncate">{doc.file_name}</span>
      ) : (
        <span className="text-warn">
          {t("office.tax_returns.accrual_missing")}
        </span>
      )}
      <div className="ms-auto flex flex-wrap gap-2">
        {doc ? (
          <Button
            size="sm"
            variant="outline"
            className="max-md:min-h-11"
            onClick={() =>
              void taxReturnsService
                .downloadAccrual(
                  slug,
                  engagement,
                  kind,
                  p.period,
                  doc.file_name,
                )
                .catch(() =>
                  toast.error(t("office.tax_returns.accrual_download_failed")),
                )
            }
          >
            <Download className="size-4" aria-hidden />
            {t("office.tax_returns.accrual_download")}
          </Button>
        ) : null}
        {canWrite ? (
          <>
            <input
              ref={input}
              type="file"
              accept="application/pdf,.pdf"
              className="sr-only"
              aria-label={t("office.tax_returns.accrual_pick")}
              tabIndex={-1}
              onChange={(e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (f) upload.mutate(f);
              }}
            />
            <Button
              size="sm"
              variant={doc ? "outline" : "default"}
              className="max-md:min-h-11"
              disabled={upload.isPending}
              onClick={() => input.current?.click()}
            >
              <FileUp className="size-4" aria-hidden />
              {doc
                ? t("office.tax_returns.accrual_replace")
                : t("office.tax_returns.accrual_upload")}
            </Button>
          </>
        ) : null}
      </div>
    </section>
  );
}

/** Kontrol listesinin çapası: "Kontrol listesine git" buraya iner. */
const CHECKLIST_ID = "tr-checklist";

function Checklist({
  items,
  editable,
  busy,
  onToggle,
}: {
  items: TaxReturnCheckItem[];
  editable: boolean;
  busy: boolean;
  onToggle: (code: string, on: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const engagement = useEngagementScope();
  return (
    <section
      id={CHECKLIST_ID}
      aria-labelledby={`${CHECKLIST_ID}-title`}
      className="bg-surface border-line scroll-mt-20 rounded-xl border p-4"
    >
      <h2
        id={`${CHECKLIST_ID}-title`}
        className="font-display mb-3 text-base font-bold"
      >
        {t("office.tax_returns.checklist")}
      </h2>
      {items.length === 0 ? (
        <p className="text-text-2 flex items-center gap-2 text-sm">
          <CheckCircle2 className="text-ok size-4" aria-hidden />
          {t("office.tax_returns.checklist_empty")}
        </p>
      ) : (
        <ul className="space-y-2">
          {items.map((c) => {
            const warn = c.severity === "warning";
            const Icon = warn ? AlertTriangle : Info;
            return (
              <li
                key={c.code}
                className={cn(
                  "flex flex-wrap items-start gap-3 rounded-lg border px-3 py-2",
                  // Onaylanmamış uyarı bağırmaz (#708 dili): ince sarı çerçeve.
                  warn && !c.acknowledged ? "border-warn/60" : "border-line",
                )}
              >
                <Icon
                  className={cn(
                    "mt-0.5 size-4 shrink-0",
                    warn ? "text-warn" : "text-text-3",
                  )}
                  aria-hidden
                />
                <span className="min-w-0 flex-1 text-sm">
                  <span className="sr-only">
                    {t(`office.tax_returns.severity_${c.severity}`)}:{" "}
                  </span>
                  {c.message}
                  {c.amount ? (
                    <span className="text-text-2 ms-1">
                      (<Money value={c.amount} />)
                    </span>
                  ) : null}
                </span>
                {(() => {
                  // Maddenin ilgili kayıtları: kodun sekmesi (#469).
                  const href = engagement
                    ? checklistHref(slug, engagement, c.code)
                    : null;
                  return href ? (
                    <Link
                      href={href}
                      className="text-brand-text flex min-h-11 items-center text-sm hover:underline md:min-h-0"
                      aria-label={`${t("office.tax_returns.related_records")}: ${c.message}`}
                    >
                      {t("office.tax_returns.related_records")}
                    </Link>
                  ) : null;
                })()}
                {warn ? (
                  <Label className="flex min-h-11 items-center gap-2 text-sm font-normal md:min-h-0">
                    <Checkbox
                      checked={c.acknowledged}
                      disabled={!editable || busy}
                      onCheckedChange={(v) => onToggle(c.code, v === true)}
                    />
                    {t("office.tax_returns.acknowledge")}
                  </Label>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

/** Form bölümü: başlık ve "alan · tutar" satırları. */
function Section({
  title,
  children,
  className,
}: {
  title: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn("bg-surface border-line rounded-xl border p-4", className)}
    >
      <h2 className="font-display mb-3 text-base font-bold">{title}</h2>
      {children}
    </section>
  );
}

function Grid({
  head,
  rows,
  empty,
}: {
  head: string[];
  rows: ReactNode[][];
  empty: string;
}) {
  if (rows.length === 0) return <p className="text-text-2 text-sm">{empty}</p>;
  return (
    <Table className="min-w-[32rem]">
      <TableHeader className="[&_tr]:border-line">
        <TableRow className="hover:bg-transparent">
          {head.map((h, i) => (
            <TableHead
              key={h}
              scope="col"
              className={cn(
                "text-text-2 h-auto px-0 py-2 pe-3 text-xs",
                i > 0 && "text-right",
              )}
            >
              {h}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r, i) => (
          <TableRow key={i} className="border-line hover:bg-transparent">
            {r.map((c, j) => (
              <TableCell
                key={j}
                className={cn(
                  "px-0 py-2 pe-3",
                  j > 0 && "text-right tabular-nums",
                )}
              >
                {c}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function Lines({
  rows,
}: {
  rows: { label: string; value: string; strong?: boolean }[];
}) {
  return (
    <dl className="divide-line divide-y text-sm">
      {rows.map((r) => (
        <div
          key={r.label}
          className={cn(
            "flex items-center justify-between gap-4 py-2",
            r.strong && "font-semibold",
          )}
        >
          <dt className={cn(!r.strong && "text-text-2")}>{r.label}</dt>
          <dd>
            <Money value={r.value} />
          </dd>
        </div>
      ))}
    </dl>
  );
}

function AdjustmentsForm({
  value,
  busy,
  onSave,
}: {
  value: TaxReturnAdjustments;
  busy: boolean;
  onSave: (a: TaxReturnAdjustments) => void;
}) {
  const { t } = useLocale();
  const fields = ["carried_in", "additional_vat", "other_deductions"] as const;
  // Sunucu tutarı "1250.50" döner; alan Türkçe yazılır ("1250,50"). Nokta
  // olduğu gibi kalırsa kaydederken binlik ayırıcı sayılır ve tutar 100 katına
  // çıkar (#452).
  const formId = useId();
  const [draft, setDraft] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      fields.map((f) => [f, (value[f] ?? "").replace(".", ",")]),
    ),
  );
  const [error, setError] = useState("");
  return (
    <form
      className="border-line mt-4 grid gap-3 border-t pt-4 sm:grid-cols-3"
      onSubmit={(e) => {
        e.preventDefault();
        const out: TaxReturnAdjustments = {};
        for (const f of fields) {
          const raw = (draft[f] ?? "")
            .trim()
            .replace(/\./g, "")
            .replace(",", ".");
          if (raw === "") continue;
          if (!/^\d+(\.\d{1,2})?$/.test(raw)) {
            setError(t("office.tax_returns.adjust_invalid"));
            return;
          }
          out[f] = raw;
        }
        setError("");
        onSave(out);
      }}
    >
      <p className="text-text-2 text-sm sm:col-span-3">
        {t("office.tax_returns.adjust_hint")}
      </p>
      {fields.map((f) => (
        <div key={f} className="flex flex-col gap-1 text-sm">
          <Label htmlFor={`${formId}-${f}`} className={FIELD_LABEL}>
            {t(`office.tax_returns.adj_${f}`)}
          </Label>
          <Input
            id={`${formId}-${f}`}
            inputMode="decimal"
            placeholder="0,00"
            className="text-right tabular-nums"
            value={draft[f] ?? ""}
            onChange={(e) => setDraft((d) => ({ ...d, [f]: e.target.value }))}
          />
        </div>
      ))}
      {error ? (
        <p role="alert" className="text-danger text-sm sm:col-span-3">
          {error}
        </p>
      ) : null}
      <div className="sm:col-span-3">
        <Button type="submit" size="sm" variant="outline" disabled={busy}>
          {t("office.tax_returns.adjust_save")}
        </Button>
      </div>
    </form>
  );
}

function KDV1View({
  k,
  editable,
  busy,
  onAdjust,
}: {
  k: TaxReturnKDV1;
  editable: boolean;
  busy: boolean;
  onAdjust: (a: TaxReturnAdjustments) => void;
}) {
  const { t } = useLocale();
  const x = k.totals;
  const unknown = t("office.tax_returns.rate_unknown");
  const m = (v: string) => <Money value={v} />;
  return (
    <div className="space-y-4">
      <SummaryCards
        cards={[
          {
            label: t("office.tax_returns.calculated_vat"),
            value: <Money value={x.total_vat} />,
            icon: Landmark,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.tax_returns.total_deductions"),
            value: <Money value={x.total_deductions} />,
            icon: Landmark,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.tax_returns.payable"),
            value: <Money value={x.payable} />,
            icon: Landmark,
            box: "bg-brand-tint text-brand-text",
          },
          {
            label: t("office.tax_returns.carry_out"),
            value: <Money value={x.carry_out} />,
            icon: Landmark,
            box: "bg-ok-tint text-ok",
          },
        ]}
      />
      <Section title={t("office.tax_returns.kdv1_sales")}>
        <Grid
          head={[
            t("office.tax_returns.col_rate"),
            t("office.tax_returns.col_base"),
            t("office.tax_returns.col_vat"),
            t("office.tax_returns.col_invoice_base"),
            t("office.tax_returns.col_retail_base"),
            t("office.tax_returns.col_purchase_return_base"),
          ]}
          rows={k.sales.map((s) => [
            rateLabel(s.rate, unknown),
            m(s.base),
            m(s.vat),
            m(s.invoice_base),
            m(s.retail_base),
            m(s.purchase_return_base),
          ])}
          empty={t("office.tax_returns.none")}
        />
      </Section>
      {k.withholdings.length > 0 ? (
        <Section title={t("office.tax_returns.kdv1_withholdings")}>
          <Grid
            head={[
              t("office.tax_returns.col_code"),
              t("office.tax_returns.col_base"),
              t("office.tax_returns.col_rate"),
              t("office.tax_returns.col_ratio"),
              t("office.tax_returns.col_withheld"),
              t("office.tax_returns.col_declared"),
            ]}
            rows={k.withholdings.map((w) => [
              w.code || "—",
              m(w.base),
              rateLabel(w.rate, unknown),
              w.ratio,
              m(w.withheld),
              m(w.declared_vat),
            ])}
            empty=""
          />
        </Section>
      ) : null}
      {k.special_bases.length > 0 ? (
        <Section title={t("office.tax_returns.kdv1_special")}>
          <Grid
            head={[
              t("office.tax_returns.col_code"),
              t("office.tax_returns.col_base"),
              t("office.tax_returns.col_rate"),
              t("office.tax_returns.col_vat"),
            ]}
            rows={k.special_bases.map((s) => [
              s.code || "—",
              m(s.base),
              rateLabel(s.rate, unknown),
              m(s.vat),
            ])}
            empty=""
          />
        </Section>
      ) : null}
      <div className="grid gap-4 lg:grid-cols-2">
        <Section title={t("office.tax_returns.kdv1_calculated")}>
          <Lines
            rows={[
              {
                label: t("office.tax_returns.base_total"),
                value: x.base_total,
              },
              {
                label: t("office.tax_returns.calculated_vat_line"),
                value: x.calculated_vat,
              },
              {
                label: t("office.tax_returns.additional_vat"),
                value: x.additional_vat,
              },
              {
                label: t("office.tax_returns.total_vat"),
                value: x.total_vat,
                strong: true,
              },
            ]}
          />
        </Section>
        <Section title={t("office.tax_returns.kdv1_deductions")}>
          <Lines
            rows={[
              {
                label: t("office.tax_returns.carried_in", {
                  source: t(`office.tax_returns.carry_${k.carry.source}`),
                }),
                value: x.carried_in,
              },
              {
                label: t("office.tax_returns.current_input"),
                value: x.current_input,
              },
              {
                label: t("office.tax_returns.sales_returns"),
                value: x.sales_returns_vat,
              },
              {
                label: t("office.tax_returns.responsible_vat"),
                value: x.responsible_vat,
              },
              {
                label: t("office.tax_returns.other_deductions"),
                value: x.other_deductions,
              },
              {
                label: t("office.tax_returns.total_deductions"),
                value: x.total_deductions,
                strong: true,
              },
            ]}
          />
          {editable ? (
            <AdjustmentsForm
              key={JSON.stringify(k.adjustments)}
              value={k.adjustments}
              busy={busy}
              onSave={onAdjust}
            />
          ) : null}
        </Section>
      </div>
      <Section title={t("office.tax_returns.kdv1_purchases")}>
        <Grid
          head={[
            t("office.tax_returns.col_rate"),
            t("office.tax_returns.col_base"),
            t("office.tax_returns.col_vat"),
            t("office.tax_returns.col_invoice_vat"),
            t("office.tax_returns.col_expense_vat"),
          ]}
          rows={k.purchases.map((s) => [
            rateLabel(s.rate, unknown),
            m(s.base),
            m(s.vat),
            m(s.invoice_vat),
            m(s.expense_vat),
          ])}
          empty={t("office.tax_returns.none")}
        />
      </Section>
      {k.exemptions.length > 0 ? (
        <Section title={t("office.tax_returns.kdv1_exemptions")}>
          <Grid
            head={[
              t("office.tax_returns.col_group"),
              t("office.tax_returns.col_code"),
              t("office.tax_returns.col_delivery"),
              t("office.tax_returns.col_deferred"),
            ]}
            rows={k.exemptions.map((e) => [
              t(`office.tax_returns.group_${e.group}`),
              e.code || "—",
              m(e.base),
              e.group === "export_registered" ? m(e.vat) : "—",
            ])}
            empty=""
          />
        </Section>
      ) : null}
      <div className="grid gap-4 lg:grid-cols-2">
        <Section title={t("office.tax_returns.kdv1_result")}>
          <Lines
            rows={[
              {
                label: t("office.tax_returns.deferred_vat"),
                value: x.deferred_vat,
              },
              {
                label: t("office.tax_returns.payable"),
                value: x.payable,
                strong: true,
              },
              {
                label: t("office.tax_returns.carry_out"),
                value: x.carry_out,
                strong: true,
              },
            ]}
          />
        </Section>
        <Section title={t("office.tax_returns.kdv1_extra")}>
          <Lines
            rows={[
              {
                label: t("office.tax_returns.delivery_total"),
                value: x.delivery_total,
              },
              {
                label: t("office.tax_returns.card_sales"),
                value: x.card_sales,
              },
            ]}
          />
        </Section>
      </div>
    </div>
  );
}

function MUHSGKView({ m }: { m: TaxReturnMUHSGK }) {
  const { t } = useLocale();
  const pr = m.payroll;
  const pm = m.premium;
  const columns = useMemo<ColumnDef<TaxReturnEmployee, unknown>[]>(
    () => [
      createColumn<TaxReturnEmployee>({
        id: "name",
        labelKey: "office.tax_returns.col_employee",
        accessorFn: (e) => e.name,
        gridPrimary: true,
        cell: ({ row }) => (
          <>
            <div>{row.original.name}</div>
            <div className="text-text-3 font-mono text-xs">
              {[row.original.national_id_masked, row.original.sgk_no]
                .filter(Boolean)
                .join(" · ")}
            </div>
          </>
        ),
      }),
      createColumn<TaxReturnEmployee>({
        id: "days",
        labelKey: "office.tax_returns.col_days",
        accessorFn: (e) => e.days_worked ?? "",
        cell: ({ row }) => row.original.days_worked ?? "—",
      }),
      createMoneyColumn<TaxReturnEmployee>({
        id: "gross",
        labelKey: "office.tax_returns.col_gross",
        accessor: (e) => e.gross,
      }),
      createMoneyColumn<TaxReturnEmployee>({
        id: "sgk_base",
        labelKey: "office.tax_returns.col_sgk_base",
        accessor: (e) => e.sgk_base,
      }),
      createMoneyColumn<TaxReturnEmployee>({
        id: "income_tax_base",
        labelKey: "office.tax_returns.col_income_tax_base",
        accessor: (e) => e.income_tax_base,
      }),
      createMoneyColumn<TaxReturnEmployee>({
        id: "income_tax",
        labelKey: "office.tax_returns.col_income_tax",
        accessor: (e) => e.income_tax,
      }),
      createMoneyColumn<TaxReturnEmployee>({
        id: "stamp_tax",
        labelKey: "office.tax_returns.col_stamp_tax",
        accessor: (e) => e.stamp_tax,
      }),
    ],
    [],
  );
  return (
    <div className="space-y-4">
      <SummaryCards
        cards={[
          {
            label: t("office.tax_returns.income_tax"),
            value: <Money value={m.totals.income_tax} />,
            icon: Landmark,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.tax_returns.stamp_tax"),
            value: <Money value={m.totals.stamp_tax} />,
            icon: Landmark,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.tax_returns.tax_payable"),
            value: <Money value={m.totals.tax_payable} />,
            icon: Landmark,
            box: "bg-brand-tint text-brand-text",
          },
          {
            label: t("office.tax_returns.premium_payable"),
            value: <Money value={m.totals.premium_payable} />,
            icon: Landmark,
            box: "bg-surface-3 text-text-2",
          },
        ]}
      />
      <Section title={t("office.tax_returns.muhsgk_taxes")}>
        <Grid
          head={[
            t("office.tax_returns.col_payment_kind"),
            t("office.tax_returns.col_base"),
            t("office.tax_returns.col_withholding_rate"),
            t("office.tax_returns.col_tax"),
            t("office.tax_returns.col_count"),
          ]}
          rows={m.withholdings.map((w) => [
            t(`office.tax_returns.withholding_${w.kind}`),
            <Money key="b" value={w.base} />,
            w.rate && w.rate !== "0" ? `%${w.rate}` : "—",
            <Money key="t" value={w.tax} />,
            w.count,
          ])}
          empty={t("office.tax_returns.muhsgk_empty")}
        />
      </Section>
      <div className="grid gap-4 lg:grid-cols-2">
        {pr ? (
          <Section title={t("office.tax_returns.muhsgk_payroll")}>
            <p className="text-text-2 mb-2 text-sm">
              {t("office.tax_returns.muhsgk_payroll_hint", {
                count: pr.employee_count,
                days: pr.days_total,
              })}
            </p>
            <Lines
              rows={[
                { label: t("office.tax_returns.col_gross"), value: pr.gross },
                {
                  label: t("office.tax_returns.col_income_tax_base"),
                  value: pr.income_tax_base,
                },
                {
                  label: t("office.tax_returns.income_tax_exemption"),
                  value: pr.income_tax_exemption,
                },
                {
                  label: t("office.tax_returns.stamp_tax_exemption"),
                  value: pr.stamp_tax_exemption,
                },
                {
                  label: t("office.tax_returns.col_sgk_base"),
                  value: pr.sgk_base,
                },
              ]}
            />
          </Section>
        ) : null}
        <Section title={t("office.tax_returns.muhsgk_premium")}>
          <Lines
            rows={[
              ...(pr
                ? [
                    {
                      label: t("office.tax_returns.sgk_employee"),
                      value: pr.sgk_employee,
                    },
                    {
                      label: t("office.tax_returns.unemployment_employee"),
                      value: pr.unemployment_employee,
                    },
                    {
                      label: t("office.tax_returns.sgk_employer"),
                      value: pr.sgk_employer,
                    },
                    {
                      label: t("office.tax_returns.unemployment_employer"),
                      value: pr.unemployment_employer,
                    },
                  ]
                : []),
              {
                label: t("office.tax_returns.premium_expected"),
                value: pm.expected,
                strong: true,
              },
              {
                label: t("office.tax_returns.premium_incentive"),
                value: pm.expected_incentive,
              },
              ...(pm.accrual
                ? [
                    {
                      label: t("office.tax_returns.premium_accrual"),
                      value: pm.accrual,
                      strong: true,
                    },
                    {
                      label: t("office.tax_returns.premium_difference"),
                      value: pm.difference ?? "0",
                    },
                  ]
                : []),
            ]}
          />
        </Section>
      </div>
      <Section title={t("office.tax_returns.muhsgk_employees")}>
        <EntityTable<TaxReturnEmployee>
          columns={columns}
          data={m.employees}
          getRowId={(e) => String(e.line_no)}
          emptyTitle={t("office.tax_returns.employees_empty_title")}
          emptyDescription={t("office.tax_returns.employees_empty")}
          features={{
            ...tableFeatures,
            persistKey: "office.tax_returns.employees",
          }}
        />
      </Section>
    </div>
  );
}

function BABSView({ b }: { b: TaxReturnBABS }) {
  const { t } = useLocale();
  const columns = useMemo<ColumnDef<TaxReturnBABSRow, unknown>[]>(
    () => [
      createColumn<TaxReturnBABSRow>({
        id: "name",
        labelKey: "office.tax_returns.col_counterparty",
        accessorFn: (r) => r.name,
        gridPrimary: true,
        cell: ({ row }) => row.original.name || "—",
      }),
      createColumn<TaxReturnBABSRow>({
        id: "id",
        labelKey: "office.tax_returns.col_tax_id",
        accessorFn: (r) => r.counterparty_id,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.counterparty_id}
            {row.original.id_type ? (
              <span className="text-text-3 ms-1 uppercase">
                {row.original.id_type}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<TaxReturnBABSRow>({
        id: "count",
        labelKey: "office.tax_returns.col_documents",
        accessorFn: (r) => r.document_count,
        // Masraf olarak işlenen alış belgeleri (#575) ayrıca belirtilir.
        cell: ({ row }) =>
          row.original.expense_count ? (
            <span>
              {row.original.document_count}{" "}
              <span className="text-text-2 text-xs">
                {t("office.tax_returns.babs_expense_docs", {
                  count: row.original.expense_count,
                })}
              </span>
            </span>
          ) : (
            row.original.document_count
          ),
      }),
      createMoneyColumn<TaxReturnBABSRow>({
        id: "amount",
        labelKey: "office.tax_returns.col_amount",
        accessor: (r) => r.amount,
        className: "font-semibold",
      }),
    ],
    [t],
  );
  const forms = [
    { key: "ba", form: b.ba },
    { key: "bs", form: b.bs },
  ] as const;
  return (
    <div className="space-y-4">
      <p className="text-text-2 text-sm">
        {t("office.tax_returns.babs_hint")} <Money value={b.threshold} />
      </p>
      <div className="grid gap-4 xl:grid-cols-2">
        {forms.map(({ key, form }) => (
          <Section
            key={key}
            title={`${t(`office.tax_returns.form_${key}`)} · ${form.document_count} ${t("office.tax_returns.documents")}`}
          >
            <EntityTable<TaxReturnBABSRow>
              columns={columns}
              data={form.rows}
              getRowId={(r) => r.counterparty_id}
              emptyTitle={t("office.tax_returns.babs_empty_title")}
              emptyDescription={t("office.tax_returns.babs_empty")}
              features={{
                ...tableFeatures,
                persistKey: `office.tax_returns.${key}`,
              }}
            />
            <p className="text-text-2 mt-2 flex flex-wrap justify-between gap-2 text-sm">
              <span>
                {t("office.tax_returns.below_threshold", {
                  count: form.below_threshold,
                })}
              </span>
              <span className="font-semibold">
                {t("office.tax_returns.total")}: <Money value={form.amount} />
              </span>
            </p>
          </Section>
        ))}
      </div>
    </div>
  );
}

function History({
  items,
  loading,
  error,
  onRetry,
  onPick,
}: {
  items: TaxReturnSummary[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  onPick: (x: TaxReturnSummary) => void;
}) {
  const { t } = useLocale();
  const dates = useDates();
  const columns = useMemo<ColumnDef<TaxReturnSummary, unknown>[]>(
    () => [
      createColumn<TaxReturnSummary>({
        id: "period",
        labelKey: "office.tax_returns.col_period",
        accessorFn: (x) => x.period,
        gridPrimary: true,
        cell: ({ row }) => dates.month(row.original.period),
      }),
      createStatusColumn<TaxReturnSummary>({
        id: "kind",
        labelKey: "office.tax_returns.col_kind",
        accessor: (x) => x.kind,
        variants: kindVariants,
      }),
      createStatusColumn<TaxReturnSummary>({
        id: "status",
        labelKey: "office.tax_returns.col_status",
        accessor: (x) => x.status,
        variants: statusVariants,
      }),
      createMoneyColumn<TaxReturnSummary>({
        id: "payable",
        labelKey: "office.tax_returns.col_payable",
        accessor: (x) => x.payable,
      }),
      createMoneyColumn<TaxReturnSummary>({
        id: "carry_out",
        labelKey: "office.tax_returns.col_carry_out",
        accessor: (x) => x.carry_out,
      }),
      createDateColumn<TaxReturnSummary>({
        id: "due_date",
        labelKey: "office.tax_returns.col_due",
        accessor: (x) => x.due_date,
      }),
      createDateColumn<TaxReturnSummary>({
        id: "filed_on",
        labelKey: "office.tax_returns.col_filed_on",
        accessor: (x) => x.filed_on,
      }),
      createColumn<TaxReturnSummary>({
        id: "accrual",
        labelKey: "office.tax_returns.accrual_number",
        accessorFn: (x) => x.accrual_number ?? "",
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.accrual_number ?? "—"}
          </span>
        ),
      }),
    ],
    [dates],
  );
  return (
    <section className="space-y-3">
      <h2 className="font-display text-lg font-bold">
        {t("office.tax_returns.history")}
      </h2>
      <EntityTable<TaxReturnSummary>
        columns={columns}
        data={items}
        getRowId={(x) => x.uuid}
        isLoading={loading}
        isError={error}
        errorTitle={t("office.api.load_error")}
        onRetry={onRetry}
        emptyTitle={t("office.tax_returns.history_empty_title")}
        emptyDescription={t("office.tax_returns.history_empty")}
        onRowClick={onPick}
        features={{
          ...tableFeatures,
          persistKey: "office.tax_returns.history",
        }}
      />
    </section>
  );
}
