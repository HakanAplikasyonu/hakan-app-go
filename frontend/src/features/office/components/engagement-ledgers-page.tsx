"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, Trash2, Upload } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { StatusPill } from "@/components/tables";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { ApiError } from "@/lib/api/errors";
import { formatAmount } from "@/lib/decimal";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { periodLabel } from "../period/period";
import {
  LEDGER_KINDS,
  ledgerKeys,
  ledgersService,
  type LedgerFile,
  type LedgerKind,
  type LedgerPeriodKind,
  type LedgerSummary,
} from "../services/ledgers.service";
import { fileSize } from "./engagement-archive-page";
import { Panel } from "./office-ui";

type T = (key: string, vars?: Record<string, string | number>) => string;

/** 3 aylıkta dönem çeyreğin son ayıdır: "2026-09" → "2026 3. çeyrek". */
function periodName(
  period: string,
  kind: LedgerPeriodKind,
  locale: string,
  t: T,
) {
  if (kind !== "quarterly") return periodLabel(period, locale);
  const [y, m] = period.split("-").map(Number);
  return t("office.ledgers.quarter", { year: y, q: Math.ceil(m / 3) });
}

/**
 * Mükellef defterleri (#780): ofis her dönem Luca'dan aldığı işletme
 * defteri dökümünü ve e-defter dosyalarını yükler; hangi dönemde hangi
 * dosya var, eksik dönem uyarısı ve dosyadan okunan özet.
 */
