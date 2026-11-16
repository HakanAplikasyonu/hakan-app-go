import { useQuery } from '@tanstack/react-query';

import { useAuth } from '@/core/auth/auth-provider';

import { fetchReceiptHistory } from './api';

/** The company's receipts on the server (#199); the phone's queue shows without it. */
export function useReceiptHistory() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({ queryKey: ['receipt-history', uuid], queryFn: () => fetchReceiptHistory(uuid), enabled: !!uuid });
  return { history: q.data ?? [], loading: q.isFetching, refresh: q.refetch };
}
