"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Banknote,
  Download,
  FileSpreadsheet,
  FileText,
  Landmark,
  Link2,
  Upload,
  Users,
} from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { EntityTable } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useOfficePeriod, usePeriodText } from "../period";
import {
  payrollKeys,
  payrollService,
  type PayrollDocument,
  type PayrollEmployee,
  type PayrollImportResult,
  type PayrollLine,
  type PayrollRun,
  type SgkPremium,
} from "../services/payroll.service";
import { Money } from "./office-ui";

type View = "runs" | "employees" | "sgk" | "documents";
const VIEWS: View[] = ["runs", "employees", "sgk", "documents"];
const PAGE = 50;

const paymentVariants: Record<string, StatusVariant> = {
  paid: { labelKey: "office.payroll.pay_paid", tone: "ok" },
  partial: { labelKey: "office.payroll.pay_partial", tone: "warn" },
  unpaid: { labelKey: "office.payroll.pay_unpaid", tone: "neutral" },
};
const sgkVariants: Record<string, StatusVariant> = {
  paid: { labelKey: "office.payroll.sgk_paid", tone: "ok" },
  partial: { labelKey: "office.payroll.sgk_partial", tone: "warn" },
  accrued: { labelKey: "office.payroll.sgk_accrued", tone: "info" },
  overdue: { labelKey: "office.payroll.sgk_overdue", tone: "danger" },
};
const employeeVariants: Record<string, StatusVariant> = {
  active: { labelKey: "office.payroll.employee_active", tone: "ok" },
  exited: { labelKey: "office.payroll.employee_exited", tone: "neutral" },
};

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

function usePeriodLabel() {
  const { locale } = useLocale();
  return (p: string) =>
    new Date(`${p}-01T00:00:00`).toLocaleDateString(
      locale === "tr" ? "tr-TR" : "en-GB",
      { month: "long", year: "numeric" },
    );
}

/**
 * `/t/{slug}/engagements/{uuid}/payroll` (#382): mükellefin personeli, dönem
 * bordroları, SGK tahakkuk / ödeme durumu ve bordro PDF'leri.
 */
