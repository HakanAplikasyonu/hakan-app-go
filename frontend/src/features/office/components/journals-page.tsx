"use client";

import {
  BookOpenCheck,
  Check,
  Download,
  FileSpreadsheet,
  Loader2,
  MonitorUp,
  Plus,
  Trash2,
  Undo2,
} from "lucide-react";
import { Suspense, useCallback, useMemo, useState } from "react";
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
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useExportDrivers,
  useJournalActions,
  useJournalDrafts,
  useJournalExports,
  useJournalTransfers,
} from "../hooks/use-journals";
import { formatMoney, sub, sum, toMinor } from "../lib/decimal";
import { exportIssueText } from "../lib/export-issue";
import {
  journalService,
  TRANSFER_DRIVERS,
  type JournalDraft,
  type JournalExportSummary,
  type JournalTransfer,
} from "../services/journals.service";
import { useEngagementScope } from "../engagement-scope";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import {
  JournalTransfersPanel,
  TRANSFER_STATUS,
} from "./journal-transfers-panel";
import { FilterSelect, Figure, Money, Panel } from "./office-ui";
import {
  ALL_PERIODS,
  isPeriod,
  useOfficePeriod,
  usePeriodText,
} from "../period";

type LineRow = {
  account: string;
  description: string;
  debit: string;
  credit: string;
};
const emptyLine = (): LineRow => ({
  account: "",
  description: "",
  debit: "",
  credit: "",
});
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

// Fiş kalemi formu: sıralama da kapalı (kalem sırası fişin sırasıdır).
// Mobilde kartlar bilinçli açık; kart içindeki girdiler çalışır.
const lineEditorTable: DataTableFeatures = {
  ...embeddedTable,
  sorting: false,
};

