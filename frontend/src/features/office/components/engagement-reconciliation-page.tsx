"use client";

import Link from "next/link";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  CheckCircle2,
  FileSearch,
  Landmark,
  ListChecks,
  MoreHorizontal,
  RefreshCw,
  Scale,
} from "lucide-react";
import { Suspense, useMemo, useState } from "react";
import { toast } from "sonner";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createMoneyColumn,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
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
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useOfficePeriod, usePeriodText } from "../period";
import {
  type DecisionStatus,
  type ReconciliationItem,
  type ReconciliationLatestRunDetail,
  type ReconciliationRunDetail,
  reconciliationKeys,
  reconciliationService,
} from "../services/reconciliation.service";
import { Money } from "./office-ui";
import {
  BabsView,
  CounterpartiesView,
  LettersView,
} from "./reconciliation-cari";
import { SummaryCards } from "./receipts-ui";
import {
  decisionTone,
  errText,
  NoteDialog,
  Pill,
  resultTone,
} from "./reconciliation-ui";
import { useEngagementScope } from "../engagement-scope";
import { reconciliationItemHref } from "../lib/related-href";

const VIEWS = ["accounts", "counterparties", "babs", "letters"] as const;
type View = (typeof VIEWS)[number];

/**
 * `/t/{slug}/engagements/{uuid}/reconciliation` (#389): mükellef × dönem
 * mutabakat panosu. Hesaplar: mizan hareketi ile kaynak (fatura, banka,
 * masraf, Z, bordro) fişlerinin ana hesap bazında farkı, fişlenmemiş
 * kayıtlar ve açıklama önerisi; karar ya da inceleme kuyruğu. Cari: karşı
 * firma bakiyesi ve BA/BS. BA/BS: beyan satırları. Mektuplar: cari
 * mutabakat mektubu ve yanıtı.
 */
export function EngagementReconciliationPage({
  engagement,
}: {
  engagement: string;
}) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <ReconciliationInner engagement={engagement} />
    </Suspense>
  );
}

function ReconciliationInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.reconciliation.write);
  const qc = useQueryClient();
  const { focus: period } = useOfficePeriod();
  const listState = useUrlListState({ initialPageSize: 100 });
  const rawView = listState.getParam("view") as View | null;
  const view: View = rawView && VIEWS.includes(rawView) ? rawView : "accounts";

  const latest = useQuery({
    queryKey: reconciliationKeys.run(slug, engagement, period),
    queryFn: () => reconciliationService.latest(slug, engagement, period),
  });
  // Koşu yoksa uç 200 + run: null döner (#639); boş durum gösterilir.
  const notRun = latest.data?.run === null;
  const detail: ReconciliationRunDetail | null = latest.data?.run
    ? { run: latest.data.run, items: latest.data.items }
    : null;

  const run = useMutation({
    mutationFn: () => reconciliationService.run(slug, engagement, period),
    onSuccess: (d) => {
      qc.setQueryData(reconciliationKeys.run(slug, engagement, period), d);
      void qc.invalidateQueries({
        queryKey: reconciliationKeys.all(slug, engagement),
      });
      toast.success(t("office.reconciliation.run_done"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.code === "RECONCILIATION_NO_TRIAL_BALANCE"
          ? t("office.reconciliation.no_trial_balance", {
              period: periodText(period),
            })
          : errText(e, t("office.reconciliation.run_failed")),
      ),
  });

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display truncate text-xl font-bold">
            {t("office.reconciliation.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.reconciliation.description", {
              period: periodText(period),
            })}
          </p>
        </div>
        {canWrite ? (
          <Button
            size="sm"
            className="max-md:min-h-11"
            disabled={run.isPending}
            onClick={() => run.mutate()}
          >
            <RefreshCw
              className={cn("size-4", run.isPending && "animate-spin")}
              aria-hidden
            />
            {detail
              ? t("office.reconciliation.rerun")
              : t("office.reconciliation.run")}
          </Button>
        ) : null}
      </div>
      <TabStrip aria-label={t("office.reconciliation.view_label")}>
        {VIEWS.map((v) => (
          <TabStripItem
            key={v}
            selected={view === v}
            onClick={() =>
              listState.setParams({ view: v === "accounts" ? null : v })
            }
          >
            {t(`office.reconciliation.view_${v}`)}
          </TabStripItem>
        ))}
      </TabStrip>

      {view === "babs" ? (
        <BabsView engagement={engagement} period={period} canWrite={canWrite} />
      ) : view === "letters" ? (
        <LettersView engagement={engagement} canWrite={canWrite} />
      ) : latest.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : notRun || !detail ? (
        <div className="border-line bg-surface rounded-xl border p-8 text-center">
          <Scale className="text-text-3 mx-auto size-8" aria-hidden />
          <h2 className="mt-3 font-semibold">
            {latest.isError && !notRun
              ? t("office.api.load_error")
              : t("office.reconciliation.empty_title")}
          </h2>
          <p className="text-text-2 mx-auto mt-1 max-w-lg text-sm">
            {t("office.reconciliation.empty", { period: periodText(period) })}
          </p>
        </div>
      ) : view === "counterparties" ? (
        <CounterpartiesView
          engagement={engagement}
          period={period}
          canWrite={canWrite}
        />
      ) : (
        <AccountsView
          engagement={engagement}
          period={period}
          detail={detail}
          canWrite={canWrite}
        />
      )}
    </div>
  );
}