export function EngagementPayrollPage({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const [view, setView] = useState<View>("runs");
  const icons = {
    runs: FileSpreadsheet,
    employees: Users,
    sgk: Landmark,
    documents: FileText,
  };

  return (
    <div className="space-y-5">
      <div className="min-w-0">
        <h2 className="font-display truncate text-xl font-bold">
          {t("office.payroll.title")}
        </h2>
        <p className="text-text-2 text-sm">{t("office.payroll.description")}</p>
      </div>
      <div
        role="group"
        aria-label={t("office.payroll.views")}
        className="flex flex-wrap gap-2"
      >
        {VIEWS.map((v) => {
          const Icon = icons[v];
          return (
            <Button
              key={v}
              size="sm"
              variant={view === v ? "default" : "outline"}
              aria-pressed={view === v}
              onClick={() => setView(v)}
              className="min-h-11 sm:min-h-9"
            >
              <Icon aria-hidden className="size-4" />
              {t(`office.payroll.view_${v}`)}
            </Button>
          );
        })}
      </div>
      {view === "runs" ? <RunsView engagement={engagement} /> : null}
      {view === "employees" ? <EmployeesView engagement={engagement} /> : null}
      {view === "sgk" ? <SgkView engagement={engagement} /> : null}
      {view === "documents" ? <DocumentsView engagement={engagement} /> : null}
    </div>
  );
}

// ---- dönem bordroları ----

function RunsView({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantPayrollWrite);
  const qc = useQueryClient();
  const periodLabel = usePeriodLabel();
  const keyBase = payrollKeys.all(slug, engagement);
  const runs = useQuery({
    queryKey: [...keyBase, "runs"],
    queryFn: () => payrollService.runs(slug, engagement),
  });
  const items = runs.data?.items ?? [];
  const [picked, setPicked] = useState<string | null>(null);
  const selected =
    items.find((r) => r.uuid === picked) ??
    items.find((r) => r.period === focus) ??
    items[0];
  const detail = useQuery({
    queryKey: [...keyBase, "run", selected?.uuid],
    queryFn: () => payrollService.run(slug, engagement, selected!.uuid),
    enabled: !!selected,
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: keyBase });

  const journal = useMutation({
    mutationFn: (r: PayrollRun) =>
      payrollService.journalDraft(slug, engagement, r.uuid),
    onSuccess: (d) => {
      toast.success(t("office.payroll.journal_created", { no: d.entry_no }));
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.payroll.action_failed"))),
  });
  const match = useMutation({
    mutationFn: (r: PayrollRun) =>
      payrollService.bankMatch(slug, engagement, r.period),
    onSuccess: (m) => {
      toast.success(
        t("office.payroll.match_done", {
          salary: m.salary_matched,
          sgk: m.sgk_matched,
        }),
      );
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.payroll.action_failed"))),
  });

  const columns = useMemo<ColumnDef<PayrollRun, unknown>[]>(
    () => [
      createColumn<PayrollRun>({
        id: "period",
        labelKey: "office.payroll.col_period",
        accessorKey: "period",
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium whitespace-nowrap capitalize">
            {periodLabel(row.original.period)}
          </span>
        ),
      }),
      createColumn<PayrollRun>({
        id: "employees",
        labelKey: "office.payroll.col_employees",
        accessorFn: (r) => r.totals.employee_count,
        meta: { align: "end" },
      }),
      createMoneyColumn<PayrollRun>({
        id: "gross",
        labelKey: "office.payroll.col_gross",
        accessor: (r) => r.totals.gross_total,
      }),
      createMoneyColumn<PayrollRun>({
        id: "net",
        labelKey: "office.payroll.col_net",
        accessor: (r) => r.totals.net_total,
      }),
      createMoneyColumn<PayrollRun>({
        id: "cost",
        labelKey: "office.payroll.col_cost",
        accessor: (r) => r.totals.employer_cost_total,
      }),
      createStatusColumn<PayrollRun>({
        id: "payment",
        labelKey: "office.payroll.col_payment",
        accessor: (r) => r.payment_status,
        variants: paymentVariants,
        fallback: paymentVariants.unpaid,
      }),
      createStatusColumn<PayrollRun>({
        id: "sgk",
        labelKey: "office.payroll.col_sgk",
        accessor: (r) => r.sgk?.status ?? "accrued",
        variants: sgkVariants,
        fallback: sgkVariants.accrued,
      }),
      createColumn<PayrollRun>({
        id: "journal",
        labelKey: "office.payroll.col_journal",
        accessorFn: (r) => r.journal_draft_status ?? "",
        cell: ({ row }) => (
          <span className="text-text-2 text-sm whitespace-nowrap">
            {row.original.journal_draft_status
              ? t(`office.payroll.journal_${row.original.journal_draft_status}`)
              : "–"}
          </span>
        ),
      }),
    ],
    [t, periodLabel],
  );

  return (
    <div className="space-y-5">
      {canWrite ? (
        <ImportPanel engagement={engagement} onDone={refresh} />
      ) : null}
      {selected ? (
        <section
          aria-label={t("office.payroll.selected", {
            period: periodLabel(selected.period),
          })}
          className="space-y-3"
        >
          <div className="grid gap-3 sm:grid-cols-3">
            <Stat
              icon={Banknote}
              label={t("office.payroll.stat_net")}
              value={<Money value={selected.totals.net_total} />}
              hint={t(`office.payroll.pay_${selected.payment_status}`)}
            />
            <Stat
              icon={Landmark}
              label={t("office.payroll.stat_sgk")}
              value={<Money value={selected.sgk?.payable_amount ?? "0"} />}
              hint={
                selected.sgk
                  ? t(`office.payroll.sgk_${selected.sgk.status}`)
                  : "–"
              }
            />
            <Stat
              icon={Users}
              label={t("office.payroll.stat_cost")}
              value={<Money value={selected.totals.employer_cost_total} />}
              hint={t("office.payroll.employee_count", {
                count: selected.totals.employee_count,
              })}
            />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-display me-auto text-lg font-semibold capitalize">
              {periodLabel(selected.period)}
            </h2>
            <Button
              size="sm"
              variant="outline"
              onClick={() =>
                void payrollService
                  .exportRun(slug, engagement, selected)
                  .catch(() => toast.error(t("office.payroll.action_failed")))
              }
            >
              <Download aria-hidden className="size-4" />
              {t("office.payroll.export")}
            </Button>
            {canWrite ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={match.isPending}
                  onClick={() => match.mutate(selected)}
                >
                  <Link2 aria-hidden className="size-4" />
                  {t("office.payroll.bank_match")}
                </Button>
                <Button
                  size="sm"
                  disabled={!!selected.journal_draft_uuid || journal.isPending}
                  onClick={() => journal.mutate(selected)}
                >
                  {selected.journal_draft_uuid
                    ? t("office.payroll.journal_exists")
                    : t("office.payroll.journal_create")}
                </Button>
              </>
            ) : null}
          </div>
          {selected.issues.length > 0 ? (
            <ul className="border-warn/40 bg-warn-tint text-warn space-y-1 rounded-lg border p-3 text-sm">
              {selected.issues.slice(0, 5).map((is, i) => (
                <li key={`${is.row}-${is.code}-${i}`}>
                  {is.row > 0
                    ? t("office.payroll.issue_row", { row: is.row })
                    : null}
                  {is.message}
                </li>
              ))}
            </ul>
          ) : null}
          <LinesTable
            lines={detail.data?.lines ?? []}
            loading={detail.isLoading}
          />
        </section>
      ) : null}
      <ListExportButton
        onExport={() => payrollService.exportRuns(slug, engagement)}
      />
      <EntityTable<PayrollRun>
        columns={columns}
        data={items}
        getRowId={(r) => r.uuid}
        isLoading={runs.isLoading}
        isError={runs.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={runs.error?.message}
        onRetry={() => void runs.refetch()}
        emptyTitle={t("office.payroll.runs_empty_title")}
        emptyDescription={t("office.payroll.runs_empty")}
        onRowClick={(r) => setPicked(r.uuid)}
        features={{ ...tableFeatures, persistKey: "office.payroll.runs" }}
      />
    </div>
  );
}

/** #395: listenin üstünde "Excel'e aktar" (ekrandaki süzgeçle). */
function ListExportButton({ onExport }: { onExport: () => Promise<void> }) {
  const { t } = useLocale();
  return (
    <div className="flex justify-end">
      <Button
        size="sm"
        variant="outline"
        className="max-md:min-h-11"
        onClick={() =>
          void onExport().catch(() =>
            toast.error(t("office.payroll.export_failed")),
          )
        }
      >
        <Download aria-hidden className="size-4" />
        {t("office.payroll.export_list")}
      </Button>
    </div>
  );
}

function Stat({
  icon: Icon,
  label,
  value,
  hint,
}: {
  icon: typeof Banknote;
  label: string;
  value: React.ReactNode;
  hint: string;
}) {
  return (
    <div className="bg-surface border-line flex items-center gap-3.5 rounded-xl border p-4">
      <span
        aria-hidden
        className="bg-brand-tint text-brand-text flex size-10 shrink-0 items-center justify-center rounded-lg"
      >
        <Icon className="size-5" />
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="font-display truncate text-xl font-bold tabular-nums">
          {value}
        </span>
        <span className="text-text-2 text-sm">
          {label} · {hint}
        </span>
      </span>
    </div>
  );
}

function LinesTable({
  lines,
  loading,
}: {
  lines: PayrollLine[];
  loading: boolean;
}) {
  const { t } = useLocale();
  const columns = useMemo<ColumnDef<PayrollLine, unknown>[]>(
    () => [
      createColumn<PayrollLine>({
        id: "employee_name",
        labelKey: "office.payroll.col_employee",
        accessorKey: "employee_name",
        gridPrimary: true,
        cell: ({ row }) => (
          <>
            <div className="max-w-xs truncate">
              {row.original.employee_name}
            </div>
            {row.original.national_id_masked ? (
              <div className="text-text-3 font-mono text-xs">
                {row.original.national_id_masked}
              </div>
            ) : null}
          </>
        ),
      }),
      createColumn<PayrollLine>({
        id: "days",
        labelKey: "office.payroll.col_days",
        accessorFn: (l) => l.days_worked ?? "",
        meta: { align: "end" },
      }),
      createMoneyColumn<PayrollLine>({
        id: "gross",
        labelKey: "office.payroll.col_gross",
        accessor: (l) => l.gross,
      }),
      createMoneyColumn<PayrollLine>({
        id: "sgk",
        labelKey: "office.payroll.col_sgk_employee",
        accessor: (l) =>
          (Number(l.sgk_employee) + Number(l.unemployment_employee)).toFixed(2),
      }),
      createMoneyColumn<PayrollLine>({
        id: "tax",
        labelKey: "office.payroll.col_taxes",
        accessor: (l) =>
          (Number(l.income_tax) + Number(l.stamp_tax)).toFixed(2),
      }),
      createMoneyColumn<PayrollLine>({
        id: "net",
        labelKey: "office.payroll.col_net",
        accessor: (l) => l.net,
        className: "font-semibold",
      }),
      createMoneyColumn<PayrollLine>({
        id: "cost",
        labelKey: "office.payroll.col_cost",
        accessor: (l) => l.employer_cost,
      }),
    ],
    [],
  );
  return (
    <EntityTable<PayrollLine>
      columns={columns}
      data={lines}
      getRowId={(l) => String(l.row)}
      isLoading={loading}
      emptyTitle={t("office.payroll.lines_empty")}
      features={{ ...tableFeatures, persistKey: "office.payroll.lines" }}
    />
  );
}

// ---- içe aktarma ----

function ImportPanel({
  engagement,
  onDone,
}: {
  engagement: string;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [period, setPeriod] = useState("");
  const [preview, setPreview] = useState<PayrollImportResult | null>(null);
  const run = useMutation({
    mutationFn: (p: { preview: boolean }) =>
      payrollService.importFile(slug, engagement, file!, {
        preview: p.preview,
        period: period || undefined,
      }),
    onSuccess: (res, p) => {
      if (p.preview) {
        setPreview(res);
        if (!period && !res.period) setPeriod(focus);
        return;
      }
      toast.success(
        t("office.payroll.imported", {
          count: res.line_count,
          period: periodText(res.period),
        }),
      );
      setPreview(null);
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      onDone();
    },
    onError: (e) => toast.error(errText(e, t("office.payroll.import_failed"))),
  });
  const mapped = preview?.columns.filter((c) => c.field) ?? [];

  return (
    <section
      aria-labelledby="payroll-import"
      className="bg-surface border-line space-y-3 rounded-xl border p-4"
    >
      <div>
        <h2 id="payroll-import" className="font-semibold">
          {t("office.payroll.import_title")}
        </h2>
        <p className="text-text-2 text-sm">{t("office.payroll.import_hint")}</p>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <label className="flex min-w-0 flex-col gap-1 text-sm">
          <span>{t("office.payroll.import_file")}</span>
          <Input
            ref={fileRef}
            type="file"
            accept=".xlsx,.csv,.txt"
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null);
              setPreview(null);
            }}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>{t("office.payroll.import_period")}</span>
          <Input
            type="month"
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
            className="w-44"
          />
        </label>
        <Button
          variant="outline"
          disabled={!file || run.isPending}
          onClick={() => run.mutate({ preview: true })}
        >
          {t("office.payroll.import_preview")}
        </Button>
        <Button
          disabled={
            !file ||
            run.isPending ||
            !preview ||
            preview.missing.length > 0 ||
            !(period || preview.period)
          }
          onClick={() => run.mutate({ preview: false })}
        >
          <Upload aria-hidden className="size-4" />
          {t("office.payroll.import_commit")}
        </Button>
      </div>
      {preview ? (
        <div className="space-y-2 text-sm" aria-live="polite">
          <p>
            {t("office.payroll.preview_summary", {
              count: preview.line_count,
              source: t(`office.payroll.source_${preview.source_system}`),
              period: periodText(preview.period || period, "–"),
            })}
          </p>
          <p className="text-text-2">
            {t("office.payroll.preview_net")}{" "}
            <Money value={preview.totals.net_total} /> ·{" "}
            {t("office.payroll.preview_cost")}{" "}
            <Money value={preview.totals.employer_cost_total} />
          </p>
          {preview.missing.length > 0 ? (
            <p className="text-danger font-medium">
              {t("office.payroll.preview_missing", {
                fields: preview.missing
                  .map((f) => t(`office.payroll.field_${f}`))
                  .join(", "),
              })}
            </p>
          ) : null}
          <ul className="flex flex-wrap gap-1.5">
            {mapped.map((c) => (
              <li
                key={c.column}
                className={cn(
                  "rounded-md px-2 py-0.5 text-xs",
                  c.needs_review
                    ? "bg-warn-tint text-warn"
                    : "bg-surface-3 text-text-2",
                )}
              >
                {c.header} → {t(`office.payroll.field_${c.field}`)}
              </li>
            ))}
          </ul>
          {preview.issues.length > 0 ? (
            <p className="text-warn">
              {t("office.payroll.preview_issues", {
                count: preview.issues.length,
              })}
            </p>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

// ---- personel ----

function EmployeesView({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [status, setStatus] = useState<"active" | "exited" | "all">("active");
  const [offset, setOffset] = useState(0);
  const list = useQuery({
    queryKey: [
      ...payrollKeys.all(slug, engagement),
      "employees",
      status,
      offset,
    ],
    queryFn: () =>
      payrollService.employees(slug, engagement, {
        status,
        limit: PAGE,
        offset,
      }),
    placeholderData: keepPreviousData,
  });
  const columns = useMemo<ColumnDef<PayrollEmployee, unknown>[]>(
    () => [
      createColumn<PayrollEmployee>({
        id: "full_name",
        labelKey: "office.payroll.col_employee",
        accessorKey: "full_name",
        gridPrimary: true,
      }),
      createColumn<PayrollEmployee>({
        id: "national_id",
        labelKey: "office.payroll.col_national_id",
        accessorFn: (e) => e.national_id_masked ?? "–",
        meta: { cellClassName: "font-mono text-sm" },
      }),
      createColumn<PayrollEmployee>({
        id: "sgk_no",
        labelKey: "office.payroll.col_sgk_no",
        accessorFn: (e) => e.sgk_no ?? "–",
      }),
      createColumn<PayrollEmployee>({
        id: "job_title",
        labelKey: "office.payroll.col_job",
        accessorFn: (e) => e.job_title ?? "–",
      }),
      createDateColumn<PayrollEmployee>({
        id: "hire_date",
        labelKey: "office.payroll.col_hire",
        accessor: (e) => e.hire_date,
      }),
      createDateColumn<PayrollEmployee>({
        id: "exit_date",
        labelKey: "office.payroll.col_exit",
        accessor: (e) => e.exit_date,
      }),
      createStatusColumn<PayrollEmployee>({
        id: "status",
        labelKey: "office.payroll.col_status",
        accessor: (e) => e.status,
        variants: employeeVariants,
        fallback: employeeVariants.active,
      }),
    ],
    [],
  );
  const data = list.data;
  return (
    <div className="space-y-3">
      <div
        role="group"
        aria-label={t("office.payroll.col_status")}
        className="flex flex-wrap gap-2"
      >
        {(["active", "exited", "all"] as const).map((s) => (
          <Button
            key={s}
            size="sm"
            variant={status === s ? "secondary" : "ghost"}
            aria-pressed={status === s}
            onClick={() => {
              setStatus(s);
              setOffset(0);
            }}
          >
            {t(`office.payroll.filter_${s}`)}
            {data && s !== "all" ? (
              <span className="text-text-3 tabular-nums">
                {s === "active" ? data.active : data.exited}
              </span>
            ) : null}
          </Button>
        ))}
      </div>
      <ListExportButton
        onExport={() =>
          payrollService.exportEmployees(slug, engagement, {
            status: status === "all" ? undefined : status,
          })
        }
      />
      <EntityTable<PayrollEmployee>
        columns={columns}
        data={data?.items ?? []}
        getRowId={(e) => e.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.payroll.employees_empty_title")}
        emptyDescription={t("office.payroll.employees_empty")}
        features={{ ...tableFeatures, persistKey: "office.payroll.employees" }}
      />
      {data && data.total > PAGE ? (
        <div className="flex items-center justify-end gap-2 text-xs">
          <span className="text-text-3">
            {t("office.api.page_of", {
              from: offset + 1,
              to: offset + data.items.length,
              total: data.total,
            })}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE))}
          >
            {t("office.api.prev")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={offset + PAGE >= data.total}
            onClick={() => setOffset(offset + PAGE)}
          >
            {t("office.api.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

// ---- SGK ----

function SgkView({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantPayrollWrite);
  const qc = useQueryClient();
  const periodLabel = usePeriodLabel();
  const keyBase = payrollKeys.all(slug, engagement);
  const list = useQuery({
    queryKey: [...keyBase, "sgk"],
    queryFn: () => payrollService.premiums(slug, engagement),
  });
  const pay = useMutation({
    mutationFn: (p: SgkPremium) =>
      payrollService.markPremiumPaid(slug, engagement, p),
    onSuccess: () => {
      toast.success(t("office.payroll.sgk_marked"));
      void qc.invalidateQueries({ queryKey: keyBase });
    },
    onError: (e) => toast.error(errText(e, t("office.payroll.action_failed"))),
  });
  const columns = useMemo<ColumnDef<SgkPremium, unknown>[]>(
    () => [
      createColumn<SgkPremium>({
        id: "period",
        labelKey: "office.payroll.col_period",
        accessorKey: "period",
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap capitalize">
            {periodLabel(row.original.period)}
          </span>
        ),
      }),
      createMoneyColumn<SgkPremium>({
        id: "accrual",
        labelKey: "office.payroll.col_accrual",
        accessor: (p) => p.accrual_amount,
      }),
      createMoneyColumn<SgkPremium>({
        id: "incentive",
        labelKey: "office.payroll.col_incentive",
        accessor: (p) => p.incentive_amount,
      }),
      createMoneyColumn<SgkPremium>({
        id: "payable",
        labelKey: "office.payroll.col_payable",
        accessor: (p) => p.payable_amount,
        className: "font-semibold",
      }),
      createDateColumn<SgkPremium>({
        id: "due",
        labelKey: "office.payroll.col_due",
        accessor: (p) => p.due_date,
      }),
      createMoneyColumn<SgkPremium>({
        id: "paid",
        labelKey: "office.payroll.col_paid",
        accessor: (p) => p.paid_amount,
      }),
      createStatusColumn<SgkPremium>({
        id: "status",
        labelKey: "office.payroll.col_status",
        accessor: (p) => p.status,
        variants: sgkVariants,
        fallback: sgkVariants.accrued,
      }),
      createColumn<SgkPremium>({
        id: "action",
        labelKey: "common.actions",
        accessorFn: () => "",
        cell: ({ row }) =>
          canWrite && row.original.status !== "paid" ? (
            <Button
              size="sm"
              variant="outline"
              disabled={pay.isPending}
              onClick={() => pay.mutate(row.original)}
            >
              {t("office.payroll.sgk_mark_paid")}
            </Button>
          ) : null,
      }),
    ],
    [t, periodLabel, canWrite, pay],
  );
  return (
    <div className="space-y-3">
      <ListExportButton
        onExport={() => payrollService.exportPremiums(slug, engagement)}
      />
      <EntityTable<SgkPremium>
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(p) => p.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.payroll.sgk_empty_title")}
        emptyDescription={t("office.payroll.sgk_empty")}
        features={{ ...tableFeatures, persistKey: "office.payroll.sgk" }}
      />
    </div>
  );
}

// ---- belgeler ----

function DocumentsView({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { period, focus } = useOfficePeriod();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantPayrollWrite);
  const qc = useQueryClient();
  const keyBase = payrollKeys.all(slug, engagement);
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [docPeriod, setDocPeriod] = useState("");
  const [visible, setVisible] = useState(true);
  const list = useQuery({
    queryKey: [...keyBase, "documents", period],
    queryFn: () =>
      payrollService.documents(slug, engagement, {
        period,
        limit: 200,
        offset: 0,
      }),
  });
  const up = useMutation({
    mutationFn: () =>
      payrollService.uploadDocument(slug, engagement, file!, {
        period: docPeriod || focus,
        visible,
      }),
    onSuccess: () => {
      toast.success(t("office.payroll.document_uploaded"));
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      void qc.invalidateQueries({ queryKey: keyBase });
    },
    onError: (e) => toast.error(errText(e, t("office.payroll.action_failed"))),
  });
  const del = useMutation({
    mutationFn: (d: PayrollDocument) =>
      payrollService.deleteDocument(slug, engagement, d),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keyBase }),
  });
  const columns = useMemo<ColumnDef<PayrollDocument, unknown>[]>(
    () => [
      createColumn<PayrollDocument>({
        id: "title",
        labelKey: "office.payroll.col_document",
        accessorKey: "title",
        gridPrimary: true,
        cell: ({ row }) => (
          <>
            <div className="max-w-sm truncate">{row.original.title}</div>
            <div className="text-text-3 text-xs">
              {row.original.period} ·{" "}
              {t(`office.payroll.kind_${row.original.kind}`)}
            </div>
          </>
        ),
      }),
      createColumn<PayrollDocument>({
        id: "visible",
        labelKey: "office.payroll.col_visible",
        accessorFn: (d) => (d.visible_to_taxpayer ? 1 : 0),
        cell: ({ row }) =>
          row.original.visible_to_taxpayer
            ? t("office.payroll.visible_yes")
            : t("office.payroll.visible_no"),
      }),
      createDateColumn<PayrollDocument>({
        id: "created_at",
        labelKey: "office.col_date",
        accessor: (d) => d.created_at,
      }),
      createColumn<PayrollDocument>({
        id: "actions",
        labelKey: "common.actions",
        accessorFn: () => "",
        cell: ({ row }) => (
          <div className="flex gap-1">
            <Button
              size="sm"
              variant="ghost"
              aria-label={t("office.payroll.download")}
              onClick={() =>
                void payrollService
                  .downloadDocument(slug, engagement, row.original)
                  .catch(() => toast.error(t("office.payroll.action_failed")))
              }
            >
              <Download aria-hidden className="size-4" />
            </Button>
            {canWrite ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={del.isPending}
                onClick={() => del.mutate(row.original)}
              >
                {t("office.payroll.delete")}
              </Button>
            ) : null}
          </div>
        ),
      }),
    ],
    [t, slug, engagement, canWrite, del],
  );
  return (
    <div className="space-y-3">
      {canWrite ? (
        <section className="bg-surface border-line flex flex-wrap items-end gap-2 rounded-xl border p-4">
          <label className="flex min-w-0 flex-col gap-1 text-sm">
            <span>{t("office.payroll.document_file")}</span>
            <Input
              ref={fileRef}
              type="file"
              accept="application/pdf,.pdf"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span>{t("office.payroll.import_period")}</span>
            <Input
              type="month"
              value={docPeriod || focus}
              onChange={(e) => setDocPeriod(e.target.value)}
              className="w-44"
            />
          </label>
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={visible}
              onChange={(e) => setVisible(e.target.checked)}
              className="size-4"
            />
            {t("office.payroll.document_visible")}
          </label>
          <Button disabled={!file || up.isPending} onClick={() => up.mutate()}>
            <Upload aria-hidden className="size-4" />
            {t("office.payroll.document_upload")}
          </Button>
        </section>
      ) : null}
      <EntityTable<PayrollDocument>
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(d) => d.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.payroll.documents_empty_title")}
        emptyDescription={t("office.payroll.documents_empty")}
        features={{ ...tableFeatures, persistKey: "office.payroll.documents" }}
      />
    </div>
  );
}
