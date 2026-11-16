import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type OfficeDashboard = Schemas['OfficeDashboard'];
export type DashboardUpload = Schemas['OfficeDashboardUpload'];

/** Ana pano (`GET /v1/tenant/dashboard`, #237). */
export function useDashboard() {
  const org = useOrganization();
  return useQuery({
    queryKey: ['dashboard', org],
    queryFn: () => api.get<OfficeDashboard>('/v1/tenant/dashboard', { organization: org }),
    refetchInterval: 60_000,
  });
}
