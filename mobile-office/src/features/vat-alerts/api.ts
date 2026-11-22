import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type VatAlert = Schemas['OfficeVATWatchEvent'];

/** Ofisin son KDV uyarıları (`GET /v1/tenant/vat-forecast/watch-events`, #799). */
export function useVatAlerts(limit = 50, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['vat-alerts', org, limit],
    enabled,
    queryFn: () => api.get<{ items: VatAlert[] }>('/v1/tenant/vat-forecast/watch-events', { organization: org, query: { limit } }),
  });
}
