import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

import type { SunucuGonderimi } from './gonderim';

type Liste = Schemas['TaxpayerSubmissionList'];

/**
 * Gönderdiklerim (#780 Senaryo B): `GET /v1/taxpayer/submissions`; fişler ve
 * belge isteği yüklemeleri ofisteki durumlarıyla, şirketin bugünkü ofisinin
 * adı (onay ekranı "Müşavirinize ulaştı · Ofis"). Bağlantı yokken telefon
 * kuyruğu bu olmadan da gösterilir.
 */
export function useGonderimler(refetchInterval?: number) {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({
    queryKey: ['submissions', uuid],
    enabled: !!uuid,
    staleTime: 30_000,
    refetchInterval,
    queryFn: () => api.get<Liste>('/v1/taxpayer/submissions', { taxpayer: uuid, query: { limit: 100 } }),
  });
  const items: SunucuGonderimi[] = (q.data?.items ?? []) as SunucuGonderimi[];
  return { ...q, items, office: q.data?.office_name || null };
}
