"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, FileText, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createStatusColumn,
  MoneyCell,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
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
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  bankPostingService,
  type BankLedgerAccount,
  type BankPosting,
  type PostingStatus,
} from "../services/bank-posting.service";

type Filter = "all" | PostingStatus;
const FILTERS: Filter[] = [
  "all",
  "unassigned",
  "suggested",
  "approved",
  "drafted",
];
const PAGE = 50;

const statusVariants: Record<string, StatusVariant> = {
  unassigned: {
    labelKey: "office.bank.posting.status_unassigned",
    tone: "warn",
  },
  suggested: { labelKey: "office.bank.posting.status_suggested", tone: "info" },
  approved: { labelKey: "office.bank.posting.status_approved", tone: "ok" },
  drafted: { labelKey: "office.bank.posting.status_drafted", tone: "neutral" },
};

const pct = new Intl.NumberFormat("tr-TR", {
  style: "percent",
  maximumFractionDigits: 0,
});

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

const pending = (x: BankPosting) =>
  x.status === "suggested" && x.suggestion_status === "pending";

/**
 * Banka hareketi → mizan hesabı eşleştirme (#383): eşleşmeyenler, öneriler
 * (hesap kodu, güven, gerekçe), tek tıkla ve toplu onay, elle hesap seçimi
 * ve onaylananlardan fiş taslağı. Elle seçim ve onay karşı tarafın takma
 * adını öğretir; sonraki ekstrede aynı ödeyen aynı cariye önerilir.
 */
