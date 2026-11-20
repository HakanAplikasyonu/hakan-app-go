"use client";

import { useMutation } from "@tanstack/react-query";
import { Download, FileCheck2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn, StatusPill } from "@/components/tables";
import type { StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodMatrixAt } from "../hooks/use-office-data";
import { createDocumentRequest } from "../services/engagement-documents.service";
import { useOfficePeriod, usePeriodText } from "../period";
import type {
  PeriodMatrix,
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";
import { FilterSelect, Figure, Money } from "./office-ui";

/** Ofis görünümünde izlenen aylık beyanlar: KDV1, MUHSGK ve Ba/Bs. */
const KINDS = ["kdv1", "muhsgk", "babs"] as const;
type Kind = (typeof KINDS)[number];

type Filter = "all" | "open" | "documents" | "blocked";

type Row = PeriodMatrixRow & { cell?: PeriodMatrixCell };
type Deadline = PeriodMatrix["deadlines"][number];

/** Son güne kalan gün (bugün 0, geçmişse eksi), tarayıcının gününe göre. */
export function daysLeft(due: string, today = new Date()): number {
  const [y, m, d] = due.split("-").map(Number);
  const a = Date.UTC(y, m - 1, d);
  const b = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  return Math.round((a - b) / 86_400_000);
}

const statusTone: Record<string, StatusTone> = {
  draft: "warn",
  prepared: "info",
  filed: "ok",
};

function returnOf(cell: PeriodMatrixCell | undefined, kind: Kind) {
  return cell?.returns.find((r) => r.kind === kind);
}

/**
 * Mükelleften bir şey bekleniyor: istenen ya da gelip işlenmeyen belge,
 * yüklenmemiş banka ekstresi (#568).
 */
function waitsForDocuments(c: PeriodMatrixCell) {
  return !!(
    c.open_document_tasks ||
    c.submitted_documents ||
    c.missing_bank_statements.length
  );
}

/**
 * Ofis beyan tablosu Excel'i (#669 H): ekrandaki (süzülmüş, sıralı)
 * satırlar; her beyan için durum ve ödenecek tutar (sayı), belgeler ve
 * engeller. İlk satır başlıklar.
 */
export function taxReturnsSheet(
  rows: Row[],
  t: (key: string, vars?: Record<string, string | number>) => string,
  late: Kind[],
): (string | number)[][] {
  const head: string[] = [
    t("office.col_taxpayer"),
    t("office.tax_returns.office.col_tax_id"),
  ];
  for (const k of KINDS) {
    const kind = t(`office.tax_returns.kind_${k}`);
    head.push(kind, `${kind} · ${t("office.tax_returns.office.col_payable")}`);
  }
  head.push(
    t("office.tax_returns.office.col_documents"),
    t("office.tax_returns.office.col_blockers"),
  );
  const out: (string | number)[][] = [head];
  for (const r of rows) {
    const c = r.cell;
    const line: (string | number)[] = [r.taxpayer_title, r.taxpayer_tax_id];
    for (const k of KINDS) {
      const ret = returnOf(c, k);
      let status = ret
        ? t(`office.tax_returns.status_${ret.status}`)
        : t("office.tax_returns.office.not_started");
      if (late.includes(k) && ret?.status !== "filed")
        status = `${status} (${t("office.tax_returns.office.late")})`;
      const payable =
        ret?.payable_amount && ret.status !== "draft"
          ? Number(ret.payable_amount)
          : "";
      line.push(status, payable);
    }
    const docs: string[] = [];
    if (c) {
      for (const name of c.missing_bank_statements)
        docs.push(t("office.tax_returns.office.statement_missing", { name }));
      if (c.open_document_tasks)
        docs.push(
          t("office.tax_returns.office.documents_open", {
            count: c.open_document_tasks,
          }),
        );
      if (c.submitted_documents)
        docs.push(
          t("office.tax_returns.office.documents_submitted", {
            count: c.submitted_documents,
          }),
        );
    }
    line.push(docs.join("; "), c?.open_blockers ?? 0);
    out.push(line);
  }
  return out;
}

async function downloadSheet(data: (string | number)[][], fileName: string) {
  const XLSX = await import("xlsx");
  const ws = XLSX.utils.aoa_to_sheet(data);
  ws["!cols"] = data[0].map((_, i) => ({ wch: i === 0 ? 36 : 16 }));
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, "Beyannameler");
  XLSX.writeFile(wb, fileName);
}

