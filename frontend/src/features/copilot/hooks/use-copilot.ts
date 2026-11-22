"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  copilotKeys,
  copilotService,
  type CopilotStepResult,
  type CopilotWorkflowQuery,
} from "../services/copilot.service";

export function useCopilotWorkflow(slug: string, q: CopilotWorkflowQuery) {
  return useQuery({
    queryKey: copilotKeys.workflow(slug, q),
    queryFn: () => copilotService.workflow(slug, q),
    placeholderData: keepPreviousData,
  });
}

export function useEngagementWorkflow(
  slug: string,
  engagement: string,
  period: string,
) {
  return useQuery({
    queryKey: copilotKeys.engagement(slug, engagement, period),
    queryFn: () => copilotService.engagement(slug, engagement, period),
    enabled: !!engagement,
  });
}

export function useCreatePlan(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { command: string; period?: string }) =>
      copilotService.createPlan(slug, v.command, v.period),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: copilotKeys.plans(slug) }),
  });
}

export function useRecordStep(slug: string) {
  return useMutation({
    mutationFn: (v: {
      plan: string;
      index: number;
      result: CopilotStepResult;
    }) => copilotService.recordStep(slug, v.plan, v.index, v.result),
  });
}

export function useCancelPlan(slug: string) {
  return useMutation({
    mutationFn: (plan: string) => copilotService.cancel(slug, plan),
  });
}

/** After steps ran, every office list may have changed. */
export function useRefreshOffice(slug: string) {
  const qc = useQueryClient();
  return () =>
    void qc.invalidateQueries({ queryKey: ["tenant", slug.toLowerCase()] });
}
