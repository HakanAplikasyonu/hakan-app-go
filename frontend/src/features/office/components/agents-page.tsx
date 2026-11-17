"use client";

import { MonitorSmartphone } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import {
  createActionsColumn,
  createColumn,
  DataTable,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useDesktopAgents, useRevokeDesktopAgent } from "../hooks/use-agents";
import type { DesktopAgent } from "../services/agents.service";
import { Panel } from "./office-ui";

/**
 * `/t/{slug}/agents` (#285): the office's desktop agents (issue #51) with
 * platform, version, online/offline state, last heartbeat and last error.
 */
export function AgentsPage() {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.agents.title")}
        description={t("office.agents.description")}
        icon={<MonitorSmartphone className="size-6" />}
      />
      <AgentsPanel />
    </div>
  );
}

/** Ajan listesi: /agents sayfası ve Ayarlar → Masaüstü ajanları (#330). */
export function AgentsPanel() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const agents = useDesktopAgents(slug);
  const items = agents.data?.items ?? [];
  const offline = items.filter((a) => a.status === "offline").length;
  const columns = useAgentColumns(can(permissions.tenant.agents.write));

  return (
    <Panel
      title={t("office.agents.list_title", { count: items.length })}
      description={
        offline
          ? t("office.agents.offline_count", { count: offline })
          : undefined
      }
      actions={
        <Link
          href={routes.help.guide("masaustu")}
          className="text-brand inline-flex min-h-6 items-center text-xs font-medium hover:underline max-md:min-h-11"
        >
          {t("office.agents.guide_link")}
        </Link>
      }
    >
      {agents.isLoading ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : agents.isError ? (
        <ErrorState
          title={t("office.agents.load_error")}
          onRetry={() => void agents.refetch()}
        />
      ) : items.length === 0 ? (
        <div className="space-y-1 p-6 text-center">
          <p className="font-medium">{t("office.agents.empty_title")}</p>
          <p className="text-muted-foreground text-sm">
            {t("office.agents.empty_description")}
          </p>
        </div>
      ) : (
        <DataTable<DesktopAgent>
          columns={columns}
          data={items}
          getRowId={(a) => a.uuid}
          bordered={false}
          features={panelTable}
        />
      )}
    </Panel>
  );
}

const platformKey: Record<DesktopAgent["platform"], string> = {
  windows: "office.agents.platform_windows",
  macos: "office.agents.platform_macos",
  linux: "office.agents.platform_linux",
};

// Panel içine gömülü istemci tarafı liste: araç çubuğu ve sayfalama yok,
// sıralama açık; mobilde kartlar.
const panelTable: DataTableFeatures = {
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

function useAgentColumns(canRevoke: boolean) {
  const { t, locale } = useLocale();
  return useMemo<ColumnDef<DesktopAgent, unknown>[]>(
    () => [
      createColumn<DesktopAgent>({
        id: "name",
        labelKey: "office.agents.col_name",
        accessorKey: "name",
        gridPrimary: true,
        meta: { cellClassName: "font-medium" },
      }),
      createColumn<DesktopAgent>({
        id: "platform",
        labelKey: "office.agents.col_platform",
        accessorFn: (a) => t(platformKey[a.platform]),
      }),
      createColumn<DesktopAgent>({
        id: "version",
        labelKey: "office.agents.col_version",
        accessorFn: (a) => a.version || "—",
        meta: { cellClassName: "font-mono text-xs" },
      }),
      createColumn<DesktopAgent>({
        id: "status",
        labelKey: "office.agents.col_status",
        accessorKey: "status",
        cell: ({ row }) => {
          const online = row.original.status === "online";
          return (
            <StatusChip
              tone={online ? "success" : "danger"}
              label={
                online ? t("office.agents.online") : t("office.agents.offline")
              }
            />
          );
        },
      }),
      createColumn<DesktopAgent>({
        id: "last_seen_at",
        labelKey: "office.agents.col_last_seen",
        accessorKey: "last_seen_at",
        gridSecondary: true,
        cell: ({ row }) => (
          <time
            dateTime={row.original.last_seen_at}
            title={datetime(row.original.last_seen_at, undefined, locale)}
          >
            {relativeDatetime(row.original.last_seen_at, locale)}
          </time>
        ),
      }),
      createColumn<DesktopAgent>({
        id: "last_error",
        labelKey: "office.agents.col_last_error",
        accessorFn: (a) => a.last_error?.at ?? "",
        meta: { cellClassName: "text-sm" },
        cell: ({ row }) => {
          const err = row.original.last_error;
          return err ? (
            <span className="text-status-blocking">
              {t("office.agents.error_AGENT_OFFLINE")} ·{" "}
              {datetime(err.at, undefined, locale)}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          );
        },
      }),
      ...(canRevoke
        ? [
            createActionsColumn<DesktopAgent>({
              labelKey: "office.agents.col_actions",
              cell: (a) => <RevokeAgentButton agent={a} />,
            }),
          ]
        : []),
    ],
    [canRevoke, locale, t],
  );
}

/** Satırın "Kaldır" düğmesi; onay iletişimiyle ajanı iptal eder. */
function RevokeAgentButton({ agent }: { agent: DesktopAgent }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const revoke = useRevokeDesktopAgent(slug);

  const onRevoke = async () => {
    const ok = await confirmDelete({
      title: t("office.agents.revoke_title", { name: agent.name }),
      description: t("office.agents.revoke_description"),
      confirmLabel: t("office.agents.revoke"),
    });
    if (ok) revoke.mutate(agent.uuid);
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => void onRevoke()}
      disabled={revoke.isPending}
    >
      {t("office.agents.revoke")}
    </Button>
  );
}

/**
 * Overview card (#285): shown only when at least one agent is offline, so
 * folder watching that silently stopped gets noticed.
 */
export function OfflineAgentsCard() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const agents = useDesktopAgents(slug, can(permissions.tenant.agents.read));
  const offline = (agents.data?.items ?? []).filter(
    (a) => a.status === "offline",
  );
  if (!offline.length) return null;
  return (
    <section
      role="status"
      aria-labelledby="offline-agents-title"
      className="border-status-blocking/40 bg-status-blocking/5 rounded-xl border p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="space-y-1">
          <h2 id="offline-agents-title" className="text-sm font-semibold">
            {t("office.agents.card_title", { count: offline.length })}
          </h2>
          <p className="text-foreground text-xs">
            {t("office.agents.card_description")}
          </p>
        </div>
        <Link
          href={routes.tenant.agents(slug)}
          className="text-brand inline-flex min-h-6 items-center text-xs font-medium hover:underline max-md:min-h-11"
        >
          {t("office.agents.card_open")}
        </Link>
      </div>
      <ul className="mt-2 space-y-0.5 text-sm">
        {offline.slice(0, 5).map((a) => (
          <li key={a.uuid}>
            <span className="font-medium">{a.name}</span>
            <span className="text-foreground">
              {" · "}
              {t("office.agents.last_seen", {
                when: relativeDatetime(a.last_seen_at, locale),
              })}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
