"use client";

import { Building2, Check, UserRound, UsersRound } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenantMembers } from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOffice, useOfficeActions } from "../hooks/use-office";
import type { OfficeMember, Taxpayer } from "../types";
import {
  DemoNotice,
  Figure,
  OfficeDataGate,
  Panel,
  TableBox,
} from "./office-ui";

/** Ofisin gerçek üyeleri (API) + demo üyeler; atama seçicisi için. */
function useAssignableMembers(): OfficeMember[] {
  const { slug, permissions: perms } = useTenant();
  const demo = useOffice((s) => s.members);
  const canRead = perms.includes(permissions.tenant.members.read);
  const { data } = useTenantMembers(slug, canRead);
  return useMemo(() => {
    const real: OfficeMember[] = (data?.items ?? []).map((m) => ({
      id: m.user.uuid,
      name: `${m.user.name} ${m.user.surname}`.trim() || m.user.email,
      role: m.role.name,
    }));
    return [...real, ...demo];
  }, [data, demo]);
}

function AssigneePicker({
  taxpayer,
  members,
}: {
  taxpayer: Taxpayer;
  members: OfficeMember[];
}) {
  const { t } = useLocale();
  const { setAssignees } = useOfficeActions();
  const toggle = (id: string) => {
    const has = taxpayer.assigneeIds.includes(id);
    setAssignees(
      taxpayer.id,
      has
        ? taxpayer.assigneeIds.filter((x) => x !== id)
        : [...taxpayer.assigneeIds, id],
    );
  };
  return (
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
          const on = taxpayer.assigneeIds.includes(m.id);
          return (
            <button
              key={m.id}
              type="button"
              onClick={() => toggle(m.id)}
              className="hover:bg-accent flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm"
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
  );
}

function TaxpayersInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const taxpayers = useOffice((s) => s.taxpayers);
  const invoices = useOffice((s) => s.invoices);
  const warnings = useOffice((s) => s.warnings);
  const suggestions = useOffice((s) => s.suggestions);
  const members = useAssignableMembers();
  const nameOf = (id: string) => members.find((m) => m.id === id)?.name ?? "—";

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.taxpayers_title")}
        description={t("office.taxpayers_description")}
        icon={<Building2 className="size-6" />}
      />
      <DemoNotice />
      <Panel>
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.col_taxpayer")}</th>
              <th>{t("office.col_tax_id")}</th>
              <th>{t("office.col_tax_office")}</th>
              <th>{t("office.col_assignees")}</th>
              <th className="text-right">{t("office.col_invoices")}</th>
              <th className="text-right">{t("office.col_review")}</th>
              <th className="text-right">{t("office.col_warnings")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {taxpayers.map((tp) => (
              <tr key={tp.id}>
                <td>
                  <div className="flex items-center gap-2 font-medium">
                    {tp.kind === "person" ? (
                      <UserRound
                        className="text-muted-foreground size-4"
                        aria-hidden
                      />
                    ) : (
                      <Building2
                        className="text-muted-foreground size-4"
                        aria-hidden
                      />
                    )}
                    <Link
                      className="hover:underline"
                      href={`${routes.tenant.invoices(slug)}?taxpayer=${tp.id}`}
                    >
                      {tp.title}
                    </Link>
                  </div>
                </td>
                <td>
                  <Figure>{tp.taxId}</Figure>
                  <span className="text-muted-foreground ml-1.5 text-xs">
                    {tp.taxId.length === 11 ? "TCKN" : "VKN"}
                  </span>
                </td>
                <td>{tp.taxOffice}</td>
                <td>
                  {tp.assigneeIds.length ? (
                    <div className="flex flex-wrap gap-1">
                      {tp.assigneeIds.map((id) => (
                        <span
                          key={id}
                          className="bg-secondary rounded-md px-1.5 py-0.5 text-xs"
                        >
                          {nameOf(id)}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <span className="text-muted-foreground text-xs">
                      {t("office.unassigned")}
                    </span>
                  )}
                </td>
                <td className="text-right">
                  <Figure>
                    {invoices.filter((i) => i.taxpayerId === tp.id).length}
                  </Figure>
                </td>
                <td className="text-right">
                  <Figure>
                    {
                      suggestions.filter(
                        (s) => s.taxpayerId === tp.id && s.status === "pending",
                      ).length
                    }
                  </Figure>
                </td>
                <td className="text-right">
                  <Figure>
                    {
                      warnings.filter(
                        (w) => w.taxpayerId === tp.id && w.status === "open",
                      ).length
                    }
                  </Figure>
                </td>
                <td className="text-right">
                  <AssigneePicker taxpayer={tp} members={members} />
                </td>
              </tr>
            ))}
          </tbody>
        </TableBox>
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.taxpayers_footnote")}
      </p>
    </div>
  );
}

export function TaxpayersPage() {
  return (
    <OfficeDataGate>
      <TaxpayersInner />
    </OfficeDataGate>
  );
}
