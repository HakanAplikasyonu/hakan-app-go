"use client";

import { Building2, Check, Plus, UsersRound } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
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
  useEngagementAssignments,
  usePeriodMatrix,
  useToggleAssignment,
} from "../hooks/use-office-data";
import type { PeriodMatrixRow } from "../services/periods.service";
import { CellBadge } from "./command-center";
import { Figure, Panel, TableBox } from "./office-ui";

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
  members,
  canWrite,
}: {
  engagement: string;
  members: Member[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { data, isLoading } = useEngagementAssignments(engagement);
  const toggle = useToggleAssignment(engagement);
  const assigned = new Set((data?.items ?? []).map((a) => a.user_uuid));

  const chips = isLoading ? (
    <Skeleton className="h-5 w-24" />
  ) : data?.items.length ? (
    <div className="flex flex-wrap gap-1">
      {data.items.map((a) => (
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
        <PopoverContent align="end" className="w-64 p-1">
          <p className="text-muted-foreground px-2 py-1.5 text-xs">
            {t("office.assign_hint")}
          </p>
          {members.map((m) => {
            const on = assigned.has(m.uuid);
            return (
              <button
                key={m.uuid}
                type="button"
                disabled={toggle.isPending}
                onClick={() => toggle.mutate({ user: m.uuid, assigned: on })}
                className="hover:bg-accent flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm disabled:opacity-60"
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
              </button>
            );
          })}
        </PopoverContent>
      </Popover>
    </div>
  );
}

/**
 * Office taxpayers (active engagements). Rows come from the period matrix
 * (issue #32), which also gives the current period's state; assignments are
 * read and changed per engagement.
 */
export function TaxpayersPage() {
  const { t } = useLocale();
  const { slug, permissions: perms } = useTenant();
  const matrix = usePeriodMatrix(2);
  const members = useAssignableMembers();
  const canWrite = perms.includes(permissions.tenant.engagements.write);
  const [query, setQuery] = useState("");

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
      <input
        className="bg-background h-8 w-64 rounded-md border px-2 text-sm"
        placeholder={t("office.command.search")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        aria-label={t("office.command.search")}
      />
      <Panel>
        {matrix.isLoading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : matrix.isError ? (
          <p className="text-destructive p-4 text-sm">
            {t("office.load_failed")}
          </p>
        ) : rows.length === 0 ? (
          <p className="text-muted-foreground p-6 text-center text-sm">
            {t("office.taxpayers_empty")}
          </p>
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.col_taxpayer")}</th>
                <th>{t("office.col_tax_id")}</th>
                <th>{t("office.col_current_period")}</th>
                <th>{t("office.col_assignees")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                // Beyan dönemi: bir önceki ay.
                const cell = r.cells[0];
                return (
                  <tr key={r.engagement_uuid}>
                    <td>
                      <Link
                        className="font-medium hover:underline"
                        href={routes.tenant.engagementDocuments(
                          slug,
                          r.engagement_uuid,
                        )}
                      >
                        {r.taxpayer_title}
                      </Link>
                    </td>
                    <td>
                      <Figure>{r.taxpayer_tax_id}</Figure>
                      <span className="text-muted-foreground ml-1.5 text-xs">
                        {r.taxpayer_tax_id.length === 11 ? "TCKN" : "VKN"}
                      </span>
                    </td>
                    <td>{cell ? <CellBadge cell={cell} /> : null}</td>
                    <td className="min-w-56">
                      <Assignees
                        engagement={r.engagement_uuid}
                        members={members}
                        canWrite={canWrite}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </TableBox>
        )}
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.taxpayers_footnote_api")}
      </p>
    </div>
  );
}
