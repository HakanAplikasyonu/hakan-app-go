"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  agentTaskKeys,
  agentTaskService,
  OPEN_TASK_STATUSES,
  type DesktopHelperSettingsInput,
  type LucaExportInput,
} from "../services/agent-tasks.service";

export function useDesktopHelperSettings(slug: string) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: agentTaskKeys.settings(slug),
    queryFn: () => agentTaskService.settings(slug),
  });
  const save = useMutation({
    mutationFn: (input: DesktopHelperSettingsInput) =>
      agentTaskService.saveSettings(slug, input),
    onSuccess: (data) => qc.setQueryData(agentTaskKeys.settings(slug), data),
  });
  return { query, save };
}

/** Açık görev varken 15 sn'de bir yenilenir; hepsi bitince durur. */
export function useAgentTasks(slug: string, enabled = true) {
  return useQuery({
    queryKey: agentTaskKeys.all(slug),
    queryFn: () => agentTaskService.list(slug),
    enabled,
    refetchInterval: (q) =>
      q.state.data?.items.some((t) => OPEN_TASK_STATUSES.has(t.status))
        ? 15_000
        : false,
  });
}

export function useAgentTaskActions(slug: string) {
  const qc = useQueryClient();
  const refresh = () =>
    qc.invalidateQueries({ queryKey: agentTaskKeys.all(slug) });
  return {
    start: useMutation({
      mutationFn: (input: LucaExportInput) =>
        agentTaskService.startLucaExport(slug, input),
      onSuccess: refresh,
    }),
    approve: useMutation({
      mutationFn: (id: string) => agentTaskService.approve(slug, id),
      onSuccess: refresh,
    }),
    cancel: useMutation({
      mutationFn: (id: string) => agentTaskService.cancel(slug, id),
      onSuccess: refresh,
    }),
    resolve: useMutation({
      mutationFn: (v: { id: string; imported: boolean }) =>
        agentTaskService.resolve(slug, v.id, v.imported),
      onSuccess: () => {
        void refresh();
        // Fiş aktarımı da kapanır; aktarım listeleri yenilenir.
        void qc.invalidateQueries({
          queryKey: ["tenant", slug.toLowerCase(), "journal-transfers"],
        });
      },
    }),
  };
}
