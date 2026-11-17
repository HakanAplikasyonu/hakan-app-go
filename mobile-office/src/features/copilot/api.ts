import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type WorkflowRow = Schemas['CopilotWorkflowRow'];
type WorkflowPage = Schemas['CopilotWorkflowPage'];

/**
 * Bir mükellefin dönem kapanış zinciri (`GET .../copilot/workflow`, #392).
 * Dönem verilmez: sunucu önceki ayı (beyan dönemi) kullanır, ofis paneliyle aynı.
 */
export function useEngagementWorkflow(engagement: string, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['copilot-workflow', org, engagement],
    enabled: enabled && !!engagement,
    queryFn: () => api.get<WorkflowRow>(`/v1/tenant/engagements/${engagement}/copilot/workflow`, { organization: org }),
  });
}

/** Ofisin bütün mükelleflerinde sıradaki iş (`GET /v1/tenant/copilot/workflow`). */
export function useOfficeWorkflow(enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['copilot-workflow', org, 'office'],
    enabled,
    queryFn: () => api.get<WorkflowPage>('/v1/tenant/copilot/workflow', { organization: org, query: { limit: 200 } }),
  });
}
