/**
 * Mesajlarım (#780): ofisin şirkete gönderdiği SMS ve e-postalar
 * (`GET /v1/taxpayer/messages`), yeniden eskiye. Deneme modu kayıtları
 * sunucuda elenir; adresler maskeli gelir. Liste sınırlıdır: son 100 mesaj.
 */
import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { type Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

export type TaxpayerMessage = Schemas['TaxpayerMessage'];
type MessageList = Schemas['EnvelopeTaxpayerMessageList']['data'];

export const SAYFA = 100;

export function useMessages() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({
    queryKey: ['taxpayer-messages', uuid],
    enabled: !!uuid,
    queryFn: () => api.get<MessageList>('/v1/taxpayer/messages', { taxpayer: uuid, query: { limit: SAYFA } }),
  });
  return { ...q, items: q.data?.items ?? [], total: q.data?.total ?? 0 };
}