/** Verilmemiş beyanı, belge bekleyeni ve engeli olanı öne alır. */
function urgency(cell: PeriodMatrixCell | undefined) {
  let n = 0;
  for (const k of KINDS) if (returnOf(cell, k)?.status !== "filed") n += 4;
  if (cell?.open_blockers) n += 2;
  if (cell && waitsForDocuments(cell)) n += 1;
  return n;
}

function ReturnCell({
  cell,
  kind,
  late,
}: {
  cell?: PeriodMatrixCell;
  kind: Kind;
  /** Son gün geçti: verilmemiş beyan "gecikti" görünür. */
  late: boolean;
}) {
  const { t } = useLocale();
  const r = returnOf(cell, kind);
  if (!r)
    return (
      <StatusPill tone={late ? "danger" : "neutral"}>
        {t(
          late
            ? "office.tax_returns.office.late"
            : "office.tax_returns.office.not_started",
        )}
      </StatusPill>
    );
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <StatusPill
        tone={
          late && r.status !== "filed"
            ? "danger"
            : (statusTone[r.status] ?? "neutral")
        }
      >
        {t(`office.tax_returns.status_${r.status}`)}
      </StatusPill>
      {late && r.status !== "filed" ? (
        <span className="text-text-2 text-xs font-medium">
          {t("office.tax_returns.office.late")}
        </span>
      ) : null}
      {r.payable_amount && r.status !== "draft" ? (
        <Money value={r.payable_amount} className="text-xs" />
      ) : null}
    </span>
  );
}

/**
 * Eksik ekstreyi mükelleften ister (#568): mükellefin görev listesine
 * "Ekim 2026 Ziraat TL vadesiz banka ekstresi" düşer, hatırlatma gider.
 */
