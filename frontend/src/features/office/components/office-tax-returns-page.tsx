"use client";

import { FileCheck2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn, StatusPill } from "@/components/tables";
import type { StatusTone } from "@/components/tables";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrixAt } from "../hooks/use-office-data";
import { useOfficePeriod, usePeriodText } from "../period";
import type {
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";
import { FilterSelect, Figure, Money } from "./office-ui";

/** Ofis görünümünde izlenen beyanlar: aylık KDV1 ve MUHSGK. */
const KINDS = ["kdv1", "muhsgk"] as const;
type Kind = (typeof KINDS)[number];

type Filter = "all" | "open" | "documents" | "blocked";

type Row = PeriodMatrixRow & { cell?: PeriodMatrixCell };

const statusTone: Record<string, StatusTone> = {
  draft: "warn",
  prepared: "info",
  filed: "ok",
};

function returnOf(cell: PeriodMatrixCell | undefined, kind: Kind) {
  return cell?.returns.find((r) => r.kind === kind);
}

/** Verilmemiş beyanı, belge bekleyeni ve engeli olanı öne alır. */
function urgency(cell: PeriodMatrixCell | undefined) {
  let n = 0;
  for (const k of KINDS) if (returnOf(cell, k)?.status !== "filed") n += 4;
  if (cell?.open_blockers) n += 2;
  if (cell?.open_document_tasks || cell?.submitted_documents) n += 1;
  return n;
}

function ReturnCell({ cell, kind }: { cell?: PeriodMatrixCell; kind: Kind }) {
  const { t } = useLocale();
  const r = returnOf(cell, kind);
  if (!r)
    return (
      <StatusPill tone="neutral">
        {t("office.tax_returns.office.not_started")}
      </StatusPill>
    );
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <StatusPill tone={statusTone[r.status] ?? "neutral"}>
        {t(`office.tax_returns.status_${r.status}`)}
      </StatusPill>
      {r.payable_amount && r.status !== "draft" ? (
        <Money value={r.payable_amount} className="text-xs" />
      ) : null}
    </span>
  );
}

/**
 * `/t/{slug}/tax-returns` (#575): ofisin beyan dönemi tek tabloda. Her
 * mükellefin KDV1 ve MUHSGK durumu, ödenecek tutar, mükelleften beklenen
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
              return !!(c?.open_document_tasks || c?.submitted_documents);
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
      if (r.cell?.open_document_tasks || r.cell?.submitted_documents)
        out.documents++;
    }
    return out;
  }, [all]);

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
            <ReturnCell cell={row.original.cell} kind={kind} />
          ),
        }),
      ),
      createColumn<Row>({
        id: "documents",
        labelKey: "office.tax_returns.office.col_documents",
        enableSorting: false,
        cell: ({ row }) => {
          const c = row.original.cell;
          if (!c?.open_document_tasks && !c?.submitted_documents)
            return <span className="text-muted-foreground text-xs">—</span>;
          return (
            <span className="flex flex-col text-xs">
              {c.open_document_tasks ? (
                <span className="text-status-warning">
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
    [t],
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
