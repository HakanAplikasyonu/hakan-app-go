"use client";

import {
  BookOpenCheck,
  Check,
  CircleAlert,
  CircleCheck,
  Download,
  FileSpreadsheet,
  ListTree,
  Loader2,
  MonitorUp,
  Undo2,
} from "lucide-react";
import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { ColumnDef } from "@tanstack/react-table";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createMoneyColumn,
  DataTable,
  StatusPill,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Hint } from "@/components/ui/hint";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DRAFTS_ANCHOR,
  EXPORT_ANCHOR,
  TRANSFERS_ANCHOR,
} from "@/config/routes";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useExportDrivers,
  useJournalActions,
  useJournalDraft,
  useJournalDrafts,
  useJournalExports,
  useJournalTransfers,
} from "../hooks/use-journals";
import {
  absDiff,
  formatMoney,
  fromMinor,
  parseAmountInput,
  sum,
  toMinor,
} from "../lib/decimal";
import { exportIssueText } from "../lib/export-issue";
import { periodService } from "../services/periods.service";
import { journalErrorText } from "../lib/journal-error";
import {
  journalService,
  TRANSFER_DRIVERS,
  type JournalDraft,
  type JournalDraftIssue,
  type JournalExportSummary,
  type JournalTransfer,
} from "../services/journals.service";
import { useEngagementScope } from "../engagement-scope";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import {
  DOUBLE_BOOKING_CODES,
  DoubleBookingReview,
  ExportDocumentsPanel,
  LucaPullSettingsPanel,
} from "./journal-double-booking";
import {
  JournalTransfersPanel,
  TRANSFER_STATUS,
} from "./journal-transfers-panel";
import { FilterSelect, Figure, Panel } from "./office-ui";
import { JournalDraftLines } from "./journal-draft-lines";
import { NewDraft } from "./journal-new-draft";
import { useRelatedColumn } from "./related-column";
import {
  ALL_PERIODS,
  isPeriod,
  usePeriodText,
  useUrlOfficePeriod,
} from "../period";

const thisMonth = () => new Date().toISOString().slice(0, 7);

const statusClass: Record<JournalDraft["status"], string> = {
  draft: "bg-secondary text-secondary-foreground",
  approved: "bg-status-approved/12 text-status-approved",
  exported: "bg-brand/12 text-brand",
};

type Translate = (key: string) => string;

// Gömülü tablolar: araç çubuğu, sayfalama, seçim ve görünüm ayarları yok.
const embeddedTable: DataTableFeatures = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  facetedFilters: false,
  globalFilter: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
};

/** Program label for an export driver id; the id itself when unknown. */
function driverLabel(t: Translate, name: string) {
  const key = `office.journals.driver_${name}`;
  const msg = t(key);
  return msg === key ? name : msg;
}

/** Onayı durduran engel var mı (#566); hesap kodu sorunları uyarıdır. */
const isBlocked = (d: JournalDraft) => !!d.issues?.some((i) => i.blocking);
/** Onayı engellemeyen ama açıkça sorulan uyarılar (#669, ofis kararı). */
const CONFIRM_CODES = ["missing_document", "possible_duplicate"];
const warningsOf = (d: JournalDraft) =>
  (d.issues ?? []).filter((i) => CONFIRM_CODES.includes(i.code));
/**
 * Mükerrer olabilir (#710, ofis kararı): onay engellenir; yalnız fişin
 * kendi satırındaki ayrı "Yine de gönder" ve zorunlu gerekçeyle geçer.
 * Toplu onay bu fişleri atlar.
 */
const isDuplicate = (d: JournalDraft) =>
  !!d.issues?.some((i) => i.code === "possible_duplicate");

function issueText(
  t: (key: string, vars?: Record<string, string | number>) => string,
  is: JournalDraftIssue,
) {
  return t(`office.journals.check.${is.code}`, {
    line: is.line_no ?? "",
    account: is.account ?? "",
    amount: formatMoney(is.difference ?? "0"),
  });
}

/**
 * Taslak fişin kontrolü (#566): engeller ve hesap kodu engellerinde
 * "Hesabı düzelt". Düzeltilince liste yenilenir, engel kalkar.
 */
