import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

import type { InboxType } from './format';

export type InboxItem = Schemas['InboxItem'];
export type InboxList = Schemas['InboxList'];

/**
 * Gelen kutusu (`GET /v1/tenant/inbox`, #806): bu dönem müşavirin elini
 * bekleyen işler ve türe göre sayılar. Ana ekran rozeti `limit` 1 ile yalnız
 * sayıları kullanır.
 */
export function useInbox(type: InboxType | null, enabled = true, limit = 50) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['inbox', org, type, limit],
    enabled,
    queryFn: () => api.get<InboxList>('/v1/tenant/inbox', { organization: org, query: { type: type ?? undefined, limit } }),
  });
}

/** Tek dokunuş onay: masraf fişi ve Z raporu (`POST .../expenses|retail-sales/{uuid}/approve`). */
export function useApproveInboxItem() {
  const org = useOrganization();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (item: InboxItem) => {
      const kind = item.type === 'expense' ? 'expenses' : 'retail-sales';
      const path = `/v1/tenant/engagements/${encodeURIComponent(item.engagement_uuid)}/${kind}/${encodeURIComponent(item.uuid)}/approve`;
      return api.post<unknown>(path, undefined, { organization: org });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['inbox', org] });
    },
  });
}
