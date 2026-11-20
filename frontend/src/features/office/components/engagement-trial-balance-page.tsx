"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileDown, Upload } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useRef, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
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
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  trialBalanceKeys,
  trialBalanceService,
  type LiveTrialBalanceQuery,
  type TrialBalance,
  type TrialBalanceLine,
  type TrialBalanceQuery,
} from "../services/trial-balance.service";
import { Money, Panel } from "./office-ui";

/** "2026-10" → "Ekim 2026"; dönem yoksa dosya adı. */
function trialBalanceLabel(tb: TrialBalance, locale: string) {
  if (!tb.period) return tb.file_name;
  const d = new Date(`${tb.period}-01T00:00:00`);
  const month = Number.isNaN(d.getTime())
    ? tb.period
    : d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB", {
        month: "long",
        year: "numeric",
      });
  return `${month} · ${tb.file_name}`;
}

/** İşaretli bakiye → borç / alacak bakiyesi. */
export function splitBalance(balance: string) {
  const b = balance.trim();
  if (b.startsWith("-")) return { debit: "", credit: b.slice(1) };
  return { debit: Number(b) === 0 ? "" : b, credit: "" };
}

/**
 * Mükellef mizanı (#669 E): Luca'dan yüklenen mizanlar dönem dönem seçilir;
 * ana hesap ya da tüm hesaplar, önceki dönemle karşılaştırma, Excel / PDF
 * indirme ve yeni mizan yükleme.
 */
export function EngagementTrialBalancePage({
  engagement,
}: {
  engagement: string;
}) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <TrialBalanceInner engagement={engagement} />
    </Suspense>
  );
}

/** URL'deki seçimleri okur / yazar (`?kaynak=`, `?mizan=` …). */
function useParams() {
  const router = useRouter();
  const search = useSearchParams();
  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(search.toString());
    if (value === null) next.delete(key);
    else next.set(key, value);
    router.replace(`?${next.toString()}`, { scroll: false });
  };
  return { search, setParam };
}

function currentMonth() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

function TrialBalanceInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { search, setParam } = useParams();
  const live = search.get("kaynak") === "canli";
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-2">
        <div className="space-y-1">
          <Label htmlFor="trial-balance-source">
            {t("office.trial_balance.source")}
          </Label>
          <NativeSelect
            id="trial-balance-source"
            value={live ? "canli" : "luca"}
            onChange={(e) =>
              setParam("kaynak", e.target.value === "canli" ? "canli" : null)
            }
          >
            <NativeSelectOption value="luca">
              {t("office.trial_balance.source_luca")}
            </NativeSelectOption>
            <NativeSelectOption value="canli">
              {t("office.trial_balance.source_live")}
            </NativeSelectOption>
          </NativeSelect>
        </div>
      </div>
      {live ? (
        <LiveTrialBalance engagement={engagement} />
      ) : (
        <UploadedTrialBalance engagement={engagement} />
      )}
    </div>
  );
}

