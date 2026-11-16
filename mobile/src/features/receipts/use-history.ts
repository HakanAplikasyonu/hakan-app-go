import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useAuth } from '@/core/auth/auth-provider';

import { fetchReceiptHistory } from './api';

/** The company's receipts on the server (#199); the phone's queue shows without it. */
export function useReceiptHistory() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({ queryKey: ['receipt-history', uuid], queryFn: () => fetchReceiptHistory(uuid), enabled: !!uuid });
  return { history: q.data ?? [], loading: q.isFetching, refresh: q.refetch };
}

export type TaxpayerInvoice = import('@/core/api/schema').components['schemas']['TaxpayerInvoice'];

/** Şirketin e-Faturaları (`GET /v1/taxpayer/invoices`), yeniden eskiye. Yalnızca sekme açılınca çekilir. */
export function useInvoiceHistory(enabled: boolean) {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  return useQuery({
    queryKey: ['invoice-history', uuid],
    enabled: enabled && !!uuid,
    queryFn: async () => (await api.get<{ items: TaxpayerInvoice[] }>('/v1/taxpayer/invoices', { taxpayer: uuid, query: { limit: 50 } }))?.items ?? [],
  });
}
