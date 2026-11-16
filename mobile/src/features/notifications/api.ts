import { inboxEndpoint, normalizeInbox, type InboxItem } from '@hakan/mobile-core/notifications/inbox';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useAuth } from '@/core/auth/auth-provider';

/**
 * Mükellef bildirim listesi. Uç #333 ile gelecek; o zamana kadar `available`
 * false döner ve hiç istek atılmaz (ekran boş durumu gösterir).
 */
export function useInbox() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const ep = inboxEndpoint('taxpayer');
  const q = useQuery({
    queryKey: ['inbox', uuid],
    enabled: ep !== null && !!uuid,
    queryFn: async (): Promise<InboxItem[]> => normalizeInbox(await api.get<unknown>(ep!.list, { taxpayer: uuid })),
    refetchInterval: 60_000,
  });
  return { ...q, available: ep !== null, items: q.data ?? [] };
}

export function useMarkRead() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const qc = useQueryClient();
  const ep = inboxEndpoint('taxpayer');
  return useMutation({
    mutationFn: async (id: string | 'all') => {
      if (!ep) return;
      await api.post<unknown>(id === 'all' ? ep.readAll : ep.read(id), {}, { taxpayer: uuid });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ['inbox', uuid] }),
  });
}