/** API error in Turkish when the code is known, else the server message (#161). */
function errorText(t: Translate, err: unknown, fallback: string) {
  if (isApiError(err)) {
    const key = `office.journals.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message || fallback;
  }
  return err instanceof Error ? err.message : fallback;
}

/** Program label for an export driver id; the id itself when unknown. */
function driverLabel(t: Translate, name: string) {
  const key = `office.journals.driver_${name}`;
  const msg = t(key);
  return msg === key ? name : msg;
}

function NewDraft({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { create } = useJournalActions(slug, engagement);
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [description, setDescription] = useState("");
  const [lines, setLines] = useState<LineRow[]>([emptyLine(), emptyLine()]);
  const debit = sum(lines.map((l) => l.debit || "0"));
  const credit = sum(lines.map((l) => l.credit || "0"));
  const balanced = toMinor(debit) === toMinor(credit) && toMinor(debit) > 0;
  const set = useCallback(
    (i: number, k: keyof LineRow, v: string) =>
      setLines((ls) => ls.map((l, j) => (j === i ? { ...l, [k]: v } : l))),
    [],
  );
  // Kolonlar yalnızca t ve set'e bağlı: her tuşta yeniden oluşursa hücreler
  // yeniden bağlanır ve girdi odağı kaybolur.
  const lineColumns = useMemo<ColumnDef<LineRow, unknown>[]>(
    () => [
      createColumn<LineRow>({
        id: "account",
        labelKey: "office.col_account",
        gridPrimary: true,
        cell: ({ row }) => (
          <Input
            className="h-8 w-36 font-mono"
            placeholder="770.01"
            aria-label={`${t("office.col_account")} ${row.index + 1}`}
            value={row.original.account}
            onChange={(e) => set(row.index, "account", e.target.value)}
          />
        ),
      }),
      createColumn<LineRow>({
        id: "description",
        labelKey: "office.journals.line_description",
        cell: ({ row }) => (
          <Input
            className="h-8"
            aria-label={`${t("office.journals.line_description")} ${row.index + 1}`}
            value={row.original.description}
            onChange={(e) => set(row.index, "description", e.target.value)}
          />
        ),
      }),
      createColumn<LineRow>({
        id: "debit",
        labelKey: "office.journals.debit",
        meta: { align: "end" },
        cell: ({ row }) => (
          <Input
            className="ms-auto h-8 w-32 text-right font-mono"
            inputMode="decimal"
            aria-label={`${t("office.journals.debit")} ${row.index + 1}`}
            value={row.original.debit}
            onChange={(e) => set(row.index, "debit", e.target.value)}
          />
        ),
      }),
      createColumn<LineRow>({
        id: "credit",
        labelKey: "office.journals.credit",
        meta: { align: "end" },
        cell: ({ row }) => (
          <Input
            className="ms-auto h-8 w-32 text-right font-mono"
            inputMode="decimal"
            aria-label={`${t("office.journals.credit")} ${row.index + 1}`}
            value={row.original.credit}
            onChange={(e) => set(row.index, "credit", e.target.value)}
          />
        ),
      }),
      // Silme düğmesi satır sırasına ve kalem sayısına bakar; bunları hücre
      // bağlamından okumak için yardımcının cell'i geçersiz kılınır.
      {
        ...createActionsColumn<LineRow>({
          labelKey: "office.journals.remove_line",
          cell: () => null,
        }),
        cell: ({ row, table }) => (
          <Button
            variant="ghost"
            size="sm"
            className="max-md:min-w-11"
            aria-label={t("office.journals.remove_line")}
            disabled={table.options.data.length <= 2}
            onClick={() =>
              setLines((ls) => ls.filter((_, j) => j !== row.index))
            }
          >
            <Trash2 className="size-4" />
          </Button>
        ),
      },
    ],
    [set, t],
  );

  const submit = () => {
    create.mutate(
      {
        entry_type: "mahsup",
        entry_date: `${date}T00:00:00Z`,
        description,
        lines: lines
          .filter((l) => l.account.trim())
          .map((l) => ({
            account: l.account.trim(),
            description: l.description,
            debit: l.debit || "0",
            credit: l.credit || "0",
          })),
      },
      {
        onSuccess: () => {
          setDescription("");
          setLines([emptyLine(), emptyLine()]);
        },
      },
    );
  };

  return (
    <Panel title={t("office.journals.new_title")}>
      <div className="space-y-3 p-4">
        <div className="flex flex-wrap gap-3">
          <label className="flex items-center gap-2 text-xs">
            <span className="text-muted-foreground">
              {t("office.col_date")}
            </span>
            <DatePicker
              className="h-8 w-40"
              value={date}
              onChange={(v) => setDate(v)}
            />
          </label>
          <Input
            className="h-8 max-w-md flex-1"
            placeholder={t("office.journals.description")}
            aria-label={t("office.journals.description")}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <DataTable<LineRow>
          columns={lineColumns}
          data={lines}
          getRowId={(_, i) => String(i)}
          features={lineEditorTable}
        />
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setLines((ls) => [...ls, emptyLine()])}
          >
            <Plus className="size-4" />
            {t("office.journals.add_line")}
          </Button>
          <div className="flex items-center gap-3 text-xs">
            <span
              className={cn(
                balanced ? "text-status-approved" : "text-status-warning",
              )}
            >
              {balanced
                ? t("office.journals.balanced")
                : t("office.journals.difference", {
                    amount: formatMoney(sub(debit, credit), "TRY"),
                  })}
            </span>
            <Figure>
              <Money value={debit} /> / <Money value={credit} />
            </Figure>
            <Button size="sm" onClick={submit} disabled={create.isPending}>
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              {t("office.journals.create")}
            </Button>
          </div>
        </div>
        {create.isError ? (
          <p className="text-status-blocking text-sm">
            {errorText(t, create.error, t("office.api.load_error"))}
          </p>
        ) : null}
        <p className="text-muted-foreground text-xs">
          {t("office.journals.new_hint")}
        </p>
      </div>
    </Panel>
  );
}

function ExportPanel({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const drivers = useExportDrivers(slug);
  const { export: run } = useJournalActions(slug, engagement);
  const items = drivers.data?.items ?? [];
  const [driver, setDriver] = useState("luca");
  const chosen = items.find((d) => d.name === driver);
  const blocked =
    run.isError && isApiError(run.error) && run.error.code === "EXPORT_BLOCKED"
      ? run.error.details
      : [];

  return (
    <Panel
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
            onChange={setDriver}
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
            disabled={!period || run.isPending || !chosen}
            onClick={() => run.mutate({ driver, period })}
          >
            {run.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <FileSpreadsheet className="size-4" />
            )}
            {t("office.journals.export")}
          </Button>
        </div>
        {!period ? (
          <p className="text-muted-foreground text-xs">
            {t("office.journals.pick_period")}
          </p>
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
        ) : run.isError ? (
          <p className="text-status-blocking text-sm">
            {errorText(t, run.error, t("office.api.load_error"))}
          </p>
        ) : null}
        {run.data ? (
          <div className="text-sm">
            <p className="text-status-approved">
              {t("office.journals.exported", { count: run.data.entries })}
            </p>
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
          {errorText(t, requestTransfer.error, t("office.api.load_error"))}
        </span>
      ) : null}
    </span>
  );
}

function JournalsInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  // Mükellef detayının Fişler sekmesinde mükellef sabit (#468).
  const scope = useEngagementScope();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug, scope);
  const engagement = scope ?? selected?.engagement ?? "";
  // Global dönem seçici (#326); "Tüm dönemler"de bu ay (yeni fişin dönemi).
  const { period: selectedPeriod, setPeriod: setGlobalPeriod } =
    useOfficePeriod();
  const period = selectedPeriod ?? thisMonth();
  const setPeriod = (value: string) =>
    setGlobalPeriod(isPeriod(value) ? value : ALL_PERIODS);
  const drafts = useJournalDrafts(slug, engagement, period);
  const exports = useJournalExports(slug, engagement);
  const transfers = useJournalTransfers(slug, engagement);
  // Newest transfer per export (the list is newest first).
  const latestTransfer = new Map<string, JournalTransfer>();
  for (const tr of transfers.data?.items ?? [])
    if (!latestTransfer.has(tr.batch_uuid))
      latestTransfer.set(tr.batch_uuid, tr);
  const { approve, reverse } = useJournalActions(slug, engagement);
  const fmt = (d: string) =>
    new Date(d).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");
  const approveDraft = approve.mutate;
  const approving = approve.isPending;
  const reverseDraft = reverse.mutate;
  const reversing = reverse.isPending;
  const draftColumns = useMemo<ColumnDef<JournalDraft, unknown>[]>(
    () => [
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
        id: "entry_type",
        labelKey: "office.col_type",
        accessorKey: "entry_type",
        meta: { cellClassName: "text-xs" },
      }),
      createMoneyColumn<JournalDraft>({
        id: "debit",
        labelKey: "office.journals.debit",
        accessor: (d) => d.debit,
      }),
      createMoneyColumn<JournalDraft>({
        id: "credit",
        labelKey: "office.journals.credit",
        accessor: (d) => d.credit,
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
      createActionsColumn<JournalDraft>({
        cell: (d) =>
          d.status === "draft" ? (
            <Button
              size="sm"
              variant="outline"
              disabled={approving}
              onClick={() => approveDraft(d.uuid)}
            >
              <Check className="size-4" />
              {t("office.journals.approve")}
            </Button>
          ) : d.status === "exported" ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={reversing}
              onClick={() => {
                const reason = window.prompt(
                  t("office.journals.reverse_prompt"),
                );
                if (reason?.trim()) reverseDraft({ draft: d.uuid, reason });
              }}
            >
              <Undo2 className="size-4" />
              {t("office.journals.reverse")}
            </Button>
          ) : null,
      }),
    ],
    [approveDraft, approving, reverseDraft, reversing, t],
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
  const actionError = approve.error ?? reverse.error;
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
        <label className="flex items-center gap-2 text-xs">
          <span className="text-muted-foreground">
            {t("office.api.col_period")}
          </span>
          <Input
            type="month"
            className="h-8 w-40"
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          />
        </label>
      </div>

      <NewDraft engagement={engagement} />

      <Panel title={t("office.journals.drafts_title")}>
        {actionError ? (
          <p className="text-status-blocking px-4 pt-3 text-sm">
            {errorText(t, actionError, t("office.api.load_error"))}
          </p>
        ) : null}
        {drafts.isLoading ? (
          <Skeleton className="m-4 h-32" />
        ) : rows.length === 0 ? (
          <EmptyState className="m-4" title={t("office.journals.empty")} />
        ) : (
          <DataTable<JournalDraft>
            columns={draftColumns}
            data={rows}
            getRowId={(d) => d.uuid}
            bordered={false}
            features={embeddedTable}
          />
        )}
      </Panel>

      <ExportPanel engagement={engagement} period={period} />

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
