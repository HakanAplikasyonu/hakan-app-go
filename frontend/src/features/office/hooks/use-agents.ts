"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  agentKeys,
  agentService,
} from "@/features/office/services/agents.service";

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
