"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Check,
  Clock,
  Download,
  FileSpreadsheet,
  Plus,
  Receipt,
  Tags,
  Upload,
  X,
} from "lucide-react";
import Link from "next/link";
import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { EntityTable, useUrlListState } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
  formatCellMoney,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useOfficePeriod } from "../period";
import {
  type Expense,
  type ExpenseCategory,
  receiptKeys,
  receiptsService,
} from "../services/receipts.service";
import { FOCUS_PARAM } from "../lib/related-href";
import { useLedgerType } from "./ledger-type";
import { ExpenseCategoriesDialog } from "./expense-categories-dialog";
import { ExpenseCreateDialog } from "./expense-create-dialog";
import { ExpenseLucaDialog } from "./expense-luca-dialog";
import { ExpenseVatBreakdown } from "./expense-vat-breakdown";
import { Money } from "./office-ui";
import {
  FreshBadge,
  ImportResultNote,
  isFreshCapture,
  LIVE_REFRESH_MS,
  ReviewFlag,
  SummaryCards,
  useImportFile,
} from "./receipts-ui";
import { UnreadableReceipts } from "./unreadable-receipts";
import { useRelatedColumn } from "./related-column";

type StatusFilter = "all" | "pending" | "approved" | "rejected";
const STATUS_FILTERS: StatusFilter[] = [
  "all",
  "pending",
  "approved",
  "rejected",
];
const PAGE = 50;

export const approvalVariants: Record<string, StatusVariant> = {
  pending: { labelKey: "office.receipts.status_pending", tone: "warn" },
  approved: { labelKey: "office.receipts.status_approved", tone: "ok" },
  rejected: { labelKey: "office.receipts.status_rejected", tone: "neutral" },
};

export function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

function AccountInput({
  value,
  label,
  onCommit,
}: {
  value: string;
  label: string;
  onCommit: (v: string) => void;
}) {
  const [account, setAccount] = useState(value);
  return (
    <Input
      aria-label={label}
      className="h-7 w-28 px-2 py-0 font-mono text-xs shadow-none"
      value={account}
      onChange={(e) => setAccount(e.target.value)}
      onBlur={() => {
        const v = account.trim();
        if (v && v !== value) onCommit(v);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
      }}
    />
  );
}

/** Kategori ve hesap kodu: bekleyen satırda seçilebilir. */
function PostingCell({
  x,
  categories,
  canWrite,
  singleEntry,
  onChange,
}: {
  x: Expense;
  categories: ExpenseCategory[];
  canWrite: boolean;
  /** İşletme defteri / serbest meslek (#780): hesap kodu yok. */
  singleEntry: boolean;
  onChange: (input: { category_code?: string; account_code?: string }) => void;
}) {
  const { t } = useLocale();
  const editable = canWrite && x.approval_status === "pending";
  const source = x.account_source ?? "";
  return (
    <div className="flex min-w-44 flex-col gap-1">
      {editable ? (
        <NativeSelect
          size="sm"
          aria-label={t("office.expenses.col_category")}
          wrapperClassName="max-w-52"
          className="shadow-none"
          value={x.category_code ?? ""}
          onChange={(e) => onChange({ category_code: e.target.value })}
        >
          {x.category_code ? null : (
            <NativeSelectOption value="">—</NativeSelectOption>
          )}
          {categories
            .filter((c) => c.is_active)
            .map((c) => (
              <NativeSelectOption key={c.code} value={c.code}>
                {c.name}
              </NativeSelectOption>
            ))}
        </NativeSelect>
      ) : (
        <span className="text-sm">{x.category_name ?? "—"}</span>
      )}
      <div className={cn("flex items-center gap-1.5", singleEntry && "hidden")}>
        {editable ? (
          // Yeni kod gelince (öneri / kategori değişti) alan baştan çizilir.
          <AccountInput
            key={x.account_code ?? ""}
            value={x.account_code ?? ""}
            label={t("office.expenses.col_account")}
            onCommit={(v) => onChange({ account_code: v })}
          />
        ) : (
          <span className="font-mono text-xs">{x.account_code ?? "—"}</span>
        )}
        {source ? (
          <span
            className={cn(
              "rounded px-1.5 py-0.5 text-[11px] whitespace-nowrap",
              x.suggested
                ? "bg-brand-tint text-brand-text"
                : "bg-surface-3 text-text-2",
            )}
            title={t(`office.expenses.source_${source}_hint`)}
          >
            {x.suggested
              ? t("office.expenses.suggested")
              : t(`office.expenses.source_${source}`)}
          </span>
        ) : null}
      </div>
    </div>
  );
}

