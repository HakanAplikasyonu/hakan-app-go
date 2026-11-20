"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileDown, Upload } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useRef, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
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

function TrialBalanceInner({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const router = useRouter();
  const search = useSearchParams();
  const fileRef = useRef<HTMLInputElement>(null);
  const [exporting, setExporting] = useState<"xlsx" | "pdf" | null>(null);

  const which = search.get("mizan") ?? "latest";
  const level = search.get("level") === "1" ? 1 : 0;
  const compare = search.get("compare") === "none" ? "none" : "previous";
  const query: TrialBalanceQuery = { which, level, compare };
  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(search.toString());
    if (value === null) next.delete(key);
    else next.set(key, value);
    router.replace(`?${next.toString()}`, { scroll: false });
  };

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
      <Button
        size="sm"
        variant="outline"
        disabled={upload.isPending}
        onClick={() => fileRef.current?.click()}
      >
        <Upload className="size-4" aria-hidden />
        {t("office.trial_balance.upload")}
      </Button>
    </>
  ) : null;

  if (list.isLoading) return <Skeleton className="h-64 w-full" />;
  if (list.isError)
    return <p className="text-danger text-sm">{t("office.api.load_error")}</p>;
  if (items.length === 0)
    return (
      <EmptyState
        title={t("office.trial_balance.empty_title")}
        description={t("office.trial_balance.empty_body")}
        action={uploadButton}
      />
    );

  const rep = report.data;
  const lines = rep?.lines ?? [];
  const compared = !!rep?.compared_to;
  const totals = lines
    .filter((l) => (level === 1 ? l.level === 1 : l.is_leaf))
    .reduce(
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
        <div className="bg-card rounded-xl border">
          <Table aria-label={t("office.trial_balance.table")}>
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
                    <TableHead className="text-end">
                      {t("office.trial_balance.col_previous")}
                    </TableHead>
                    <TableHead className="text-end">
                      {t("office.trial_balance.col_change")}
                    </TableHead>
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
                  <Money value={totals.debit.toFixed(2)} />
                </TableCell>
                <TableCell className="px-3 py-2 text-end">
                  <Money value={totals.credit.toFixed(2)} />
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
      )}
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
