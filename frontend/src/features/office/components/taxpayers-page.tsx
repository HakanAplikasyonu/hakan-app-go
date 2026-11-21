"use client";

import { Building2, Check, Plus, UsersRound } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenantMembers } from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useOrgAssignments,
  usePeriodMatrix,
  useToggleAssignment,
} from "../hooks/use-office-data";
import type { EngagementAssignment } from "../services/engagements.service";
import type { PeriodMatrixRow } from "../services/periods.service";
import { cellHref, cellTarget } from "../lib/command";
import { useOfficePeriod } from "../period";
import { CellBadge } from "./command-center";
import { Figure } from "./office-ui";

type Member = { uuid: string; name: string; role: string };

/** Office members for the assignee picker (needs tenant.members.read). */
function useAssignableMembers(): Member[] {
  const { slug, permissions: perms } = useTenant();
  const canRead = perms.includes(permissions.tenant.members.read);
  const { data } = useTenantMembers(slug, canRead);
  return useMemo(
    () =>
      (data?.items ?? []).map((m) => ({
        uuid: m.user.uuid,
        name: `${m.user.name} ${m.user.surname}`.trim() || m.user.email,
        role: m.role.name,
      })),
    [data],
  );
}

/** Assignees of one engagement, with a picker for users who may change them. */
function Assignees({
  engagement,
  items,
  isLoading,
  members,
  canWrite,
}: {
  engagement: string;
  items: EngagementAssignment[];
  isLoading: boolean;
  members: Member[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const toggle = useToggleAssignment(engagement);
  const assigned = new Set(items.map((a) => a.user_uuid));

  const chips = isLoading ? (
    <Skeleton className="h-5 w-24" />
  ) : items.length ? (
    <div className="flex flex-wrap gap-1">
      {items.map((a) => (
        <span
          key={a.user_uuid}
          className="bg-secondary rounded-md px-1.5 py-0.5 text-xs"
        >
          {`${a.name} ${a.surname}`.trim() || a.email}
        </span>
      ))}
    </div>
  ) : (
    <span className="text-muted-foreground text-xs">
      {t("office.unassigned")}
    </span>
  );

  if (!canWrite || members.length === 0) return chips;
  return (
    <div className="flex items-center justify-between gap-2">
      {chips}
      <Popover>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm">
            <UsersRound className="size-3.5" />
            {t("office.assign")}
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-64 p-1" data-no-row-click>
          <p className="text-muted-foreground px-2 py-1.5 text-xs">
            {t("office.assign_hint")}
          </p>
          {members.map((m) => {
            const on = assigned.has(m.uuid);
            return (
              <Button
                key={m.uuid}
                variant="ghost"
                size="sm"
                disabled={toggle.isPending}
                onClick={() => toggle.mutate({ user: m.uuid, assigned: on })}
                className="h-auto w-full justify-start gap-2 px-2 py-1.5 text-left font-normal disabled:opacity-60"
                aria-pressed={on}
              >
                <span
                  className={cn(
                    "flex size-4 items-center justify-center rounded border",
                    on && "bg-primary text-primary-foreground border-primary",
                  )}
                >
                  {on ? <Check className="size-3" /> : null}
                </span>
                <span className="flex-1 truncate">{m.name}</span>
                <span className="text-muted-foreground text-xs">{m.role}</span>
              </Button>
            );
          })}
        </PopoverContent>
      </Popover>
    </div>
  );
}

type TaxpayerRow = PeriodMatrixRow;

function useTaxpayerColumns(members: Member[], canWrite: boolean) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const { setPeriod } = useOfficePeriod();
  // Atananlar tek istekte gelir; satır başına istek yok (#639).
  const assignments = useOrgAssignments();
  const byEngagement = assignments.data;
  const assignmentsLoading = assignments.isLoading;
  return useMemo<ColumnDef<TaxpayerRow, unknown>[]>(
    () => [
      createColumn<TaxpayerRow>({
        id: "taxpayer_title",
        labelKey: "office.col_taxpayer",
        accessorKey: "taxpayer_title",
        gridPrimary: true,
        sortingFn: (a, b) =>
          a.original.taxpayer_title.localeCompare(
            b.original.taxpayer_title,
            "tr",
          ),
        cell: ({ row }) => (
          <Link
            className="font-medium hover:underline"
            href={routes.tenant.engagement(slug, row.original.engagement_uuid)}
          >
            {row.original.taxpayer_title}
          </Link>
        ),
      }),
      createColumn<TaxpayerRow>({
        id: "taxpayer_tax_id",
        labelKey: "office.col_tax_id",
        accessorKey: "taxpayer_tax_id",
        gridSecondary: true,
        cell: ({ row }) => (
          <>
            <Figure>{row.original.taxpayer_tax_id}</Figure>
            <span className="text-muted-foreground ml-1.5 text-xs">
              {row.original.taxpayer_tax_id.length === 11 ? "TCKN" : "VKN"}
            </span>
          </>
        ),
      }),
      createColumn<TaxpayerRow>({
        id: "current_period",
        labelKey: "office.col_current_period",
        enableSorting: false,
        // Beyan dönemi: bir önceki ay.
        // Rozet o dönemin işine götürür (#710): Komuta Merkezi'yle aynı kural.
        cell: ({ row }) => {
          const cell = row.original.cells[0];
          if (!cell) return null;
          const target = cellTarget(cell);
          return (
            <Link
              href={cellHref(slug, row.original.engagement_uuid, target)}
              data-target={target}
              onClick={() => setPeriod(cell.period)}
              className="focus-visible:ring-ring inline-block rounded-md transition-opacity hover:opacity-85 focus-visible:ring-2 focus-visible:outline-none"
            >
              <CellBadge
                cell={cell}
                hint={t(`office.command.goes_${target}`)}
              />
            </Link>
          );
        },
      }),
      createColumn<TaxpayerRow>({
        id: "assignees",
        labelKey: "office.col_assignees",
        enableSorting: false,
        meta: { cellClassName: "min-w-56" },
        cell: ({ row }) => (
          <Assignees
            engagement={row.original.engagement_uuid}
            items={byEngagement?.get(row.original.engagement_uuid) ?? []}
            isLoading={assignmentsLoading}
            members={members}
            canWrite={canWrite}
          />
        ),
      }),
    ],
    [canWrite, members, slug, byEngagement, assignmentsLoading, t, setPeriod],
  );
}

/**
 * Office taxpayers (active engagements). Rows come from the period matrix
 * (issue #32), which also gives the current period's state; assignments are
 * read for the whole office in one request (#639) and changed per engagement.
 */
export function TaxpayersPage() {
  const { t } = useLocale();
  const { slug, permissions: perms } = useTenant();
  const router = useRouter();
  const matrix = usePeriodMatrix(2);
  const members = useAssignableMembers();
  const canWrite = perms.includes(permissions.tenant.engagements.write);
  const [query, setQuery] = useState("");
  const columns = useTaxpayerColumns(members, canWrite);

  // Arama Türkçe küçük harfle ve VKN/TCKN üzerinde; istemci tarafı liste.
  const rows = useMemo(() => {
    const q = query.trim().toLocaleLowerCase("tr");
    return (matrix.data?.items ?? [])
      .filter(
        (r: PeriodMatrixRow) =>
          !q ||
          r.taxpayer_title.toLocaleLowerCase("tr").includes(q) ||
          r.taxpayer_tax_id.includes(q),
      )
      .sort((a, b) => a.taxpayer_title.localeCompare(b.taxpayer_title, "tr"));
  }, [matrix.data, query]);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.taxpayers_title")}
        description={t("office.taxpayers_description")}
        icon={<Building2 className="size-6" />}
        actions={
          <Button asChild size="sm">
            <Link href={routes.tenant.taxpayerRequests(slug)}>
              <Plus className="size-4" />
              {t("office.taxpayers_add")}
            </Link>
          </Button>
        }
      />
      <Input
        className="h-8 w-64 max-w-full px-2 max-md:h-11"
        placeholder={t("office.command.search")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        aria-label={t("office.command.search")}
      />
      <EntityTable<TaxpayerRow>
        columns={columns}
        data={rows}
        getRowId={(r) => r.engagement_uuid}
        // Satırın tamamı mükellefe götürür (#567).
        onRowClick={(r) =>
          router.push(routes.tenant.engagement(slug, r.engagement_uuid))
        }
        isLoading={matrix.isLoading}
        isError={matrix.isError}
        errorTitle={t("office.load_failed")}
        onRetry={() => void matrix.refetch()}
        emptyTitle={t("office.taxpayers_empty")}
        emptyDescription=""
        manual={{ sorting: false, filtering: false, pagination: false }}
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        pageSizeOptions={[25, 50, 100]}
        // Arama yukarıdaki kutuda (Türkçe harf ve VKN); seçim/toplu işlem yok.
        features={{
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          rowSelection: false,
          viewMode: false,
          persistKey: "office.taxpayers.table",
        }}
      />
      <p className="text-muted-foreground text-xs">
        {t("office.taxpayers_footnote_api")}
      </p>
    </div>
  );
}
