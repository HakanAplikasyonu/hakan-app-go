import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, request } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

export type ForeignInvoice = Schemas['ForeignInvoice'];
export type ForeignInvoiceList = Schemas['ForeignInvoiceList'];
type ForeignServiceKinds = Schemas['ForeignServiceKinds'];

const base = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}/foreign-invoices`;

/** Ofis ayarı (`GET /v1/tenant/foreign-service-kinds`, #780): `enabled` false ise bölüm gizli. */
export function useForeignInvoicesEnabled(enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['foreign-service-kinds', org],
    queryFn: async () => (await api.get<ForeignServiceKinds>('/v1/tenant/foreign-service-kinds', { organization: org })).enabled,
    enabled,
    staleTime: 5 * 60_000,
  });
}

/** Dönemin yabancı faturaları ve toplamları (`GET .../foreign-invoices?period=`). */
export function useForeignInvoices(engagement: string, period: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'foreign-invoices', period],
    queryFn: () => api.get<ForeignInvoiceList>(base(engagement), { organization: org, query: { period } }),
    enabled,
  });
}

export type ForeignInvoiceAction = { uuid: string } & ({ kind: 'confirm' } | { kind: 'unconfirm' } | { kind: 'rate'; rate: string });

/** Onayla / onayı geri al (`POST .../confirm|unconfirm`) ve kur girişi (`PATCH`, `exchange_rate`). */
export function useForeignInvoiceAction(engagement: string) {
  const org = useOrganization();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (a: ForeignInvoiceAction) => {
      const one = `${base(engagement)}/${encodeURIComponent(a.uuid)}`;
      if (a.kind === 'rate') return request<ForeignInvoice>('PATCH', one, { organization: org, body: { exchange_rate: a.rate } });
      return api.post<ForeignInvoice>(`${one}/${a.kind}`, undefined, { organization: org });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['engagement', org, engagement, 'foreign-invoices'] });
    },
  });
}
