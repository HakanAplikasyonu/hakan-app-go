import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type CopilotAction = components["schemas"]["CopilotAction"];
export type CopilotStep = components["schemas"]["CopilotStep"];
export type CopilotStepKey = CopilotStep["key"];
export type CopilotStepStatus = CopilotStep["status"];
export type CopilotWorkflowRow = components["schemas"]["CopilotWorkflowRow"];
export type CopilotWorkflowPage = components["schemas"]["CopilotWorkflowPage"];
export type CopilotWorkflowSummary =
  components["schemas"]["CopilotWorkflowSummary"];
/** Sıradaki iş listesinin süzgeci; step ve summary ofis sayaçları (#780). */
export type CopilotWorkflowQuery = {
  period: string;
  search: string;
  limit: number;
  offset: number;
  step?: CopilotStepKey | null;
  summary?: boolean;
};
export type CopilotPlan = components["schemas"]["CopilotPlan"];
export type CopilotPlanStep = components["schemas"]["CopilotPlanStep"];
export type CopilotNote = components["schemas"]["CopilotNote"];
export type CopilotStepResult = {
  status: "done" | "failed" | "skipped";
  error_code?: string;
};

export const copilotKeys = {
  all: (slug: string) => ["tenant", slug.toLowerCase(), "copilot"] as const,
  workflow: (slug: string, q: CopilotWorkflowQuery) =>
    [...copilotKeys.all(slug), "workflow", q] as const,
  engagement: (slug: string, engagement: string, period: string) =>
    [
      ...copilotKeys.all(slug),
      "engagement",
      engagement.toLowerCase(),
      period,
    ] as const,
  plans: (slug: string) => [...copilotKeys.all(slug), "plans"] as const,
};

/**
 * Co-pilot (#392): the period close chain of every engagement and the
 * command bar's action plans. A plan is only a proposal: each step runs
 * through its own existing endpoint after the user approves it
 * (`lib/run-action.ts`), and the outcome is recorded here.
 */
export const copilotService = {
  workflow(slug: string, q: CopilotWorkflowQuery) {
    return tenantRequest<CopilotWorkflowPage>(
      slug,
      "GET",
      "/v1/tenant/copilot/workflow",
      {
        query: {
          period: q.period,
          q: q.search.trim() || undefined,
          limit: q.limit,
          offset: q.offset,
          step: q.step || undefined,
          summary: q.summary ? "true" : undefined,
        },
      },
    );
  },
  engagement(slug: string, engagement: string, period: string) {
    return tenantRequest<CopilotWorkflowRow>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/copilot/workflow`,
      // Kart sayfanın yardımcısı: hata olursa kart kalkar, sayfa sürer.
      { query: { period }, silent: true },
    );
  },
  plans(slug: string) {
    return tenantRequest<{ items: CopilotPlan[] }>(
      slug,
      "GET",
      "/v1/tenant/copilot/plans",
      { query: { limit: 10 } },
    );
  },
  createPlan(slug: string, command: string, period?: string) {
    return tenantRequest<CopilotPlan>(
      slug,
      "POST",
      "/v1/tenant/copilot/plans",
      { body: { command, period }, silent: true },
    );
  },
  recordStep(
    slug: string,
    plan: string,
    index: number,
    result: CopilotStepResult,
  ) {
    return tenantRequest<CopilotPlan>(
      slug,
      "POST",
      `/v1/tenant/copilot/plans/${encodeURIComponent(plan)}/steps/${index}`,
      { body: result, silent: true },
    );
  },
  cancel(slug: string, plan: string) {
    return tenantRequest<CopilotPlan>(
      slug,
      "POST",
      `/v1/tenant/copilot/plans/${encodeURIComponent(plan)}/cancel`,
      { silent: true },
    );
  },
};