export function BankPostingPanel({
  engagement,
  period,
}: {
  engagement: string;
  period?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const listState = useUrlListState({ initialPageSize: PAGE });
  const raw = listState.getParam("posting") as Filter | null;
  const filter: Filter = raw && FILTERS.includes(raw) ? raw : "all";
  const { limit, offset } = listState.params;
  const [editing, setEditing] = useState<BankPosting | null>(null);

  const key = ["tenant", slug.toLowerCase(), "bank-postings", engagement];
  const list = useQuery({
    queryKey: [...key, { period, filter, limit, offset }],
    queryFn: () =>
      bankPostingService.list(slug, engagement, {
        period,
        status: filter === "all" ? undefined : filter,
        limit,
        offset,
      }),
    placeholderData: keepPreviousData,
  });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: key });
    void qc.invalidateQueries({
      queryKey: ["tenant", slug.toLowerCase(), "bank-aliases", engagement],
    });
  };

  const suggest = useMutation({
    mutationFn: () =>
      bankPostingService.suggest(slug, engagement, period ?? ""),
    onSuccess: (r) => {
      toast.success(
        t("office.bank.posting.suggest_done", {
          suggested: r.suggested,
          auto: r.auto_approved,
          none: r.no_suggestion,
        }),
      );
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  const approve = useMutation({
    mutationFn: (ids: string[]) =>
      bankPostingService.approve(slug, engagement, ids),
    onSuccess: (r) => {
      toast.success(
        t("office.bank.posting.approve_done", {
          approved: r.approved,
          skipped: r.skipped,
        }),
      );
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  const drafts = useMutation({
    mutationFn: () => bankPostingService.drafts(slug, engagement, period),
    onSuccess: (r) => {
      toast.success(
        t("office.bank.posting.drafts_done", {
          drafts: r.drafts.length,
          count: r.transactions,
        }),
      );
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });

  const items = list.data?.items ?? [];
  const sum = list.data?.summary;
  const total = sum ? (filter === "all" ? sum.total : sum[filter]) : 0;

  const columns = useMemo<ColumnDef<BankPosting, unknown>[]>(
    () => [
      createDateColumn<BankPosting>({
        id: "transaction_date",
        labelKey: "office.col_date",
        accessor: (x) => x.transaction_date,
        className: "text-sm",
        gridSecondary: true,
      }),
      createColumn<BankPosting>({
        id: "counterparty",
        labelKey: "office.bank.col_description",
        accessorFn: (x) => x.counterparty_name || x.description,
        gridPrimary: true,
        meta: { cellClassName: "min-w-56" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <>
              <div className="max-w-sm truncate">
                {x.counterparty_name || x.description}
              </div>
              <div className="text-text-3 max-w-sm truncate text-xs">
                {x.counterparty_name ? x.description : x.cash_box_name}
              </div>
            </>
          );
        },
      }),
      createColumn<BankPosting>({
        id: "amount",
        labelKey: "office.bank.col_amount",
        accessorKey: "amount",
        meta: { align: "end" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <span
              className={cn(
                "font-semibold whitespace-nowrap",
                x.direction === "A" ? "text-ok" : "text-foreground",
              )}
            >
              <span className="sr-only">
                {t(
                  x.direction === "A"
                    ? "office.bank.direction_in"
                    : "office.bank.direction_out",
                )}
                :{" "}
              </span>
              {x.direction === "A" ? "+" : "−"}
              <MoneyCell value={x.amount} currency={x.currency} />
            </span>
          );
        },
      }),
      createColumn<BankPosting>({
        id: "account",
        labelKey: "office.bank.posting.col_account",
        accessorFn: (x) => x.account_code ?? "",
        meta: { cellClassName: "min-w-56" },
        cell: ({ row }) => {
          const x = row.original;
          if (!x.account_code)
            return (
              <span className="text-text-3 text-sm">
                {t("office.bank.posting.no_suggestion")}
              </span>
            );
          return (
            <div className="min-w-0">
              <div className="flex items-baseline gap-2">
                <span className="figure font-semibold">{x.account_code}</span>
                <span className="text-text-2 truncate text-sm">
                  {x.account_name}
                </span>
              </div>
              {x.reason ? (
                <div
                  className="text-text-3 max-w-sm truncate text-xs"
                  title={x.reason}
                >
                  {x.reason}
                </div>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<BankPosting>({
        id: "confidence",
        labelKey: "office.bank.posting.col_confidence",
        accessorFn: (x) => x.confidence ?? "",
        meta: { align: "end" },
        cell: ({ row }) => {
          const c = row.original.confidence;
          if (!c) return null;
          const n = Number(c);
          return (
            <span
              className={cn(
                "figure text-sm",
                n >= 0.9
                  ? "text-ok"
                  : n >= 0.7
                    ? "text-foreground"
                    : "text-warn",
              )}
            >
              {pct.format(n)}
            </span>
          );
        },
      }),
      createStatusColumn<BankPosting>({
        id: "status",
        labelKey: "office.bank.posting.col_status",
        accessor: (x) => x.status,
        variants: statusVariants,
      }),
    ],
    [t],
  );

  const filters = FILTERS.map((f) => ({
    key: f,
    label: t(
      f === "all"
        ? "office.bank.filter_all"
        : `office.bank.posting.status_${f}`,
    ),
    count: sum ? (f === "all" ? sum.total : sum[f]) : undefined,
  }));

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          role="group"
          aria-label={t("office.bank.posting.filter_label")}
          className="flex flex-wrap gap-2"
        >
          {filters.map((f) => (
            <FilterChip
              key={f.key}
              pressed={filter === f.key}
              onPressedChange={() =>
                listState.setParams({ posting: f.key === "all" ? null : f.key })
              }
            >
              {f.label}
              {f.count !== undefined ? (
                <span className="ms-1.5 tabular-nums">{f.count}</span>
              ) : null}
            </FilterChip>
          ))}
        </div>
        {canWrite ? (
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={!period || suggest.isPending}
              title={period ? undefined : t("office.bank.posting.need_period")}
              onClick={() => suggest.mutate()}
            >
              <Sparkles className="size-4" aria-hidden />
              {t("office.bank.posting.suggest")}
            </Button>
            <Button
              disabled={!sum?.approved || drafts.isPending}
              onClick={() => drafts.mutate()}
            >
              <FileText className="size-4" aria-hidden />
              {t("office.bank.posting.drafts", { count: sum?.approved ?? 0 })}
            </Button>
          </div>
        ) : null}
      </div>

      <EntityTable<BankPosting>
        columns={columns}
        data={items}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.bank.posting.empty_title")}
        emptyDescription={t("office.bank.posting.empty")}
        state={listState.tableState}
        rowCount={total}
        pageSizeOptions={[25, 50, 100]}
        bulkActions={
          canWrite
            ? [
                {
                  id: "approve",
                  label: t("office.bank.posting.approve_selected"),
                  icon: CheckCheck,
                  disabled: (rows) => !rows.some(pending),
                  onClick: (rows) =>
                    approve.mutate(rows.filter(pending).map((x) => x.uuid)),
                },
              ]
            : undefined
        }
        rowActions={
          canWrite
            ? (x) => (
                <div className="flex justify-end gap-1.5">
                  {pending(x) ? (
                    <Button
                      size="sm"
                      disabled={approve.isPending}
                      onClick={() => approve.mutate([x.uuid])}
                    >
                      {t("office.bank.posting.approve")}
                    </Button>
                  ) : null}
                  {x.status !== "drafted" ? (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setEditing(x)}
                    >
                      {t("office.bank.posting.choose_account")}
                    </Button>
                  ) : null}
                </div>
              )
            : undefined
        }
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: canWrite,
          facetedFilters: false,
          viewMode: false,
          persistKey: "office.bank.posting.table",
        }}
      />

      <LedgerAccounts engagement={engagement} canWrite={canWrite} />

      <AccountDialog
        engagement={engagement}
        row={editing}
        onClose={() => setEditing(null)}
        onSaved={() => {
          setEditing(null);
          refresh();
        }}
      />
    </div>
  );
}

function AccountDialog({
  engagement,
  row,
  onClose,
  onSaved,
}: {
  engagement: string;
  row: BankPosting | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [form, setForm] = useState({ key: "", code: "", name: "" });
  // Satır değişince alanlar öneriyle dolar.
  const current =
    row && form.key === row.uuid
      ? form
      : {
          key: row?.uuid ?? "",
          code: row?.account_code ?? "",
          name: row?.account_name ?? "",
        };
  const save = useMutation({
    mutationFn: () =>
      bankPostingService.setAccount(
        slug,
        engagement,
        row?.uuid ?? "",
        current.code.trim(),
        current.name.trim(),
      ),
    onSuccess: () => {
      toast.success(t("office.bank.posting.account_saved"));
      onSaved();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  return (
    <Dialog open={row !== null} onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("office.bank.posting.choose_account")}</DialogTitle>
          <DialogDescription>
            {row?.counterparty_name || row?.description}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="bank-account-code">
              {t("office.bank.posting.account_code")}
            </Label>
            <Input
              id="bank-account-code"
              value={current.code}
              inputMode="decimal"
              placeholder="120.01.015"
              onChange={(e) => setForm({ ...current, code: e.target.value })}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bank-account-name">
              {t("office.bank.posting.account_name")}
            </Label>
            <Input
              id="bank-account-name"
              value={current.name}
              onChange={(e) => setForm({ ...current, name: e.target.value })}
            />
          </div>
          <p className="text-text-3 text-xs">
            {t("office.bank.posting.learn_hint")}
          </p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={
                !/^\d{3}(\.[0-9A-Za-z]+)*$/.test(current.code.trim()) ||
                save.isPending
              }
            >
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Banka hesaplarının mizan kodu (102.01 ...): fiş taslağının banka tarafı. */
function LedgerAccounts({
  engagement,
  canWrite,
}: {
  engagement: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = [
    "tenant",
    slug.toLowerCase(),
    "bank-ledger-accounts",
    engagement,
  ];
  const q = useQuery({
    queryKey: key,
    queryFn: () => bankPostingService.ledgerAccounts(slug, engagement),
  });
  const [edits, setEdits] = useState<Record<string, string>>({});
  const save = useMutation({
    mutationFn: (a: BankLedgerAccount) =>
      bankPostingService.setLedgerAccount(
        slug,
        engagement,
        a.cash_box_uuid,
        (edits[a.cash_box_uuid] ?? a.account_code).trim(),
      ),
    onSuccess: () => {
      toast.success(t("office.bank.posting.account_saved"));
      void qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  const items = q.data?.items ?? [];
  if (items.length === 0) return null;
  return (
    <section className="bg-surface border-line space-y-3 rounded-xl border p-4">
      <div>
        <h2 className="font-display text-base font-semibold">
          {t("office.bank.posting.ledger_title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.bank.posting.ledger_description")}
        </p>
      </div>
      <ul className="divide-line divide-y">
        {items.map((a) => {
          const value = edits[a.cash_box_uuid] ?? a.account_code;
          return (
            <li
              key={a.cash_box_uuid}
              className="flex flex-wrap items-center justify-between gap-2 py-2"
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">
                  {a.name || a.iban}
                </span>
                <span className="text-text-3 figure block truncate text-xs">
                  {a.iban} · {a.currency}
                </span>
              </span>
              <span className="flex items-center gap-2">
                <Input
                  aria-label={t("office.bank.posting.account_code")}
                  className="figure w-32"
                  value={value}
                  disabled={!canWrite}
                  onChange={(e) =>
                    setEdits({ ...edits, [a.cash_box_uuid]: e.target.value })
                  }
                />
                {canWrite ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={value === a.account_code && !a.default}
                    onClick={() => save.mutate(a)}
                  >
                    {t("common.save")}
                  </Button>
                ) : null}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
