import { useQueries, useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

import { integrationIssues, type IntegrationLists } from './format';

export type VatForecast = Schemas['VATForecast'];
export type MissingDocuments = Schemas['MissingDocuments'];
type RetailSale = Schemas['RetailSale'];

const eng = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;
const key = (org: string, engagement: string, ...rest: unknown[]) => ['engagement', org, engagement, 'summary', ...rest];

/** Dönemin KDV hesabı (`GET .../vat-forecast?period=`): faturalardan hesaplanan / indirilecek, devreden, oran kırılımı. */
export function useVatForecast(engagement: string, period: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, 'vat', period),
    queryFn: () => api.get<VatForecast>(`${eng(engagement)}/vat-forecast`, { organization: org, query: { period } }),
    enabled: enabled && !!engagement,
    staleTime: 60_000,
  });
}

/** Kapanmış ayın gelmeyen belgeleri (`GET .../document-tasks/missing`): banka ekstresi (hesap hesap), Z raporu, fiş, alış faturası. */
export function useMissingDocuments(engagement: string, period: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, 'missing', period),
    queryFn: () => api.get<MissingDocuments>(`${eng(engagement)}/document-tasks/missing`, { organization: org, query: { period } }),
    enabled: enabled && !!engagement,
    staleTime: 60_000,
  });
}

/** Dönemin Z raporu tarihleri (`GET .../retail-sales`, en çok 100 kayıt); toplam 0 ise mükellef Z raporu kullanmıyor sayılır. */
export function useZReportDates(engagement: string, period: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, 'z-dates', period),
    queryFn: async () => {
      const page = await api.get<{ items: RetailSale[]; total: number }>(`${eng(engagement)}/retail-sales`, {
        organization: org,
        query: { period, limit: 100, offset: 0 },
      });
      return { total: page.total, dates: page.items.filter((z) => z.approval_status !== 'rejected').map((z) => z.sale_date ?? '') };
    },
    enabled: enabled && !!engagement,
    staleTime: 60_000,
  });
}

const CONNECTIONS = [
  { name: 'einvoice', path: '/einvoice-connections', perm: 'invoices' },
  { name: 'bank', path: '/bank-connections', perm: 'bank' },
  { name: 'easytrade', path: '/easytrade-connections', perm: 'invoices' },
  { name: 'kolaybi', path: '/kolaybi-connections', perm: 'invoices' },
  { name: 'parasut', path: '/parasut-connections', perm: 'invoices' },
] as const;

/**
 * Eşitleme hatası olan bağlantılar: mükellefin e-Fatura, banka, EasyTrade,
 * KolayBi ve Paraşüt bağlantı listeleri. Alınamayan liste (kapalı modül,
 * yetki) atlanır; kart yalnız bilinen hataları gösterir.
 */
export function useIntegrationIssues(engagement: string, can: { invoices: boolean; bank: boolean }) {
  const org = useOrganization();
  const results = useQueries({
    queries: CONNECTIONS.map((c) => ({
      queryKey: key(org, engagement, 'connections', c.name),
      queryFn: () => api.get<{ items: unknown[] }>(`${eng(engagement)}${c.path}`, { organization: org }),
      enabled: can[c.perm] && !!engagement,
      staleTime: 5 * 60_000,
      retry: false,
    })),
  });
  const lists: Record<string, unknown[]> = {};
  CONNECTIONS.forEach((c, i) => {
    const items = results[i]?.data?.items;
    if (items) lists[c.name] = items;
  });
  return {
    issues: integrationIssues(lists as unknown as IntegrationLists),
    loading: results.some((r) => r.isLoading),
  };
}
