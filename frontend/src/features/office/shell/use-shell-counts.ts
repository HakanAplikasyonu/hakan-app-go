"use client";

import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { usePermission } from "@/providers/permission-provider";

import { useDesktopAgents } from "../hooks/use-agents";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import { useOfficeDashboard } from "../hooks/use-office-data";

export type ShellCounts = {
  /** Kenar menü sayaçları, menü öğesi kimliğine göre. */
  items: Partial<Record<string, number>>;
  agents: { online: number; offline: number } | null;
};

/**
 * Kenar menü sayaçları (OfisNav): kuyruk (bekleyen öneri), uyarılar (açık
 * engelleyici) ve mükellef sayısı. Panodan ve mükellef dizininden okunur;
 * izin yoksa sayaç görünmez.
 */
export function useShellCounts(): ShellCounts {
  const { slug } = useTenant();
  const { can } = usePermission();
  const dashboard = useOfficeDashboard(
    can(permissions.tenant.engagements.read),
  );
  const directory = useEngagementDirectory();
  const agents = useDesktopAgents(slug, can(permissions.tenant.agents.read));

  const d = dashboard.data;
  const items: ShellCounts["items"] = {};
  if (d?.review) items.review = d.review.pending;
  if (d?.warnings) items.warnings = d.warnings.blocker;
  if (directory.list.length) items.taxpayers = directory.list.length;

  const list = agents.data?.items;
  return {
    items,
    agents: list?.length
      ? {
          online: list.filter((a) => a.status === "online").length,
          offline: list.filter((a) => a.status === "offline").length,
        }
      : null,
  };
}
