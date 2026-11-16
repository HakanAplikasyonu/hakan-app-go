"use client";

import { MonitorSmartphone } from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
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
import { Panel, TableBox } from "./office-ui";

/**
 * `/t/{slug}/agents` (#285): the office's desktop agents (issue #51) with
 * platform, version, online/offline state, last heartbeat and last error.
 */
export function AgentsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const agents = useDesktopAgents(slug);
  const items = agents.data?.items ?? [];
  const offline = items.filter((a) => a.status === "offline").length;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.agents.title")}
        description={t("office.agents.description")}
        icon={<MonitorSmartphone className="size-6" />}
      />
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
            className="text-brand text-xs font-medium hover:underline"
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
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.agents.col_name")}</th>
                <th>{t("office.agents.col_platform")}</th>
                <th>{t("office.agents.col_version")}</th>
                <th>{t("office.agents.col_status")}</th>
                <th>{t("office.agents.col_last_seen")}</th>
                <th>{t("office.agents.col_last_error")}</th>
                {can(permissions.tenant.agents.write) ? (
                  <th>
                    <span className="sr-only">
                      {t("office.agents.col_actions")}
                    </span>
                  </th>
                ) : null}
              </tr>
            </thead>
            <tbody>
              {items.map((a) => (
                <AgentRow
                  key={a.uuid}
                  agent={a}
                  canRevoke={can(permissions.tenant.agents.write)}
                />
              ))}
            </tbody>
          </TableBox>
        )}
      </Panel>
    </div>
  );
}

const platformKey: Record<DesktopAgent["platform"], string> = {
  windows: "office.agents.platform_windows",
  macos: "office.agents.platform_macos",
  linux: "office.agents.platform_linux",
};

function AgentRow({
  agent,
  canRevoke,
}: {
  agent: DesktopAgent;
  canRevoke: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const revoke = useRevokeDesktopAgent(slug);
  const online = agent.status === "online";

  const onRevoke = async () => {
    const ok = await confirmDelete({
      title: t("office.agents.revoke_title", { name: agent.name }),
      description: t("office.agents.revoke_description"),
      confirmLabel: t("office.agents.revoke"),
    });
    if (ok) revoke.mutate(agent.uuid);
  };

  return (
    <tr data-agent={agent.uuid}>
      <td className="font-medium">{agent.name}</td>
      <td>{t(platformKey[agent.platform])}</td>
      <td className="font-mono text-xs">{agent.version || "—"}</td>
      <td>
        <StatusChip
          tone={online ? "success" : "danger"}
          label={
            online ? t("office.agents.online") : t("office.agents.offline")
          }
        />
      </td>
      <td>
        <time
          dateTime={agent.last_seen_at}
          title={datetime(agent.last_seen_at, undefined, locale)}
        >
          {relativeDatetime(agent.last_seen_at, locale)}
        </time>
      </td>
      <td className="text-sm">
        {agent.last_error ? (
          <span className="text-status-blocking">
            {t("office.agents.error_AGENT_OFFLINE")} ·{" "}
            {datetime(agent.last_error.at, undefined, locale)}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      {canRevoke ? (
        <td className="text-right">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void onRevoke()}
            disabled={revoke.isPending}
          >
            {t("office.agents.revoke")}
          </Button>
        </td>
      ) : null}
    </tr>
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
          className="text-brand text-xs font-medium hover:underline"
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