export function EngagementLedgersPage({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canWrite = can(permissions.tenant.documents.write);
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [period, setPeriod] = useState("");
  const [kind, setKind] = useState<LedgerKind | "">("");
  const [viewing, setViewing] = useState<LedgerFile | null>(null);
  const [removing, setRemoving] = useState<LedgerFile | null>(null);

  const list = useQuery({
    queryKey: ledgerKeys.all(slug, engagement),
    queryFn: () => ledgersService.list(slug, engagement),
  });
  const data = list.data;
  const periodKind: LedgerPeriodKind = data?.period_kind ?? "monthly";
  const uploadPeriod = period || data?.last_completed || "";
  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: ledgerKeys.all(slug, engagement) });

  const upload = useMutation({
    mutationFn: () =>
      ledgersService.upload(slug, engagement, {
        file: file as File,
        period: uploadPeriod,
        kind: kind || undefined,
      }),
    onSuccess: (out) => {
      invalidate();
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      toast.success(
        out.replayed
          ? t("office.ledgers.replayed")
          : out.item.readable
            ? t("office.ledgers.uploaded")
            : t("office.ledgers.uploaded_unreadable"),
      );
    },
    onError: (err) => {
      const e = err as ApiError;
      toast.error(
        e?.code === "STORAGE_UNAVAILABLE"
          ? t("office.ledgers.storage_error")
          : e?.status === 400
            ? t("office.ledgers.bad_file")
            : t("office.ledgers.error"),
      );
    },
  });

  const settings = useMutation({
    mutationFn: (k: LedgerPeriodKind) =>
      ledgersService.saveSettings(slug, engagement, k),
    onSuccess: () => {
      invalidate();
      toast.success(t("office.ledgers.settings_saved"));
    },
    onError: () => toast.error(t("office.ledgers.error")),
  });

  const remove = useMutation({
    mutationFn: (f: LedgerFile) =>
      ledgersService.remove(slug, engagement, f.uuid),
    onSuccess: () => {
      invalidate();
      toast.success(t("office.ledgers.deleted"));
    },
    onError: () => toast.error(t("office.ledgers.error")),
    onSettled: () => setRemoving(null),
  });

  const missing = data?.missing ?? [];
  const items = data?.items ?? [];
  const periods = data?.periods ?? [];

  return (
    <div className="space-y-4">
      <Panel
        title={t("office.ledgers.title")}
        description={t("office.ledgers.description")}
        actions={
          <div className="flex items-center gap-2">
            <Label htmlFor="ledger-period-kind" className="text-sm">
              {t("office.ledgers.period_kind")}
            </Label>
            <NativeSelect
              id="ledger-period-kind"
              size="sm"
              value={periodKind}
              disabled={!canWrite || settings.isPending || !data}
              onChange={(e) =>
                settings.mutate(e.target.value as LedgerPeriodKind)
              }
            >
              <NativeSelectOption value="monthly">
                {t("office.ledgers.period_monthly")}
              </NativeSelectOption>
              <NativeSelectOption value="quarterly">
                {t("office.ledgers.period_quarterly")}
              </NativeSelectOption>
            </NativeSelect>
          </div>
        }
      >
        <div className="space-y-4 p-4">
          {canWrite ? (
            <form
              className="flex flex-wrap items-end gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (file && uploadPeriod) upload.mutate();
              }}
            >
              <div className="space-y-1">
                <Label htmlFor="ledger-month">
                  {t("office.ledgers.upload_period")}
                </Label>
                <MonthPicker
                  id="ledger-month"
                  aria-label={t("office.ledgers.upload_period")}
                  value={uploadPeriod}
                  onChange={(v) => setPeriod(v)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="ledger-kind">{t("office.ledgers.kind")}</Label>
                <NativeSelect
                  id="ledger-kind"
                  value={kind}
                  onChange={(e) => setKind(e.target.value as LedgerKind | "")}
                >
                  <NativeSelectOption value="">
                    {t("office.ledgers.kind_auto")}
                  </NativeSelectOption>
                  {LEDGER_KINDS.map((k) => (
                    <NativeSelectOption key={k} value={k}>
                      {t(`office.ledgers.kind_${k}`)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
              <div className="space-y-1">
                <Label htmlFor="ledger-file">{t("office.ledgers.file")}</Label>
                <Input
                  id="ledger-file"
                  ref={fileRef}
                  type="file"
                  className="w-72"
                  accept=".xlsx,.xls,.csv,.xml,.zip"
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                />
              </div>
              <Button
                type="submit"
                size="sm"
                disabled={!file || !uploadPeriod || upload.isPending}
              >
                <Upload className="size-4" aria-hidden />
                {t("office.ledgers.upload")}
              </Button>
            </form>
          ) : null}

          {missing.length > 0 ? (
            <p
              role="status"
              data-testid="ledgers-missing"
              className="border-status-warning/40 bg-status-warning/10 text-status-warning rounded-lg border px-3 py-2 text-sm"
            >
              {t("office.ledgers.missing", {
                count: missing.length,
                periods: missing
                  .slice(0, 6)
                  .map((p) => periodName(p, periodKind, locale, t))
                  .join(", "),
              })}
            </p>
          ) : data && periods.some((p) => p.expected) ? (
            <p className="text-status-success text-sm">
              {t("office.ledgers.none_missing")}
            </p>
          ) : null}
        </div>
      </Panel>

      <Panel title={t("office.ledgers.help_title")}>
        <ul className="text-muted-foreground list-disc space-y-1 p-4 ps-8 text-sm">
          <li>{t("office.ledgers.help_isletme")}</li>
          <li>{t("office.ledgers.help_edefter")}</li>
          <li>{t("office.ledgers.help_luca")}</li>
          <li>{t("office.ledgers.help_period")}</li>
        </ul>
      </Panel>

      {list.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : list.isError || !data ? (
        <p className="text-danger text-sm">{t("office.api.load_error")}</p>
      ) : (
        <>
          <Panel title={t("office.ledgers.periods_title")}>
            {periods.length === 0 ? (
              <p className="text-muted-foreground p-4 text-sm">
                {t("office.ledgers.periods_empty")}
              </p>
            ) : (
              <Table aria-label={t("office.ledgers.periods_title")}>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("office.ledgers.col_period")}</TableHead>
                    <TableHead>{t("office.ledgers.col_files")}</TableHead>
                    <TableHead>{t("office.ledgers.col_status")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {periods.map((p) => (
                    <TableRow key={p.period}>
                      <TableCell className="font-medium">
                        {periodName(p.period, periodKind, locale, t)}
                      </TableCell>
                      <TableCell className="text-sm">
                        {p.kinds.length > 0
                          ? p.kinds
                              .map((k) => t(`office.ledgers.kind_${k}`))
                              .join(", ")
                          : "—"}
                      </TableCell>
                      <TableCell>
                        {p.missing ? (
                          <StatusPill tone="warn">
                            {t("office.ledgers.status_missing")}
                          </StatusPill>
                        ) : p.files > 0 ? (
                          <StatusPill tone="ok">
                            {t("office.ledgers.status_uploaded")}
                          </StatusPill>
                        ) : (
                          <StatusPill>—</StatusPill>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>

          {items.length === 0 ? (
            <EmptyState
              title={t("office.ledgers.empty_title")}
              description={t("office.ledgers.empty_body")}
            />
          ) : (
            <Panel title={t("office.ledgers.files_title")}>
              <Table aria-label={t("office.ledgers.files_title")}>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("office.ledgers.col_period")}</TableHead>
                    <TableHead>{t("office.ledgers.col_kind")}</TableHead>
                    <TableHead>{t("office.ledgers.col_file")}</TableHead>
                    <TableHead>{t("office.ledgers.col_summary")}</TableHead>
                    <TableHead>{t("office.ledgers.col_uploaded_by")}</TableHead>
                    <TableHead className="text-end">
                      {t("office.ledgers.col_actions")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((f) => (
                    <TableRow key={f.uuid}>
                      <TableCell>{periodLabel(f.period, locale)}</TableCell>
                      <TableCell className="text-sm">
                        {t(`office.ledgers.kind_${f.kind}`)}
                      </TableCell>
                      <TableCell className="text-sm">
                        <span className="font-medium break-all">
                          {f.file_name}
                        </span>
                        <span className="text-muted-foreground block text-xs">
                          {fileSize(f.size_bytes, locale)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <StatusPill tone={f.readable ? "ok" : "warn"}>
                          {f.readable
                            ? t("office.ledgers.readable")
                            : t("office.ledgers.unreadable")}
                        </StatusPill>
                      </TableCell>
                      <TableCell className="text-sm">
                        {f.uploaded_by_name || "—"}
                      </TableCell>
                      <TableCell className="text-end whitespace-nowrap">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setViewing(f)}
                        >
                          <Eye className="size-4" aria-hidden />
                          {t("office.ledgers.view_summary")}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={t("office.ledgers.download")}
                          onClick={() =>
                            void ledgersService
                              .download(slug, engagement, f)
                              .catch(() =>
                                toast.error(t("office.ledgers.error")),
                              )
                          }
                        >
                          <Download className="size-4" aria-hidden />
                        </Button>
                        {canWrite ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={t("office.ledgers.delete")}
                            onClick={() => setRemoving(f)}
                          >
                            <Trash2 className="size-4" aria-hidden />
                          </Button>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>
          )}
        </>
      )}

      <SummaryDialog
        engagement={engagement}
        file={viewing}
        onClose={() => setViewing(null)}
      />

      <AlertDialog
        open={removing !== null}
        onOpenChange={(o) => {
          if (!o) setRemoving(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("office.ledgers.delete_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("office.ledgers.delete_body", {
                name: removing?.file_name ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("office.ledgers.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (removing) remove.mutate(removing);
              }}
            >
              {t("office.ledgers.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function SummaryDialog({
  engagement,
  file,
  onClose,
}: {
  engagement: string;
  file: LedgerFile | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const q = useQuery({
    queryKey: ledgerKeys.detail(slug, engagement, file?.uuid ?? ""),
    queryFn: () => ledgersService.get(slug, engagement, file?.uuid ?? ""),
    enabled: file !== null,
  });
  return (
    <Dialog
      open={file !== null}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("office.ledgers.summary_title")}</DialogTitle>
          <DialogDescription>
            {file
              ? `${file.file_name} · ${periodLabel(file.period, locale)}`
              : ""}
          </DialogDescription>
        </DialogHeader>
        {q.isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : q.isError || !q.data ? (
          <p className="text-danger text-sm">{t("office.api.load_error")}</p>
        ) : (
          <SummaryBody summary={q.data.summary} />
        )}
      </DialogContent>
    </Dialog>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-surface-2 rounded-lg border p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="text-lg font-semibold tabular-nums">₺ {value}</p>
    </div>
  );
}

function SummaryBody({ summary }: { summary: LedgerSummary }) {
  const { t } = useLocale();
  if (!summary.readable) {
    return (
      <div className="space-y-1 text-sm" data-testid="ledger-unreadable">
        <p className="font-medium">{t("office.ledgers.unreadable_title")}</p>
        <p className="text-muted-foreground">
          {t(`office.ledgers.reason_${summary.reason ?? "unsupported"}`)}
        </p>
      </div>
    );
  }
  const is = summary.isletme;
  const ed = summary.e_defter;
  const mz = summary.mizan;
  const br = summary.bordro;
  return (
    <div className="space-y-4" data-testid="ledger-summary">
      {summary.files && summary.files.length > 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("office.ledgers.zip_files", { files: summary.files.join(", ") })}
        </p>
      ) : null}
      {is ? (
        <>
          <div className="grid gap-3 sm:grid-cols-4">
            <Stat
              label={t("office.ledgers.sum_purchases")}
              value={formatAmount(is.purchases)}
            />
            <Stat
              label={t("office.ledgers.sum_expenses")}
              value={formatAmount(is.expenses)}
            />
            <Stat
              label={t("office.ledgers.sum_sales")}
              value={formatAmount(is.sales)}
            />
            <Stat
              label={t("office.ledgers.sum_income")}
              value={formatAmount(is.income)}
            />
          </div>
          <p className="text-muted-foreground text-xs">
            {t("office.ledgers.rows_read", { rows: is.rows })}
          </p>
          {is.top.length > 0 ? (
            <Table aria-label={t("office.ledgers.top_lines")}>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("office.ledgers.col_date")}</TableHead>
                  <TableHead>{t("office.ledgers.col_description")}</TableHead>
                  <TableHead>{t("office.ledgers.col_side")}</TableHead>
                  <TableHead className="text-end">
                    {t("office.ledgers.col_amount")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {is.top.map((l, i) => (
                  <TableRow key={`${l.date ?? ""}-${i}`}>
                    <TableCell className="whitespace-nowrap">
                      {l.date ? l.date.split("-").reverse().join(".") : "—"}
                    </TableCell>
                    <TableCell>{l.description || "—"}</TableCell>
                    <TableCell>{t(`office.ledgers.side_${l.side}`)}</TableCell>
                    <TableCell className="text-end tabular-nums">
                      {formatAmount(l.amount)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : null}
        </>
      ) : null}
      {ed ? (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            <Stat
              label={t("office.ledgers.debit_total")}
              value={formatAmount(ed.debit_total)}
            />
            <Stat
              label={t("office.ledgers.credit_total")}
              value={formatAmount(ed.credit_total)}
            />
          </div>
          <p className="text-muted-foreground text-xs">
            {t("office.ledgers.entries_read", {
              entries: ed.entries,
              lines: ed.lines,
            })}
          </p>
          <Table aria-label={t("office.ledgers.accounts")}>
            <TableHeader>
              <TableRow>
                <TableHead>{t("office.ledgers.col_code")}</TableHead>
                <TableHead>{t("office.ledgers.col_account")}</TableHead>
                <TableHead className="text-end">
                  {t("office.ledgers.col_debit")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.ledgers.col_credit")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {ed.accounts.map((a) => (
                <TableRow key={a.code}>
                  <TableCell className="font-mono">{a.code}</TableCell>
                  <TableCell>{a.name || "—"}</TableCell>
                  <TableCell className="text-end tabular-nums">
                    {formatAmount(a.debit)}
                  </TableCell>
                  <TableCell className="text-end tabular-nums">
                    {formatAmount(a.credit)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      ) : null}
      {mz ? <MizanBody mizan={mz} /> : null}
      {br ? <BordroBody bordro={br} /> : null}
      {!is && !ed && !mz && !br ? (
        <p className="text-muted-foreground text-sm">
          {t("office.ledgers.no_details")}
        </p>
      ) : null}
    </div>
  );
}

type MizanSummary = NonNullable<LedgerSummary["mizan"]>;
type BordroSummary = NonNullable<LedgerSummary["bordro"]>;

/** "2026-10" → "Ekim 2026"; boşsa tire. */
function filePeriod(period: string | undefined, locale: string) {
  return period ? periodLabel(period, locale) : "—";
}

/** Luca mizanı: ana hesap toplamları, denge ve hesap listesi. */
function MizanBody({ mizan }: { mizan: MizanSummary }) {
  const { t, locale } = useLocale();
  return (
    <div className="space-y-3" data-testid="ledger-mizan">
      <p className="text-muted-foreground text-xs">
        {t("office.ledgers.mizan_read", {
          accounts: mizan.accounts,
          row: mizan.header_row,
          period: filePeriod(mizan.period, locale),
        })}
      </p>
      <div className="grid gap-3 sm:grid-cols-4">
        <Stat
          label={t("office.ledgers.debit_total")}
          value={formatAmount(mizan.debit_total)}
        />
        <Stat
          label={t("office.ledgers.credit_total")}
          value={formatAmount(mizan.credit_total)}
        />
        <Stat
          label={t("office.ledgers.debit_balance")}
          value={formatAmount(mizan.debit_balance)}
        />
        <Stat
          label={t("office.ledgers.credit_balance")}
          value={formatAmount(mizan.credit_balance)}
        />
      </div>
      <StatusPill tone={mizan.balanced ? "ok" : "warn"}>
        {mizan.balanced
          ? t("office.ledgers.mizan_balanced")
          : t("office.ledgers.mizan_unbalanced")}
      </StatusPill>
      <Table aria-label={t("office.ledgers.mizan_accounts")}>
        <TableHeader>
          <TableRow>
            <TableHead>{t("office.ledgers.col_code")}</TableHead>
            <TableHead>{t("office.ledgers.col_account")}</TableHead>
            <TableHead className="text-end">
              {t("office.ledgers.col_debit")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.ledgers.col_credit")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.ledgers.col_debit_balance")}
            </TableHead>
            <TableHead className="text-end">
              {t("office.ledgers.col_credit_balance")}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {mizan.lines.map((a) => (
            <TableRow key={a.code}>
              <TableCell className="font-mono">{a.code}</TableCell>
              <TableCell>{a.name || "—"}</TableCell>
              <TableCell className="text-end tabular-nums">
                {formatAmount(a.debit)}
              </TableCell>
              <TableCell className="text-end tabular-nums">
                {formatAmount(a.credit)}
              </TableCell>
              <TableCell className="text-end tabular-nums">
                {formatAmount(a.debit_balance)}
              </TableCell>
              <TableCell className="text-end tabular-nums">
                {formatAmount(a.credit_balance)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

/** Luca ücret bordrosu: alt özet toplamları (muhasebe kaydı için). */
function BordroBody({ bordro }: { bordro: BordroSummary }) {
  const { t, locale } = useLocale();
  const rows: [string, string][] = [
    ["bordro_gross", bordro.gross],
    ["bordro_sgk_employee", bordro.sgk_employee],
    ["bordro_sgk_employer", bordro.sgk_employer],
    ["bordro_unemployment_employee", bordro.unemployment_employee],
    ["bordro_unemployment_employer", bordro.unemployment_employer],
    ["bordro_income_tax", bordro.income_tax],
    ["bordro_stamp_tax", bordro.stamp_tax],
    ["bordro_net", bordro.net],
    ["bordro_employer_cost", bordro.employer_cost],
  ];
  return (
    <div className="space-y-3" data-testid="ledger-bordro">
      <p className="text-muted-foreground text-xs">
        {t("office.ledgers.bordro_read", {
          persons: bordro.persons,
          period: filePeriod(bordro.period, locale),
        })}
      </p>
      {!bordro.from_totals ? (
        <p className="text-status-warning text-xs">
          {t("office.ledgers.bordro_no_totals")}
        </p>
      ) : null}
      <Table aria-label={t("office.ledgers.bordro_totals")}>
        <TableBody>
          {rows.map(([key, value]) => (
            <TableRow key={key}>
              <TableCell>{t(`office.ledgers.${key}`)}</TableCell>
              <TableCell className="text-end tabular-nums">
                {formatAmount(value)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <p className="text-muted-foreground text-xs">
        {t("office.ledgers.bordro_payroll_hint")}
      </p>
    </div>
  );
}
