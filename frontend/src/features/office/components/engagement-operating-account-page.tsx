"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useId, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { formatMoney } from "@/lib/decimal";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodText, useUrlOfficePeriod } from "../period";
import {
  bookkeepingKeys,
  bookkeepingService,
  type BookScope,
  type OperatingAccount,
  type OperatingAccountCategory,
  type OperatingAccountRow,
} from "../services/bookkeeping.service";
import { useLedgerType } from "./ledger-type";
import { Money, Panel } from "./office-ui";

/**
 * İşletme hesabı (#780): işletme defteri ve serbest meslek mükellefinin
 * gelir / gider defteri. Hesap planı yok; alış faturası, masraf ve fiş
 * gider, satış faturası ve Z raporu gelir satırıdır. KDV ayrı gösterilir.
 */
export function EngagementOperatingAccountPage({
  engagement,
}: {
  engagement: string;
}) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <OperatingAccountInner engagement={engagement} />
    </Suspense>
  );
}

function OperatingAccountInner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  // Dönem adreste de durur (`?period=`), bağlantıyla gelinir.
  const { focus } = useUrlOfficePeriod();
  const periodText = usePeriodText();
  const router = useRouter();
  const search = useSearchParams();
  const yearly = search.get("gorunum") === "yil";
  const year = focus.slice(0, 4);
  const scope: BookScope = yearly ? { year } : { period: focus };
  const { singleEntry, isLoading: ledgerLoading } = useLedgerType(engagement);
  const q = useQuery({
    queryKey: bookkeepingKeys.book(slug, engagement, scope),
    queryFn: () => bookkeepingService.book(slug, engagement, scope),
    enabled: singleEntry,
  });
  const setView = (y: boolean) => {
    const next = new URLSearchParams(search.toString());
    if (y) next.set("gorunum", "yil");
    else next.delete("gorunum");
    router.replace(`?${next.toString()}`, { scroll: false });
  };

  if (ledgerLoading) return <Skeleton className="h-64 w-full" />;
  if (!singleEntry) {
    return (
      <EmptyState
        title={t("office.bookkeeping.balance_title")}
        description={t("office.bookkeeping.balance_hint")}
      />
    );
  }

  return (
    <div className="space-y-4" data-testid="operating-account">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-display me-auto text-lg font-bold">
          {yearly
            ? t("office.bookkeeping.title_year", { year })
            : t("office.bookkeeping.title_period", {
                period: periodText(focus),
              })}
        </h2>
        <div
          role="group"
          aria-label={t("office.bookkeeping.view")}
          className="flex gap-1"
        >
          <Button
            variant={yearly ? "outline" : "default"}
            size="sm"
            aria-pressed={!yearly}
            onClick={() => setView(false)}
          >
            {t("office.bookkeeping.view_period")}
          </Button>
          <Button
            variant={yearly ? "default" : "outline"}
            size="sm"
            aria-pressed={yearly}
            onClick={() => setView(true)}
          >
            {t("office.bookkeeping.view_year")}
          </Button>
        </div>
      </div>
      {q.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : q.isError || !q.data ? (
        <ErrorState
          title={t("office.api.load_error")}
          onRetry={() => void q.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <Book engagement={engagement} book={q.data} />
      )}
    </div>
  );
}

