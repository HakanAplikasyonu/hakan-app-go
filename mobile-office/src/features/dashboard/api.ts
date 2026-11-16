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

type PeriodMatrix = { periods: string[]; items: Schemas['PeriodMatrixRow'][]; total: number; limit: number; offset: number };

/**
 * Dönem tacı (MusavirMobil · 1): beyan dönemi için beyana hazır / toplam.
 * `GET /v1/tenant/periods/matrix` (#32), tek dönem. `tenant.engagements.read_all`
 * yetkisi olmayan kullanıcıda yalnızca kendisine atanan mükellefler döner.
 * İlk sayfa 100 satır; toplam `total` alanından gelir, hazır sayısı ilk sayfadan.
 */
export function usePeriodCrown(period: string, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['period-crown', org, period],
    enabled,
    queryFn: () =>
      api.get<PeriodMatrix>('/v1/tenant/periods/matrix', { organization: org, query: { until: period, periods: 1, limit: 100 } }),
    staleTime: 5 * 60_000,
  });
}
