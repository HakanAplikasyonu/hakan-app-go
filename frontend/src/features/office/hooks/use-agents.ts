"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  agentKeys,
  agentService,
  type AgentDocumentKind,
} from "@/features/office/services/agents.service";
import { fetchLatestDesktopRelease } from "@/features/office/services/desktop-release.service";

/**
 * Agents of the office. The server marks an agent offline after 30 minutes
 * without a heartbeat (sweep), so a minute of polling is enough.
 */
export function useDesktopAgents(slug: string, enabled = true) {
  return useQuery({
    queryKey: agentKeys.list(slug),
    queryFn: () => agentService.list(slug),
    enabled,
    refetchInterval: 60_000,
  });
}

export function useRevokeDesktopAgent(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => agentService.revoke(slug, uuid),
    onSuccess: () => qc.invalidateQueries({ queryKey: agentKeys.list(slug) }),
  });
}

/** Files from the agent waiting for the office (#780 e). */
export function useAgentDocuments(slug: string, enabled = true) {
  return useQuery({
    queryKey: agentKeys.documents(slug),
    queryFn: () => agentService.documents(slug),
    enabled,
    refetchInterval: 60_000,
  });
}

export function useResolveAgentDocument(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      uuid: string;
      engagement: string;
      kind: AgentDocumentKind;
      period?: string;
    }) =>
      agentService.resolveDocument(slug, v.uuid, {
        engagement_uuid: v.engagement,
        kind: v.kind,
        ...(v.period ? { period: v.period } : {}),
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: agentKeys.documents(slug) }),
  });
}

export function useDismissAgentDocument(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => agentService.dismissDocument(slug, uuid),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: agentKeys.documents(slug) }),
  });
}

/**
 * En yeni masaüstü sürümü (GitHub Release). Herkese açık API saatte 60 istek
 * sınırlı: bir saat önbellekte kalır, hata olursa bir kez denenir.
 */
export function useLatestDesktopRelease() {
  return useQuery({
    queryKey: ["desktop-release", "latest"],
    queryFn: fetchLatestDesktopRelease,
    staleTime: 60 * 60_000,
    gcTime: 60 * 60_000,
    retry: 1,
    refetchOnWindowFocus: false,
  });
}