function Book({
  engagement,
  book,
}: {
  engagement: string;
  book: OperatingAccount;
}) {
  const { t } = useLocale();
  const s = book.summary;
  const loss = Number(s.profit) < 0;
  const pending =
    book.pending.expenses +
    book.pending.retail_sales +
    book.pending.missing_exchange_rate;
  return (
    <>
      <Panel title={t("office.bookkeeping.summary_title")}>
        <dl
          className="grid gap-4 p-4 sm:grid-cols-3"
          data-testid="operating-summary"
        >
          <SummaryFigure
            label={t("office.bookkeeping.income")}
            value={s.income}
            sub={t("office.bookkeeping.income_vat", {
              amount: formatMoney(s.income_vat, "TRY"),
            })}
            testId="operating-income"
          />
          <SummaryFigure
            label={t("office.bookkeeping.expense")}
            value={s.expense}
            sub={t("office.bookkeeping.expense_vat", {
              amount: formatMoney(s.expense_vat, "TRY"),
            })}
            testId="operating-expense"
          />
          <SummaryFigure
            label={
              loss
                ? t("office.bookkeeping.loss")
                : t("office.bookkeeping.profit")
            }
            value={s.profit}
            tone={loss ? "danger" : "ok"}
            sub={
              s.inventory_opening != null
                ? t("office.bookkeeping.profit_with_inventory")
                : t("office.bookkeeping.profit_hint")
            }
            testId="operating-profit"
          />
        </dl>
        {pending > 0 ? (
          <p
            className="text-warn border-t px-4 py-3 text-sm"
            data-testid="operating-pending"
          >
            {t("office.bookkeeping.pending", {
              expenses: book.pending.expenses,
              zreports: book.pending.retail_sales,
              rates: book.pending.missing_exchange_rate,
            })}
          </p>
        ) : null}
      </Panel>
      {book.inventory ? (
        <InventoryPanel
          key={`${book.inventory.year}-${book.inventory.opening}-${book.inventory.closing ?? ""}`}
          engagement={engagement}
          year={book.inventory.year}
          opening={book.inventory.opening}
          closing={book.inventory.closing}
        />
      ) : null}
      <Panel title={t("office.bookkeeping.categories_title")}>
        {book.categories.length === 0 ? (
          <EmptyState
            title={t("office.bookkeeping.empty")}
            description={t("office.bookkeeping.empty_hint")}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("office.bookkeeping.col_side")}</TableHead>
                <TableHead>{t("office.bookkeeping.col_category")}</TableHead>
                <TableHead className="text-end">
                  {t("office.bookkeeping.col_count")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.bookkeeping.col_net")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.bookkeeping.col_vat")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {book.categories.map((c) => (
                <TableRow key={`${c.side}-${c.category}-${c.category_name}`}>
                  <TableCell>
                    {t(`office.bookkeeping.side_${c.side}`)}
                  </TableCell>
                  <TableCell>
                    {c.category_name ||
                      t(`office.bookkeeping.cat_${c.category}`)}
                  </TableCell>
                  <TableCell className="text-end tabular-nums">
                    {c.count}
                  </TableCell>
                  <TableCell className="text-end">
                    <Money value={c.net} />
                  </TableCell>
                  <TableCell className="text-end">
                    <Money value={c.vat} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Panel>
      {book.rows.length > 0 ? (
        <Panel title={t("office.bookkeeping.rows_title")}>
          <RowsTable engagement={engagement} book={book} />
        </Panel>
      ) : null}
    </>
  );
}

function SummaryFigure({
  label,
  value,
  sub,
  tone,
  testId,
}: {
  label: string;
  value: string;
  sub: string;
  tone?: "danger" | "ok";
  testId: string;
}) {
  return (
    <div data-testid={testId}>
      <dt className="text-text-2 text-sm">{label}</dt>
      <dd
        className={cn(
          "font-display text-2xl font-bold",
          tone === "danger" && "text-danger",
        )}
      >
        <Money value={value} />
      </dd>
      <dd className="text-text-3 text-xs">{sub}</dd>
    </div>
  );
}

/** Yıllık dönem başı / dönem sonu mal mevcudu (işletme defteri). */
function InventoryPanel({
  engagement,
  year,
  opening,
  closing,
}: {
  engagement: string;
  year: number;
  opening: string;
  closing: string | null;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const id = useId();
  const canWrite = can(permissions.tenant.invoices.write);
  const [open, setOpen] = useState(opening);
  const [close, setClose] = useState(closing ?? "");
  const valid = (v: string) => /^\d+([.,]\d{1,2})?$/.test(v.trim());
  const norm = (v: string) => v.trim().replace(",", ".");
  const ok = valid(open) && (close.trim() === "" || valid(close));
  const save = useMutation({
    mutationFn: () =>
      bookkeepingService.setInventory(slug, engagement, {
        year,
        opening: norm(open),
        closing: close.trim() === "" ? null : norm(close),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: bookkeepingKeys.all(slug, engagement),
      });
      toast.success(t("office.bookkeeping.inventory_saved"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.bookkeeping.inventory_failed"),
      ),
  });
  return (
    <Panel
      title={t("office.bookkeeping.inventory_title", { year })}
      description={t("office.bookkeeping.inventory_hint")}
    >
      <form
        className="flex flex-wrap items-end gap-3 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (ok) save.mutate();
        }}
      >
        <div className="space-y-1">
          <Label htmlFor={`${id}-open`}>
            {t("office.bookkeeping.inventory_opening")}
          </Label>
          <Input
            id={`${id}-open`}
            inputMode="decimal"
            value={open}
            disabled={!canWrite}
            onChange={(e) => setOpen(e.target.value)}
            aria-invalid={!valid(open)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-close`}>
            {t("office.bookkeeping.inventory_closing")}
          </Label>
          <Input
            id={`${id}-close`}
            inputMode="decimal"
            value={close}
            placeholder={t("office.bookkeeping.inventory_closing_empty")}
            disabled={!canWrite}
            onChange={(e) => setClose(e.target.value)}
            aria-invalid={close.trim() !== "" && !valid(close)}
          />
        </div>
        {canWrite ? (
          <Button type="submit" disabled={!ok || save.isPending}>
            {t("office.bookkeeping.inventory_save")}
          </Button>
        ) : null}
      </form>
    </Panel>
  );
}

const SOURCE_KEY: Record<OperatingAccountRow["source"], string> = {
  invoice: "office.bookkeeping.source_invoice",
  expense: "office.bookkeeping.source_expense",
  retail_sale: "office.bookkeeping.source_retail_sale",
};

/** Defter satırları; yazma izni olan kategoriyi değiştirir. */
function RowsTable({
  engagement,
  book,
}: {
  engagement: string;
  book: OperatingAccount;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canWrite = can(permissions.tenant.invoices.write);
  const save = useMutation({
    mutationFn: (v: {
      row: OperatingAccountRow;
      category: OperatingAccountCategory | "";
    }) =>
      bookkeepingService.setCategory(slug, engagement, {
        source: v.row.source,
        record_uuid: v.row.record_uuid,
        category: v.category,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: bookkeepingKeys.all(slug, engagement),
      });
      toast.success(t("office.bookkeeping.category_saved"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.bookkeeping.category_failed"),
      ),
  });
  const choices = [...book.choices.income, ...book.choices.expense];
  const date = (d: string | null) =>
    d
      ? new Date(`${d}T00:00:00`).toLocaleDateString(
          locale === "tr" ? "tr-TR" : "en-GB",
        )
      : "—";
  return (
    <Table data-testid="operating-rows">
      <TableHeader>
        <TableRow>
          <TableHead>{t("office.bookkeeping.col_date")}</TableHead>
          <TableHead>{t("office.bookkeeping.col_document")}</TableHead>
          <TableHead>{t("office.bookkeeping.col_counterparty")}</TableHead>
          <TableHead>{t("office.bookkeeping.col_side")}</TableHead>
          <TableHead>{t("office.bookkeeping.col_category")}</TableHead>
          <TableHead className="text-end">
            {t("office.bookkeeping.col_net")}
          </TableHead>
          <TableHead className="text-end">
            {t("office.bookkeeping.col_vat")}
          </TableHead>
          <TableHead className="text-end">
            {t("office.bookkeeping.col_total")}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {book.rows.map((r) => (
          <TableRow key={`${r.source}-${r.record_uuid}`}>
            <TableCell className="whitespace-nowrap">{date(r.date)}</TableCell>
            <TableCell>
              <span className="block text-xs">{t(SOURCE_KEY[r.source])}</span>
              <span className="font-mono text-xs">{r.document_no || "—"}</span>
            </TableCell>
            <TableCell className="max-w-56 truncate">
              {r.counterparty || "—"}
            </TableCell>
            <TableCell>{t(`office.bookkeeping.side_${r.side}`)}</TableCell>
            <TableCell>
              {canWrite ? (
                <NativeSelect
                  aria-label={t("office.bookkeeping.category_for", {
                    document: r.document_no || t(SOURCE_KEY[r.source]),
                  })}
                  value={r.overridden ? r.category : ""}
                  disabled={save.isPending}
                  onChange={(e) =>
                    save.mutate({
                      row: r,
                      category: e.target.value as OperatingAccountCategory | "",
                    })
                  }
                >
                  <NativeSelectOption value="">
                    {r.overridden
                      ? t("office.bookkeeping.category_reset")
                      : t("office.bookkeeping.category_default", {
                          category:
                            r.category_name ||
                            t(`office.bookkeeping.cat_${r.category}`),
                        })}
                  </NativeSelectOption>
                  {choices.map((c) => (
                    <NativeSelectOption key={c} value={c}>
                      {t(`office.bookkeeping.cat_${c}`)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              ) : (
                r.category_name || t(`office.bookkeeping.cat_${r.category}`)
              )}
            </TableCell>
            <TableCell className="text-end">
              <Money value={r.net} />
            </TableCell>
            <TableCell className="text-end">
              <Money value={r.vat} />
            </TableCell>
            <TableCell className="text-end">
              <Money value={r.total} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

/**
 * İşletme defteri / serbest meslek mükellefinde hesap kodu paneli yerine
 * (#780): belgenin işletme hesabındaki gelir / gider kategorisi.
 */
export function OperatingCategoryPanel({
  engagement,
  period,
  source,
  record,
}: {
  engagement: string;
  period: string;
  source: OperatingAccountRow["source"];
  record: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const scope: BookScope = { period };
  const q = useQuery({
    queryKey: bookkeepingKeys.book(slug, engagement, scope),
    queryFn: () => bookkeepingService.book(slug, engagement, scope),
    enabled: !!period,
  });
  const row = q.data?.rows.find(
    (r) => r.source === source && r.record_uuid === record,
  );
  return (
    <Panel
      title={t("office.bookkeeping.document_title")}
      description={t("office.bookkeeping.document_hint")}
    >
      <div className="p-4 text-sm" data-testid="operating-category">
        {q.isLoading ? (
          <Skeleton className="h-6 w-48" />
        ) : row ? (
          <p>
            <span className="font-semibold">
              {t(`office.bookkeeping.side_${row.side}`)}
            </span>
            {" · "}
            {row.category_name || t(`office.bookkeeping.cat_${row.category}`)}
          </p>
        ) : (
          <p className="text-text-2">{t("office.bookkeeping.document_none")}</p>
        )}
      </div>
    </Panel>
  );
}