function StatementRequest({
  engagement,
  period,
  account,
}: {
  engagement: string;
  period: string;
  account: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const periodText = usePeriodText();
  const request = useMutation({
    mutationFn: () =>
      createDocumentRequest(slug, engagement, {
        title: t("office.tax_returns.office.statement_request_title", {
          period: periodText(period),
          account,
        }),
        note: t("office.tax_returns.office.statement_request_note"),
        due_days: 3,
      }),
    onSuccess: () =>
      toast.success(t("office.tax_returns.office.statement_requested")),
    onError: () => toast.error(t("office.api.load_error")),
  });
  if (!can(permissions.tenant.docTasks.write)) return null;
  return (
    <Button
      size="sm"
      variant="link"
      className="h-auto p-0 text-xs"
      data-no-row-click
      disabled={request.isPending || request.isSuccess}
      onClick={(e) => {
        e.stopPropagation();
        request.mutate();
      }}
    >
      {request.isSuccess
        ? t("office.tax_returns.office.statement_request_sent")
        : t("office.tax_returns.office.statement_request")}
    </Button>
  );
}

/** Dönemin beyan son günleri; yaklaşan uyarı, geçen kırmızı görünür. */
function DeadlineStrip({
  deadlines,
}: {
  deadlines: Partial<Record<Kind, Deadline>>;
}) {
  const { t, locale } = useLocale();
  const items = KINDS.flatMap((k) => {
    const d = deadlines[k];
    return d ? [{ kind: k, d, left: daysLeft(d.due_date) }] : [];
  });
  if (!items.length) return null;
  return (
    <ul
      className="flex flex-wrap gap-x-5 gap-y-1 text-xs"
      aria-label={t("office.tax_returns.office.deadlines")}
    >
      {items.map(({ kind, d, left }) => (
        <li key={kind} className="flex items-baseline gap-1.5">
          <span className="font-medium">
            {t(`office.tax_returns.kind_${kind}`)}
          </span>
          <span className="text-muted-foreground">
            {t("office.tax_returns.office.due", {
              date: new Date(`${d.due_date}T00:00:00`).toLocaleDateString(
                locale,
                { day: "numeric", month: "long", weekday: "short" },
              ),
            })}
          </span>
          <span
            className={cn(
              left < 0
                ? "text-status-blocking"
                : left <= 3
                  ? "text-status-warning"
                  : "text-muted-foreground",
            )}
          >
            {left < 0
              ? t("office.tax_returns.office.due_passed", { count: -left })
              : left === 0
                ? t("office.tax_returns.office.due_today")
                : t("office.tax_returns.office.due_left", { count: left })}
          </span>
          {d.rolled_forward ? (
            <span className="text-muted-foreground">
              {t("office.tax_returns.office.due_rolled")}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

/**
 * `/t/{slug}/tax-returns` (#575): ofisin beyan dönemi tek tabloda. Her
 * mükellefin KDV1, MUHSGK ve Ba/Bs durumu ve son günler, ödenecek tutar, mükelleften beklenen
 * belgeler ve açık engeller; satır mükellefin beyan ekranını açar. Dönem
 * üst çubuktaki ofis dönemidir.
 */
export function OfficeTaxReturnsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const periodText = usePeriodText();
  const { focus: period } = useOfficePeriod();
  const matrix = usePeriodMatrixAt(period);
  const [filter, setFilter] = useState<Filter>("all");

  const all = useMemo<Row[]>(
    () =>
      (matrix.data?.items ?? []).map((r) => ({
        ...r,
        cell: r.cells.find((c) => c.period === period),
      })),
    [matrix.data, period],
  );
  const rows = useMemo(
    () =>
      all
        .filter((r) => {
          const c = r.cell;
          switch (filter) {
            case "open":
              return KINDS.some((k) => returnOf(c, k)?.status !== "filed");
            case "documents":
              return !!c && waitsForDocuments(c);
            case "blocked":
              return !!c?.open_blockers;
            default:
              return true;
          }
        })
        .sort(
          (a, b) =>
            urgency(b.cell) - urgency(a.cell) ||
            a.taxpayer_title.localeCompare(b.taxpayer_title, "tr"),
        ),
    [all, filter],
  );
  const counts = useMemo(() => {
    const out = { filed: 0, prepared: 0, open: 0, documents: 0 };
    for (const r of all) {
      const k = returnOf(r.cell, "kdv1")?.status;
      if (k === "filed") out.filed++;
      else if (k === "prepared") out.prepared++;
      else out.open++;
      if (r.cell && waitsForDocuments(r.cell)) out.documents++;
    }
    return out;
  }, [all]);

  // Bu dönemin beyan son günleri (sunucudaki kural: hafta sonu → Pazartesi).
  const deadlines = useMemo(() => {
    const out: Partial<Record<Kind, Deadline>> = {};
    for (const d of matrix.data?.deadlines ?? []) {
      if (d.period === period) out[d.kind as Kind] = d;
    }
    return out;
  }, [matrix.data, period]);
  const lateKey = KINDS.map((k) => {
    const d = deadlines[k];
    return d && daysLeft(d.due_date) < 0 ? k : "";
  }).join(",");

  const columns = useMemo<ColumnDef<Row, unknown>[]>(
    () => [
      createColumn<Row>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        gridPrimary: true,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span className="font-medium">{row.original.taxpayer_title}</span>
            <Figure className="text-muted-foreground text-xs">
              {row.original.taxpayer_tax_id}
            </Figure>
          </span>
        ),
      }),
      ...KINDS.map((kind) =>
        createColumn<Row>({
          id: kind,
          labelKey: `office.tax_returns.kind_${kind}`,
          enableSorting: false,
          cell: ({ row }) => (
            <ReturnCell
              cell={row.original.cell}
              kind={kind}
              late={lateKey.split(",").includes(kind)}
            />
          ),
        }),
      ),
      createColumn<Row>({
        id: "documents",
        labelKey: "office.tax_returns.office.col_documents",
        enableSorting: false,
        cell: ({ row }) => {
          const c = row.original.cell;
          if (!c || !waitsForDocuments(c))
            return <span className="text-muted-foreground text-xs">—</span>;
          return (
            <span className="flex flex-col text-xs">
              {c.missing_bank_statements.map((name) => (
                <span
                  key={name}
                  className="text-text-2 flex flex-wrap items-center gap-x-2"
                >
                  <span
                    className="bg-warn size-1.5 shrink-0 rounded-full"
                    aria-hidden
                  />
                  {t("office.tax_returns.office.statement_missing", { name })}
                  <StatementRequest
                    engagement={row.original.engagement_uuid}
                    period={c.period}
                    account={name}
                  />
                </span>
              ))}
              {c.open_document_tasks ? (
                <span className="text-text-2 flex items-center gap-x-2">
                  <span
                    className="bg-warn size-1.5 shrink-0 rounded-full"
                    aria-hidden
                  />
                  {t("office.tax_returns.office.documents_open", {
                    count: c.open_document_tasks,
                  })}
                </span>
              ) : null}
              {c.submitted_documents ? (
                <span>
                  {t("office.tax_returns.office.documents_submitted", {
                    count: c.submitted_documents,
                  })}
                </span>
              ) : null}
            </span>
          );
        },
      }),
      createColumn<Row>({
        id: "blockers",
        labelKey: "office.tax_returns.office.col_blockers",
        enableSorting: false,
        cell: ({ row }) => {
          const n = row.original.cell?.open_blockers ?? 0;
          return n ? (
            <StatusPill tone="danger">
              {t("office.tax_returns.office.blockers", { count: n })}
            </StatusPill>
          ) : (
            <span className="text-muted-foreground text-xs">—</span>
          );
        },
      }),
    ],
    [t, lateKey],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.tax_returns.title")}
        description={t("office.tax_returns.office.description", {
          period: periodText(period),
        })}
        icon={<FileCheck2 className="size-6" />}
      />
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <FilterSelect<Filter>
          label={t("office.tax_returns.office.filter")}
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: t("office.tax_returns.office.filter_all") },
            {
              value: "open",
              label: t("office.tax_returns.office.filter_open"),
            },
            {
              value: "documents",
              label: t("office.tax_returns.office.filter_documents"),
            },
            {
              value: "blocked",
              label: t("office.tax_returns.office.filter_blocked"),
            },
          ]}
        />
        <Button
          size="sm"
          variant="outline"
          disabled={!matrix.data || rows.length === 0}
          onClick={() =>
            void downloadSheet(
              taxReturnsSheet(
                rows,
                t,
                KINDS.filter((k) => lateKey.split(",").includes(k)),
              ),
              `beyannameler_${period}.xlsx`,
            ).catch(() => toast.error(t("office.tax_returns.export_failed")))
          }
        >
          <Download className="size-4" aria-hidden />
          {t("office.tax_returns.export")}
        </Button>
        {matrix.data ? (
          <span className="text-muted-foreground text-xs">
            {t("office.tax_returns.office.summary", {
              total: all.length,
              filed: counts.filed,
              prepared: counts.prepared,
              open: counts.open,
              documents: counts.documents,
            })}
          </span>
        ) : null}
      </div>
      <DeadlineStrip deadlines={deadlines} />
      <EntityTable<Row>
        columns={columns}
        data={rows}
        getRowId={(r) => r.engagement_uuid}
        onRowClick={(r) =>
          router.push(
            `${routes.tenant.engagementTaxReturns(slug, r.engagement_uuid)}?kind=kdv1`,
          )
        }
        isLoading={matrix.isLoading}
        isError={matrix.isError}
        errorTitle={t("office.load_failed")}
        onRetry={() => void matrix.refetch()}
        emptyTitle={t("office.tax_returns.office.empty")}
        emptyDescription=""
        manual={{ sorting: false, filtering: false, pagination: false }}
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        pageSizeOptions={[25, 50, 100]}
        features={{
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          rowSelection: false,
          viewMode: false,
          persistKey: "office.tax_returns.table",
        }}
      />
    </div>
  );
}
