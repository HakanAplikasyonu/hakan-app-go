import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type TrialBalance = Schemas['TrialBalance'];
export type TrialBalanceLine = Schemas['TrialBalanceLine'];
export type TrialBalanceReport = Schemas['TrialBalanceReport'];

const base = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}/account-plan/trial-balances`;

/** Tutarlı mizanlar (`GET .../account-plan/trial-balances`, #393), yeni yükleme önce. */
export function useTrialBalances(engagement: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'trial-balances'],
    queryFn: async () => (await api.get<{ items: TrialBalance[] }>(base(engagement), { organization: org })).items,
  });
}

/**
 * Mizan raporu: seçili mizan (`latest` ya da uuid), önceki dönemle
 * karşılaştırma ve kırılım (1: ana hesaplar, 0: tümü).
 */
export function useTrialBalanceReport(engagement: string, which: string | undefined, compare: boolean, level: number) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'trial-balance', which ?? 'latest', compare, level],
    queryFn: () =>
      api.get<TrialBalanceReport>(`${base(engagement)}/${encodeURIComponent(which ?? 'latest')}`, {
        organization: org,
        query: { compare: compare ? 'previous' : 'none', level },
      }),
    enabled: which !== undefined,
    placeholderData: keepPreviousData,
  });
}