function AccountsView({
  engagement,
  period,
  detail,
  canWrite,
}: {
  engagement: string;
  period: string;
  detail: ReconciliationRunDetail;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [onlyOpen, setOnlyOpen] = useState(true);
  const [shown, setShown] = useState<ReconciliationItem | null>(null);
  const [noteFor, setNoteFor] = useState<{
    item: ReconciliationItem;
    mode: "explain" | "review";
  } | null>(null);
  const sum = detail.run.summary;

  const update = (item: ReconciliationItem) => {
    qc.setQueryData<ReconciliationLatestRunDetail>(
      reconciliationKeys.run(slug, engagement, period),
      (old) =>
        old && {
          ...old,
          items: old.items.map((i) => (i.uuid === item.uuid ? item : i)),
        },
    );
  };
  const decide = useMutation({
    mutationFn: (v: { uuid: string; status: DecisionStatus; note?: string }) =>
      reconciliationService.decideItem(
        slug,
        engagement,
        v.uuid,
        v.status,
        v.note,
      ),
    onSuccess: (item) => {
      update(item);
      setNoteFor(null);
      toast.success(t("office.reconciliation.saved"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });
  const review = useMutation({
    mutationFn: (v: { uuid: string; note?: string }) =>
      reconciliationService.reviewItem(slug, engagement, v.uuid, v.note),
    onSuccess: (item) => {
      update(item);
      setNoteFor(null);
      toast.success(t("office.reconciliation.sent_to_review"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });

  const items = useMemo(
    () =>
      onlyOpen
        ? detail.items.filter(
            (i) => i.result !== "matched" && i.status !== "resolved",
          )
        : detail.items,
    [detail.items, onlyOpen],
  );

  // `mutate` kararlı; mutasyon nesnesi her durumda değişir ve sütunları
  // (dolayısıyla satır düğmelerini) yeniden kurup odağı kaybettiriyordu.
  const decideItem = decide.mutate;
  const columns = useMemo<ColumnDef<ReconciliationItem, unknown>[]>(
    () => [
      createColumn<ReconciliationItem>({
        id: "account",
        labelKey: "office.reconciliation.col_account",
        accessorFn: (x) => x.main_code,
        gridPrimary: true,
        meta: { cellClassName: "min-w-44" },
        cell: ({ row }) => (
          <div>
            <span className="font-mono text-sm font-semibold">
              {row.original.main_code}
            </span>
            <span className="text-text-2 ms-2 text-sm">
              {row.original.account_name}
            </span>
          </div>
        ),
      }),
      createMoneyColumn<ReconciliationItem>({
        id: "ledger_net",
        labelKey: "office.reconciliation.col_ledger",
        accessor: (x) => x.ledger_net,
      }),
      createMoneyColumn<ReconciliationItem>({
        id: "source_net",
        labelKey: "office.reconciliation.col_sources",
        accessor: (x) => x.source_net,
      }),
      createMoneyColumn<ReconciliationItem>({
        id: "difference",
        labelKey: "office.reconciliation.col_difference",
        accessor: (x) => x.difference,
        className: "font-semibold",
      }),
      createColumn<ReconciliationItem>({
        id: "result",
        labelKey: "office.reconciliation.col_result",
        accessorFn: (x) => x.result,
        cell: ({ row }) => (
          <div className="flex flex-col items-start gap-1">
            <Pill
              group="result"
              value={row.original.result}
              tones={resultTone}
            />
            <Pill
              group="status"
              value={row.original.status}
              tones={decisionTone}
            />
          </div>
        ),
      }),
      createColumn<ReconciliationItem>({
        id: "suggestion",
        labelKey: "office.reconciliation.col_explanation",
        accessorFn: (x) => x.note ?? x.suggestion ?? "",
        meta: { cellClassName: "min-w-64 max-w-md" },
        cell: ({ row }) => {
          const x = row.original;
          return x.note ? (
            <p className="text-sm">{x.note}</p>
          ) : x.suggestion ? (
            <p className="text-text-2 text-sm">
              <span className="bg-brand-tint text-brand-text me-1 rounded px-1.5 py-0.5 text-[11px]">
                {t("office.reconciliation.suggested")}
              </span>
              {x.suggestion}
            </p>
          ) : (
            <span className="text-text-3">—</span>
          );
        },
      }),
      createActionsColumn<ReconciliationItem>({
        cell: (x) => (
          <div className="flex justify-end gap-1.5">
            <Button
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              onClick={() => setShown(x)}
            >
              <FileSearch className="size-4" aria-hidden />
              {t("office.reconciliation.detail")}
            </Button>
            {canWrite && x.result !== "matched" ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="max-md:min-h-11"
                    aria-label={t("office.reconciliation.decide_label", {
                      code: x.main_code,
                    })}
                    disabled={x.status === "in_review"}
                  >
                    <MoreHorizontal className="size-4" aria-hidden />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onSelect={() => setNoteFor({ item: x, mode: "explain" })}
                  >
                    {t("office.reconciliation.explain")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() =>
                      decideItem({ uuid: x.uuid, status: "resolved" })
                    }
                  >
                    {t("office.reconciliation.resolve")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() =>
                      decideItem({ uuid: x.uuid, status: "ignored" })
                    }
                  >
                    {t("office.reconciliation.ignore")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() => setNoteFor({ item: x, mode: "review" })}
                  >
                    {t("office.reconciliation.send_review")}
                  </DropdownMenuItem>
                  {x.status !== "open" ? (
                    <DropdownMenuItem
                      onSelect={() =>
                        decideItem({ uuid: x.uuid, status: "open" })
                      }
                    >
                      {t("office.reconciliation.reopen")}
                    </DropdownMenuItem>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>
        ),
      }),
    ],
    [t, canWrite, decideItem],
  );

  const unpostedAmount = Object.values(sum.unposted).reduce(
    (a, k) => a + Number(k.amount),
    0,
  );

  return (
    <div className="space-y-4">
      <SummaryCards
        cards={[
          {
            label: t("office.reconciliation.accounts"),
            value: sum.accounts,
            icon: ListChecks,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.reconciliation.matched"),
            value: sum.matched,
            icon: CheckCircle2,
            box: "bg-ok-tint text-ok",
          },
          {
            label: t("office.reconciliation.differences"),
            value:
              sum.differences + sum.missing_in_ledger + sum.missing_in_sources,
            icon: AlertTriangle,
            box: "bg-warn-tint text-warn",
          },
          {
            label: t("office.reconciliation.difference_total"),
            value: <Money value={sum.difference_total} />,
            icon: Scale,
            box: "bg-brand-tint text-brand-text",
          },
        ]}
      />
      <div className="text-text-2 flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span>
          {t("office.reconciliation.window", {
            from: periodText(detail.run.window_from),
            to: periodText(detail.run.window_to),
          })}
          {sum.cumulative ? ` · ${t("office.reconciliation.cumulative")}` : ""}
        </span>
        {detail.run.trial_balance ? (
          <span>
            {t("office.reconciliation.trial_balance", {
              file: detail.run.trial_balance.file_name,
            })}
          </span>
        ) : null}
        {sum.unposted_count > 0 ? (
          <span>
            {t("office.reconciliation.unposted_summary", {
              count: sum.unposted_count,
            })}{" "}
            <Money value={unpostedAmount.toFixed(2)} />
          </span>
        ) : null}
      </div>
      {sum.bank ? (
        <div
          className={cn(
            "flex flex-wrap items-center gap-3 rounded-xl border p-3 text-sm",
            sum.bank.matched
              ? "border-line bg-surface"
              : "border-warn bg-warn-tint",
          )}
        >
          <Landmark className="size-4" aria-hidden />
          <span className="font-medium">
            {t("office.reconciliation.bank_check")}
          </span>
          <span>
            {t("office.reconciliation.bank_ledger")}{" "}
            <Money value={sum.bank.ledger_balance} />
          </span>
          <span>
            {t("office.reconciliation.bank_statement", {
              count: sum.bank.accounts,
            })}{" "}
            <Money value={sum.bank.statement_balance} />
          </span>
          <span className="font-semibold">
            {sum.bank.matched
              ? t("office.reconciliation.bank_matched")
              : t("office.reconciliation.bank_difference")}{" "}
            {sum.bank.matched ? null : <Money value={sum.bank.difference} />}
          </span>
        </div>
      ) : null}

      <Label className="flex w-fit items-center gap-2 text-sm leading-normal font-normal">
        <Checkbox
          checked={onlyOpen}
          onCheckedChange={(v) => setOnlyOpen(v === true)}
        />
        {t("office.reconciliation.only_open")}
      </Label>

      <EntityTable<ReconciliationItem>
        columns={columns}
        data={items}
        getRowId={(x) => x.uuid}
        emptyTitle={t("office.reconciliation.accounts_empty_title")}
        emptyDescription={t("office.reconciliation.accounts_empty")}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          pagination: false,
          persistKey: "office.reconciliation.accounts",
        }}
      />

      <ItemDetailDialog item={shown} onClose={() => setShown(null)} />
      <NoteDialog
        open={noteFor !== null}
        onOpenChange={(o) => !o && setNoteFor(null)}
        title={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review_title", {
                code: noteFor.item.main_code,
              })
            : t("office.reconciliation.explain_title", {
                code: noteFor?.item.main_code ?? "",
              })
        }
        description={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review_hint")
            : undefined
        }
        initial={noteFor?.item.note ?? noteFor?.item.suggestion ?? ""}
        required={noteFor?.mode === "explain"}
        pending={decide.isPending || review.isPending}
        confirmLabel={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review")
            : t("office.reconciliation.save")
        }
        onConfirm={(note) => {
          if (!noteFor) return;
          if (noteFor.mode === "review")
            review.mutate({ uuid: noteFor.item.uuid, note: note || undefined });
          else
            decide.mutate({
              uuid: noteFor.item.uuid,
              status: "explained",
              note,
            });
        }}
      />
    </div>
  );
}

function ItemDetailDialog({
  item,
  onClose,
}: {
  item: ReconciliationItem | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const engagement = useEngagementScope();
  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-2xl">
        {item ? (
          <>
            <DialogHeader>
              <DialogTitle>
                {item.main_code} {item.account_name}
              </DialogTitle>
              <DialogDescription>
                {t("office.reconciliation.detail_hint")}
              </DialogDescription>
            </DialogHeader>
            <section className="space-y-2">
              <h3 className="text-sm font-semibold">
                {t("office.reconciliation.by_source")}
              </h3>
              {item.sources.length === 0 ? (
                <p className="text-text-3 text-sm">
                  {t("office.reconciliation.no_sources")}
                </p>
              ) : (
                <Table>
                  <TableHeader className="[&_tr]:border-line text-xs">
                    <TableRow className="hover:bg-transparent">
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-start">
                        {t("office.reconciliation.col_source")}
                      </TableHead>
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-end">
                        {t("office.reconciliation.col_debit")}
                      </TableHead>
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-end">
                        {t("office.reconciliation.col_credit")}
                      </TableHead>
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-end">
                        {t("office.reconciliation.col_draft")}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {item.sources.map((s) => (
                      <TableRow
                        key={s.kind}
                        className="border-line hover:bg-transparent"
                      >
                        <TableCell className="px-0 py-1.5">
                          {t(`office.reconciliation.kind_${s.kind}`)}{" "}
                          <span className="text-text-3 text-xs">
                            (
                            {t("office.reconciliation.lines", {
                              count: s.lines,
                            })}
                            )
                          </span>
                        </TableCell>
                        <TableCell className="px-0 py-1.5 text-end">
                          <Money value={s.debit} />
                        </TableCell>
                        <TableCell className="px-0 py-1.5 text-end">
                          <Money value={s.credit} />
                        </TableCell>
                        <TableCell className="px-0 py-1.5 text-end">
                          <Money value={s.draft_net} />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </section>
            <section className="space-y-2">
              <h3 className="text-sm font-semibold">
                {t("office.reconciliation.unposted")}
              </h3>
              {item.unposted.length === 0 ? (
                <p className="text-text-3 text-sm">
                  {t("office.reconciliation.no_unposted")}
                </p>
              ) : (
                <ul className="divide-line divide-y text-sm">
                  {item.unposted.map((u) => (
                    <li
                      key={`${u.kind}-${u.id}`}
                      className="flex items-center justify-between gap-3 py-1.5"
                    >
                      <span className="min-w-0">
                        <span className="text-text-3 me-2 text-xs">
                          {u.date}
                        </span>
                        {t(`office.reconciliation.kind_${u.kind}`)} ·{" "}
                        {(() => {
                          // Farkı oluşturan kayıt, mükellef sekmesinde seçili (#469).
                          const href = engagement
                            ? reconciliationItemHref(slug, engagement, u)
                            : null;
                          return href ? (
                            <Link
                              href={href}
                              className="text-brand-text truncate hover:underline"
                            >
                              {u.label}
                            </Link>
                          ) : (
                            <span className="truncate">{u.label}</span>
                          );
                        })()}
                      </span>
                      <Money value={u.effect} />
                    </li>
                  ))}
                </ul>
              )}
            </section>
            {item.sub_accounts.length > 0 ? (
              <section className="space-y-2">
                <h3 className="text-sm font-semibold">
                  {t("office.reconciliation.sub_accounts")}
                </h3>
                <Table>
                  <TableHeader className="[&_tr]:border-line text-xs">
                    <TableRow className="hover:bg-transparent">
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-start">
                        {t("office.reconciliation.col_account")}
                      </TableHead>
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-end">
                        {t("office.reconciliation.col_ledger")}
                      </TableHead>
                      <TableHead className="text-text-2 h-auto px-0 py-1 text-end">
                        {t("office.reconciliation.col_sources")}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {item.sub_accounts.map((s) => (
                      <TableRow
                        key={s.code}
                        className="border-line hover:bg-transparent"
                      >
                        <TableCell className="px-0 py-1.5">
                          <span className="font-mono">{s.code}</span>{" "}
                          <span className="text-text-2">{s.name}</span>
                        </TableCell>
                        <TableCell className="px-0 py-1.5 text-end">
                          <Money value={s.ledger_net} />
                        </TableCell>
                        <TableCell className="px-0 py-1.5 text-end">
                          <Money value={s.source_net} />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </section>
            ) : null}
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