/** Yüklenen Luca mizanları. */
function UploadedTrialBalance({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const { search, setParam } = useParams();
  const fileRef = useRef<HTMLInputElement>(null);
  const [exporting, setExporting] = useState<"xlsx" | "pdf" | null>(null);

  const which = search.get("mizan") ?? "latest";
  const level = search.get("level") === "1" ? 1 : 0;
  const compare = search.get("compare") === "none" ? "none" : "previous";
  const query: TrialBalanceQuery = { which, level, compare };

  const list = useQuery({
    queryKey: trialBalanceKeys.all(slug, engagement),
    queryFn: () => trialBalanceService.list(slug, engagement),
  });
  const items = list.data?.items ?? [];
  const report = useQuery({
    queryKey: trialBalanceKeys.report(slug, engagement, query),
    queryFn: () => trialBalanceService.report(slug, engagement, query),
    enabled: items.length > 0,
  });

  const upload = useMutation({
    mutationFn: (file: File) =>
      trialBalanceService.upload(slug, engagement, file),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: trialBalanceKeys.all(slug, engagement),
      });
      setParam("mizan", null);
      toast.success(t("office.trial_balance.uploaded"));
    },
    onError: (err) =>
      toast.error(err.message || t("office.trial_balance.upload_error")),
  });

  const download = async (format: "xlsx" | "pdf") => {
    setExporting(format);
    try {
      await trialBalanceService.download(
        slug,
        engagement,
        query,
        format,
        locale,
      );
    } catch (err) {
      toast.error(
        (err as Error)?.message || t("office.trial_balance.export_error"),
      );
    } finally {
      setExporting(null);
    }
  };

  const canUpload = can(permissions.tenant.accountPlan.write);
  const uploadButton = canUpload ? (
    <>
      <input
        ref={fileRef}
        type="file"
        accept=".xlsx,.csv"
        className="sr-only"
        aria-label={t("office.trial_balance.upload")}
        data-testid="trial-balance-file"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) upload.mutate(file);
          e.target.value = "";
        }}
      />
      {/* Sayfanın tek birincil eylemi (#710); dışa aktarmalar ikincil. */}
      <Button
        size="sm"
        disabled={upload.isPending}
        onClick={() => fileRef.current?.click()}
      >
        <Upload className="size-4" aria-hidden />
        {t("office.trial_balance.upload")}
      </Button>
    </>
  ) : null;

  // Liste yokken de sayfa başlığı görünür (#710): kullanıcı nerede olduğunu
  // ve ne yapacağını okur; hata durumunda yeniden dene.
  const header = (
    <PageHeader
      headingLevel={2}
      title={t("office.trial_balance.title")}
      description={t("office.trial_balance.description")}
    />
  );
  if (list.isLoading) return <Skeleton className="h-64 w-full" />;
  if (list.isError)
    return (
      <div className="space-y-4">
        {header}
        <ErrorState
          title={t("office.api.load_error")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  if (items.length === 0)
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={t("office.trial_balance.empty_title")}
          description={t("office.trial_balance.empty_body")}
          action={uploadButton}
        />
      </div>
    );

  const rep = report.data;
  const selected = rep?.trial_balance.uuid ?? "";

  return (
    <div className="space-y-4">
      <Panel
        title={t("office.trial_balance.title")}
        description={
          rep?.compared_to
            ? t("office.trial_balance.compared_to", {
                label: trialBalanceLabel(rep.compared_to, locale),
              })
            : t("office.trial_balance.description")
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {uploadButton}
            <Button
              size="sm"
              variant="outline"
              disabled={!rep || exporting !== null}
              onClick={() => void download("xlsx")}
            >
              <FileDown className="size-4" aria-hidden />
              {t("office.trial_balance.export_xlsx")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!rep || exporting !== null}
              onClick={() => void download("pdf")}
            >
              <FileDown className="size-4" aria-hidden />
              {t("office.trial_balance.export_pdf")}
            </Button>
          </div>
        }
      >
        <div className="flex flex-wrap items-end gap-4 p-4">
          <div className="space-y-1">
            <Label htmlFor="trial-balance-period">
              {t("office.trial_balance.period")}
            </Label>
            <NativeSelect
              id="trial-balance-period"
              value={which === "latest" ? selected : which}
              onChange={(e) => setParam("mizan", e.target.value)}
            >
              {items.map((tb) => (
                <NativeSelectOption key={tb.uuid} value={tb.uuid}>
                  {trialBalanceLabel(tb, locale)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
          <div className="space-y-1">
            <Label htmlFor="trial-balance-level">
              {t("office.trial_balance.level")}
            </Label>
            <NativeSelect
              id="trial-balance-level"
              value={String(level)}
              onChange={(e) =>
                setParam("level", e.target.value === "1" ? "1" : null)
              }
            >
              <NativeSelectOption value="0">
                {t("office.trial_balance.level_all")}
              </NativeSelectOption>
              <NativeSelectOption value="1">
                {t("office.trial_balance.level_main")}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={compare === "previous"}
              onCheckedChange={(v) =>
                setParam("compare", v === true ? null : "none")
              }
            />
            {t("office.trial_balance.compare")}
          </Label>
        </div>
      </Panel>

      {report.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : report.isError || !rep ? (
        <p className="text-danger text-sm">{t("office.api.load_error")}</p>
      ) : (
        <TrialBalanceTable
          lines={rep.lines}
          level={level}
          compared={!!rep.compared_to}
          previousLabel={t("office.trial_balance.col_previous")}
          changeLabel={t("office.trial_balance.col_change")}
        />
      )}
    </div>
  );
}

/**
 * Canlı mizan (#669 E): ofisin kendi fişlerinden, dönemin yılının
 * Ocak'ından seçilen aya birikimli. Varsayılan onaylı ve aktarılmış
 * fişler; taslaklar istenirse katılır. Aynı dönemin yüklenen Luca
 * mizanıyla karşılaştırılabilir.
 */
function LiveTrialBalance({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { search, setParam } = useParams();
  const [exporting, setExporting] = useState<"xlsx" | "pdf" | null>(null);

  const raw = search.get("donem") ?? "";
  const period = /^\d{4}-(0[1-9]|1[0-2])$/.test(raw) ? raw : currentMonth();
  const query: LiveTrialBalanceQuery = {
    period,
    includeDrafts: search.get("taslak") === "1",
    compare: search.get("luca") === "1" ? "luca" : "none",
    level: search.get("level") === "1" ? 1 : 0,
  };
  const report = useQuery({
    queryKey: trialBalanceKeys.live(slug, engagement, query),
    queryFn: () => trialBalanceService.live(slug, engagement, query),
  });
  const rep = report.data;

  const download = async (format: "xlsx" | "pdf") => {
    setExporting(format);
    try {
      await trialBalanceService.downloadLive(
        slug,
        engagement,
        query,
        format,
        locale,
      );
    } catch (err) {
      toast.error(
        (err as Error)?.message || t("office.trial_balance.export_error"),
      );
    } finally {
      setExporting(null);
    }
  };

  let description = t("office.trial_balance.live_description", {
    from: rep?.from_period ?? `${period.slice(0, 4)}-01`,
    to: period,
  });
  if (query.compare === "luca") {
    description = rep?.compared_to
      ? t("office.trial_balance.live_compared_to", {
          label: trialBalanceLabel(rep.compared_to, locale),
        })
      : t("office.trial_balance.live_no_luca");
  }

  return (
    <div className="space-y-4">
      <Panel
        title={t("office.trial_balance.live_title")}
        description={description}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={!rep || exporting !== null}
              onClick={() => void download("xlsx")}
            >
              <FileDown className="size-4" aria-hidden />
              {t("office.trial_balance.export_xlsx")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!rep || exporting !== null}
              onClick={() => void download("pdf")}
            >
              <FileDown className="size-4" aria-hidden />
              {t("office.trial_balance.export_pdf")}
            </Button>
          </div>
        }
      >
        <div className="flex flex-wrap items-end gap-4 p-4">
          <div className="space-y-1">
            <Label htmlFor="trial-balance-live-period">
              {t("office.trial_balance.live_period")}
            </Label>
            <MonthPicker
              id="trial-balance-live-period"
              value={period}
              onChange={(v) => setParam("donem", v)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="trial-balance-level">
              {t("office.trial_balance.level")}
            </Label>
            <NativeSelect
              id="trial-balance-level"
              value={String(query.level)}
              onChange={(e) =>
                setParam("level", e.target.value === "1" ? "1" : null)
              }
            >
              <NativeSelectOption value="0">
                {t("office.trial_balance.level_all")}
              </NativeSelectOption>
              <NativeSelectOption value="1">
                {t("office.trial_balance.level_main")}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={query.includeDrafts}
              onCheckedChange={(v) =>
                setParam("taslak", v === true ? "1" : null)
              }
            />
            {t("office.trial_balance.include_drafts")}
          </Label>
          <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={query.compare === "luca"}
              onCheckedChange={(v) => setParam("luca", v === true ? "1" : null)}
            />
            {t("office.trial_balance.compare_luca")}
          </Label>
        </div>
      </Panel>

      {report.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : report.isError || !rep ? (
        <p className="text-danger text-sm">{t("office.api.load_error")}</p>
      ) : rep.lines.length === 0 ? (
        <EmptyState
          title={t("office.trial_balance.live_empty_title")}
          description={t("office.trial_balance.live_empty_body")}
        />
      ) : (
        <TrialBalanceTable
          lines={rep.lines}
          level={query.level}
          compared={!!rep.compared_to}
          previousLabel={t("office.trial_balance.col_luca")}
          changeLabel={t("office.trial_balance.col_difference")}
          totals={rep.totals}
        />
      )}
    </div>
  );
}

/**
 * Mizan tablosu: kod, ad, borç, alacak, borç / alacak bakiyesi ve
 * karşılaştırmada önceki (ya da Luca) bakiye ile fark. `totals` verilmezse
 * borç / alacak toplamı yaprak (ana hesap düzeyinde ana) hesaplardan.
 */
function TrialBalanceTable({
  lines,
  level,
  compared,
  previousLabel,
  changeLabel,
  totals: given,
}: {
  lines: TrialBalanceLine[];
  level: number;
  compared: boolean;
  previousLabel: string;
  changeLabel: string;
  totals?: { debit: string; credit: string };
}) {
  const { t } = useLocale();
  const counted = lines.filter((l) =>
    level === 1 ? l.level === 1 : l.is_leaf,
  );
  const totals = counted.reduce(
    (acc, l) => {
      const b = splitBalance(l.balance);
      return {
        debit: acc.debit + Number(l.debit),
        credit: acc.credit + Number(l.credit),
        debitBalance: acc.debitBalance + Number(b.debit || 0),
        creditBalance: acc.creditBalance + Number(b.credit || 0),
      };
    },
    { debit: 0, credit: 0, debitBalance: 0, creditBalance: 0 },
  );
  return (
    // Kaydırılan bölge klavyeyle odaklanabilir olmalı (axe scrollable-region-focusable).
    <div
      className="bg-card overflow-x-auto rounded-xl border"
      role="region"
      tabIndex={0}
      aria-label={t("office.trial_balance.table")}
    >
      <Table
        aria-label={t("office.trial_balance.table")}
        containerClassName="overflow-x-visible"
      >
        <TableHeader>
          <TableRow>
            <TableHead>{t("office.trial_balance.col_code")}</TableHead>
            <TableHead>{t("office.trial_balance.col_name")}</TableHead>
            <TableHead className="text-end">
              {t("office.trial_balance.col_debit")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.trial_balance.col_credit")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.trial_balance.col_debit_balance")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.trial_balance.col_credit_balance")}
            </TableHead>
            {compared ? (
              <>
                <TableHead className="text-end">{previousLabel}</TableHead>
                <TableHead className="text-end">{changeLabel}</TableHead>
              </>
            ) : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.map((l) => (
            <TrialBalanceRow key={l.code} line={l} compared={compared} />
          ))}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell className="px-3 py-2" colSpan={2}>
              {t("office.trial_balance.total")}
            </TableCell>
            <TableCell className="px-3 py-2 text-end">
              <Money value={given?.debit ?? totals.debit.toFixed(2)} />
            </TableCell>
            <TableCell className="px-3 py-2 text-end">
              <Money value={given?.credit ?? totals.credit.toFixed(2)} />
            </TableCell>
            <TableCell className="px-3 py-2 text-end">
              <Money value={totals.debitBalance.toFixed(2)} />
            </TableCell>
            <TableCell className="px-3 py-2 text-end">
              <Money value={totals.creditBalance.toFixed(2)} />
            </TableCell>
            {compared ? <TableCell colSpan={2} /> : null}
          </TableRow>
        </TableFooter>
      </Table>
    </div>
  );
}

function TrialBalanceRow({
  line: l,
  compared,
}: {
  line: TrialBalanceLine;
  compared: boolean;
}) {
  const b = splitBalance(l.balance);
  const main = l.level === 1;
  return (
    <TableRow
      className={main ? "bg-muted/30 border-t font-medium" : "border-t"}
    >
      <TableCell
        className="px-3 py-1.5 font-mono whitespace-nowrap"
        style={{ paddingInlineStart: `${0.75 + (l.level - 1) * 0.75}rem` }}
      >
        {l.code}
      </TableCell>
      <TableCell className="px-3 py-1.5">{l.name}</TableCell>
      <TableCell className="px-3 py-1.5 text-end">
        <Money value={l.debit} />
      </TableCell>
      <TableCell className="px-3 py-1.5 text-end">
        <Money value={l.credit} />
      </TableCell>
      <TableCell className="px-3 py-1.5 text-end">
        {b.debit ? <Money value={b.debit} /> : null}
      </TableCell>
      <TableCell className="px-3 py-1.5 text-end">
        {b.credit ? <Money value={b.credit} /> : null}
      </TableCell>
      {compared ? (
        <>
          <TableCell className="px-3 py-1.5 text-end">
            {l.previous_balance != null ? (
              <Money value={l.previous_balance} />
            ) : null}
          </TableCell>
          <TableCell className="px-3 py-1.5 text-end">
            {l.change != null ? <Money value={l.change} /> : null}
          </TableCell>
        </>
      ) : null}
    </TableRow>
  );
}