function DraftCheck({
  draft,
  engagement,
}: {
  draft: JournalDraft;
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { prompt } = useDialogs();
  const { setLineAccount } = useJournalActions(slug, engagement);
  const [amountsOpen, setAmountsOpen] = useState(false);
  const { permissions: perms } = useTenant();
  const canUnlock = perms.includes(permissions.tenant.periods.lock);
  const qc = useQueryClient();
  const unlock = useMutation({
    mutationFn: (reason: string) =>
      periodService.unlock(slug, engagement, draft.period, reason),
    onSuccess: () => {
      toast.success(
        t("office.journals.check.unlocked", { period: draft.period }),
      );
      // Fiş engelleri ve dönem durumu (şerit, komuta merkezi) yenilenir.
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase()],
      });
    },
    onError: (err) =>
      toast.error(journalErrorText(t, err, t("office.api.load_error"))),
  });
  const openPeriod = async () => {
    const reason = await prompt({
      title: t("office.journals.check.unlock_title", { period: draft.period }),
      description: t("office.journals.check.unlock_prompt"),
      confirmLabel: t("office.journals.check.unlock_period"),
    });
    if (reason?.trim()) unlock.mutate(reason.trim());
  };
  const issues = draft.issues ?? [];
  if (draft.status !== "draft") return null;
  if (!issues.length) {
    return (
      <span className="flex flex-col gap-0.5">
        <span className="text-status-approved inline-flex items-center gap-1 text-xs">
          <CircleCheck className="size-3.5" aria-hidden />
          {t("office.journals.check.ok")}
        </span>
        {/* Onayı geri alınmış fiş (#566): neden taslağa döndüğü görünür. */}
        {draft.approval_reverted ? (
          <span
            className="text-muted-foreground text-xs"
            data-testid="approval-reverted"
          >
            {t("office.journals.approval_reverted", {
              reason: draft.approval_reverted.reason,
            })}
          </span>
        ) : null}
      </span>
    );
  }
  const fix = async (is: JournalDraftIssue) => {
    const account = await prompt({
      title: t("office.journals.check.fix_title", { line: is.line_no ?? "" }),
      description: t("office.journals.check.fix_description", {
        account: is.account ?? "",
      }),
      placeholder: "770.01",
      confirmLabel: t("office.journals.check.fix_confirm"),
    });
    const code = account?.trim();
    if (!code || !is.line_no) return;
    setLineAccount.mutate(
      { draft: draft.uuid, line: is.line_no, account: code },
      {
        onSuccess: () =>
          toast.success(
            t("office.journals.check.fixed", {
              line: is.line_no ?? "",
              account: code,
            }),
          ),
        onError: (err) =>
          toast.error(journalErrorText(t, err, t("office.api.load_error"))),
      },
    );
  };
  return (
    <>
      <ul className="space-y-1 text-xs">
        {issues.map((is, i) => (
          <li key={i} className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <span
              className={cn(
                "inline-flex items-center gap-1",
                is.blocking ? "text-status-blocking" : "text-status-warning",
              )}
            >
              <CircleAlert className="size-3.5 shrink-0" aria-hidden />
              {issueText(t, is)}
            </span>
            {/* Kapalı dönem (#669 A): yetkili kullanıcı dönemi buradan,
              gerekçesiyle açar; yetkisi olmayan kime başvuracağını görür. */}
            {is.code === "period_locked" ? (
              canUnlock ? (
                <Button
                  size="sm"
                  variant="link"
                  className="h-auto p-0 text-xs"
                  disabled={unlock.isPending}
                  onClick={() => void openPeriod()}
                >
                  {t("office.journals.check.unlock_period")}
                </Button>
              ) : (
                <span className="text-muted-foreground">
                  {t("office.journals.check.unlock_no_permission")}
                </span>
              )
            ) : null}
            {is.code === "unbalanced" ? (
              <Button
                size="sm"
                variant="link"
                className="h-auto p-0 text-xs"
                onClick={() => setAmountsOpen(true)}
              >
                {t("office.journals.check.fix_amounts")}
              </Button>
            ) : null}
            {is.code === "unknown_account" || is.code === "parent_account" ? (
              <Button
                size="sm"
                variant="link"
                className="h-auto p-0 text-xs"
                disabled={setLineAccount.isPending}
                onClick={() => void fix(is)}
              >
                {t("office.journals.check.fix_account")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      {amountsOpen ? (
        <DraftAmountsDialog
          draft={draft}
          engagement={engagement}
          onClose={() => setAmountsOpen(false)}
        />
      ) : null}
    </>
  );
}

type AmountEdit = { debit: string; credit: string };

/**
 * Borç-alacak tutmayan taslağın tutar düzeltmesi (#566): kaynaksız (elle
 * girilmiş) satırların borç/alacağı yazılır, fark canlı görünür. Belgeden
 * gelen satırın tutarı belgesinden düzeltilir; burada yalnız okunur.
 */
function DraftAmountsDialog({
  draft,
  engagement,
  onClose,
}: {
  draft: JournalDraft;
  engagement: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const detail = useJournalDraft(slug, engagement, draft.uuid);
  const { setLineAmounts } = useJournalActions(slug, engagement);
  const [edits, setEdits] = useState<Record<number, AmountEdit>>({});
  const lines = detail.data?.items ?? [];
  const value = (no: number, side: keyof AmountEdit, current: string) =>
    edits[no]?.[side] ?? (Number(current) ? current.replace(".", ",") : "");
  const parsed = lines.map((l) => {
    const debit = parseAmountInput(value(l.line_no, "debit", l.debit));
    const credit = parseAmountInput(value(l.line_no, "credit", l.credit));
    return { line: l, debit, credit };
  });
  const invalid = parsed.some(
    (p) =>
      p.debit === null ||
      p.credit === null ||
      // Tek taraf dolu olmalı: ya borç ya alacak.
      Number(p.debit || 0) > 0 === Number(p.credit || 0) > 0,
  );
  const debitTotal = sum(parsed.map((p) => p.debit || "0"));
  const creditTotal = sum(parsed.map((p) => p.credit || "0"));
  const diff = invalid ? null : fromMinor(absDiff(debitTotal, creditTotal));
  const changed = parsed.filter(
    (p) =>
      !p.line.source_kind &&
      (toMinor(p.debit) !== toMinor(p.line.debit) ||
        toMinor(p.credit) !== toMinor(p.line.credit)),
  );
  const set = (no: number, side: keyof AmountEdit, v: string, l: AmountEdit) =>
    setEdits((prev) => ({ ...prev, [no]: { ...l, ...prev[no], [side]: v } }));
  const save = () => {
    setLineAmounts.mutate(
      {
        draft: draft.uuid,
        lines: changed.map((p) => ({
          line: p.line.line_no,
          debit: p.debit || "0",
          credit: p.credit || "0",
        })),
      },
      {
        onSuccess: () => {
          toast.success(
            t("office.journals.check.amounts_fixed", { count: changed.length }),
          );
          onClose();
        },
        onError: (err) =>
          toast.error(journalErrorText(t, err, t("office.api.load_error"))),
      },
    );
  };
  return (
    <Dialog open onOpenChange={(open) => (open ? null : onClose())}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("office.journals.check.amounts_title", {
              entry: draft.entry_no,
            })}
          </DialogTitle>
          <DialogDescription>
            {t("office.journals.check.amounts_description")}
          </DialogDescription>
        </DialogHeader>
        {detail.isPending ? (
          <Skeleton className="h-24 w-full" />
        ) : (
          <Table data-testid="draft-amounts">
            <TableHeader>
              <TableRow className="text-muted-foreground text-left text-xs">
                <TableHead className="py-1 pr-2 font-medium">
                  {t("office.journals.check.amounts_line")}
                </TableHead>
                <TableHead className="py-1 pr-2 font-medium">
                  {t("office.journals.check.amounts_account")}
                </TableHead>
                <TableHead className="py-1 pr-2 text-right font-medium">
                  {t("office.journals.check.amounts_debit")}
                </TableHead>
                <TableHead className="py-1 text-right font-medium">
                  {t("office.journals.check.amounts_credit")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l) => {
                const base = {
                  debit: value(l.line_no, "debit", l.debit),
                  credit: value(l.line_no, "credit", l.credit),
                };
                return (
                  <TableRow key={l.line_no} className="border-t align-top">
                    <TableCell className="py-1.5 pr-2 tabular-nums">
                      {l.line_no}
                    </TableCell>
                    <TableCell className="py-1.5 pr-2">
                      <span className="font-mono">{l.account}</span>
                      {l.source_kind ? (
                        <span className="text-muted-foreground block text-xs">
                          {t("office.journals.check.amounts_from_source")}
                        </span>
                      ) : null}
                    </TableCell>
                    {(["debit", "credit"] as const).map((side) => (
                      <TableCell
                        key={side}
                        className={cn(
                          "py-1.5 text-right tabular-nums",
                          side === "debit" && "pr-2",
                        )}
                      >
                        {l.source_kind ? (
                          formatMoney(l[side])
                        ) : (
                          <Input
                            inputMode="decimal"
                            className="h-8 w-32 text-right tabular-nums"
                            aria-label={t(
                              `office.journals.check.amounts_${side}_label`,
                              { line: l.line_no },
                            )}
                            value={base[side]}
                            onChange={(e) =>
                              set(l.line_no, side, e.target.value, base)
                            }
                          />
                        )}
                      </TableCell>
                    ))}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
        {lines.length ? (
          <p
            className={cn(
              "text-sm",
              diff === "0.00" ? "text-status-approved" : "text-status-blocking",
            )}
            data-testid="draft-amounts-diff"
            aria-live="polite"
          >
            {diff === null
              ? t("office.journals.check.amounts_invalid")
              : diff === "0.00"
                ? t("office.journals.check.amounts_balanced")
                : t("office.journals.check.amounts_diff", {
                    amount: formatMoney(diff),
                  })}
          </p>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("office.journals.check.amounts_cancel")}
          </Button>
          <Button
            disabled={invalid || !changed.length || setLineAmounts.isPending}
            onClick={save}
          >
            {setLineAmounts.isPending ? (
              <Loader2 className="size-4 animate-spin" aria-hidden />
            ) : null}
            {t("office.journals.check.amounts_save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Engelsiz taslakları toplu onay (#566): yalnız kontrolden geçen fişler;
 * engelliler taslakta kalır.
 */
function ApproveClean({
  engagement,
  drafts,
}: {
  engagement: string;
  drafts: JournalDraft[];
}) {
  const { t } = useLocale();
  const { slug, permissions: perms } = useTenant();
  const { confirm, prompt } = useDialogs();
  const { approve } = useJournalActions(slug, engagement);
  const [busy, setBusy] = useState(false);
  const open = drafts.filter((d) => d.status === "draft");
  const clean = open.filter((d) => !isBlocked(d) && !isDuplicate(d));
  const duplicates = open.filter((d) => !isBlocked(d) && isDuplicate(d));
  // Son onay sorumlu müşavirde (#566).
  if (!clean.length || !perms.includes(permissions.tenant.journals.approve))
    return null;
  const warned = clean.filter((d) => warningsOf(d).length > 0).length;
  const run = async () => {
    const description = [
      t("office.journals.check.approve_clean_description", {
        blocked: open.length - clean.length - duplicates.length,
      }),
      duplicates.length
        ? t("office.journals.check.approve_clean_duplicates", {
            count: duplicates.length,
          })
        : "",
    ]
      .filter(Boolean)
      .join(" ");
    // Uyarılı fiş varsa kaç tanesi olduğu açıkça söylenir; gerekçe isteğe bağlı.
    let note = "";
    if (warned > 0) {
      const answer = await prompt({
        title: t("office.journals.check.approve_clean_title", {
          count: clean.length,
        }),
        description: `${description} ${t("office.journals.check.approve_clean_warned", { count: warned })}`,
        placeholder: t("office.journals.check.confirm_note"),
        confirmLabel: t("office.journals.check.confirm_approve"),
        optional: true,
      });
      if (answer === null) return;
      note = answer.trim();
    } else {
      const ok = await confirm({
        title: t("office.journals.check.approve_clean_title", {
          count: clean.length,
        }),
        description,
        confirmLabel: t("office.journals.approve"),
      });
      if (!ok) return;
    }
    setBusy(true);
    let done = 0;
    let failed = 0;
    let reason = "";
    for (const d of clean) {
      try {
        await approve.mutateAsync(
          warningsOf(d).length
            ? { draft: d.uuid, confirm: true, note }
            : d.uuid,
        );
        done += 1;
      } catch (err) {
        failed += 1;
        reason = journalErrorText(t, err, t("office.api.load_error"));
      }
    }
    setBusy(false);
    if (failed) {
      toast.error(
        t("office.journals.check.approve_partial", {
          ok: done,
          failed,
          reason,
        }),
      );
    } else {
      toast.success(t("office.journals.check.approved", { count: done }));
    }
  };
  return (
    <Button size="sm" disabled={busy} onClick={() => void run()}>
      {busy ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <Check className="size-4" />
      )}
      {t("office.journals.check.approve_clean", { count: clean.length })}
    </Button>
  );
}

/**
 * Where an exported entry went (#574): only the file, or the transfer to
 * the program and its outcome. A failed transfer is retried in the
 * transfers panel below.
 */
function DraftTransfer({ draft }: { draft: JournalDraft }) {
  const { t } = useLocale();
  const x = draft.export;
  if (!x) return null;
  if (!x.transfer_status)
    return (
      <span className="text-muted-foreground text-xs">
        {t("office.journals.transfer_file_only")}
      </span>
    );
  const v = TRANSFER_STATUS[x.transfer_status];
  const pill = (
    <StatusPill tone={v?.tone}>
      {v ? t(v.labelKey) : x.transfer_status}
    </StatusPill>
  );
  return x.transfer_status === "failed" && x.transfer_error ? (
    <Hint label={x.transfer_error} wrap>
      {pill}
    </Hint>
  ) : (
    pill
  );
}

function ExportPanel({
  engagement,
  period,
  selected,
  onClearSelection,
}: {
  engagement: string;
  period: string;
  /** Fişler tablosunda seçilen onaylı fişler (#574); boşsa dönemin tamamı. */
  selected: string[];
  onClearSelection: () => void;
}) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const drivers = useExportDrivers(slug);
  const { export: run, check } = useJournalActions(slug, engagement);
  const items = drivers.data?.items ?? [];
  const [driver, setDriver] = useState("luca");
  // Documents the accountant confirmed after the pre-export check (#466).
  const [confirmed, setConfirmed] = useState<Set<string>>(new Set());
  const chosen = items.find((d) => d.name === driver);
  const review =
    check.data && check.data.driver === driver && check.data.excluded.length
      ? check.data
      : undefined;
  const blocked =
    run.isError && isApiError(run.error) && run.error.code === "EXPORT_BLOCKED"
      ? run.error.details.filter((d) => !DOUBLE_BOOKING_CODES.has(d.code ?? ""))
      : [];

  // Seçim varsa yalnız seçilen fişler, yoksa dönemin onaylı fişleri.
  const scopeInput = selected.length ? { drafts: selected } : { period };
  const exportNow = (confirm: string[]) =>
    run.mutate(
      {
        driver,
        ...scopeInput,
        confirm_documents: confirm.length ? confirm : undefined,
      },
      {
        onSuccess: () => {
          check.reset();
          setConfirmed(new Set());
          onClearSelection();
        },
      },
    );
  // Önce ön kontrol: uyarılı belge yoksa doğrudan çıktı alınır; varsa
  // müşavir hangi belgelerin yine de aktarılacağını seçer.
  const start = () => {
    run.reset();
    setConfirmed(new Set());
    check.mutate(
      { driver, ...scopeInput },
      {
        onSuccess: (c) => {
          if (!c.excluded.length) exportNow([]);
        },
      },
    );
  };

  return (
    <Panel
      id={EXPORT_ANCHOR}
      title={t("office.journals.export_title")}
      description={t("office.journals.export_description", {
        period: periodText(period),
      })}
    >
      <div className="space-y-3 p-4">
        <div className="flex flex-wrap items-center gap-3">
          <FilterSelect
            label={t("office.journals.driver")}
            value={driver}
            onChange={(v) => {
              setDriver(v);
              check.reset();
              setConfirmed(new Set());
            }}
            options={items.map((d) => ({
              value: d.name,
              label: driverLabel(t, d.name),
            }))}
          />
          {chosen && !chosen.verified ? (
            <span className="bg-status-warning/12 text-status-warning rounded-md px-2 py-0.5 text-xs">
              {t("office.journals.template_unverified", {
                version: chosen.template_version,
              })}
            </span>
          ) : null}
          <Button
            size="sm"
            disabled={
              (!period && !selected.length) ||
              run.isPending ||
              check.isPending ||
              !chosen
            }
            onClick={start}
          >
            {run.isPending || check.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <FileSpreadsheet className="size-4" />
            )}
            {selected.length
              ? t("office.journals.export_selected", {
                  count: selected.length,
                })
              : t("office.journals.export")}
          </Button>
          {selected.length ? (
            <Button size="sm" variant="ghost" onClick={onClearSelection}>
              {t("office.journals.selection_clear")}
            </Button>
          ) : null}
        </div>
        {chosen && !chosen.verified && chosen.name === "luca" ? (
          <p className="text-status-warning text-xs">
            {t("office.journals.luca_format_unverified")}
          </p>
        ) : null}
        {!period && !selected.length ? (
          <p className="text-muted-foreground text-xs">
            {t("office.journals.pick_period")}
          </p>
        ) : (
          <p className="text-muted-foreground text-xs">
            {selected.length
              ? t("office.journals.export_scope_selected", {
                  count: selected.length,
                })
              : t("office.journals.export_scope_period")}
          </p>
        )}
        {review ? (
          <div className="space-y-2">
            <DoubleBookingReview
              check={review}
              confirmed={confirmed}
              onToggle={(key, on) =>
                setConfirmed((prev) => {
                  const next = new Set(prev);
                  if (on) next.add(key);
                  else next.delete(key);
                  return next;
                })
              }
            />
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                disabled={run.isPending || (!review.entries && !confirmed.size)}
                onClick={() => exportNow([...confirmed])}
              >
                <FileSpreadsheet className="size-4" />
                {confirmed.size
                  ? t("office.journals.dup.export_confirmed", {
                      count: confirmed.size,
                    })
                  : t("office.journals.dup.export_rest", {
                      count: review.entries,
                    })}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  check.reset();
                  setConfirmed(new Set());
                }}
              >
                {t("office.journals.dup.cancel")}
              </Button>
            </div>
          </div>
        ) : null}
        {blocked.length ? (
          <div className="border-status-blocking/40 rounded-md border p-3 text-sm">
            <p className="text-status-blocking mb-1 font-medium">
              {t("office.journals.blocked")}
            </p>
            <ul className="space-y-0.5 text-xs">
              {blocked.map((d, i) => (
                <li key={i}>{exportIssueText(d, t)}</li>
              ))}
            </ul>
          </div>
        ) : run.isError &&
          !(isApiError(run.error) && run.error.code === "EXPORT_BLOCKED") ? (
          <p className="text-status-blocking text-sm">
            {journalErrorText(t, run.error, t("office.api.load_error"))}
          </p>
        ) : check.isError ? (
          <p className="text-status-blocking text-sm">
            {journalErrorText(t, check.error, t("office.api.load_error"))}
          </p>
        ) : null}
        {run.data ? (
          <div className="text-sm">
            <p className="text-status-approved">
              {t("office.journals.exported", { count: run.data.entries })}
            </p>
            {run.data.excluded?.length ? (
              <p className="text-status-warning text-xs">
                {t("office.journals.dup.left_out", {
                  count: run.data.excluded.length,
                })}
              </p>
            ) : null}
            <ul className="mt-1 space-y-1">
              {run.data.files.map((f) => (
                <li key={f.name}>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      void journalService.download(
                        slug,
                        engagement,
                        run.data.uuid,
                        f.name,
                      )
                    }
                  >
                    <Download className="size-4" />
                    {f.name}
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </Panel>
  );
}

/**
 * "Masaüstü ajanına gönder" for a Luca / Zirve export (#391); the latest
 * open or imported transfer of the export is shown instead.
 */
function SendToAgent({
  engagement,
  batch,
  latest,
}: {
  engagement: string;
  batch: JournalExportSummary;
  latest?: JournalTransfer;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { requestTransfer } = useJournalActions(slug, engagement);
  if (!TRANSFER_DRIVERS.has(batch.driver) || batch.entries <= batch.reversed)
    return null;
  if (latest && latest.status !== "failed" && latest.status !== "cancelled") {
    const v = TRANSFER_STATUS[latest.status];
    return (
      <StatusPill tone={v?.tone}>
        {v ? t(v.labelKey) : latest.status}
      </StatusPill>
    );
  }
  return (
    <span className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={requestTransfer.isPending}
        onClick={() => requestTransfer.mutate(batch.uuid)}
      >
        {requestTransfer.isPending ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <MonitorUp className="size-4" />
        )}
        {t("office.journals.transfer_send")}
      </Button>
      {requestTransfer.isError ? (
        <span className="text-status-blocking text-xs">
          {journalErrorText(
            t,
            requestTransfer.error,
            t("office.api.load_error"),
          )}
        </span>
      ) : null}
    </span>
  );
}

function JournalsInner() {
  const { t, locale } = useLocale();
  const { prompt } = useDialogs();
  const { slug, permissions: perms } = useTenant();
  // Mükellef detayının Fişler sekmesinde mükellef sabit (#468).
  const scope = useEngagementScope();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug, scope);
  const engagement = scope ?? selected?.engagement ?? "";
  // Global dönem seçici (#326); "Tüm dönemler"de bu ay (yeni fişin dönemi).
  // Seçili dönem adreste de durur (?period=, #669 D): geri tuşu aynı yeri açar.
  const { period: selectedPeriod, setPeriod: setGlobalPeriod } =
    useUrlOfficePeriod();
  const period = selectedPeriod ?? thisMonth();
  const setPeriod = (value: string) =>
    setGlobalPeriod(isPeriod(value) ? value : ALL_PERIODS);
  const drafts = useJournalDrafts(slug, engagement, period);
  // Sıradaki İşler'den ve ofis Aktarımlar ekranından (#574) gelen bağlantı
  // bölüme götürür (#566): liste
  // yüklendikten sonra kaydırılır, yoksa tarayıcı boş sayfada kalır.
  const loaded = drafts.isSuccess;
  useEffect(() => {
    if (!loaded) return;
    const id = window.location.hash.slice(1);
    if (![DRAFTS_ANCHOR, EXPORT_ANCHOR, TRANSFERS_ANCHOR].includes(id)) return;
    const el = document.getElementById(id);
    el?.scrollIntoView({ block: "start" });
    el?.focus({ preventScroll: true });
  }, [loaded]);
  const exports = useJournalExports(slug, engagement);
  const transfers = useJournalTransfers(slug, engagement);
  // Newest transfer per export (the list is newest first).
  const latestTransfer = new Map<string, JournalTransfer>();
  for (const tr of transfers.data?.items ?? [])
    if (!latestTransfer.has(tr.batch_uuid))
      latestTransfer.set(tr.batch_uuid, tr);
  const { approve, unapprove, cancelExport } = useJournalActions(
    slug,
    engagement,
  );
  // Hazırlayan stajyer onaylayamaz; fiş müşavirin onayını bekler (#566).
  const canApprove = perms.includes(permissions.tenant.journals.approve);
  // Çıktıya alınacak onaylı fişler (#574); mükellef ya da dönem değişince
  // seçim sıfırlanır.
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [pickedFor, setPickedFor] = useState(`${engagement}|${period}`);
  if (pickedFor !== `${engagement}|${period}`) {
    setPickedFor(`${engagement}|${period}`);
    setPicked(new Set());
  }
  // Kalemleri açık fiş: hangi hesap borçlu, hangisi alacaklı orada görünür.
  const [linesOf, setLinesOf] = useState<JournalDraft | null>(null);
  const togglePick = useCallback((uuid: string, on: boolean) => {
    setPicked((prev) => {
      const next = new Set(prev);
      if (on) next.add(uuid);
      else next.delete(uuid);
      return next;
    });
  }, []);
  const fmt = (d: string) =>
    new Date(d).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");
  const approveDraft = approve.mutate;
  const approving = approve.isPending;
  // Uyarılı fiş (eksik belge / mükerrer olabilir, #669): engellenmez, ama
  // onaydan önce açıkça sorulur; gerekçe isteğe bağlı, fişte kalır.
  const approveOne = useCallback(
    async (d: JournalDraft) => {
      const warnings = warningsOf(d);
      if (!warnings.length) {
        approveDraft(d.uuid);
        return;
      }
      if (isDuplicate(d)) {
        // Engel: ayrı ve açık "yine de gönder", gerekçe zorunlu (#710).
        const reason = await prompt({
          title: t("office.journals.check.duplicate_title"),
          description: [
            ...warnings.map((w) => issueText(t, w)),
            t("office.journals.check.duplicate_description"),
          ].join(" "),
          placeholder: t("office.journals.check.duplicate_reason"),
          confirmLabel: t("office.journals.check.duplicate_send"),
        });
        if (reason === null || !reason.trim()) return;
        approveDraft({
          draft: d.uuid,
          confirm: true,
          sendDuplicate: true,
          reason: reason.trim(),
        });
        return;
      }
      const note = await prompt({
        title: t("office.journals.check.confirm_title"),
        description: warnings.map((w) => issueText(t, w)).join(" "),
        placeholder: t("office.journals.check.confirm_note"),
        confirmLabel: t("office.journals.check.confirm_approve"),
        optional: true,
      });
      if (note === null) return;
      approveDraft({ draft: d.uuid, confirm: true, note: note.trim() });
    },
    [approveDraft, prompt, t],
  );
  // Aktarılmış fiş (#710, ofis kararı): düzeltme fişi yok; fiş tamamen
  // iptal edilir, taslağa döner; programdaki fiş kullanıcı tarafından silinir.
  const cancelDraftExport = cancelExport.mutate;
  const cancelling = cancelExport.isPending;
  const unapproveDraft = unapprove.mutate;
  const unapproving = unapprove.isPending;
  const baseDraftColumns = useMemo<ColumnDef<JournalDraft, unknown>[]>(
    () => [
      createColumn<JournalDraft>({
        id: "pick",
        labelKey: "office.journals.pick_col",
        enableSorting: false,
        meta: { cellClassName: "w-8" },
        cell: ({ row }) =>
          row.original.status === "approved" ? (
            <Checkbox
              checked={picked.has(row.original.uuid)}
              onCheckedChange={(v) => togglePick(row.original.uuid, v === true)}
              aria-label={t("office.journals.pick_entry", {
                entry: row.original.entry_no,
              })}
              data-no-row-click
            />
          ) : null,
      }),
      createColumn<JournalDraft>({
        id: "entry_no",
        labelKey: "office.journals.entry_no",
        accessorFn: (d) => `${d.period} / ${d.entry_no}`,
        gridPrimary: true,
        cell: ({ row }) => (
          <Figure>
            {row.original.period} / {row.original.entry_no}
          </Figure>
        ),
      }),
      createDateColumn<JournalDraft>({
        id: "entry_date",
        labelKey: "office.col_date",
        accessor: (d) => d.entry_date,
        gridSecondary: true,
      }),
      createColumn<JournalDraft>({
        id: "description",
        labelKey: "office.journals.description",
        accessorKey: "description",
        meta: { cellClassName: "max-w-72 truncate" },
        cell: ({ row }) => (
          <span title={row.original.description}>
            {row.original.description || "—"}
          </span>
        ),
      }),
      createColumn<JournalDraft>({
        id: "entry_type",
        labelKey: "office.col_type",
        accessorKey: "entry_type",
        meta: { cellClassName: "text-xs" },
      }),
      // Fişin toplamları: dengeli fişte borç ve alacak toplamı hep eşittir;
      // hangi hesabın borçlu olduğu "Kalemler"de görünür.
      createMoneyColumn<JournalDraft>({
        id: "debit",
        labelKey: "office.journals.debit_total",
        accessor: (d) => d.debit,
      }),
      createMoneyColumn<JournalDraft>({
        id: "credit",
        labelKey: "office.journals.credit_total",
        accessor: (d) => d.credit,
      }),
      createColumn<JournalDraft>({
        id: "lines",
        labelKey: "office.journals.lines.col",
        accessorKey: "lines",
        enableSorting: false,
        cell: ({ row }) => (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setLinesOf(row.original)}
            aria-label={t("office.journals.lines.open_entry", {
              entry: row.original.entry_no,
            })}
          >
            <ListTree className="size-4" />
            {t("office.journals.lines.open", { count: row.original.lines })}
          </Button>
        ),
      }),
      createColumn<JournalDraft>({
        id: "status",
        labelKey: "office.col_status",
        accessorKey: "status",
        cell: ({ row }) => (
          <span
            className={cn(
              "rounded-md px-2 py-0.5 text-xs font-medium",
              statusClass[row.original.status],
            )}
          >
            {t(`office.journals.status_${row.original.status}`)}
          </span>
        ),
      }),
      createColumn<JournalDraft>({
        id: "transfer",
        labelKey: "office.journals.transfer_col",
        accessorFn: (d) => d.export?.transfer_status ?? "",
        enableSorting: false,
        cell: ({ row }) => <DraftTransfer draft={row.original} />,
      }),
      createColumn<JournalDraft>({
        id: "check",
        labelKey: "office.journals.check.col",
        accessorFn: (d) => d.issues?.length ?? 0,
        enableSorting: false,
        cell: ({ row }) => (
          <DraftCheck draft={row.original} engagement={engagement} />
        ),
      }),
      createActionsColumn<JournalDraft>({
        cell: (d) =>
          d.status === "draft" && !canApprove ? (
            <span className="text-muted-foreground text-xs">
              {t("office.journals.awaiting_approval")}
            </span>
          ) : d.status === "approved" && canApprove ? (
            // Onaylı ama henüz aktarılmamış fiş (#566): onay gerekçeyle geri
            // alınır, fiş taslağa döner ve düzeltilip yeniden onaylanır.
            <Button
              size="sm"
              variant="ghost"
              disabled={unapproving}
              onClick={async () => {
                const reason = await prompt({
                  title: t("office.journals.unapprove"),
                  description: t("office.journals.unapprove_prompt"),
                  confirmLabel: t("office.journals.unapprove"),
                });
                if (reason?.trim())
                  unapproveDraft(
                    { draft: d.uuid, reason },
                    {
                      onSuccess: () =>
                        toast.success(t("office.journals.unapproved")),
                    },
                  );
              }}
            >
              <Undo2 className="size-4" />
              {t("office.journals.unapprove")}
            </Button>
          ) : d.status === "draft" ? (
            // Engelli fiş onaylanmaz (#566); nedeni ipucunda.
            <Hint
              label={
                isBlocked(d)
                  ? t("office.journals.check.approve_blocked")
                  : undefined
              }
              wrap
            >
              <Button
                size="sm"
                variant="outline"
                disabled={approving || isBlocked(d)}
                onClick={() => void approveOne(d)}
              >
                <Check className="size-4" />
                {t("office.journals.approve")}
              </Button>
            </Hint>
          ) : d.status === "exported" && canApprove ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={cancelling}
              onClick={async () => {
                const vars = {
                  no: d.entry_no,
                  date: new Date(d.entry_date).toLocaleDateString("tr-TR"),
                };
                const reason = await prompt({
                  title: t("office.journals.cancel_export_title", vars),
                  description: t(
                    "office.journals.cancel_export_description",
                    vars,
                  ),
                  placeholder: t("office.journals.cancel_export_reason"),
                  confirmLabel: t("office.journals.cancel_export"),
                });
                if (!reason?.trim()) return;
                cancelDraftExport(
                  { draft: d.uuid, reason: reason.trim() },
                  {
                    onSuccess: () =>
                      toast.success(
                        t("office.journals.cancel_export_done", vars),
                      ),
                  },
                );
              }}
            >
              <Undo2 className="size-4" />
              {t("office.journals.cancel_export")}
            </Button>
          ) : null,
      }),
    ],
    [
      approveOne,
      approving,
      canApprove,
      engagement,
      picked,
      prompt,
      cancelDraftExport,
      cancelling,
      t,
      togglePick,
      unapproveDraft,
      unapproving,
    ],
  );
  // Fişin ilişkili kayıtları: kaynak belge ve hareketler (#469).
  const relatedColumn = useRelatedColumn<JournalDraft>(
    "journal_draft",
    (d) => d.uuid,
    (d) => `#${d.entry_no}`,
  );
  const draftColumns = useMemo<ColumnDef<JournalDraft, unknown>[]>(
    () => [...baseDraftColumns, ...relatedColumn],
    [baseDraftColumns, relatedColumn],
  );

  const header = (
    <PageHeader
      title={t("office.journals.title")}
      description={t("office.journals.page_description")}
      icon={<BookOpenCheck className="size-6" />}
      headingLevel={scope ? 2 : 1}
    />
  );
  if (!scope && isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (!scope && (isError || options.length === 0)) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={
            isError
              ? t("office.api.load_error")
              : t("office.api.no_engagements")
          }
        />
      </div>
    );
  }

  const rows = drafts.data?.items ?? [];
  const openDrafts = rows.filter((d) => d.status === "draft");
  const blockedDrafts = openDrafts.filter((d) => d.issues?.length).length;
  const stoppedDrafts = openDrafts.filter(isBlocked).length;
  const actionError = approve.error ?? unapprove.error ?? cancelExport.error;
  return (
    <div className="space-y-4">
      {header}
      <div className="flex flex-wrap items-center gap-3">
        {scope ? null : (
          <EngagementPicker
            options={options}
            selected={selected}
            onSelect={select}
          />
        )}
        <div className="flex items-center gap-2 text-xs">
          <Label
            htmlFor="journals-period"
            className="text-muted-foreground text-xs font-normal"
          >
            {t("office.api.col_period")}
          </Label>
          <MonthPicker
            id="journals-period"
            className="h-8 w-40"
            value={period}
            onChange={setPeriod}
          />
        </div>
      </div>

      {/* Mükellef değişince yarım fiş öbür mükellefe taşınmaz (#573). */}
      <NewDraft key={engagement} engagement={engagement} />

      <Panel
        id={DRAFTS_ANCHOR}
        title={t("office.journals.drafts_title")}
        description={
          openDrafts.length
            ? blockedDrafts
              ? t("office.journals.check.summary", {
                  drafts: openDrafts.length,
                  blocked: blockedDrafts,
                  stopped: stoppedDrafts,
                })
              : t("office.journals.check.summary_clean", {
                  drafts: openDrafts.length,
                })
            : undefined
        }
        actions={<ApproveClean engagement={engagement} drafts={rows} />}
      >
        {actionError ? (
          <p className="text-status-blocking px-4 pt-3 text-sm">
            {journalErrorText(t, actionError, t("office.api.load_error"))}
          </p>
        ) : null}
        {drafts.isLoading ? (
          <Skeleton className="m-4 h-32" />
        ) : rows.length === 0 ? (
          <EmptyState
            className="m-4"
            title={t("office.journals.empty")}
            description={t("office.journals.empty_hint")}
          />
        ) : (
          <DataTable<JournalDraft>
            columns={draftColumns}
            data={rows}
            getRowId={(d) => d.uuid}
            bordered={false}
            features={embeddedTable}
            onRowClick={setLinesOf}
          />
        )}
      </Panel>
      <JournalDraftLines
        engagement={engagement}
        draft={linesOf}
        onClose={() => setLinesOf(null)}
      />

      <ExportPanel
        engagement={engagement}
        period={period}
        selected={[...picked]}
        onClearSelection={() => setPicked(new Set())}
      />

      {exports.data?.items.length ? (
        <Panel title={t("office.journals.history_title")}>
          <ul className="divide-y text-sm">
            {exports.data.items.map((b) => (
              <li
                key={b.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-4 py-2"
              >
                <span>
                  <Figure className="font-medium">{b.driver}</Figure>{" "}
                  <span className="text-muted-foreground text-xs">
                    {b.period ?? ""} ·{" "}
                    {t("office.journals.history_row", {
                      entries: b.entries,
                      reversed: b.reversed,
                    })}{" "}
                    · <Figure>{fmt(b.created_at)}</Figure>
                  </span>
                </span>
                <span className="flex flex-wrap items-center gap-1">
                  <SendToAgent
                    engagement={engagement}
                    batch={b}
                    latest={latestTransfer.get(b.uuid)}
                  />
                  {b.files.map((f) => (
                    <Button
                      key={f.name}
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        void journalService.download(
                          slug,
                          engagement,
                          b.uuid,
                          f.name,
                        )
                      }
                    >
                      <Download className="size-4" />
                      {f.name}
                    </Button>
                  ))}
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}

      <ExportDocumentsPanel engagement={engagement} />

      <LucaPullSettingsPanel engagement={engagement} />

      <JournalTransfersPanel engagement={engagement} />
    </div>
  );
}

export function JournalsPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <JournalsInner />
    </Suspense>
  );
}
