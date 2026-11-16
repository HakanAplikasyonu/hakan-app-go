import { inboxEndpoint, normalizeInbox, type InboxItem } from '@hakan/mobile-core/notifications/inbox';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useOrganization } from '@/core/auth/auth-provider';

/**
 * Ofis bildirim listesi (`GET /v1/tenant/notifications`, #333). Uç tablosu boşsa `available`
 * false döner ve hiç istek atılmaz (ekran boş durumu gösterir).
 */
export function useInbox() {
  const org = useOrganization();
  const ep = inboxEndpoint('office');
  const q = useQuery({
    queryKey: ['inbox', org],
    enabled: ep !== null,
    queryFn: async (): Promise<InboxItem[]> => normalizeInbox(await api.get<unknown>(ep!.list, { organization: org, query: { limit: 50 } })),
    refetchInterval: 60_000,
  });
  return { ...q, available: ep !== null, items: q.data ?? [] };
}

export function useMarkRead() {
  const org = useOrganization();
  const qc = useQueryClient();
  const ep = inboxEndpoint('office');
  return useMutation({
    mutationFn: async (uuid: string | 'all') => {
      if (!ep) return;
      await api.post<unknown>(uuid === 'all' ? ep.readAll : ep.read(uuid), {}, { organization: org });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ['inbox', org] }),
  });
}