/**
 * `/t/{slug}/engagements/{uuid}/expenses` (#384): mükellefin masrafları.
 * Dönem üst çubuktan; durum ve kategori süzgeci URL'de. Bekleyen satırda
 * kategori / hesap seçilir, onay yevmiye taslağı üretir. Excel içe / dışa
 * aktarma ve kategori yönetimi (ofis şablonu + mükellefe özel).
 */
export function EngagementExpensesPage({ engagement }: { engagement: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <ExpensesInner engagement={engagement} />
    </Suspense>
  );
}

function ExpensesInner({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.receipts.write);
  const { singleEntry } = useLedgerType(engagement);
  const canApprove = can(permissions.tenant.receipts.approve);
  const qc = useQueryClient();
  const { period } = useOfficePeriod();
  const listState = useUrlListState({ initialPageSize: PAGE });
  const rawStatus = listState.getParam("status") as StatusFilter | null;
  const status: StatusFilter =
    rawStatus && STATUS_FILTERS.includes(rawStatus) ? rawStatus : "all";
  const category = listState.getParam("category") ?? "";
  const { limit, offset } = listState.params;
  const [categoriesOpen, setCategoriesOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [lucaOpen, setLucaOpen] = useState(false);

  const lastPeriod = useRef(period);
  const { onPaginationChange } = listState;
  useEffect(() => {
    if (lastPeriod.current === period) return;
    lastPeriod.current = period;
    onPaginationChange((p) => ({ ...p, pageIndex: 0 }));
  }, [onPaginationChange, period]);

  const filter = {
    period,
    status: status === "all" ? undefined : status,
    category: category || undefined,
  };
  const list = useQuery({
    queryKey: [
      ...receiptKeys.expenses(slug, engagement),
      { ...filter, limit, offset },
    ],
    queryFn: () =>
      receiptsService.expenses(slug, engagement, { ...filter, limit, offset }),
    placeholderData: keepPreviousData,
    // Canlı (#780): mükellefin telefondan çektiği fiş birkaç saniyede gelir;
    // sekme arka plandayken sorgu durur.
    refetchInterval: LIVE_REFRESH_MS,
  });
  const cats = useQuery({
    queryKey: receiptKeys.categories(slug, engagement),
    queryFn: () => receiptsService.categories(slug, engagement),
  });
  const categories = useMemo(() => cats.data?.items ?? [], [cats.data]);
  const refresh = () =>
    void qc.invalidateQueries({
      queryKey: receiptKeys.expenses(slug, engagement),
    });

  const posting = useMutation({
    mutationFn: (v: {
      uuid: string;
      input: { category_code?: string; account_code?: string };
    }) => receiptsService.setPosting(slug, engagement, v.uuid, v.input),
    onSuccess: refresh,
    onError: (e) => toast.error(errText(e, t("office.receipts.save_failed"))),
  });
  const decide = useMutation({
    mutationFn: (v: { uuid: string; approve: boolean }) =>
      receiptsService.decideExpense(slug, engagement, v.uuid, v.approve),
    onSuccess: (x, v) => {
      refresh();
      toast.success(
        v.approve
          ? x.journal
            ? t("office.expenses.approved_with_journal", {
                no: x.journal.entry_no,
              })
            : t("office.receipts.approved")
          : t("office.receipts.rejected"),
      );
    },
    onError: (e) => toast.error(errText(e, t("office.receipts.decide_failed"))),
  });
  const imp = useImportFile(
    (file) => receiptsService.importExpenses(slug, engagement, file),
    refresh,
  );

  const baseColumns = useMemo<ColumnDef<Expense, unknown>[]>(
    () => [
      createDateColumn<Expense>({
        id: "expense_date",
        labelKey: "office.col_date",
        accessor: (x) => x.expense_date,
        className: "text-sm",
        gridSecondary: true,
      }),
      createColumn<Expense>({
        id: "vendor",
        labelKey: "office.expenses.col_vendor",
        accessorFn: (x) => x.vendor_name ?? x.description,
        gridPrimary: true,
        meta: { cellClassName: "min-w-48" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <>
              <div className="max-w-xs truncate">
                {x.vendor_name ||
                  x.description ||
                  t("office.expenses.no_vendor")}
                {isFreshCapture(x) ? <FreshBadge /> : null}
              </div>
              <div className="text-text-3 max-w-xs truncate text-xs">
                {[
                  x.vendor_tax_number,
                  x.receipt_number && `#${x.receipt_number}`,
                  x.vehicle_plate,
                ]
                  .filter(Boolean)
                  .join(" · ")}
                {x.capture_method === "excel_import" ? (
                  <span className="ms-1">
                    · {t("office.receipts.from_excel")}
                  </span>
                ) : x.capture_method === "ocr" ? (
                  <span className="ms-1">
                    · {t("office.receipts.from_photo")}
                  </span>
                ) : null}
              </div>
              <ReviewFlag fields={x.review_fields} />
              {x.exported_at ? (
                <div
                  className="text-text-3 text-xs"
                  title={x.exported_file ?? undefined}
                  data-testid="expense-exported"
                >
                  {t("office.expenses.exported_before", {
                    date: date(x.exported_at, "dd.MM.yyyy", locale),
                  })}
                </div>
              ) : null}
            </>
          );
        },
      }),
      createColumn<Expense>({
        id: "posting",
        labelKey: "office.expenses.col_posting",
        accessorFn: (x) => x.account_code ?? "",
        cell: ({ row }) => (
          <PostingCell
            x={row.original}
            categories={categories}
            canWrite={canWrite}
            singleEntry={singleEntry}
            onChange={(input) =>
              posting.mutate({ uuid: row.original.uuid, input })
            }
          />
        ),
      }),
      createColumn<Expense>({
        id: "tax_amount",
        labelKey: "office.expenses.col_tax",
        accessorFn: (x) => x.tax_amount ?? "",
        cell: ({ row }) => {
          const x = row.original;
          return (
            <div className="text-end tabular-nums">
              <div className="text-sm whitespace-nowrap">
                {x.tax_amount ? formatCellMoney(x.tax_amount, x.currency) : "—"}
              </div>
              {/* Oran kırılımı (#780): oran, matrah, KDV; yoksa "kırılım yok". */}
              <ExpenseVatBreakdown
                lines={x.vat_lines}
                receiptRead={x.receipt_read}
                currency={x.currency}
                className="ms-auto mt-1 w-fit"
              />
            </div>
          );
        },
      }),
      createMoneyColumn<Expense>({
        id: "amount",
        labelKey: "office.expenses.col_total",
        accessor: (x) => x.amount,
        currency: (x) => x.currency,
        className: "font-semibold",
      }),
      createStatusColumn<Expense>({
        id: "approval_status",
        labelKey: "office.receipts.col_status",
        accessor: (x) => x.approval_status,
        variants: approvalVariants,
      }),
      createColumn<Expense>({
        id: "journal",
        labelKey: "office.receipts.col_journal",
        accessorFn: (x) => x.journal?.entry_no ?? "",
        cell: ({ row }) =>
          row.original.journal ? (
            <Link
              href={`${routes.tenant.engagementJournals(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`journal_draft:${row.original.journal.uuid}`)}`}
              className="text-brand-text font-mono text-sm hover:underline"
            >
              #{row.original.journal.entry_no}
            </Link>
          ) : (
            <span className="text-text-3">—</span>
          ),
      }),
      createActionsColumn<Expense>({
        cell: (x) =>
          canApprove && x.approval_status === "pending" ? (
            <div className="flex justify-end gap-1.5">
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ uuid: x.uuid, approve: true })}
              >
                <Check className="size-4" aria-hidden />
                {t("office.receipts.approve")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                aria-label={t("office.receipts.reject")}
                onClick={() => decide.mutate({ uuid: x.uuid, approve: false })}
              >
                <X className="size-4" aria-hidden />
              </Button>
            </div>
          ) : null,
      }),
    ],
    [
      t,
      locale,
      slug,
      engagement,
      categories,
      canWrite,
      canApprove,
      posting,
      decide,
      singleEntry,
    ],
  );
  // Satırın ilişkili kayıtları: mükellef detayında seçili açılır (#469).
  const relatedColumn = useRelatedColumn<Expense>(
    "expense",
    (x) => x.uuid,
    (x) => x.vendor_name ?? x.description,
  );
  const columns = useMemo<ColumnDef<Expense, unknown>[]>(
    () => [...baseColumns, ...relatedColumn],
    [baseColumns, relatedColumn],
  );

  const sum = list.data;
  const filters: { key: StatusFilter; label: string }[] = STATUS_FILTERS.map(
    (k) => ({
      key: k,
      label:
        k === "all"
          ? t("office.receipts.filter_all")
          : t(`office.receipts.status_${k}`),
    }),
  );

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display truncate text-xl font-bold">
            {t("office.expenses.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.expenses.description")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {canWrite ? (
            <Button
              size="sm"
              className="max-md:min-h-11"
              onClick={() => setCreateOpen(true)}
              data-testid="expense-create-open"
            >
              <Plus className="size-4" aria-hidden />
              {t("office.expenses.create")}
            </Button>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            onClick={() => setCategoriesOpen(true)}
          >
            <Tags className="size-4" aria-hidden />
            {t("office.expenses.categories")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            onClick={() =>
              void receiptsService
                .exportExpenses(slug, engagement, filter)
                .catch(() => toast.error(t("office.receipts.export_failed")))
            }
          >
            <Download className="size-4" aria-hidden />
            {t("office.receipts.export")}
          </Button>
          {!singleEntry ? (
            <Button
              variant="outline"
              size="sm"
              className="max-md:min-h-11"
              onClick={() => setLucaOpen(true)}
              data-testid="expense-luca-open"
            >
              <FileSpreadsheet className="size-4" aria-hidden />
              {t("office.expenses.luca")}
            </Button>
          ) : null}
          {canWrite ? (
            <Button
              variant="outline"
              size="sm"
              className="max-md:min-h-11"
              onClick={imp.open}
              disabled={imp.pending}
            >
              <Upload className="size-4" aria-hidden />
              {t("office.receipts.import")}
            </Button>
          ) : null}
          {imp.input}
        </div>
      </div>

      <SummaryCards
        cards={[
          {
            label: t("office.expenses.total"),
            value: sum ? <Money value={sum.amount} /> : "–",
            icon: Receipt,
            box: "bg-brand-tint text-brand-text",
          },
          {
            label: t("office.expenses.tax_total"),
            value: sum ? <Money value={sum.tax_amount} /> : "–",
            icon: Receipt,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.receipts.pending_count"),
            value: sum ? sum.pending : "–",
            icon: Clock,
            box: "bg-warn-tint text-warn",
          },
        ]}
      />

      <ImportResultNote result={imp.result} onClose={imp.clear} />

      <UnreadableReceipts engagement={engagement} />

      <div className="flex flex-wrap items-center gap-2">
        <div
          role="group"
          aria-label={t("office.receipts.filter_label")}
          className="flex flex-wrap gap-2"
        >
          {filters.map((f) => (
            <FilterChip
              key={f.key}
              pressed={status === f.key}
              onPressedChange={() =>
                listState.setParams({ status: f.key === "all" ? null : f.key })
              }
            >
              {f.label}
            </FilterChip>
          ))}
        </div>
        <div className="ms-auto flex items-center gap-2 text-xs">
          <Label
            htmlFor="expenses-category"
            className="text-muted-foreground text-xs font-normal"
          >
            {t("office.expenses.col_category")}
          </Label>
          <NativeSelect
            id="expenses-category"
            className="pl-2 shadow-none"
            value={category}
            onChange={(e) =>
              listState.setParams({ category: e.target.value || null })
            }
          >
            <NativeSelectOption value="">
              {t("office.receipts.filter_all")}
            </NativeSelectOption>
            <NativeSelectOption value="-">
              {t("office.expenses.no_category")}
            </NativeSelectOption>
            {categories.map((c) => (
              <NativeSelectOption key={c.code} value={c.code}>
                {c.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
      </div>

      <EntityTable<Expense>
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.expenses.empty_title")}
        emptyDescription={t("office.expenses.empty")}
        state={listState.tableState}
        rowCount={list.data?.total ?? 0}
        pageSizeOptions={[25, 50, 100]}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          persistKey: "office.expenses.table",
        }}
      />

      <ExpenseLucaDialog
        open={lucaOpen}
        onOpenChange={setLucaOpen}
        engagement={engagement}
        period={period ?? ""}
      />

      <ExpenseCreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        engagement={engagement}
        onCreated={refresh}
      />

      <ExpenseCategoriesDialog
        open={categoriesOpen}
        onOpenChange={setCategoriesOpen}
        engagement={engagement}
        categories={categories}
        canEdit={canApprove}
        onChanged={() => {
          void qc.invalidateQueries({
            queryKey: receiptKeys.categories(slug, engagement),
          });
          refresh();
        }}
      />
    </div>
  );
}
